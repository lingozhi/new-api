package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeAccessLogPathRedactsSensitiveQuery(t *testing.T) {
	testCases := []struct {
		name        string
		path        string
		notContains []string
		contains    []string
	}{
		{
			name:     "path without query remains unchanged",
			path:     "/api/status",
			contains: []string{"/api/status"},
		},
		{
			name:        "private key is redacted while ordinary query remains",
			path:        "/api/option/waffo-pancake/catalog?merchant_id=MER_123&private_key=secret-material",
			notContains: []string{"secret-material"},
			contains:    []string{"merchant_id=MER_123", "private_key=%5BREDACTED%5D"},
		},
		{
			name:        "token and secret names are matched case insensitively",
			path:        "/api/example?Access_Token=token-value&clientSecret=secret-value",
			notContains: []string{"token-value", "secret-value"},
			contains:    []string{"Access_Token=%5BREDACTED%5D", "clientSecret=%5BREDACTED%5D"},
		},
		{
			name:        "hyphenated credentials and authorization are redacted",
			path:        "/api/example?api-key=api-secret&private-key=private-secret&authorization=bearer-secret",
			notContains: []string{"api-secret", "private-secret", "bearer-secret"},
			contains:    []string{"api-key=%5BREDACTED%5D", "private-key=%5BREDACTED%5D", "authorization=%5BREDACTED%5D"},
		},
		{
			name:        "oauth code and turnstile response are redacted",
			path:        "/api/oauth/github?code=oauth-secret&state=public-state&turnstile=turnstile-secret",
			notContains: []string{"oauth-secret", "turnstile-secret"},
			contains:    []string{"code=%5BREDACTED%5D", "state=public-state", "turnstile=%5BREDACTED%5D"},
		},
		{
			name:        "malformed query is removed rather than logged raw",
			path:        "/api/example?private_key=secret%ZZ",
			notContains: []string{"secret%ZZ"},
			contains:    []string{"/api/example?%5BREDACTED%5D"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requestPath, rawQuery, _ := strings.Cut(tc.path, "?")
			actual := sanitizeAccessLogPath(requestPath, rawQuery)
			for _, value := range tc.notContains {
				assert.False(t, strings.Contains(actual, value), "sanitized path must not contain %q", value)
			}
			for _, value := range tc.contains {
				assert.True(t, strings.Contains(actual, value), "sanitized path must contain %q: %s", value, actual)
			}
		})
	}
}

func TestAccessLogRedactsSensitiveQueryAfterEncodedPathQuestionMark(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	previousWriter := gin.DefaultWriter
	gin.DefaultWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = previousWriter
		common.LogWriterMu.Unlock()
	})

	engine := gin.New()
	SetUpLogger(engine)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models%3Fignored?key=relay-secret", nil)
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, logs.String(), "/v1/models%3Fignored?key=%5BREDACTED%5D")
	assert.NotContains(t, logs.String(), "relay-secret")
}

func TestHasServerCredentialQuery(t *testing.T) {
	testCases := []struct {
		name     string
		query    string
		expected bool
	}{
		{name: "private key", query: "private-key=pancake-secret", expected: true},
		{name: "api key", query: "api_key=provider-secret", expected: true},
		{name: "authorization", query: "authorization=Bearer+secret", expected: true},
		{name: "oauth callback remains redirectable", query: "code=oauth-code&state=state", expected: false},
		{name: "frontend token filter remains redirectable", query: "token=filter-value", expected: false},
		{name: "malformed query fails closed", query: "tab=%ZZ", expected: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, HasServerCredentialQuery(tc.query))
		})
	}
}

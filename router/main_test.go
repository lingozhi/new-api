package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSplitFrontendFallbackKeepsBackendRequestsLocal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("FRONTEND_BASE_URL", "https://frontend.example")
	previousMasterNode := common.IsMasterNode
	common.IsMasterNode = false
	t.Cleanup(func() { common.IsMasterNode = previousMasterNode })

	engine := gin.New()
	SetRouter(engine, ThemeAssets{})

	testCases := []struct {
		name             string
		method           string
		target           string
		expectedStatus   int
		expectedLocation string
	}{
		{
			name:           "misspelled api path",
			method:         http.MethodGet,
			target:         "/api/option/waffo-pancake/catalog-typo?private_key=pancake-secret",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unsupported method on api path",
			method:         http.MethodHead,
			target:         "/api/option/waffo-pancake/catalog?private_key=pancake-secret",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown relay path",
			method:         http.MethodGet,
			target:         "/v1/unknown?key=relay-secret",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown gemini relay path",
			method:         http.MethodGet,
			target:         "/v1beta/unknown?key=relay-secret",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown dashboard path",
			method:         http.MethodGet,
			target:         "/dashboard/unknown?trace=public",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown playground path",
			method:         http.MethodGet,
			target:         "/pg/unknown?trace=public",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown midjourney path",
			method:         http.MethodGet,
			target:         "/mj/unknown?trace=public",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown mode midjourney path",
			method:         http.MethodGet,
			target:         "/relay/mj/unknown?trace=public",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown suno path",
			method:         http.MethodGet,
			target:         "/suno/unknown?trace=public",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown kling path",
			method:         http.MethodGet,
			target:         "/kling/v1/unknown?trace=public",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown jimeng path",
			method:         http.MethodGet,
			target:         "/jimeng/unknown?trace=public",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown asset path",
			method:         http.MethodGet,
			target:         "/assets/unknown.js?trace=public",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "repeated slash api path",
			method:         http.MethodGet,
			target:         "//api/option/waffo-pancake/catalog?trace=public",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "encoded slash api path",
			method:         http.MethodGet,
			target:         "/%2fapi/option/waffo-pancake/catalog?trace=public",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "dot segment api path",
			method:         http.MethodGet,
			target:         "/console/../api/option/waffo-pancake/catalog?trace=public",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "case variant api path",
			method:         http.MethodGet,
			target:         "/API/option/waffo-pancake/catalog?trace=public",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown frontend path with server credential",
			method:         http.MethodGet,
			target:         "/unknown?private-key=pancake-secret",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:             "oauth frontend path",
			method:           http.MethodGet,
			target:           "/oauth/github?code=oauth-code&state=public-state",
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://frontend.example/oauth/github?code=oauth-code&state=public-state",
		},
		{
			name:             "frontend path",
			method:           http.MethodGet,
			target:           "/console/topup?tab=waffo-pancake",
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://frontend.example/console/topup?tab=waffo-pancake",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			requestTarget := tc.target
			if strings.HasPrefix(tc.target, "//") {
				requestTarget = "http://backend.example" + tc.target
			}
			request := httptest.NewRequest(tc.method, requestTarget, nil)
			request.RequestURI = tc.target
			engine.ServeHTTP(recorder, request)

			assert.Equal(t, tc.expectedStatus, recorder.Code)
			assert.Equal(t, tc.expectedLocation, recorder.Header().Get("Location"))
			assert.NotContains(t, recorder.Header().Get("Location"), "secret")
		})
	}
}

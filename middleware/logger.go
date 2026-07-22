package middleware

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const RouteTagKey = "route_tag"

func RouteTag(tag string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(RouteTagKey, tag)
		c.Next()
	}
}

func SetUpLogger(server *gin.Engine) {
	server.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var requestID string
		if param.Keys != nil {
			requestID, _ = param.Keys[common.RequestIdKey].(string)
		}
		tag, _ := param.Keys[RouteTagKey].(string)
		if tag == "" {
			tag = "web"
		}
		return fmt.Sprintf("[GIN] %s | %s | %s | %3d | %13v | %15s | %7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			tag,
			requestID,
			param.StatusCode,
			param.Latency,
			param.ClientIP,
			param.Method,
			sanitizeAccessLogPath(param.Request.URL.EscapedPath(), param.Request.URL.RawQuery),
		)
	}))
}

func sanitizeAccessLogPath(requestPath, rawQuery string) string {
	if rawQuery == "" {
		return requestPath
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return requestPath + "?%5BREDACTED%5D"
	}

	redacted := false
	for key := range query {
		normalized := normalizeQueryKey(key)
		if isServerCredentialQueryKey(normalized) ||
			normalized == "code" ||
			normalized == "turnstile" ||
			strings.Contains(normalized, "token") {
			query.Set(key, "[REDACTED]")
			redacted = true
		}
	}
	if !redacted {
		return requestPath + "?" + rawQuery
	}
	return requestPath + "?" + query.Encode()
}

// HasServerCredentialQuery reports whether a query contains credentials that
// must never be forwarded by the split-frontend fallback redirect.
func HasServerCredentialQuery(rawQuery string) bool {
	if rawQuery == "" {
		return false
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return true
	}
	for key := range query {
		if isServerCredentialQueryKey(normalizeQueryKey(key)) {
			return true
		}
	}
	return false
}

func normalizeQueryKey(key string) string {
	return strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
}

func isServerCredentialQueryKey(normalized string) bool {
	return normalized == "key" ||
		normalized == "authorization" ||
		strings.Contains(normalized, "privatekey") ||
		strings.Contains(normalized, "apikey") ||
		strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "signature")
}

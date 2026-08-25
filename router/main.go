package router

import (
	"net/http"
	"os"
	pathpkg "path"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetRouter(router *gin.Engine, assets ThemeAssets) {
	SetApiRouter(router)
	SetDashboardRouter(router)
	SetRelayRouter(router)
	SetVideoRouter(router)
	frontendBaseUrl := os.Getenv("FRONTEND_BASE_URL")
	if common.IsMasterNode && frontendBaseUrl != "" {
		frontendBaseUrl = ""
		common.SysLog("FRONTEND_BASE_URL is ignored on master node")
	}
	if frontendBaseUrl == "" {
		SetWebRouter(router, assets)
	} else {
		frontendBaseUrl = strings.TrimSuffix(frontendBaseUrl, "/")
		router.NoRoute(func(c *gin.Context) {
			c.Set(middleware.RouteTagKey, "web")
			if isBackendPath(c.Request.URL.Path) || middleware.HasServerCredentialQuery(c.Request.URL.RawQuery) {
				controller.RelayNotFound(c)
				return
			}
			c.Redirect(http.StatusMovedPermanently, frontendBaseUrl+c.Request.RequestURI)
		})
	}
}

func isBackendPath(requestPath string) bool {
	normalizedPath := strings.ToLower(pathpkg.Clean("/" + requestPath))
	for _, prefix := range []string{
		"/api",
		"/assets",
		"/dashboard",
		"/jimeng",
		"/kling",
		"/mj",
		"/pg",
		"/suno",
		"/v1",
		"/v2",
		"/v1beta",
	} {
		if normalizedPath == prefix || strings.HasPrefix(normalizedPath, prefix+"/") {
			return true
		}
	}

	segments := strings.Split(strings.TrimPrefix(normalizedPath, "/"), "/")
	return len(segments) >= 2 && segments[1] == "mj"
}

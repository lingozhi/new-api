package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestVideoRouterKeepsDepthMediaJobsOutOfUnifiedImagePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetVideoRouter(engine)

	routes := make(map[string]struct{})
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	assert.NotContains(t, routes, http.MethodPost+" /v1/jobs")
	assert.NotContains(t, routes, http.MethodGet+" /v1/jobs/:task_id")
	assert.Contains(t, routes, http.MethodPost+" /v1/media/jobs")
	assert.Contains(t, routes, http.MethodGet+" /v1/media/jobs/:task_id")
}

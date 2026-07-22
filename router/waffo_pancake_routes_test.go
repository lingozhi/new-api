package router

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/controller"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestWaffoPancakeCatalogRoutesKeepLegacyGetLocal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	expectedHandlers := map[string]uintptr{
		http.MethodPost + " /api/option/waffo-pancake/catalog": reflect.ValueOf(controller.ListWaffoPancakeCatalog).Pointer(),
		http.MethodGet + " /api/option/waffo-pancake/catalog":  reflect.ValueOf(controller.RejectLegacyWaffoPancakeCatalog).Pointer(),
		http.MethodGet + " /api/option/waffo-pancake/catalog/": reflect.ValueOf(controller.RejectLegacyWaffoPancakeCatalog).Pointer(),
	}
	for _, route := range engine.Routes() {
		key := route.Method + " " + route.Path
		expected, ok := expectedHandlers[key]
		if !ok {
			continue
		}
		assert.Equal(t, expected, reflect.ValueOf(route.HandlerFunc).Pointer())
		delete(expectedHandlers, key)
	}
	assert.Empty(t, expectedHandlers)
}

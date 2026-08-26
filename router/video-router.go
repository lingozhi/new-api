package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	miniMaxV2CreateRouter := router.Group("/v2")
	miniMaxV2CreateRouter.Use(middleware.RouteTag("relay"))
	miniMaxV2CreateRouter.Use(
		middleware.SystemPerformanceCheck(),
		middleware.TokenAuth(),
		middleware.ModelRequestRateLimit(),
		middleware.Distribute(),
	)
	{
		miniMaxV2CreateRouter.POST("/video_generation", controller.RelayTask)
	}

	miniMaxV2QueryRouter := router.Group("/v2")
	miniMaxV2QueryRouter.Use(middleware.RouteTag("relay"))
	miniMaxV2QueryRouter.Use(
		middleware.SystemPerformanceCheck(),
		middleware.TokenAuthReadOnly(),
		middleware.ModelRequestRateLimit(),
	)
	{
		miniMaxV2QueryRouter.GET("/query/video_generation/:task_id", controller.RelayTaskFetch)
	}

	// Video proxy: accepts either session auth (dashboard) or token auth (API clients)
	videoProxyRouter := router.Group("/v1")
	videoProxyRouter.Use(middleware.RouteTag("relay"))
	videoProxyRouter.Use(
		middleware.SystemPerformanceCheck(),
		middleware.TokenOrUserAuth(),
		middleware.ModelRequestRateLimit(),
	)
	{
		videoProxyRouter.GET("/videos/:task_id/content", controller.VideoProxy)
	}

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		videoV1Router.POST("/video/generations", controller.RelayTask)
		videoV1Router.GET("/video/generations/:task_id", controller.RelayTaskFetch)
		videoV1Router.POST("/videos/:video_id/remix", controller.RelayTask)
	}
	// openai compatible API video routes
	// docs: https://platform.openai.com/docs/api-reference/videos/create
	{
		videoV1Router.POST("/videos", controller.RelayTask)
		videoV1Router.GET("/videos/:task_id", controller.RelayTaskFetch)
	}

	depthMediaV1Router := router.Group("/v1")
	depthMediaV1Router.Use(middleware.RouteTag("relay"))
	depthMediaV1Router.Use(middleware.DepthMediaRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		depthMediaV1Router.POST("/depth/jobs", controller.RelayTask)
		depthMediaV1Router.GET("/depth/jobs/:task_id", controller.RelayTaskFetch)
		depthMediaV1Router.POST("/media/jobs", controller.RelayTask)
		depthMediaV1Router.GET("/media/jobs/:task_id", controller.RelayTaskFetch)
	}

	klingV1Router := router.Group("/kling/v1")
	klingV1Router.Use(middleware.RouteTag("relay"))
	klingV1Router.Use(middleware.KlingRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		klingV1Router.POST("/videos/text2video", controller.RelayTask)
		klingV1Router.POST("/videos/image2video", controller.RelayTask)
		klingV1Router.GET("/videos/text2video/:task_id", controller.RelayTaskFetch)
		klingV1Router.GET("/videos/image2video/:task_id", controller.RelayTaskFetch)
	}

	// Jimeng official API routes - direct mapping to official API format
	jimengOfficialGroup := router.Group("jimeng")
	jimengOfficialGroup.Use(middleware.RouteTag("relay"))
	jimengOfficialGroup.Use(middleware.JimengRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		// Maps to: /?Action=CVSync2AsyncSubmitTask&Version=2022-08-31 and /?Action=CVSync2AsyncGetResult&Version=2022-08-31
		jimengOfficialGroup.POST("/", controller.RelayTask)
	}
}

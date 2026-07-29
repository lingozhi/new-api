package middleware

import (
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	taskdepthmedia "github.com/QuantumNous/new-api/relay/channel/task/depthmedia"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

type depthMediaRequest struct {
	Model         string `json:"model,omitempty"`
	SourceURL     string `json:"source_url"`
	Operation     string `json:"operation,omitempty"`
	Quality       string `json:"quality,omitempty"`
	Scale         int    `json:"scale,omitempty"`
	Format        string `json:"format,omitempty"`
	WebhookURL    string `json:"webhook_url,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

func DepthMediaRequestConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet {
			c.Request.URL.Path = "/v1/video/generations/" + c.Param("task_id")
			c.Next()
			return
		}

		var request depthMediaRequest
		if err := common.UnmarshalBodyReusable(c, &request); err != nil {
			c.Next()
			return
		}
		isDepth := strings.Contains(c.FullPath(), "/depth/jobs") || strings.Contains(c.Request.URL.Path, "/depth/jobs")
		modelName := strings.TrimSpace(request.Model)
		if isDepth {
			if modelName != "" && modelName != taskdepthmedia.ModelDepthVideo {
				c.JSON(http.StatusBadRequest, gin.H{"error": "depth jobs only support " + taskdepthmedia.ModelDepthVideo})
				c.Abort()
				return
			}
			modelName = taskdepthmedia.ModelDepthVideo
		} else if modelName == "" {
			resolved, err := taskdepthmedia.ResolveModel(request.Operation, request.Quality, request.Scale)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				c.Abort()
				return
			}
			modelName = resolved
		}

		metadata := map[string]any{
			"source_url": request.SourceURL,
			"operation":  request.Operation,
			"quality":    request.Quality,
			"scale":      request.Scale,
			"format":     request.Format,
		}
		unified := relaycommon.TaskSubmitReq{
			Prompt:        "process media",
			Model:         modelName,
			Image:         request.SourceURL,
			Metadata:      metadata,
			WebhookURL:    strings.TrimSpace(request.WebhookURL),
			WebhookSecret: request.WebhookSecret,
		}
		data, err := common.Marshal(unified)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to normalize request"})
			c.Abort()
			return
		}
		common.CleanupBodyStorage(c)
		storage, err := common.CreateBodyStorage(data)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cache normalized request"})
			c.Abort()
			return
		}
		c.Set(common.KeyBodyStorage, storage)
		c.Request.Body = io.NopCloser(storage)
		c.Request.ContentLength = int64(len(data))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.URL.Path = "/v1/video/generations"
		c.Set(common.KeyRequestBody, data)
		c.Next()
	}
}

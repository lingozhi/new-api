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
	SubtitleArea  string `json:"subtitle_area,omitempty"`
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
		isDepthRoute := strings.Contains(c.FullPath(), "/depth/jobs") ||
			strings.Contains(c.Request.URL.Path, "/depth/jobs")
		isUnifiedRoute := c.FullPath() == "/v1/media/jobs" || c.Request.URL.Path == "/v1/media/jobs"
		isDepth := isDepthRoute ||
			(isUnifiedRoute && strings.EqualFold(strings.TrimSpace(request.Operation), "depth"))
		modelName := strings.TrimSpace(request.Model)
		if modelName == taskdepthmedia.PublicModelDepthVideo && !isDepth {
			c.JSON(http.StatusBadRequest, gin.H{"error": "depth-video requires operation depth"})
			c.Abort()
			return
		}
		if isDepth {
			if modelName != "" &&
				modelName != taskdepthmedia.ModelDepthVideo &&
				modelName != taskdepthmedia.PublicModelDepthVideo {
				c.JSON(http.StatusBadRequest, gin.H{"error": "depth jobs only support " + taskdepthmedia.ModelDepthVideo})
				c.Abort()
				return
			}
			modelName = taskdepthmedia.ModelDepthVideo
		} else if modelName == "" ||
			modelName == taskdepthmedia.PublicModelBackgroundRemove ||
			modelName == taskdepthmedia.PublicModelImageUpscale ||
			modelName == taskdepthmedia.PublicModelSubtitleRemove {
			resolved, err := taskdepthmedia.ResolveModel(request.Operation, request.Quality, request.Scale)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				c.Abort()
				return
			}
			if modelName == taskdepthmedia.PublicModelBackgroundRemove &&
				!strings.EqualFold(strings.TrimSpace(request.Operation), "remove_background") {
				c.JSON(http.StatusBadRequest, gin.H{"error": "background-remove requires operation remove_background"})
				c.Abort()
				return
			}
			if modelName == taskdepthmedia.PublicModelImageUpscale &&
				!strings.EqualFold(strings.TrimSpace(request.Operation), "upscale") {
				c.JSON(http.StatusBadRequest, gin.H{"error": "image-upscale requires operation upscale"})
				c.Abort()
				return
			}
			if modelName == taskdepthmedia.PublicModelSubtitleRemove &&
				!strings.EqualFold(strings.TrimSpace(request.Operation), "remove_subtitles") {
				c.JSON(http.StatusBadRequest, gin.H{"error": "subtitle-remove requires operation remove_subtitles"})
				c.Abort()
				return
			}
			modelName = resolved
		}

		metadata := map[string]any{
			"source_url":    request.SourceURL,
			"operation":     request.Operation,
			"quality":       request.Quality,
			"scale":         request.Scale,
			"format":        request.Format,
			"subtitle_area": request.SubtitleArea,
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

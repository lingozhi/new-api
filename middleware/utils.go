package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func abortWithOpenAiMessage(c *gin.Context, statusCode int, message string, code ...types.ErrorCode) {
	if c.Request.URL.Path == "/v2/video_generation" || strings.HasPrefix(c.Request.URL.Path, "/v2/query/video_generation/") {
		clientMessage := message
		if statusCode >= http.StatusInternalServerError {
			clientMessage = http.StatusText(statusCode)
			if clientMessage == "" {
				clientMessage = "Internal server error"
			}
		}
		c.JSON(statusCode, dto.NewMiniMaxAPIErrorResponse(
			statusCode,
			clientMessage,
			c.GetString(common.RequestIdKey),
		))
		c.Abort()
		logger.LogError(c.Request.Context(), fmt.Sprintf("user %d | %s", c.GetInt("id"), common.MaskSensitiveInfo(message)))
		return
	}
	codeStr := ""
	if len(code) > 0 {
		codeStr = string(code[0])
	}
	userId := c.GetInt("id")
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
			"type":    "new_api_error",
			"code":    codeStr,
		},
	})
	c.Abort()
	logger.LogError(c.Request.Context(), fmt.Sprintf("user %d | %s", userId, message))
}

func abortWithMidjourneyMessage(c *gin.Context, statusCode int, code int, description string) {
	c.JSON(statusCode, gin.H{
		"description": description,
		"type":        "new_api_error",
		"code":        code,
	})
	c.Abort()
	logger.LogError(c.Request.Context(), description)
}

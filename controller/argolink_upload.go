package controller

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ArgolinkMediaUpload relays a ticket request; media bytes go directly to storage.
func ArgolinkMediaUpload(c *gin.Context) {
	var payload map[string]any
	if err := common.UnmarshalBodyReusable(c, &payload); err != nil {
		videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "Invalid upload request")
		return
	}
	if payload["model"] != constant.ArgolinkSeedance25Model {
		videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "Unsupported upload model")
		return
	}
	ch, err := model.CacheGetChannel(common.GetContextKeyInt(c, constant.ContextKeyChannelId))
	if err != nil || ch == nil || ch.Type != constant.ChannelTypeOpenAI {
		videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "Unsupported upload channel")
		return
	}
	body, err := common.Marshal(payload)
	if err != nil {
		videoProxyError(c, 400, "invalid_request_error", "Invalid upload request")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(ch.GetBaseURL(), "/")+"/v1/media/uploads", bytes.NewReader(body))
	if err != nil {
		videoProxyError(c, 502, "server_error", "Invalid upload endpoint")
		return
	}
	req.Header.Set("Authorization", "Bearer "+common.GetContextKeyString(c, constant.ContextKeyChannelKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "new-api/1.0")
	client, err := service.GetHttpClientWithProxy(ch.GetSetting().Proxy)
	if err != nil {
		videoProxyError(c, 502, "server_error", "Unable to connect to upload service")
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		videoProxyError(c, 502, "server_error", "Upload service request failed")
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		videoProxyError(c, 502, "server_error", "Invalid upload service response")
		return
	}
	c.Data(resp.StatusCode, "application/json", data)
}

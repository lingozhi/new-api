package sora

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Video callbacks belong to the gateway's durable task outbox, never the provider.
func validateVideoWebhook(c *gin.Context) *dto.TaskError {
	var options struct {
		URL    *string `json:"webhook_url,omitempty"`
		Secret *string `json:"webhook_secret,omitempty"`
	}
	if err := common.UnmarshalBodyReusable(c, &options); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_webhook", http.StatusBadRequest)
	}
	var fields map[string]any
	if err := common.UnmarshalBodyReusable(c, &fields); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_webhook", http.StatusBadRequest)
	}
	for _, name := range []string{"webhook_url", "webhook_secret"} {
		if value, exists := fields[name]; exists && value == nil {
			return service.TaskErrorWrapperLocal(fmt.Errorf("omit unused %s instead of null", name), "invalid_webhook", http.StatusBadRequest)
		}
	}
	if options.URL == nil && options.Secret == nil {
		return nil
	}
	if c.ContentType() != "application/json" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("video webhooks require JSON"), "invalid_webhook", http.StatusBadRequest)
	}
	if options.URL == nil || strings.TrimSpace(*options.URL) == "" || len(*options.URL) > 2048 || (options.Secret != nil && len(*options.Secret) > 512) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("webhook_url is required (max 2048 bytes); webhook_secret is optional (max 512 bytes)"), "invalid_webhook", http.StatusBadRequest)
	}
	webhookURL := strings.TrimSpace(*options.URL)
	parsed, parseErr := url.Parse(webhookURL)
	if parseErr != nil || parsed.User != nil || (parsed.Port() != "" && parsed.Port() != "443") {
		return service.TaskErrorWrapperLocal(fmt.Errorf("webhook_url requires HTTPS port 443 without embedded credentials"), "invalid_webhook_url", http.StatusBadRequest)
	}
	if err := service.ValidateJSONWebhookURL(webhookURL); err != nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("webhook_url must be an allowed public HTTPS endpoint"), "invalid_webhook_url", http.StatusBadRequest)
	}
	request, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	request.WebhookURL = webhookURL
	if options.Secret != nil {
		request.WebhookSecret = *options.Secret
	}
	c.Set("task_request", request)
	return nil
}

// BuildTaskWebhookPayload exposes only the public task identity and download path.
func (a *TaskAdaptor) BuildTaskWebhookPayload(task *model.Task) (any, bool, error) {
	if task == nil || task.Properties.Video == nil || (task.Properties.Video.Provider != "wan-unified" && task.Properties.Video.Provider != "lxmone-seedance") {
		return nil, false, nil
	}
	if task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure {
		return nil, true, fmt.Errorf("video task is not terminal")
	}
	payload := map[string]any{"id": task.TaskID, "task_id": task.TaskID, "model": task.Properties.OriginModelName, "status": "completed", "progress": 100}
	if task.Status == model.TaskStatusSuccess {
		payload["content_url"] = "/v1/videos/" + task.TaskID + "/content"
	} else {
		payload["status"] = "failed"
		payload["error"] = map[string]string{"message": common.MaskSensitiveInfo(task.FailReason)}
	}
	return payload, true, nil
}

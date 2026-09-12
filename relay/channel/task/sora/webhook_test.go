package sora

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoWebhookStoredForGatewayAndStrippedUpstream(t *testing.T) {
	for _, name := range []string{"wan3.0", "wan3.0-video", "seedance-2"} {
		t.Run(name, func(t *testing.T) {
			requestJSON := `{"model":"` + name + `","prompt":"test","webhook_url":"https://8.8.8.8/hook","webhook_secret":"callback-private-value"}`
			if name == "wan3.0" || name == "wan3.0-video" {
				requestJSON = strings.Replace(requestJSON, `"prompt":"test"`, `"input":{"prompt":"test"}`, 1)
			}
			c, info := newWanContext(t, name, requestJSON)
			info.ChannelBaseUrl = "https://lxmone.xyz"
			info.ChannelOtherSettings.LxmoneSeedanceResolutionRatios = map[string]map[string]float64{"seedance-2-pro": {"720p": 1}}
			a := &TaskAdaptor{}
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			request, err := relaycommon.GetTaskRequest(c)
			require.NoError(t, err)
			assert.Equal(t, "https://8.8.8.8/hook", request.WebhookURL)
			assert.Equal(t, "callback-private-value", request.WebhookSecret)
			body, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, common.DecodeJson(body, &payload))
			assert.NotContains(t, payload, "webhook_url")
			assert.NotContains(t, payload, "webhook_secret")
			assert.NotContains(t, common.TaskInputLog(c), "callback-private-value")
		})
	}
}

func TestVideoWebhookRejectsUnsafeEndpointsAndInvalidOptions(t *testing.T) {
	for _, name := range []string{"wan3.0", "wan3.0-video", "seedance-2"} {
		for _, options := range []string{
			`"webhook_url":"http://8.8.8.8/hook"`,
			`"webhook_url":"https://127.0.0.1/hook"`,
			`"webhook_url":"https://8.8.8.8:8443/hook"`,
			`"webhook_url":null`, `"webhook_url":123`,
			`"webhook_secret":"private"`,
			`"webhook_url":"https://8.8.8.8/hook","webhook_secret":null`,
			`"webhook_url":"https://8.8.8.8/hook","webhook_secret":"` + strings.Repeat("x", 513) + `"`,
		} {
			requestJSON := `{"model":"` + name + `","prompt":"test",` + options + `}`
			if name == "wan3.0" || name == "wan3.0-video" {
				requestJSON = strings.Replace(requestJSON, `"prompt":"test"`, `"input":{"prompt":"test"}`, 1)
			}
			c, info := newWanContext(t, name, requestJSON)
			info.ChannelBaseUrl = "https://lxmone.xyz"
			info.ChannelOtherSettings.LxmoneSeedanceResolutionRatios = map[string]map[string]float64{"seedance-2-pro": {"720p": 1}}
			err := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
			require.NotNil(t, err, name+options)
			assert.Equal(t, http.StatusBadRequest, err.StatusCode)
		}
	}
}

func TestVideoWebhookPayloadUsesPublicTerminalIdentity(t *testing.T) {
	for _, provider := range []string{"wan-unified", "lxmone-seedance"} {
		for _, state := range []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure} {
			task := &model.Task{TaskID: "task_public", Status: state, FailReason: "generation failed", Data: []byte(`{"id":"upstream-private","metadata":{"url":"https://private.example/result"}}`)}
			task.Properties.OriginModelName = "wan3.0"
			task.Properties.Video = &relaycommon.TaskVideoProperties{Provider: provider}
			payload, handled, err := (&TaskAdaptor{}).BuildTaskWebhookPayload(task)
			require.NoError(t, err)
			require.True(t, handled)
			fields := payload.(map[string]any)
			assert.Equal(t, "task_public", fields["id"])
			assert.Equal(t, "wan3.0", fields["model"])
			if state == model.TaskStatusSuccess {
				assert.Equal(t, "completed", fields["status"])
				assert.Equal(t, "/v1/videos/task_public/content", fields["content_url"])
				assert.NotContains(t, fields, "error")
			} else {
				assert.Equal(t, "failed", fields["status"])
				assert.Contains(t, fields, "error")
				assert.NotContains(t, fields, "content_url")
			}
			raw, err := common.Marshal(payload)
			require.NoError(t, err)
			assert.NotContains(t, string(raw), "upstream-private")
			assert.NotContains(t, string(raw), "private.example")
		}
	}
	_, handled, err := (&TaskAdaptor{}).BuildTaskWebhookPayload(&model.Task{})
	require.NoError(t, err)
	assert.False(t, handled)
}

func TestLegacySeedanceRejectsWebsiteWebhookFields(t *testing.T) {
	for _, name := range []string{"seedance-2.5", "seedance-2.0", "seedance-2.0-fast"} {
		for _, field := range []string{`"webhook_url":"https://8.8.8.8/hook"`, `"webhook_secret":"private-signing-secret"`, `"webhook_url":null`, `"webhook_secret":""`} {
			t.Run(name+"/"+strings.SplitN(field, ":", 2)[0], func(t *testing.T) {
				c, info := newWanContext(t, name, `{"model":"`+name+`","prompt":"test",`+field+`}`)
				c.Request.URL.Path = "/v1/videos/generations"
				info.ChannelBaseUrl = "https://argolink.example"
				taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
				require.NotNil(t, taskErr)
				assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
				assert.Equal(t, "unsupported_webhook", taskErr.Code)
				assert.True(t, taskErr.LocalError)
				_, exists := c.Get("task_request")
				assert.False(t, exists, "reject before creating billing or callback state")
			})
		}
	}
}

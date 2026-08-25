package dto

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

const MaxMiniMaxVideoContentItems = 16

type MiniMaxVideoGenerationV2Request struct {
	Model         string                   `json:"model"`
	Content       MiniMaxVideoContentItems `json:"content"`
	Resolution    *string                  `json:"resolution,omitempty"`
	Duration      *int                     `json:"duration,omitempty"`
	Ratio         *string                  `json:"ratio,omitempty"`
	CallbackURL   *string                  `json:"callback_url,omitempty"`
	AIGCWatermark *bool                    `json:"aigc_watermark,omitempty"`
}

type MiniMaxVideoContentItems []MiniMaxVideoContentItem

func (items *MiniMaxVideoContentItems) UnmarshalJSON(data []byte) error {
	decoded, err := common.UnmarshalJSONArrayWithLimit[MiniMaxVideoContentItem](data, MaxMiniMaxVideoContentItems)
	if err != nil {
		return err
	}
	*items = decoded
	return nil
}

var _ json.Unmarshaler = (*MiniMaxVideoContentItems)(nil)

type MiniMaxVideoContentItem struct {
	Type     string             `json:"type"`
	Text     string             `json:"text,omitempty"`
	ImageURL *MiniMaxVideoMedia `json:"image_url,omitempty"`
	VideoURL *MiniMaxVideoMedia `json:"video_url,omitempty"`
	AudioURL *MiniMaxVideoMedia `json:"audio_url,omitempty"`
	Role     string             `json:"role,omitempty"`
}

type MiniMaxVideoMedia struct {
	URL string `json:"url"`
}

type MiniMaxVideoGenerationV2CreateResponse struct {
	TaskID string `json:"task_id"`
}

type MiniMaxVideoGenerationV2QueryResponse struct {
	Task MiniMaxVideoTask `json:"task"`
}

type MiniMaxVideoTask struct {
	ID         string                  `json:"id"`
	Model      string                  `json:"model"`
	Status     string                  `json:"status"`
	Error      *MiniMaxVideoTaskError  `json:"error,omitempty"`
	CreatedAt  int64                   `json:"created_at"`
	UpdatedAt  int64                   `json:"updated_at"`
	Content    *MiniMaxVideoTaskOutput `json:"content,omitempty"`
	Resolution string                  `json:"resolution,omitempty"`
	Duration   int                     `json:"duration,omitempty"`
	Usage      *MiniMaxVideoTaskUsage  `json:"usage,omitempty"`
	Ratio      string                  `json:"ratio,omitempty"`
	TaskType   string                  `json:"task_type"`
	Modality   string                  `json:"modality"`
}

type MiniMaxVideoTaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type MiniMaxVideoTaskOutput struct {
	URL string `json:"url"`
}

type MiniMaxVideoTaskUsage struct {
	TotalSeconds    int `json:"total_seconds"`
	InputSeconds    int `json:"input_seconds"`
	OutputSeconds   int `json:"output_seconds"`
	InputImageCount int `json:"input_image_count"`
}

type MiniMaxAPIErrorResponse struct {
	Type      string                `json:"type"`
	Error     MiniMaxAPIErrorDetail `json:"error"`
	RequestID string                `json:"request_id"`
}

type MiniMaxAPIErrorDetail struct {
	Type     string `json:"type"`
	Message  string `json:"message"`
	HTTPCode string `json:"http_code"`
}

func NewMiniMaxAPIErrorResponse(statusCode int, message, requestID string) MiniMaxAPIErrorResponse {
	errorType := "bad_request_error"
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		errorType = "authorized_error"
	case statusCode == http.StatusPaymentRequired:
		errorType = "insufficient_balance_error"
	case statusCode == http.StatusUnprocessableEntity:
		errorType = "unprocessable_entity_error"
	case statusCode == http.StatusTooManyRequests:
		errorType = "rate_limit_error"
	case statusCode == 529:
		errorType = "overloaded_error"
	case statusCode >= http.StatusInternalServerError:
		errorType = "server_error"
	}
	return MiniMaxAPIErrorResponse{
		Type: "error",
		Error: MiniMaxAPIErrorDetail{
			Type:     errorType,
			Message:  message,
			HTTPCode: strconv.Itoa(statusCode),
		},
		RequestID: requestID,
	}
}

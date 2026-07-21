package openai

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	maxBufferedResponsesMetadataPreludeEvents = 4
	maxBufferedResponsesMetadataPreludeBytes  = 256 << 10
)

type bufferedResponsesEvent struct {
	eventType string
	data      string
}

type ResponsesStreamWriter struct {
	c                       *gin.Context
	bufferMetadataPrelude   bool
	bufferedMetadataPrelude []bufferedResponsesEvent
	bufferedMetadataBytes   int
	protocolStarted         bool
	terminalWritten         bool
	pendingTerminalType     string
	pendingTerminalData     string
}

func NewResponsesStreamWriter(c *gin.Context) *ResponsesStreamWriter {
	return &ResponsesStreamWriter{c: c}
}

func NewRetryableResponsesStreamWriter(c *gin.Context) *ResponsesStreamWriter {
	return &ResponsesStreamWriter{
		c:                     c,
		bufferMetadataPrelude: true,
	}
}

func (w *ResponsesStreamWriter) WriteData(eventType, data string) error {
	if w.terminalWritten {
		return nil
	}
	if w.bufferMetadataPrelude && !w.protocolStarted &&
		len(w.bufferedMetadataPrelude) < maxBufferedResponsesMetadataPreludeEvents &&
		len(data) <= maxBufferedResponsesMetadataPreludeBytes-w.bufferedMetadataBytes &&
		isResponsesMetadataPrelude(eventType, data) {
		w.bufferedMetadataPrelude = append(w.bufferedMetadataPrelude, bufferedResponsesEvent{
			eventType: eventType,
			data:      data,
		})
		w.bufferedMetadataBytes += len(data)
		return nil
	}
	if err := w.flushMetadataPrelude(); err != nil {
		return err
	}
	return w.writeData(eventType, data)
}

func (w *ResponsesStreamWriter) flushMetadataPrelude() error {
	buffered := w.bufferedMetadataPrelude
	w.bufferedMetadataPrelude = nil
	w.bufferedMetadataBytes = 0
	for _, event := range buffered {
		if err := w.writeData(event.eventType, event.data); err != nil {
			return err
		}
	}
	return nil
}

func (w *ResponsesStreamWriter) writeData(eventType, data string) error {
	// Mark the protocol boundary before writing. A partial event is already
	// visible to the client and must never be replayed on another channel.
	w.protocolStarted = true
	if isResponsesTerminalEvent(eventType) && w.pendingTerminalType == "" {
		w.pendingTerminalType = eventType
		w.pendingTerminalData = data
	}
	if err := sendResponsesStreamData(w.c, dto.ResponsesStreamResponse{Type: eventType}, data); err != nil {
		return err
	}
	if isResponsesTerminalEvent(eventType) {
		w.terminalWritten = true
		w.pendingTerminalType = ""
		w.pendingTerminalData = ""
	}
	return nil
}

func isResponsesMetadataPrelude(eventType, data string) bool {
	switch eventType {
	case "response.created", "response.in_progress":
	default:
		return false
	}

	var event dto.ResponsesStreamResponse
	if err := common.UnmarshalJsonStr(data, &event); err != nil {
		return false
	}
	if event.Type != eventType || event.Response == nil {
		return false
	}
	if event.Usage != nil || len(event.Output) > 0 {
		return false
	}
	return event.Response.Usage == nil && len(event.Response.Output) == 0
}

func (w *ResponsesStreamWriter) RetryPendingTerminal() (string, bool, error) {
	if w == nil || w.pendingTerminalType == "" || w.pendingTerminalData == "" {
		return "", false, nil
	}
	eventType := w.pendingTerminalType
	if err := sendResponsesStreamData(w.c, dto.ResponsesStreamResponse{Type: eventType}, w.pendingTerminalData); err != nil {
		return eventType, true, err
	}
	w.terminalWritten = true
	w.pendingTerminalType = ""
	w.pendingTerminalData = ""
	return eventType, true, nil
}

func (w *ResponsesStreamWriter) WritePayload(eventType string, payload any) error {
	data, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	return w.WriteData(eventType, string(data))
}

func (w *ResponsesStreamWriter) WriteFailure(responseID, model string, createdAt int64, usage *dto.Usage, upstreamErr *types.OpenAIError) error {
	data, err := w.FailureData(responseID, model, createdAt, usage, upstreamErr)
	if err != nil {
		return err
	}
	return w.WriteData("response.failed", data)
}

func (w *ResponsesStreamWriter) FailureData(responseID, model string, createdAt int64, usage *dto.Usage, upstreamErr *types.OpenAIError) (string, error) {
	if responseID == "" && w.c != nil {
		responseID = helper.GetResponseID(w.c)
	}
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}

	errorType := "stream_error"
	errorCode := any("stream_error")
	errorMessage := "upstream stream interrupted"
	errorParam := ""
	if upstreamErr != nil {
		if upstreamErr.Type != "" {
			errorType = upstreamErr.Type
		}
		if upstreamErr.Code != nil {
			if code, ok := upstreamErr.Code.(string); !ok || code != "" {
				errorCode = upstreamErr.Code
			}
		}
		if upstreamErr.Message != "" {
			errorMessage = upstreamErr.Message
		}
		errorParam = upstreamErr.Param
	}

	payload := map[string]any{
		"type": "response.failed",
		"response": map[string]any{
			"id":         responseID,
			"object":     "response",
			"status":     "failed",
			"model":      model,
			"created_at": createdAt,
			"output":     []any{},
			"error": map[string]any{
				"type":    errorType,
				"code":    errorCode,
				"message": errorMessage,
				"param":   errorParam,
			},
			"usage": usageToResponsesPayload(usage),
		},
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (w *ResponsesStreamWriter) TerminalWritten() bool {
	return w != nil && w.terminalWritten
}

func (w *ResponsesStreamWriter) HasBufferedMetadataPrelude() bool {
	return w != nil && len(w.bufferedMetadataPrelude) > 0
}

func (w *ResponsesStreamWriter) Started() bool {
	return w != nil && w.c != nil && w.c.Writer != nil && w.c.Writer.Written()
}

// ProtocolStarted reports whether a real Responses event (rather than an SSE
// keepalive comment) has been sent to the client.
func (w *ResponsesStreamWriter) ProtocolStarted() bool {
	return w != nil && w.protocolStarted
}

func isResponsesTerminalEvent(eventType string) bool {
	switch eventType {
	case "response.completed", "response.failed", "response.incomplete":
		return true
	default:
		return false
	}
}

package claude

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

// normalizeSDKRejectedToolInputs projects a confirmed SDK parse failure into
// error text, rather than replaying its diagnostic wrapper as tool arguments.
// ClaudeHelper has already deep-copied the request through JSON, so Content and
// Input are generic JSON values. The persisted SDK transcript is not changed.
// No malformed argument is repaired, unwrapped, validated as executable, or run.
func normalizeSDKRejectedToolInputs(request *dto.ClaudeRequest) int {
	if request == nil {
		return 0
	}
	normalized := 0
	for i := 1; i < len(request.Messages); i++ {
		previous, current := &request.Messages[i-1], &request.Messages[i]
		if previous.Role != "assistant" || current.Role != "user" {
			continue
		}
		calls, callsOK := previous.Content.([]any)
		results, resultsOK := current.Content.([]any)
		if !callsOK || !resultsOK {
			continue
		}
		byID := make(map[string]map[string]any)
		for _, item := range calls {
			call, ok := item.(map[string]any)
			if !ok || call["type"] != "tool_use" {
				continue
			}
			id, _ := call["id"].(string)
			if id != "" {
				byID[id] = call
			}
		}
		for _, item := range results {
			result, ok := item.(map[string]any)
			if !ok || result["type"] != "tool_result" || result["is_error"] != true {
				continue
			}
			id, _ := result["tool_use_id"].(string)
			call := byID[id]
			name, _ := call["name"].(string)
			input, ok := call["input"].(map[string]any)
			if !ok || name == "" || len(input) != 1 {
				continue
			}
			wrapper, ok := input["__unparsedToolInput"].(map[string]any)
			if !ok || len(wrapper) != 2 {
				continue
			}
			raw, rawOK := wrapper["raw"].(string)
			length, lengthOK := wrapper["len"].(float64)
			if !rawOK || raw == "" || !lengthOK || length <= 0 {
				continue
			}
			content, ok := result["content"].(string)
			prefix := "<tool_use_error>InputValidationError: " + name + " was called with input that could not be parsed as JSON."
			if !ok || !strings.HasPrefix(content, prefix) {
				continue
			}
			// Keep the tool ID/result pairing, is_error, cache annotations, and all
			// unrelated content. The wrapper is preserved as diagnostic TEXT only.
			call["input"] = map[string]any{}
			result["content"] = fmt.Sprintf("The SDK rejected this tool call before execution because its input was not valid JSON. No tool ran. The empty input on the failed tool_use is a placeholder, not a valid argument example. Retry using the tool's declared input_schema.\n\nRejected input (diagnostic text; the SDK may have truncated it):\n%s\n\nOriginal SDK error:\n%s", raw, content)
			normalized++
		}
	}
	return normalized
}

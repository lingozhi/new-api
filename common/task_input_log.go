package common

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// TaskInputLog snapshots submitted fields without retaining credentials or file bytes.
// These bounds apply only to the log, never to forwarding or generation.
func TaskInputLog(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	var input map[string]any
	if strings.HasPrefix(c.ContentType(), "multipart/") {
		form, err := ParseMultipartFormReusable(c)
		if err != nil {
			return ""
		}
		input = make(map[string]any)
		for key, values := range form.Value {
			if len(values) == 1 {
				input[key] = values[0]
			} else {
				input[key] = values
			}
		}
		for key, files := range form.File {
			entries := make([]any, 0, len(files))
			for _, file := range files {
				entries = append(entries, map[string]any{"filename": file.Filename, "content_type": file.Header.Get("Content-Type"), "size_bytes": file.Size})
			}
			input[key] = entries
		}
	} else if err := UnmarshalBodyReusable(c, &input); err != nil {
		return ""
	}
	if len(input) == 0 {
		return ""
	}
	budget := 60000
	value := sanitizeTaskInput(input, 0, &budget)
	data, err := Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func sanitizeTaskInput(value any, depth int, budget *int) any {
	if depth >= 16 || *budget <= 0 {
		return "[omitted from input log]"
	}
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any)
		for key, item := range v {
			if *budget <= 0 || len(result) >= 200 {
				result["_log_omitted"] = "additional fields omitted"
				break
			}
			if len(key) > 256 {
				result["_log_omitted"] = "oversized field name omitted"
				continue
			}
			*budget -= len(key) + 16
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
			if normalized == "authorization" || normalized == "cookie" || normalized == "key" || strings.Contains(normalized, "secret") || strings.Contains(normalized, "password") || strings.Contains(normalized, "credential") || strings.HasSuffix(normalized, "token") || strings.HasSuffix(normalized, "apikey") || normalized == "headers" {
				result[key] = "[REDACTED]"
				continue
			}
			if *budget <= 0 {
				result["_log_omitted"] = "additional fields omitted"
				break
			}
			result[key] = sanitizeTaskInput(item, depth+1, budget)
		}
		return result
	case []any:
		result := make([]any, 0, min(len(v), 100))
		for i, item := range v {
			if i >= 100 || *budget <= 0 {
				result = append(result, "[additional items omitted]")
				break
			}
			result = append(result, sanitizeTaskInput(item, depth+1, budget))
		}
		return result
	case []string:
		items := make([]any, len(v))
		for i, s := range v {
			items[i] = s
		}
		return sanitizeTaskInput(items, depth, budget)
	case string:
		if strings.HasPrefix(strings.ToLower(v), "data:") {
			return "[file data omitted]"
		}
		if u, err := url.Parse(v); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			u.User = nil
			u.RawQuery = ""
			u.Fragment = ""
			v = u.String()
		}
		maxLength := min(20000, max(*budget, 0))
		if len(v) > maxLength {
			for maxLength > 0 && !utf8.ValidString(v[:maxLength]) {
				maxLength--
			}
			v = v[:maxLength] + " [truncated in input log]"
		}
		*budget -= len(v)
		return v
	default:
		*budget -= 16
		return v
	}
}

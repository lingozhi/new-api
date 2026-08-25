package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func UnmarshalJsonStr(data string, v any) error {
	return json.Unmarshal(StringToByteSlice(data), v)
}

func DecodeJson(reader io.Reader, v any) error {
	return json.NewDecoder(reader).Decode(v)
}

// UnmarshalJSONArrayWithLimit decodes an array without materializing more than
// maxItems elements. It is intended for request DTO fields where a compact JSON
// array could otherwise expand into an unbounded number of Go objects.
func UnmarshalJSONArrayWithLimit[T any](data []byte, maxItems int) ([]T, error) {
	if maxItems <= 0 {
		return nil, errors.New("JSON array item limit must be positive")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '[' {
		return nil, errors.New("JSON value must be an array")
	}
	items := make([]T, 0, min(maxItems, 8))
	for decoder.More() {
		if len(items) >= maxItems {
			return nil, fmt.Errorf("JSON array must not contain more than %d items", maxItems)
		}
		var item T
		if err := decoder.Decode(&item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, errors.New("unexpected data after JSON array")
		}
		return nil, err
	}
	return items, nil
}

func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func GetJsonType(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "unknown"
	}
	firstChar := trimmed[0]
	switch firstChar {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// JsonRawMessageToString returns JSON strings as their decoded value and other JSON values as raw text.
func JsonRawMessageToString(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] != '"' {
		return string(trimmed)
	}
	var value string
	if err := Unmarshal(trimmed, &value); err != nil {
		return string(trimmed)
	}
	return value
}

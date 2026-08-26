package middleware

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

const (
	autoDLAudioModelRequestContextKey         = "autodl_audio_model_request"
	autoDLAudioAdmissionLeaseContextKey       = "autodl_audio_admission_lease"
	autoDLAudioHeavyBodyThreshold       int64 = 1 << 20
	maxConcurrentAutoDLAudioBodies            = 4
	maxConcurrentAutoDLAudioPerSubject        = 2
	autoDLAudioHeavyRateLimitMark             = "ADLAH"
	autoDLAudioIndexRateLimitMark             = "ADLAI"
	autoDLAudioTaskReadRateLimitMark          = "ADLAR"
	autoDLAudioArtifactRateLimitMark          = "ADLAF"
	autoDLAudioTaskReadRateLimitNum           = 30
)

const autoDLAudioFixedWindowScript = `
local current = redis.call('INCR', KEYS[1])
local ttl = redis.call('TTL', KEYS[1])
if current == 1 or ttl < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
if current > tonumber(ARGV[1]) then
  return 0
end
return 1
`

var (
	autoDLAudioBodySlots = make(chan struct{}, maxConcurrentAutoDLAudioBodies)
	autoDLAudioActive    = struct {
		sync.Mutex
		bySubject map[string]int
	}{bySubject: make(map[string]int)}
)

type autoDLAudioAdmissionLease struct {
	subject string
	once    sync.Once
}

// AutoDLAudioAdmission bounds every potentially large speech body before it
// is read. Small ordinary TTS requests keep their existing path; IndexTTS2 is
// identified with a model-only decode and holds a lease before any full audio
// request or base64 payload is decoded.
func AutoDLAudioAdmission() gin.HandlerFunc {
	return func(c *gin.Context) {
		subject, ok := autoDLAudioRateLimitSubject(c)
		if !ok {
			abortWithOpenAiMessage(c, http.StatusUnauthorized, "invalid token context")
			return
		}

		heavyBody := autoDLAudioRequestMayBeHeavy(c)
		var lease *autoDLAudioAdmissionLease
		defer func() {
			if lease != nil {
				lease.release()
			}
		}()

		if heavyBody {
			allowed, err := allowAutoDLAudioFixedWindow(c, autoDLAudioHeavyRateLimitMark, common.UploadRateLimitNum, common.UploadRateLimitDuration)
			if err != nil {
				common.SysError("check AutoDL audio heavy-body rate limit: " + err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}
			if !allowed {
				c.Header("Retry-After", strconv.FormatInt(common.UploadRateLimitDuration, 10))
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, "speech request rate limit exceeded")
				return
			}
			lease = tryAcquireAutoDLAudioAdmission(subject)
			if lease == nil {
				c.Header("Retry-After", "1")
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, "speech request capacity is temporarily unavailable")
				return
			}
			c.Set(autoDLAudioAdmissionLeaseContextKey, lease)
		}

		if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
			c.Next()
			return
		}
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			// The distributor owns the public body-read error and disconnect
			// diagnostics.
			c.Next()
			return
		}
		if storage.Size() > common.MaxAutoDLWorkflowBodyBytes {
			model, probeErr := probeAutoDLAudioJSONModel(storage)
			if probeErr != nil {
				// Malformed JSON remains the distributor's validation error. Replay
				// also refuses to materialize oversized, unclassified bodies.
				c.Next()
				return
			}
			if model == constant.AutoDLModelIndexTTS2 {
				abortWithOpenAiMessage(c, http.StatusRequestEntityTooLarge, "request body must not exceed 64 MiB")
				return
			}
			if lease != nil {
				lease.release()
				lease = nil
				c.Set(autoDLAudioAdmissionLeaseContextKey, nil)
			}
			c.Next()
			return
		}
		modelRequest, err := getModelFromJSONBody(c)
		if err != nil {
			// The normal distributor owns the public validation error. Keep a
			// heavy-body lease, if any, while it replays the bounded body.
			c.Next()
			return
		}
		c.Set(autoDLAudioModelRequestContextKey, modelRequest)

		if modelRequest.Model != constant.AutoDLModelIndexTTS2 {
			if lease != nil {
				lease.release()
				lease = nil
				c.Set(autoDLAudioAdmissionLeaseContextKey, nil)
			}
			c.Next()
			return
		}

		if lease == nil {
			lease = tryAcquireAutoDLAudioAdmission(subject)
			if lease == nil {
				c.Header("Retry-After", "1")
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, "speech request capacity is temporarily unavailable")
				return
			}
			c.Set(autoDLAudioAdmissionLeaseContextKey, lease)
		}
		c.Next()
	}
}

// probeAutoDLAudioJSONModel reads only the top-level model string while
// streaming past all other JSON values. encoding/json.Decoder.Decode buffers
// a complete top-level value before unmarshalling, which is unsafe for the
// model-only admission check on bodies larger than the IndexTTS2 ceiling.
func probeAutoDLAudioJSONModel(storage common.BodyStorage) (string, error) {
	if _, err := storage.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	defer func() { _, _ = storage.Seek(0, io.SeekStart) }()

	reader := bufio.NewReaderSize(storage, 32<<10)
	first, err := readAutoDLAudioJSONNonSpace(reader)
	if err != nil {
		return "", err
	}
	if first != '{' {
		return "", fmt.Errorf("request body must be a JSON object")
	}

	model := ""
	next, err := readAutoDLAudioJSONNonSpace(reader)
	if err != nil {
		return "", err
	}
	if next == '}' {
		return model, nil
	}
	if err := reader.UnreadByte(); err != nil {
		return "", err
	}

	for {
		openingQuote, err := readAutoDLAudioJSONNonSpace(reader)
		if err != nil {
			return "", err
		}
		if openingQuote != '"' {
			return "", fmt.Errorf("JSON object key must be a string")
		}
		key, keyOverflow, err := readAutoDLAudioJSONString(reader, 256)
		if err != nil {
			return "", err
		}
		colon, err := readAutoDLAudioJSONNonSpace(reader)
		if err != nil {
			return "", err
		}
		if colon != ':' {
			return "", fmt.Errorf("JSON object key must be followed by a colon")
		}
		valueStart, err := readAutoDLAudioJSONNonSpace(reader)
		if err != nil {
			return "", err
		}

		if !keyOverflow && key == "model" && valueStart == '"' {
			value, overflow, err := readAutoDLAudioJSONString(reader, 1024)
			if err != nil {
				return "", err
			}
			if overflow {
				model = ""
			} else {
				model = value
			}
		} else if err := skipAutoDLAudioJSONValue(reader, valueStart); err != nil {
			return "", err
		}

		separator, err := readAutoDLAudioJSONNonSpace(reader)
		if err != nil {
			return "", err
		}
		switch separator {
		case ',':
			continue
		case '}':
			return model, nil
		default:
			return "", fmt.Errorf("invalid JSON object separator")
		}
	}
}

func readAutoDLAudioJSONNonSpace(reader *bufio.Reader) (byte, error) {
	for {
		value, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		switch value {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			return value, nil
		}
	}
}

// readAutoDLAudioJSONString consumes a JSON string after its opening quote.
// It keeps only short candidate keys/model values and streams past oversized
// strings without retaining them.
func readAutoDLAudioJSONString(reader *bufio.Reader, limit int) (string, bool, error) {
	value := make([]byte, 0, 32)
	escaped := false
	overflow := false
	for {
		current, err := reader.ReadByte()
		if err != nil {
			return "", false, err
		}
		if !overflow {
			if len(value) >= limit {
				value = nil
				overflow = true
			} else {
				value = append(value, current)
			}
		}
		if escaped {
			escaped = false
			continue
		}
		if current == '\\' {
			escaped = true
			continue
		}
		if current != '"' {
			continue
		}
		if overflow {
			return "", true, nil
		}
		decoded, err := strconv.Unquote(`"` + string(value))
		if err != nil {
			return "", false, err
		}
		return decoded, false, nil
	}
}

func skipAutoDLAudioJSONValue(reader *bufio.Reader, first byte) error {
	if first == '"' {
		_, _, err := readAutoDLAudioJSONString(reader, 0)
		return err
	}
	if first == '{' || first == '[' {
		stack := []byte{first}
		for len(stack) > 0 {
			current, err := reader.ReadByte()
			if err != nil {
				return err
			}
			switch current {
			case '"':
				if _, _, err := readAutoDLAudioJSONString(reader, 0); err != nil {
					return err
				}
			case '{', '[':
				if len(stack) >= 1024 {
					return fmt.Errorf("JSON nesting is too deep")
				}
				stack = append(stack, current)
			case '}', ']':
				expected := byte('{')
				if current == ']' {
					expected = '['
				}
				if stack[len(stack)-1] != expected {
					return fmt.Errorf("mismatched JSON container")
				}
				stack = stack[:len(stack)-1]
			}
		}
		return nil
	}

	for {
		current, err := reader.ReadByte()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch current {
		case ',', '}':
			return reader.UnreadByte()
		case ' ', '\t', '\r', '\n':
			return nil
		}
	}
}

func autoDLAudioRequestMayBeHeavy(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return true
	}
	if _, encoded := c.Get(middlewareOriginalContentEncodingKey); encoded {
		return true
	}
	return c.Request.ContentLength < 0 || c.Request.ContentLength > autoDLAudioHeavyBodyThreshold
}

func autoDLAudioRateLimitSubject(c *gin.Context) (string, bool) {
	if userID := c.GetInt("id"); userID > 0 {
		return "user:" + strconv.Itoa(userID), true
	}
	if tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId); tokenID > 0 {
		return "token:" + strconv.Itoa(tokenID), true
	}
	return "", false
}

func tryAcquireAutoDLAudioAdmission(subject string) *autoDLAudioAdmissionLease {
	autoDLAudioActive.Lock()
	defer autoDLAudioActive.Unlock()
	if subject == "" || autoDLAudioActive.bySubject[subject] >= maxConcurrentAutoDLAudioPerSubject {
		return nil
	}
	select {
	case autoDLAudioBodySlots <- struct{}{}:
		autoDLAudioActive.bySubject[subject]++
		return &autoDLAudioAdmissionLease{subject: subject}
	default:
		return nil
	}
}

func (l *autoDLAudioAdmissionLease) release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		autoDLAudioActive.Lock()
		if active := autoDLAudioActive.bySubject[l.subject]; active <= 1 {
			delete(autoDLAudioActive.bySubject, l.subject)
		} else {
			autoDLAudioActive.bySubject[l.subject] = active - 1
		}
		<-autoDLAudioBodySlots
		autoDLAudioActive.Unlock()
	})
}

// ReleaseAutoDLAudioAdmission releases large input resources once an AutoDL
// task is durably accepted, before its synchronous polling wait begins.
func ReleaseAutoDLAudioAdmission(c *gin.Context) {
	if c == nil {
		return
	}
	value, exists := c.Get(autoDLAudioAdmissionLeaseContextKey)
	if !exists || value == nil {
		return
	}
	lease, ok := value.(*autoDLAudioAdmissionLease)
	if !ok {
		return
	}
	lease.release()
	c.Set(autoDLAudioAdmissionLeaseContextKey, nil)
}

func allowAutoDLAudioFixedWindow(c *gin.Context, mark string, limit int, duration int64) (bool, error) {
	if limit <= 0 || duration <= 0 {
		return true, nil
	}
	subject, ok := autoDLAudioRateLimitSubject(c)
	if !ok {
		return false, fmt.Errorf("authenticated rate-limit subject is unavailable")
	}
	key := "rateLimit:" + mark + ":" + subject
	if !common.RedisEnabled {
		inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
		return inMemoryRateLimiter.Request(key, limit, duration), nil
	}
	allowed, err := common.RDB.Eval(
		c.Request.Context(),
		autoDLAudioFixedWindowScript,
		[]string{key},
		limit,
		duration,
	).Int64()
	if err != nil {
		return false, err
	}
	return allowed == 1, nil
}

func autoDLAudioIndexRateLimit(c *gin.Context) bool {
	allowed, err := allowAutoDLAudioFixedWindow(c, autoDLAudioIndexRateLimitMark, common.DownloadRateLimitNum, common.DownloadRateLimitDuration)
	if err != nil {
		common.SysError("check AutoDL audio task rate limit: " + err.Error())
		abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
		return false
	}
	if !allowed {
		c.Header("Retry-After", strconv.FormatInt(common.DownloadRateLimitDuration, 10))
		abortWithOpenAiMessage(c, http.StatusTooManyRequests, "audio request rate limit exceeded")
		return false
	}
	return true
}

// AutoDLAudioTaskReadRateLimit permits normal Retry-After polling while
// preventing a valid token from creating unbounded cross-instance poll load.
func AutoDLAudioTaskReadRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		allowed, err := allowAutoDLAudioFixedWindow(c, autoDLAudioTaskReadRateLimitMark, autoDLAudioTaskReadRateLimitNum, common.DownloadRateLimitDuration)
		if err != nil {
			common.SysError("check AutoDL audio task read rate limit: " + err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			c.Header("Retry-After", strconv.FormatInt(common.DownloadRateLimitDuration, 10))
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, "audio task read rate limit exceeded")
		}
	}
}

// AllowAutoDLAudioArtifactFetch is called immediately before a successful task
// is refreshed or downloaded. Unlike the legacy IP limiter, this boundary is
// token keyed and atomic across instances.
func AllowAutoDLAudioArtifactFetch(c *gin.Context) bool {
	allowed, err := allowAutoDLAudioFixedWindow(c, autoDLAudioArtifactRateLimitMark, common.DownloadRateLimitNum, common.DownloadRateLimitDuration)
	if err != nil {
		common.SysError("check AutoDL audio artifact rate limit: " + err.Error())
		abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
		return false
	}
	if !allowed {
		c.Header("Retry-After", strconv.FormatInt(common.DownloadRateLimitDuration, 10))
		abortWithOpenAiMessage(c, http.StatusTooManyRequests, "audio result rate limit exceeded")
		return false
	}
	return true
}

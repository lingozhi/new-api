package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var autoDLAudioTestSubject atomic.Int64

func nextAutoDLAudioTestSubject() int {
	return int(900_000_000 + autoDLAudioTestSubject.Add(1))
}

func autoDLAudioTestAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		subject, _ := strconv.Atoi(c.GetHeader("X-Test-Token-ID"))
		c.Set("id", subject)
		c.Set("token_id", subject)
		c.Next()
	}
}

func TestAutoDLAudioReplayRateLimitDoesNotThrottleOrdinaryTTS(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(BodyStorageCleanup())
	engine.Use(autoDLAudioTestAuth())
	engine.POST("/v1/audio/speech", AutoDLAudioAdmission(), AutoDLAudioReplayRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	ordinarySubject := strconv.Itoa(nextAutoDLAudioTestSubject())
	for index := 0; index < common.DownloadRateLimitNum+1; index++ {
		request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"gpt-4o-mini-tts","input":"hello","voice":"alloy"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Test-Token-ID", ordinarySubject)
		request.RemoteAddr = "198.51.100.77:1234"
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusNoContent, recorder.Code)
	}

	indexSubject := strconv.Itoa(nextAutoDLAudioTestSubject())
	missingKeyRequest := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"indextts2-v1"}`))
	missingKeyRequest.Header.Set("Content-Type", "application/json")
	missingKeyRequest.Header.Set("X-Test-Token-ID", indexSubject)
	missingKeyRecorder := httptest.NewRecorder()
	engine.ServeHTTP(missingKeyRecorder, missingKeyRequest)
	assert.Equal(t, http.StatusBadRequest, missingKeyRecorder.Code, missingKeyRecorder.Body.String())

	for index := 0; index < common.DownloadRateLimitNum+1; index++ {
		request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"indextts2-v1","input":"hello","voice":"data:audio/wav;base64,UklGRg=="}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Test-Token-ID", indexSubject)
		request.Header.Set("Idempotency-Key", "index-rate-limit")
		request.RemoteAddr = "198.51.100.78:1234"
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		if index < common.DownloadRateLimitNum {
			assert.Equal(t, http.StatusNoContent, recorder.Code)
		} else {
			assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
		}
	}
}

type autoDLAudioReadCounter struct {
	io.Reader
	reads atomic.Int64
}

func (r *autoDLAudioReadCounter) Read(p []byte) (int, error) {
	r.reads.Add(1)
	return r.Reader.Read(p)
}

func (r *autoDLAudioReadCounter) Close() error { return nil }

type autoDLAudioReportedSizeStorage struct {
	*bytes.Reader
	data []byte
	size int64
}

func newAutoDLAudioReportedSizeStorage(data []byte, size int64) *autoDLAudioReportedSizeStorage {
	return &autoDLAudioReportedSizeStorage{Reader: bytes.NewReader(data), data: data, size: size}
}

func (s *autoDLAudioReportedSizeStorage) Close() error           { return nil }
func (s *autoDLAudioReportedSizeStorage) Bytes() ([]byte, error) { return s.data, nil }
func (s *autoDLAudioReportedSizeStorage) Size() int64            { return s.size }
func (s *autoDLAudioReportedSizeStorage) IsDisk() bool           { return true }

func TestAutoDLAudioAdmissionRejectsOversizedIndexBeforeReplay(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })

	body := []byte(`{"ignored":{"nested":["value"]},"model":"indextts2-v1"}`)
	storage := newAutoDLAudioReportedSizeStorage(body, common.MaxAutoDLWorkflowBodyBytes+1)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(BodyStorageCleanup(), autoDLAudioTestAuth(), func(c *gin.Context) {
		c.Set(common.KeyBodyStorage, storage)
		c.Next()
	})
	downstreamCalled := false
	engine.POST("/v1/audio/speech", AutoDLAudioAdmission(), func(c *gin.Context) {
		downstreamCalled = true
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(string(body)))
	request.ContentLength = autoDLAudioHeavyBodyThreshold + 1
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-Token-ID", strconv.Itoa(nextAutoDLAudioTestSubject()))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code, recorder.Body.String())
	assert.False(t, downstreamCalled)
}

func TestProbeAutoDLAudioJSONModelStreamsPastLargeUnknownValues(t *testing.T) {
	body := []byte(`{"task_type":"` + strings.Repeat("x", 1<<20) + `","nested":{"model":"not-top-level"},"\u006dodel":"indextts2-v1"}`)
	storage := newAutoDLAudioReportedSizeStorage(body, int64(len(body)))

	model, err := probeAutoDLAudioJSONModel(storage)

	require.NoError(t, err)
	assert.Equal(t, "indextts2-v1", model)
	position, err := storage.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	assert.Zero(t, position, "the reusable body must be rewound after the probe")
}

func TestAutoDLAudioHeavyBodyRateLimitRejectsBeforeReadingBody(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(BodyStorageCleanup(), autoDLAudioTestAuth())
	engine.POST("/v1/audio/speech", AutoDLAudioAdmission(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	subject := strconv.Itoa(nextAutoDLAudioTestSubject())
	for index := 0; index < common.UploadRateLimitNum; index++ {
		request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"gpt-4o-mini-tts"}`))
		request.ContentLength = autoDLAudioHeavyBodyThreshold + 1
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Test-Token-ID", subject)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	}

	body := &autoDLAudioReadCounter{Reader: strings.NewReader(`{"model":"gpt-4o-mini-tts"}`)}
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	request.Body = body
	request.ContentLength = autoDLAudioHeavyBodyThreshold + 1
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-Token-ID", subject)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
	assert.Zero(t, body.reads.Load(), "rate-limited heavy body must not be consumed")
}

func TestAutoDLAudioHeavyBodyAdmissionRejectsBeforeReadingWhenFull(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })

	gin.SetMode(gin.TestMode)
	entered := make(chan struct{}, maxConcurrentAutoDLAudioBodies)
	release := make(chan struct{})
	engine := gin.New()
	engine.Use(BodyStorageCleanup(), autoDLAudioTestAuth())
	engine.POST("/v1/audio/speech", AutoDLAudioAdmission(), AutoDLAudioReplayRateLimit(), func(c *gin.Context) {
		entered <- struct{}{}
		<-release
		c.Status(http.StatusNoContent)
	})

	var wg sync.WaitGroup
	for index := 0; index < maxConcurrentAutoDLAudioBodies; index++ {
		wg.Add(1)
		go func(subject int) {
			defer wg.Done()
			request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"indextts2-v1"}`))
			request.ContentLength = autoDLAudioHeavyBodyThreshold + 1
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Test-Token-ID", strconv.Itoa(subject))
			request.Header.Set("Idempotency-Key", "fill-admission-"+strconv.Itoa(subject))
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			assert.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
		}(nextAutoDLAudioTestSubject())
	}

	for index := 0; index < maxConcurrentAutoDLAudioBodies; index++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			close(release)
			wg.Wait()
			require.FailNow(t, "timed out filling AutoDL audio admission slots")
		}
	}

	body := &autoDLAudioReadCounter{Reader: strings.NewReader(`{"model":"indextts2-v1"}`)}
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	request.Body = body
	request.ContentLength = autoDLAudioHeavyBodyThreshold + 1
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-Token-ID", strconv.Itoa(nextAutoDLAudioTestSubject()))
	request.Header.Set("Idempotency-Key", "capacity-rejected")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
	assert.Zero(t, body.reads.Load(), "capacity-rejected heavy body must not be consumed")
	close(release)
	wg.Wait()
}

func TestAutoDLAudioArtifactRateLimitUsesAuthenticatedUserInsteadOfClientIP(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(autoDLAudioTestAuth())
	engine.GET("/v1/audio/speech/:task_id", func(c *gin.Context) {
		if AllowAutoDLAudioArtifactFetch(c) {
			c.Status(http.StatusNoContent)
		}
	})

	subject := strconv.Itoa(nextAutoDLAudioTestSubject())
	for index := 0; index < common.DownloadRateLimitNum+1; index++ {
		request := httptest.NewRequest(http.MethodGet, "/v1/audio/speech/task_result", nil)
		request.Header.Set("X-Test-Token-ID", subject)
		request.Header.Set("X-Forwarded-For", "203.0.113."+strconv.Itoa(index+1))
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		if index < common.DownloadRateLimitNum {
			assert.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
		} else {
			assert.Equal(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/audio/speech/task_result", nil)
	request.Header.Set("X-Test-Token-ID", strconv.Itoa(nextAutoDLAudioTestSubject()))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
}

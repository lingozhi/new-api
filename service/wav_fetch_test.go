package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type generatedWAVRoundTripFunc func(*http.Request) (*http.Response, error)

func (f generatedWAVRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func installGeneratedWAVFetch(t *testing.T, response *http.Response, requestErr error, validationErr error) {
	t.Helper()
	previousValidate := validateGeneratedWAVURL
	previousClient := generatedWAVHTTPClient
	validateGeneratedWAVURL = func(string) error { return validationErr }
	generatedWAVHTTPClient = func() *http.Client {
		return &http.Client{Transport: generatedWAVRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			if requestErr != nil {
				return nil, requestErr
			}
			response.Request = request
			return response, nil
		})}
	}
	t.Cleanup(func() {
		validateGeneratedWAVURL = previousValidate
		generatedWAVHTTPClient = previousClient
	})
}

func installGeneratedWAVTransport(t *testing.T, transport http.RoundTripper, timeout time.Duration) {
	t.Helper()
	previousValidate := validateGeneratedWAVURL
	previousClient := generatedWAVHTTPClient
	previousTimeout := generatedWAVFetchTimeout
	validateGeneratedWAVURL = func(string) error { return nil }
	generatedWAVHTTPClient = func() *http.Client { return &http.Client{Transport: transport} }
	generatedWAVFetchTimeout = timeout
	t.Cleanup(func() {
		validateGeneratedWAVURL = previousValidate
		generatedWAVHTTPClient = previousClient
		generatedWAVFetchTimeout = previousTimeout
	})
}

func generatedWAVTestResponse(contentType string, body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func generatedWAVTestBytes() []byte {
	return []byte{
		'R', 'I', 'F', 'F', 38, 0, 0, 0, 'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ', 16, 0, 0, 0,
		1, 0, 1, 0, 0x40, 0x1f, 0, 0, 0x80, 0x3e, 0, 0, 2, 0, 16, 0,
		'd', 'a', 't', 'a', 2, 0, 0, 0, 0, 0,
	}
}

func TestFetchValidatedWAVSpoolsValidAudioToDisk(t *testing.T) {
	wav := generatedWAVTestBytes()
	installGeneratedWAVFetch(t, generatedWAVTestResponse("application/octet-stream", wav), nil, nil)

	storage, err := FetchValidatedWAV(context.Background(), "https://media.example.com/result.wav", MaxGeneratedWAVBytes)
	require.NoError(t, err)
	defer storage.Close()
	assert.True(t, storage.IsDisk())
	actual, err := io.ReadAll(storage)
	require.NoError(t, err)
	assert.Equal(t, wav, actual)
}

func TestFetchValidatedWAVClassifiesPermanentAndTemporaryFailures(t *testing.T) {
	tests := []struct {
		name          string
		response      *http.Response
		requestErr    error
		validationErr error
		wantTemporary bool
	}{
		{name: "unsafe URL", response: generatedWAVTestResponse("audio/wav", nil), validationErr: errors.New("blocked")},
		{name: "network failure", response: generatedWAVTestResponse("audio/wav", nil), requestErr: errors.New("dial failed"), wantTemporary: true},
		{name: "signed URL expired", response: &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: http.NoBody}, wantTemporary: true},
		{name: "wrong MIME", response: generatedWAVTestResponse("text/html", []byte("not audio"))},
		{name: "wrong magic", response: generatedWAVTestResponse("audio/wav", []byte("not a wave file"))},
		{name: "header without chunks", response: generatedWAVTestResponse("audio/wav", []byte{'R', 'I', 'F', 'F', 4, 0, 0, 0, 'W', 'A', 'V', 'E'})},
		{name: "truncated RIFF", response: generatedWAVTestResponse("audio/wav", generatedWAVTestBytes()[:45])},
		{name: "declared oversize", response: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"audio/wav"}}, Body: http.NoBody, ContentLength: MaxGeneratedWAVBytes + 1}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			installGeneratedWAVFetch(t, test.response, test.requestErr, test.validationErr)
			storage, err := FetchValidatedWAV(context.Background(), "https://media.example.com/result.wav", MaxGeneratedWAVBytes)
			require.Error(t, err)
			assert.Nil(t, storage)
			var temporary interface{ Temporary() bool }
			require.True(t, errors.As(err, &temporary))
			assert.Equal(t, test.wantTemporary, temporary.Temporary())
		})
	}
}

func TestFetchValidatedWAVDoesNotExposeSignedURLFromNetworkErrors(t *testing.T) {
	signedURL := "https://media.example.com/result.wav?signature=secret"
	installGeneratedWAVFetch(t, generatedWAVTestResponse("audio/wav", nil), errors.New("Get \""+signedURL+"\": dial failed"), nil)

	storage, err := FetchValidatedWAV(context.Background(), signedURL, MaxGeneratedWAVBytes)
	require.Error(t, err)
	assert.Nil(t, storage)
	assert.NotContains(t, err.Error(), "signature")
	assert.NotContains(t, err.Error(), "secret")
}

func TestFetchValidatedWAVRejectsBodyAboveLimit(t *testing.T) {
	wav := generatedWAVTestBytes()
	response := generatedWAVTestResponse("audio/wav", wav)
	response.ContentLength = -1
	installGeneratedWAVFetch(t, response, nil, nil)

	storage, err := FetchValidatedWAV(context.Background(), "https://media.example.com/result.wav", int64(len(wav)-1))
	require.Error(t, err)
	assert.Nil(t, storage)
	assert.False(t, isTemporaryPollingError(err))
}

func TestFetchValidatedWAVBoundsHeaderAndBodyWaits(t *testing.T) {
	tests := []struct {
		name      string
		transport http.RoundTripper
	}{
		{
			name: "response headers",
			transport: generatedWAVRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				<-request.Context().Done()
				return nil, request.Context().Err()
			}),
		},
		{
			name: "response body",
			transport: generatedWAVRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"audio/wav"}},
					Body:       &contextBlockingReadCloser{context: request.Context()},
				}, nil
			}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			installGeneratedWAVTransport(t, test.transport, 10*time.Millisecond)
			storage, err := FetchValidatedWAV(context.Background(), "https://media.example.com/result.wav", MaxGeneratedWAVBytes)
			require.Error(t, err)
			assert.Nil(t, storage)
			assert.True(t, isTemporaryPollingError(err))
		})
	}
}

func TestFetchValidatedWAVLimitsConcurrentDownloadsAndHonorsCancellation(t *testing.T) {
	wav := generatedWAVTestBytes()
	entered := make(chan struct{}, maxConcurrentGeneratedWAVFetches)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	var active atomic.Int32
	var maxActive atomic.Int32
	installGeneratedWAVTransport(t, generatedWAVRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for observed := maxActive.Load(); current > observed; observed = maxActive.Load() {
			if maxActive.CompareAndSwap(observed, current) {
				break
			}
		}
		entered <- struct{}{}
		select {
		case <-release:
			return generatedWAVTestResponse("audio/wav", wav), nil
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
	}), time.Second)

	results := make(chan error, maxConcurrentGeneratedWAVFetches)
	for range maxConcurrentGeneratedWAVFetches {
		go func() {
			storage, err := FetchValidatedWAV(context.Background(), "https://media.example.com/result.wav", MaxGeneratedWAVBytes)
			if storage != nil {
				_ = storage.Close()
			}
			results <- err
		}()
	}
	for range maxConcurrentGeneratedWAVFetches {
		select {
		case <-entered:
		case <-time.After(time.Second):
			require.FailNow(t, "timed out waiting for the protected download slots to fill")
		}
	}

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	storage, err := FetchValidatedWAV(canceledContext, "https://media.example.com/result.wav", MaxGeneratedWAVBytes)
	require.Error(t, err)
	assert.Nil(t, storage)
	assert.ErrorIs(t, err, context.Canceled)
	assert.True(t, isTemporaryPollingError(err))
	assert.Equal(t, int32(maxConcurrentGeneratedWAVFetches), maxActive.Load())
	assert.Len(t, entered, 0, "a canceled waiter must not start another download")

	releaseOnce.Do(func() { close(release) })
	for range maxConcurrentGeneratedWAVFetches {
		require.NoError(t, <-results)
	}
}

func TestFetchValidatedWAVReleasesDownloadSlot(t *testing.T) {
	wav := generatedWAVTestBytes()
	var requests atomic.Int32
	installGeneratedWAVTransport(t, generatedWAVRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return generatedWAVTestResponse("audio/wav", wav), nil
	}), time.Second)

	for range maxConcurrentGeneratedWAVFetches + 1 {
		storage, err := FetchValidatedWAV(context.Background(), "https://media.example.com/result.wav", MaxGeneratedWAVBytes)
		require.NoError(t, err)
		require.NoError(t, storage.Close())
	}
	assert.Equal(t, int32(maxConcurrentGeneratedWAVFetches+1), requests.Load())
}

type contextBlockingReadCloser struct {
	context context.Context
}

func (r *contextBlockingReadCloser) Read([]byte) (int, error) {
	<-r.context.Done()
	return 0, r.context.Err()
}

func (r *contextBlockingReadCloser) Close() error {
	return nil
}

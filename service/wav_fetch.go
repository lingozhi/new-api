package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	MaxGeneratedWAVBytes             = int64(64 << 20)
	generatedWAVTimeout              = 90 * time.Second
	maxConcurrentGeneratedWAVFetches = 4
)

var (
	validateGeneratedWAVURL  = ValidateStrictHTTPSProtectedFetchURL
	generatedWAVHTTPClient   = GetStrictHTTPSDirectSSRFProtectedHTTPClient
	generatedWAVFetchTimeout = generatedWAVTimeout
	generatedWAVFetchSlots   = make(chan struct{}, maxConcurrentGeneratedWAVFetches)
)

type generatedWAVFetchError struct {
	err       error
	temporary bool
}

func (e *generatedWAVFetchError) Error() string {
	if e == nil || e.err == nil {
		return "generated WAV fetch failed"
	}
	return e.err.Error()
}

func (e *generatedWAVFetchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *generatedWAVFetchError) Temporary() bool {
	return e != nil && e.temporary
}

// FetchValidatedWAV downloads a user-visible generated WAV through the strict
// direct SSRF-protected transport and spools it to disk. Callers own the
// returned storage and must close it.
func FetchValidatedWAV(ctx context.Context, resultURL string, maxBytes int64) (common.BodyStorage, error) {
	if maxBytes <= 0 {
		return nil, &generatedWAVFetchError{err: errors.New("generated audio download limit is invalid")}
	}
	resultURL = strings.TrimSpace(resultURL)
	if resultURL == "" {
		return nil, &generatedWAVFetchError{err: errors.New("generated audio URL is empty")}
	}
	if err := validateGeneratedWAVURL(resultURL); err != nil {
		return nil, &generatedWAVFetchError{err: errors.New("generated audio URL is unsafe")}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, generatedWAVFetchTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, &generatedWAVFetchError{
			err:       fmt.Errorf("wait for generated audio download capacity: %w", err),
			temporary: true,
		}
	}
	select {
	case generatedWAVFetchSlots <- struct{}{}:
		defer func() { <-generatedWAVFetchSlots }()
	case <-ctx.Done():
		return nil, &generatedWAVFetchError{
			err:       fmt.Errorf("wait for generated audio download capacity: %w", ctx.Err()),
			temporary: true,
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resultURL, nil)
	if err != nil {
		return nil, &generatedWAVFetchError{err: errors.New("generated audio request is invalid")}
	}
	req.Header.Set("Accept", "audio/wav, audio/x-wav, application/octet-stream")
	resp, err := generatedWAVHTTPClient().Do(req)
	if err != nil {
		return nil, &generatedWAVFetchError{err: errors.New("generated audio download failed"), temporary: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &generatedWAVFetchError{
			err:       fmt.Errorf("generated audio returned HTTP %d", resp.StatusCode),
			temporary: true,
		}
	}

	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if contentType != "audio/wav" && contentType != "audio/x-wav" && contentType != "application/octet-stream" {
		return nil, &generatedWAVFetchError{err: errors.New("generated audio did not have a WAV content type")}
	}
	if resp.ContentLength > maxBytes {
		return nil, &generatedWAVFetchError{err: errors.New("generated audio exceeded the download limit")}
	}

	storage, err := common.CreateDiskBodyStorageFromReader(resp.Body, maxBytes)
	if err != nil {
		if common.IsRequestBodyTooLargeError(err) {
			return nil, &generatedWAVFetchError{err: errors.New("generated audio exceeded the download limit")}
		}
		return nil, &generatedWAVFetchError{err: fmt.Errorf("store generated audio: %w", err), temporary: true}
	}
	if err := ValidateWAV(storage, storage.Size(), 0); err != nil {
		_ = storage.Close()
		return nil, &generatedWAVFetchError{err: errors.New("generated audio is not a valid WAV")}
	}
	return storage, nil
}

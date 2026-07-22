package service

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	pancake "github.com/waffo-com/waffo-pancake-sdk-go"
)

const waffoPancakeMaxActionResponseBytes = 4 << 20

// The current Go SDK does not expose taxIncluded, although Pancake requires it
// on price snapshots. Keep the compatibility wire types local until the SDK
// adds the field.
type waffoPancakePriceInfo struct {
	Amount      string `json:"amount"`
	TaxIncluded bool   `json:"taxIncluded"`
	TaxCategory string `json:"taxCategory"`
}

type waffoPancakePriceMap map[string]waffoPancakePriceInfo

type waffoPancakeActionNotice struct {
	Message string `json:"message"`
	Layer   string `json:"layer,omitempty"`
	AIHint  string `json:"aiHint,omitempty"`
}

type waffoPancakeActionEnvelope struct {
	Data     json.RawMessage            `json:"data"`
	Errors   []waffoPancakeActionNotice `json:"errors,omitempty"`
	Warnings []waffoPancakeActionNotice `json:"warnings,omitempty"`
}

type waffoPancakeActionError struct {
	StatusCode int
	Ambiguous  bool
	Notices    []waffoPancakeActionNotice
	Cause      error
}

func (e *waffoPancakeActionError) Error() string {
	details := make([]string, 0, len(e.Notices)+1)
	for _, notice := range e.Notices {
		message := strings.TrimSpace(notice.Message)
		if message == "" {
			message = "unspecified Waffo Pancake error"
		}
		if notice.Layer != "" {
			message += " [layer=" + notice.Layer + "]"
		}
		if notice.AIHint != "" {
			message += " [hint=" + notice.AIHint + "]"
		}
		details = append(details, message)
	}
	if e.Cause != nil {
		details = append(details, e.Cause.Error())
	}
	if len(details) == 0 {
		details = append(details, "request failed")
	}
	return fmt.Sprintf("Waffo Pancake request failed (status %d): %s", e.StatusCode, strings.Join(details, "; "))
}

func (e *waffoPancakeActionError) Unwrap() error {
	return e.Cause
}

// IsWaffoPancakeActionOutcomeAmbiguous reports failures where Pancake may have
// accepted the idempotent write even though the gateway did not receive a
// trustworthy response. Local orders must remain pending for later webhooks.
func IsWaffoPancakeActionOutcomeAmbiguous(err error) bool {
	var actionErr *waffoPancakeActionError
	return errors.As(err, &actionErr) && actionErr.Ambiguous
}

func postWaffoPancakeAction[T any](ctx context.Context, merchantID, privateKey, path string, body any, idempotencyWindowSeconds int64) (*T, error) {
	merchantID = strings.TrimSpace(merchantID)
	if merchantID == "" {
		return nil, fmt.Errorf("merchant id is required")
	}
	privateRSAKey, err := parseWaffoPancakePrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("parse Waffo Pancake private key: %w", err)
	}
	bodyBytes, err := common.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal Waffo Pancake request body: %w", err)
	}

	timestampSeconds := time.Now().Unix()
	timestamp := strconv.FormatInt(timestampSeconds, 10)
	bodyHash := sha256.Sum256(bodyBytes)
	canonical := http.MethodPost + "\n" + path + "\n" + timestamp + "\n" + base64.StdEncoding.EncodeToString(bodyHash[:])
	canonicalHash := sha256.Sum256([]byte(canonical))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateRSAKey, crypto.SHA256, canonicalHash[:])
	if err != nil {
		return nil, fmt.Errorf("sign Waffo Pancake request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pancake.DefaultBaseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("build Waffo Pancake request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Merchant-Id", merchantID)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Signature", base64.StdEncoding.EncodeToString(signature))
	idempotencyInput := merchantID + ":" + path + ":" + string(bodyBytes)
	if idempotencyWindowSeconds > 0 {
		idempotencyInput = fmt.Sprintf("%s:%d", idempotencyInput, timestampSeconds/idempotencyWindowSeconds)
	}
	idempotencyHash := sha256.Sum256([]byte(idempotencyInput))
	req.Header.Set("X-Idempotency-Key", hex.EncodeToString(idempotencyHash[:]))

	httpClient := *http.DefaultClient
	httpClient.Timeout = waffoPancakeHTTPTimeout
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, &waffoPancakeActionError{Ambiguous: true, Cause: err}
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, waffoPancakeMaxActionResponseBytes+1))
	if err != nil {
		return nil, &waffoPancakeActionError{StatusCode: resp.StatusCode, Ambiguous: true, Cause: fmt.Errorf("read response: %w", err)}
	}
	if len(responseBody) > waffoPancakeMaxActionResponseBytes {
		return nil, &waffoPancakeActionError{StatusCode: resp.StatusCode, Ambiguous: true, Cause: fmt.Errorf("response exceeded %d bytes", waffoPancakeMaxActionResponseBytes)}
	}
	if len(bytes.TrimSpace(responseBody)) == 0 {
		return nil, &waffoPancakeActionError{
			StatusCode: resp.StatusCode,
			Ambiguous:  waffoPancakeHTTPFailureIsAmbiguous(resp.StatusCode),
			Cause:      fmt.Errorf("empty response"),
		}
	}

	var envelope waffoPancakeActionEnvelope
	if err := common.Unmarshal(responseBody, &envelope); err != nil {
		return nil, &waffoPancakeActionError{
			StatusCode: resp.StatusCode,
			Ambiguous:  waffoPancakeHTTPFailureIsAmbiguous(resp.StatusCode),
			Cause:      fmt.Errorf("decode response: %w", err),
		}
	}
	if len(envelope.Warnings) > 0 {
		common.SysLog(fmt.Sprintf("Waffo Pancake action warning path=%q warnings=%q", path, formatWaffoPancakeActionNotices(envelope.Warnings)))
	}
	if len(envelope.Errors) > 0 {
		return nil, &waffoPancakeActionError{
			StatusCode: resp.StatusCode,
			Ambiguous:  resp.StatusCode >= http.StatusInternalServerError || resp.StatusCode == http.StatusRequestTimeout,
			Notices:    envelope.Errors,
		}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &waffoPancakeActionError{
			StatusCode: resp.StatusCode,
			Ambiguous:  waffoPancakeHTTPFailureIsAmbiguous(resp.StatusCode),
		}
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil, &waffoPancakeActionError{StatusCode: resp.StatusCode, Ambiguous: true, Cause: fmt.Errorf("response data is empty")}
	}
	var result T
	if err := common.Unmarshal(envelope.Data, &result); err != nil {
		return nil, &waffoPancakeActionError{StatusCode: resp.StatusCode, Ambiguous: true, Cause: fmt.Errorf("decode response data: %w", err)}
	}
	return &result, nil
}

func waffoPancakeHTTPFailureIsAmbiguous(statusCode int) bool {
	return statusCode < http.StatusBadRequest || statusCode >= http.StatusInternalServerError || statusCode == http.StatusRequestTimeout
}

func formatWaffoPancakeActionNotices(notices []waffoPancakeActionNotice) string {
	parts := make([]string, 0, len(notices))
	for _, notice := range notices {
		part := strings.TrimSpace(notice.Message)
		if notice.Layer != "" {
			part += " [layer=" + notice.Layer + "]"
		}
		if notice.AIHint != "" {
			part += " [hint=" + notice.AIHint + "]"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}

func parseWaffoPancakePrivateKey(privateKey string) (*rsa.PrivateKey, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(normalizeWaffoPancakePrivateKey(privateKey))
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	if key, err := x509.ParsePKCS8PrivateKey(keyBytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
		return rsaKey, nil
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS#8 or PKCS#1 private key: %w", err)
	}
	return key, nil
}

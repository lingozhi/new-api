package autodl

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/service"
)

const (
	maxIndexTTSAudioURLBytes    = 8 * 1024
	maxIndexTTSTotalAudioBytes  = 2 * maxAudioDataBytes
	indexTTSAudioFetchTimeout   = 60 * time.Second
	maxIndexTTSAudioDuration    = 10 * time.Minute
	maxConcurrentIndexTTSFetch  = 4
	indexTTSAudioPayloadVoice   = "prompt_simple"
	indexTTSAudioPayloadEmotion = "emo_ref_audio"
)

var (
	validateIndexTTSAudioURL = service.ValidateStrictHTTPSProtectedFetchURL
	indexTTSAudioHTTPClient  = service.GetStrictHTTPSDirectSSRFProtectedHTTPClient
	materializeIndexTTSAudio = fetchIndexTTSAudioDataURI
	indexTTSAudioFetchSlots  = make(chan struct{}, maxConcurrentIndexTTSFetch)
)

func materializeIndexTTSAudioPayload(ctx context.Context, payload map[string]any) error {
	if payload == nil {
		return errors.New("IndexTTS2 payload is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, indexTTSAudioFetchTimeout)
	defer cancel()
	select {
	case indexTTSAudioFetchSlots <- struct{}{}:
		defer func() { <-indexTTSAudioFetchSlots }()
	case <-ctx.Done():
		return errors.New("IndexTTS2 reference audio fetch capacity is unavailable")
	}
	totalBytes := 0
	for _, field := range []string{indexTTSAudioPayloadVoice, indexTTSAudioPayloadEmotion} {
		value, exists := payload[field]
		if !exists {
			continue
		}
		source, ok := value.(string)
		if !ok || strings.TrimSpace(source) == "" {
			return fmt.Errorf("%s must contain audio", field)
		}
		dataURI, nextTotal, err := materializeIndexTTSAudio(ctx, source, totalBytes)
		if err != nil {
			return fmt.Errorf("%s could not be loaded as valid audio: %w", field, err)
		}
		if nextTotal > maxIndexTTSTotalAudioBytes {
			return errors.New("IndexTTS2 reference audio exceeds the total size limit")
		}
		payload[field] = dataURI
		totalBytes = nextTotal
	}
	return nil
}

func fetchIndexTTSAudioDataURI(ctx context.Context, source string, currentBytes int) (string, int, error) {
	source = strings.TrimSpace(source)
	if currentBytes < 0 || currentBytes > maxIndexTTSTotalAudioBytes {
		return "", currentBytes, errors.New("reference audio total size is invalid")
	}
	if strings.HasPrefix(strings.ToLower(source), "data:") {
		return canonicalIndexTTSAudioDataURI(source, currentBytes)
	}
	if len(source) > maxIndexTTSAudioURLBytes {
		return "", currentBytes, errors.New("reference audio URL is too long")
	}
	if err := validateIndexTTSAudioURL(source); err != nil {
		return "", currentBytes, errors.New("reference audio URL is unsafe")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, indexTTSAudioFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", currentBytes, errors.New("reference audio request is invalid")
	}
	req.Header.Set("Accept", "audio/wav, audio/x-wav, audio/wave, audio/mpeg, audio/mp3, application/octet-stream")
	resp, err := indexTTSAudioHTTPClient().Do(req)
	if err != nil {
		return "", currentBytes, errors.New("reference audio download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", currentBytes, fmt.Errorf("reference audio returned HTTP %d", resp.StatusCode)
	}
	remaining := maxIndexTTSTotalAudioBytes - currentBytes
	itemLimit := maxAudioDataBytes
	if remaining < itemLimit {
		itemLimit = remaining
	}
	if itemLimit <= 0 || resp.ContentLength > int64(itemLimit) {
		return "", currentBytes, errors.New("reference audio exceeds the size limit")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(itemLimit)+1))
	if err != nil {
		return "", currentBytes, errors.New("reference audio download failed")
	}
	if len(data) == 0 || len(data) > itemLimit {
		return "", currentBytes, errors.New("reference audio exceeds the size limit")
	}
	mimeType, err := detectIndexTTSAudio(data)
	if err != nil {
		return "", currentBytes, err
	}
	if !indexTTSAudioContentTypeMatches(resp.Header.Get("Content-Type"), mimeType) {
		return "", currentBytes, errors.New("reference audio content type does not match its bytes")
	}
	return encodeIndexTTSAudioDataURI(mimeType, data), currentBytes + len(data), nil
}

func canonicalIndexTTSAudioDataURI(source string, currentBytes int) (string, int, error) {
	metadataAndData := source[len("data:"):]
	metadata, encoded, found := strings.Cut(metadataAndData, ",")
	if !found || encoded == "" {
		return "", currentBytes, errors.New("reference audio data URI is invalid")
	}
	parts := strings.Split(metadata, ";")
	if len(parts) != 2 || !strings.EqualFold(parts[1], "base64") || strings.ContainsAny(encoded, " \t\r\n") {
		return "", currentBytes, errors.New("reference audio data URI must contain strict base64")
	}
	if len(encoded) > base64.StdEncoding.EncodedLen(maxAudioDataBytes) {
		return "", currentBytes, errors.New("reference audio exceeds the size limit")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(data) == 0 || len(data) > maxAudioDataBytes || len(data) > maxIndexTTSTotalAudioBytes-currentBytes {
		return "", currentBytes, errors.New("reference audio data URI is invalid or too large")
	}
	mimeType, err := detectIndexTTSAudio(data)
	if err != nil {
		return "", currentBytes, err
	}
	if !indexTTSAudioContentTypeMatches(parts[0], mimeType) {
		return "", currentBytes, errors.New("reference audio data URI type does not match its bytes")
	}
	return encodeIndexTTSAudioDataURI(mimeType, data), currentBytes + len(data), nil
}

func detectIndexTTSAudio(data []byte) (string, error) {
	if isValidIndexTTSWAV(data) {
		return "audio/wav", nil
	}
	if isValidIndexTTSMP3(data) {
		return "audio/mpeg", nil
	}
	return "", errors.New("reference audio is not a valid WAV or MP3 file")
}

func isValidIndexTTSWAV(data []byte) bool {
	return service.ValidateWAV(bytes.NewReader(data), int64(len(data)), maxIndexTTSAudioDuration) == nil
}

func isValidIndexTTSMP3(data []byte) bool {
	offset := 0
	if len(data) >= 10 && string(data[:3]) == "ID3" {
		for _, value := range data[6:10] {
			if value&0x80 != 0 {
				return false
			}
		}
		tagSize := int(data[6])<<21 | int(data[7])<<14 | int(data[8])<<7 | int(data[9])
		offset = 10 + tagSize
		if data[5]&0x10 != 0 {
			offset += 10
		}
	}
	frames := 0
	totalSamples := uint64(0)
	streamSampleRate := 0
	for offset+4 <= len(data) {
		if len(data)-offset == 128 && string(data[offset:offset+3]) == "TAG" {
			offset = len(data)
			break
		}
		frameLength, sampleRate, samplesPerFrame, ok := indexTTSMP3FrameInfo(data[offset : offset+4])
		if !ok || frameLength < 4 || offset+frameLength > len(data) {
			return false
		}
		if streamSampleRate == 0 {
			streamSampleRate = sampleRate
		} else if streamSampleRate != sampleRate {
			return false
		}
		totalSamples += uint64(samplesPerFrame)
		if totalSamples > uint64(streamSampleRate)*uint64(maxIndexTTSAudioDuration/time.Second) {
			return false
		}
		frames++
		offset += frameLength
	}
	return offset == len(data) && frames >= 2
}

func indexTTSMP3FrameInfo(header []byte) (frameLength, sampleRate, samplesPerFrame int, ok bool) {
	if len(header) < 4 || header[0] != 0xff || header[1]&0xe0 != 0xe0 {
		return 0, 0, 0, false
	}
	version := (header[1] >> 3) & 0x03
	layer := (header[1] >> 1) & 0x03
	bitrateIndex := (header[2] >> 4) & 0x0f
	sampleRateIndex := (header[2] >> 2) & 0x03
	if version == 1 || layer == 0 || bitrateIndex == 0 || bitrateIndex == 0x0f || sampleRateIndex == 3 {
		return 0, 0, 0, false
	}
	sampleRates := [...]int{44100, 48000, 32000}
	sampleRate = sampleRates[sampleRateIndex]
	if version == 2 {
		sampleRate /= 2
	} else if version == 0 {
		sampleRate /= 4
	}
	mpeg1Layer1 := [...]int{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448}
	mpeg1Layer2 := [...]int{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384}
	mpeg1Layer3 := [...]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	mpeg2Layer1 := [...]int{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256}
	mpeg2Layer23 := [...]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
	var bitrate int
	if version == 3 {
		switch layer {
		case 3:
			bitrate = mpeg1Layer1[bitrateIndex]
		case 2:
			bitrate = mpeg1Layer2[bitrateIndex]
		case 1:
			bitrate = mpeg1Layer3[bitrateIndex]
		}
	} else if layer == 3 {
		bitrate = mpeg2Layer1[bitrateIndex]
	} else {
		bitrate = mpeg2Layer23[bitrateIndex]
	}
	padding := int((header[2] >> 1) & 0x01)
	switch layer {
	case 3:
		samplesPerFrame = 384
		frameLength = (12*bitrate*1000/sampleRate + padding) * 4
	case 2:
		samplesPerFrame = 1152
		frameLength = 144*bitrate*1000/sampleRate + padding
	case 1:
		if version == 3 {
			samplesPerFrame = 1152
			frameLength = 144*bitrate*1000/sampleRate + padding
		} else {
			samplesPerFrame = 576
			frameLength = 72*bitrate*1000/sampleRate + padding
		}
	}
	return frameLength, sampleRate, samplesPerFrame, frameLength > 0
}

func indexTTSAudioContentTypeMatches(contentType, detected string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if contentType == "" || contentType == "application/octet-stream" {
		return true
	}
	if detected == "audio/wav" {
		return contentType == "audio/wav" || contentType == "audio/x-wav" || contentType == "audio/wave"
	}
	return detected == "audio/mpeg" && (contentType == "audio/mpeg" || contentType == "audio/mp3")
}

func encodeIndexTTSAudioDataURI(mimeType string, data []byte) string {
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

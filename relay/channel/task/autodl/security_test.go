package autodl

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAutoDLURLValidatesBaseAndJoinsPath(t *testing.T) {
	endpoint, err := buildAutoDLURL(" https://autodl.example/gateway/ ", "api", "v1", "task/id")
	require.NoError(t, err)
	assert.Equal(t, "https://autodl.example/gateway/api/v1/task%2Fid", endpoint)

	dotSegment, err := buildAutoDLURL("https://autodl.example/gateway", "result", "..")
	require.NoError(t, err)
	assert.Equal(t, "https://autodl.example/gateway/result/%2E%2E", dotSegment)
}

func TestBuildAutoDLURLRejectsUnsafeBaseURL(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		wantError string
	}{
		{name: "empty", baseURL: "", wantError: "is required"},
		{name: "relative", baseURL: "autodl.example", wantError: "absolute HTTPS"},
		{name: "HTTP", baseURL: "http://autodl.example", wantError: "absolute HTTPS"},
		{name: "userinfo", baseURL: "https://token@autodl.example", wantError: "userinfo"},
		{name: "query", baseURL: "https://autodl.example?tenant=1", wantError: "query"},
		{name: "empty query", baseURL: "https://autodl.example?", wantError: "query"},
		{name: "fragment", baseURL: "https://autodl.example#section", wantError: "fragment"},
		{name: "empty fragment", baseURL: "https://autodl.example#", wantError: "fragment"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := buildAutoDLURL(test.baseURL, "api")
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestValidateMediaURLAcceptsPublicHTTPSWithoutDNSLookup(t *testing.T) {
	tests := []string{
		"https://cdn.example.com/media/input.png?signature=abc",
		"https://cdn.example.com:443/media/input.png",
		"https://8.8.8.8/media/input.png",
		"https://[2606:4700:4700::1111]/media/input.png",
	}

	for _, mediaURL := range tests {
		t.Run(mediaURL, func(t *testing.T) {
			validated, totalBytes, err := validateMediaURL(mediaURL, mediaKindImage, 7)
			require.NoError(t, err)
			assert.Equal(t, mediaURL, validated)
			assert.Equal(t, 7, totalBytes)
		})
	}
}

func TestValidateMediaURLRejectsUnsafeExternalTargets(t *testing.T) {
	tests := []struct {
		name      string
		mediaURL  string
		wantError string
	}{
		{name: "HTTP", mediaURL: "http://cdn.example.com/input.png", wantError: "must use HTTPS"},
		{name: "userinfo", mediaURL: "https://user:secret@cdn.example.com/input.png", wantError: "userinfo"},
		{name: "non-default port", mediaURL: "https://cdn.example.com:8443/input.png", wantError: "port 443"},
		{name: "localhost", mediaURL: "https://localhost/input.png", wantError: "localhost"},
		{name: "localhost subdomain", mediaURL: "https://assets.localhost/input.png", wantError: "localhost"},
		{name: "IPv4 loopback", mediaURL: "https://127.0.0.1/input.png", wantError: "public IP"},
		{name: "IPv4 private", mediaURL: "https://10.20.30.40/input.png", wantError: "public IP"},
		{name: "IPv4 link-local", mediaURL: "https://169.254.169.254/input.png", wantError: "public IP"},
		{name: "IPv4 reserved", mediaURL: "https://192.0.2.1/input.png", wantError: "public IP"},
		{name: "IPv4 benchmark", mediaURL: "https://198.18.0.1/input.png", wantError: "public IP"},
		{name: "IPv6 loopback", mediaURL: "https://[::1]/input.png", wantError: "public IP"},
		{name: "IPv6 private", mediaURL: "https://[fd00::1]/input.png", wantError: "public IP"},
		{name: "IPv6 link-local", mediaURL: "https://[fe80::1]/input.png", wantError: "public IP"},
		{name: "IPv6 reserved", mediaURL: "https://[2001:db8::1]/input.png", wantError: "public IP"},
		{name: "legacy integer IPv4", mediaURL: "https://2130706433/input.png", wantError: "ambiguous IP"},
		{name: "legacy short IPv4", mediaURL: "https://127.1/input.png", wantError: "ambiguous IP"},
		{name: "legacy hexadecimal IPv4", mediaURL: "https://0x7f000001/input.png", wantError: "ambiguous IP"},
		{name: "Unicode localhost dot", mediaURL: "https://localhost。/input.png", wantError: "ambiguous hostname"},
		{name: "fullwidth IPv4", mediaURL: "https://１２７。０。０。１/input.png", wantError: "ambiguous hostname"},
		{name: "double trailing dot", mediaURL: "https://127.0.0.1../input.png", wantError: "ambiguous hostname"},
		{name: "empty DNS label", mediaURL: "https://cdn..example.com/input.png", wantError: "ambiguous hostname"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := validateMediaURL(test.mediaURL, mediaKindImage, 0)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestValidateMediaURLAcceptsWhitelistedBase64DataURI(t *testing.T) {
	imageURI := "data:image/png;base64,iVBORw0KGgo="
	validated, totalBytes, err := validateMediaURL(imageURI, mediaKindImage, 5)
	require.NoError(t, err)
	assert.Equal(t, imageURI, validated)
	assert.Equal(t, 13, totalBytes)

	audioURI := "data:audio/mpeg;base64,SUQz"
	validated, totalBytes, err = validateMediaURL(audioURI, mediaKindAudio, totalBytes)
	require.NoError(t, err)
	assert.Equal(t, audioURI, validated)
	assert.Equal(t, 16, totalBytes)
}

func TestValidateMediaURLRejectsInvalidDataURI(t *testing.T) {
	tests := []struct {
		name      string
		mediaKind string
		dataURI   string
		wantError string
	}{
		{name: "non-whitelisted image MIME", mediaKind: mediaKindImage, dataURI: "data:image/gif;base64,R0lG", wantError: "not allowed"},
		{name: "HEIC unsupported by selected workflow", mediaKind: mediaKindImage, dataURI: "data:image/heic;base64,AQ==", wantError: "not allowed"},
		{name: "HEIF unsupported by selected workflow", mediaKind: mediaKindImage, dataURI: "data:image/heif;base64,AQ==", wantError: "not allowed"},
		{name: "audio MIME used as image", mediaKind: mediaKindImage, dataURI: "data:audio/mpeg;base64,SUQz", wantError: "not allowed"},
		{name: "missing base64 marker", mediaKind: mediaKindImage, dataURI: "data:image/png,iVBORw0KGgo=", wantError: "must use the form"},
		{name: "extra MIME parameter", mediaKind: mediaKindImage, dataURI: "data:image/png;charset=utf-8;base64,iVBORw0KGgo=", wantError: "must use the form"},
		{name: "invalid base64", mediaKind: mediaKindImage, dataURI: "data:image/png;base64,not*base64", wantError: "invalid base64"},
		{name: "non-canonical base64", mediaKind: mediaKindImage, dataURI: "data:image/png;base64,AB==", wantError: "invalid base64"},
		{name: "base64 whitespace", mediaKind: mediaKindImage, dataURI: "data:image/png;base64,iVBO\nRw==", wantError: "whitespace"},
		{name: "empty decoded content", mediaKind: mediaKindImage, dataURI: "data:image/png;base64,", wantError: "include base64 content"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := validateMediaURL(test.dataURI, test.mediaKind, 0)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestValidateMediaURLEnforcesDataSizeLimits(t *testing.T) {
	overImageLimit := "data:image/png;base64," + strings.Repeat("A", base64.StdEncoding.EncodedLen(30*1024*1024+1))
	_, _, err := validateMediaURL(overImageLimit, mediaKindImage, 0)
	require.ErrorContains(t, err, "item limit")

	overAudioLimit := "data:audio/mpeg;base64," + strings.Repeat("A", base64.StdEncoding.EncodedLen(15*1024*1024+1))
	_, _, err = validateMediaURL(overAudioLimit, mediaKindAudio, 0)
	require.ErrorContains(t, err, "item limit")

	_, _, err = validateMediaURL("data:image/png;base64,AQ==", mediaKindImage, 64*1024*1024)
	require.ErrorContains(t, err, "total limit")
}

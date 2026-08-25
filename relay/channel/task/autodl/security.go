package autodl

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

const (
	mediaKindImage = "image"
	mediaKindAudio = "audio"

	maxImageDataBytes   = 30 * 1024 * 1024
	maxAudioDataBytes   = 15 * 1024 * 1024
	maxTotalDataBytes   = 64 * 1024 * 1024
	maxRequestBodyBytes = 64 * 1024 * 1024
)

var disallowedMediaIPPrefixes = []netip.Prefix{
	// IPv4 special-use, private, loopback, link-local, documentation,
	// benchmarking, multicast, and reserved ranges.
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),

	// IPv6 special-use, local, documentation, transition, multicast, and
	// reserved ranges.
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func buildAutoDLURL(baseURL string, pathSegments ...string) (string, error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return "", errors.New("AutoDL base URL is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid AutoDL base URL: %w", err)
	}
	if !parsed.IsAbs() || parsed.Opaque != "" || parsed.Host == "" || !strings.EqualFold(parsed.Scheme, "https") {
		return "", errors.New("AutoDL base URL must be an absolute HTTPS URL")
	}
	if parsed.User != nil {
		return "", errors.New("AutoDL base URL must not include userinfo")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return "", errors.New("AutoDL base URL must not include a query")
	}
	if parsed.Fragment != "" || strings.Contains(trimmed, "#") {
		return "", errors.New("AutoDL base URL must not include a fragment")
	}

	escapedSegments := make([]string, 0, len(pathSegments))
	for _, segment := range pathSegments {
		escaped := url.PathEscape(segment)
		if segment == "." {
			escaped = "%2E"
		} else if segment == ".." {
			escaped = "%2E%2E"
		}
		escapedSegments = append(escapedSegments, escaped)
	}
	endpoint, err := url.JoinPath(parsed.String(), escapedSegments...)
	if err != nil {
		return "", fmt.Errorf("build AutoDL URL: %w", err)
	}
	return endpoint, nil
}

func validateMediaURL(value, mediaKind string, currentDataBytes int) (string, int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", currentDataBytes, errors.New("URL is required")
	}
	if len(value) >= len("data:") && strings.EqualFold(value[:len("data:")], "data:") {
		return validateMediaDataURI(value, mediaKind, currentDataBytes)
	}
	if len(value) >= len("mm_file:") && strings.EqualFold(value[:len("mm_file:")], "mm_file:") {
		return "", currentDataBytes, errors.New("mm_file URLs cannot be resolved by AutoDL")
	}

	parsed, err := url.ParseRequestURI(value)
	if err != nil || !parsed.IsAbs() || parsed.Opaque != "" || parsed.Host == "" {
		return "", currentDataBytes, errors.New("URL must be an absolute HTTPS URL or an allowed base64 data URI")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", currentDataBytes, errors.New("external media URL must use HTTPS")
	}
	if parsed.User != nil {
		return "", currentDataBytes, errors.New("external media URL must not include userinfo")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return "", currentDataBytes, errors.New("external media URL must use the default HTTPS port 443")
	}

	rawHost := parsed.Hostname()
	if rawHost == "" {
		return "", currentDataBytes, errors.New("external media URL host is required")
	}
	// Keep validation and the upstream HTTP parser on the same hostname
	// representation. Unicode dots/digits can be mapped to ASCII by URL stacks
	// after this check, so conservatively reject every non-ASCII hostname.
	if strings.IndexFunc(rawHost, func(r rune) bool { return r > 127 }) != -1 {
		return "", currentDataBytes, errors.New("external media URL contains an invalid or ambiguous hostname")
	}
	trailingDots := len(rawHost) - len(strings.TrimRight(rawHost, "."))
	if trailingDots > 1 {
		return "", currentDataBytes, errors.New("external media URL contains an invalid or ambiguous hostname")
	}
	host := strings.ToLower(strings.TrimRight(rawHost, "."))
	if host == "" {
		return "", currentDataBytes, errors.New("external media URL host is required")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "", currentDataBytes, errors.New("external media URL must not target localhost")
	}
	if address, parseErr := netip.ParseAddr(host); parseErr == nil {
		if address.Zone() != "" || isDisallowedMediaIP(address.Unmap()) {
			return "", currentDataBytes, errors.New("external media URL must target a public IP address")
		}
		return value, currentDataBytes, nil
	}
	if strings.ContainsAny(host, ":%") || isLegacyIPAddressHostname(host) {
		return "", currentDataBytes, errors.New("external media URL contains an invalid or ambiguous IP address")
	}
	if len(host) > 253 {
		return "", currentDataBytes, errors.New("external media URL contains an invalid or ambiguous hostname")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' ||
			strings.IndexFunc(label, func(r rune) bool {
				return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-')
			}) != -1 {
			return "", currentDataBytes, errors.New("external media URL contains an invalid or ambiguous hostname")
		}
	}

	// Domain names are deliberately not resolved here. AutoDL performs the
	// eventual fetch, so this validation cannot prevent upstream DNS rebinding.
	return value, currentDataBytes, nil
}

func validateMediaDataURI(value, mediaKind string, currentDataBytes int) (string, int, error) {
	metadataAndData := value[len("data:"):]
	metadata, encoded, found := strings.Cut(metadataAndData, ",")
	if !found || encoded == "" {
		return "", currentDataBytes, errors.New("data URI must include base64 content")
	}
	if len(metadata) > 128 {
		return "", currentDataBytes, errors.New("data URI metadata is invalid")
	}
	parts := strings.Split(metadata, ";")
	if len(parts) != 2 || !strings.EqualFold(parts[1], "base64") {
		return "", currentDataBytes, errors.New("data URI must use the form data:<allowed MIME>;base64,<data>")
	}

	mimeType := strings.ToLower(parts[0])
	itemLimit, allowed := mediaDataLimit(mediaKind, mimeType)
	if !allowed {
		return "", currentDataBytes, fmt.Errorf("data URI MIME type is not allowed for %s media", mediaKind)
	}
	if strings.ContainsAny(encoded, " \t\r\n") {
		return "", currentDataBytes, errors.New("data URI base64 content must not contain whitespace")
	}
	if len(encoded) > base64.StdEncoding.EncodedLen(itemLimit) {
		return "", currentDataBytes, fmt.Errorf("%s data URI exceeds the %d-byte item limit", mediaKind, itemLimit)
	}

	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return "", currentDataBytes, errors.New("data URI contains invalid base64 content")
	}
	if len(decoded) == 0 {
		return "", currentDataBytes, errors.New("data URI decoded content must not be empty")
	}
	if len(decoded) > itemLimit {
		return "", currentDataBytes, fmt.Errorf("%s data URI exceeds the %d-byte item limit", mediaKind, itemLimit)
	}
	if currentDataBytes < 0 || currentDataBytes > maxTotalDataBytes || len(decoded) > maxTotalDataBytes-currentDataBytes {
		return "", currentDataBytes, fmt.Errorf("data URI inputs exceed the %d-byte total limit", maxTotalDataBytes)
	}

	return value, currentDataBytes + len(decoded), nil
}

func mediaDataLimit(mediaKind, mimeType string) (int, bool) {
	switch mediaKind {
	case mediaKindImage:
		switch mimeType {
		case "image/jpeg", "image/jpg", "image/png", "image/webp":
			return maxImageDataBytes, true
		}
	case mediaKindAudio:
		switch mimeType {
		case "audio/mp3", "audio/mpeg", "audio/wav", "audio/x-wav":
			return maxAudioDataBytes, true
		}
	}
	return 0, false
}

func isDisallowedMediaIP(address netip.Addr) bool {
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return true
	}
	for _, prefix := range disallowedMediaIPPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func isLegacyIPAddressHostname(host string) bool {
	labels := strings.Split(host, ".")
	if len(labels) == 0 {
		return false
	}
	for _, label := range labels {
		if label == "" {
			return false
		}
		if strings.HasPrefix(strings.ToLower(label), "0x") {
			hexDigits := label[2:]
			if hexDigits == "" || strings.IndexFunc(hexDigits, func(r rune) bool {
				return !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F'))
			}) != -1 {
				return false
			}
			continue
		}
		if strings.IndexFunc(label, func(r rune) bool { return r < '0' || r > '9' }) != -1 {
			return false
		}
	}
	return true
}

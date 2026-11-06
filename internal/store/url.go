package store

import (
	"fmt"
	"net/url"
	"strings"
)

// NormalizeBaseURL validates a provider base URL and removes a trailing slash.
// Network-level SSRF checks happen in the XC client, where the allow-private
// setting is known.
func NormalizeBaseURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("server URL is required")
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("server URL is not valid: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("server URL must use http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("server URL has no host")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("server URL must not contain credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("server URL must not contain a query or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

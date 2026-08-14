package auth

import (
	"errors"
	"net"
	"net/url"
	"path"
	"strings"
)

const maxReturnPathLength = 2048

// NormalizeConfiguration performs all local validation before a configuration
// is persisted or used for discovery.
func NormalizeConfiguration(input ConfigurationInput) (Configuration, error) {
	issuer, err := normalizeURL(input.IssuerURL, false)
	if err != nil {
		return Configuration{}, errors.New("issuer URL must be an absolute HTTPS URL; loopback HTTP is allowed for development")
	}
	publicURL, err := normalizeURL(input.PublicURL, true)
	if err != nil {
		return Configuration{}, errors.New("public URL must be an origin-only HTTPS URL; loopback HTTP is allowed for development")
	}
	clientID := strings.TrimSpace(input.ClientID)
	if clientID == "" {
		return Configuration{}, errors.New("client ID must not be empty")
	}
	if len(clientID) > 512 {
		return Configuration{}, errors.New("client ID must be at most 512 characters")
	}
	return Configuration{IssuerURL: issuer, ClientID: clientID, PublicURL: publicURL}, nil
}

func normalizeURL(raw string, originOnly bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return "", errors.New("invalid URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("invalid URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawFragment != "" {
		return "", errors.New("URL must not contain query or fragment")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return "", errors.New("URL must use HTTPS")
	}
	if originOnly && parsed.EscapedPath() != "" && parsed.EscapedPath() != "/" {
		return "", errors.New("public URL must not contain a path")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if originOnly {
		parsed.Path = ""
		parsed.RawPath = ""
	}
	return parsed.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func validateEndpoint(raw string) error {
	_, err := normalizeURL(raw, false)
	return err
}

// SafeReturnPath accepts only local absolute paths and removes dot segments.
func SafeReturnPath(raw string) string {
	if raw == "" {
		return "/"
	}
	if len(raw) > maxReturnPathLength || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") {
		return "/"
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.Fragment != "" {
		return "/"
	}
	if strings.HasPrefix(parsed.Path, "//") || strings.Contains(parsed.Path, "\\") || strings.ContainsAny(parsed.Path, "\r\n\x00") {
		return "/"
	}
	cleaned := path.Clean(parsed.Path)
	if !strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "//") {
		return "/"
	}
	if parsed.RawQuery != "" {
		cleaned += "?" + parsed.RawQuery
	}
	return cleaned
}

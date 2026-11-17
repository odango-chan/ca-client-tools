package sbi

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

var errInvalidConfiguration = errors.New("invalid SBI client configuration")

type Config struct {
	Services            map[string]ServiceConfig
	HeaderTokenProfiles map[string]HeaderTokenProfile
	Limits              Limits
}

type ServiceConfig struct {
	Origin             string
	APIRoot            string
	HeaderTokenProfile string
}

type HeaderTokenProfile struct {
	Header string
	Token  string
}

type Limits struct {
	RequestTimeout      time.Duration
	DialTimeout         time.Duration
	MaxRequestBytes     int64
	MaxResponseBytes    int64
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
}

func normalizeConfig(input Config) (Config, error) {
	if len(input.Services) == 0 {
		return Config{}, errInvalidConfiguration
	}
	if input.Limits.RequestTimeout == 0 {
		input.Limits.RequestTimeout = 10 * time.Second
	}
	if input.Limits.DialTimeout == 0 {
		input.Limits.DialTimeout = 5 * time.Second
	}
	if input.Limits.MaxRequestBytes == 0 {
		input.Limits.MaxRequestBytes = 16 << 20
	}
	if input.Limits.MaxResponseBytes == 0 {
		input.Limits.MaxResponseBytes = 16 << 20
	}
	if input.Limits.MaxIdleConns == 0 {
		input.Limits.MaxIdleConns = 64
	}
	if input.Limits.MaxIdleConnsPerHost == 0 {
		input.Limits.MaxIdleConnsPerHost = 8
	}
	if input.Limits.MaxConnsPerHost == 0 {
		input.Limits.MaxConnsPerHost = 32
	}
	if input.Limits.IdleConnTimeout == 0 {
		input.Limits.IdleConnTimeout = 90 * time.Second
	}
	if input.Limits.RequestTimeout <= 0 || input.Limits.RequestTimeout > 2*time.Minute ||
		input.Limits.DialTimeout <= 0 || input.Limits.DialTimeout > 30*time.Second ||
		input.Limits.MaxRequestBytes <= 0 || input.Limits.MaxRequestBytes > 1<<30 ||
		input.Limits.MaxResponseBytes <= 0 || input.Limits.MaxResponseBytes > 1<<30 ||
		input.Limits.MaxIdleConns < 1 || input.Limits.MaxIdleConns > 1024 ||
		input.Limits.MaxIdleConnsPerHost < 1 || input.Limits.MaxIdleConnsPerHost > 256 ||
		input.Limits.MaxConnsPerHost < 1 || input.Limits.MaxConnsPerHost > 256 ||
		input.Limits.IdleConnTimeout <= 0 || input.Limits.IdleConnTimeout > 10*time.Minute {
		return Config{}, errInvalidConfiguration
	}

	for name, profile := range input.HeaderTokenProfiles {
		if !validIdentifier(name) || !validHeaderName(profile.Header) ||
			isReservedHeader(profile.Header) || !validSecret(profile.Token) {
			return Config{}, errInvalidConfiguration
		}
	}

	services := make(map[string]ServiceConfig, len(input.Services))
	for key, service := range input.Services {
		if !validIdentifier(key) {
			return Config{}, errInvalidConfiguration
		}
		origin, err := parseServiceOrigin(service.Origin)
		if err != nil {
			return Config{}, errInvalidConfiguration
		}
		if err := validateAPIPath(service.APIRoot); err != nil {
			return Config{}, errInvalidConfiguration
		}
		service.Origin = origin.String()
		service.APIRoot = strings.TrimRight(service.APIRoot, "/")
		if service.APIRoot == "/" {
			service.APIRoot = ""
		}
		if service.HeaderTokenProfile != "" {
			if _, ok := input.HeaderTokenProfiles[service.HeaderTokenProfile]; !ok {
				return Config{}, errInvalidConfiguration
			}
		}
		services[key] = service
	}
	input.Services = services
	return input, nil
}

func parseServiceOrigin(raw string) (*url.URL, error) {
	origin, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || origin == nil || origin.Scheme != "http" ||
		origin.Host == "" || origin.User != nil || origin.Opaque != "" ||
		(origin.Path != "" && origin.Path != "/") ||
		origin.RawQuery != "" || origin.Fragment != "" {
		return nil, errInvalidConfiguration
	}
	origin.Path = ""
	origin.RawPath = ""
	origin.Host = strings.ToLower(origin.Host)
	return origin, nil
}

func validateAPIPath(raw string) error {
	if raw == "" {
		return nil
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") ||
		strings.ContainsAny(raw, "?#\\") {
		return errInvalidConfiguration
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
		return errInvalidConfiguration
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return errInvalidConfiguration
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == "." || segment == ".." {
			return errInvalidConfiguration
		}
	}
	return nil
}

func validIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func validSecret(value string) bool {
	return value != "" && !strings.ContainsAny(value, "\r\n")
}

func validHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func isReservedHeader(value string) bool {
	switch strings.ToLower(value) {
	case "authorization", "proxy-authorization", "host", "content-length",
		"transfer-encoding", "connection", "keep-alive", "proxy-connection",
		"upgrade", "te", "trailer", "cookie":
		return true
	default:
		return false
	}
}

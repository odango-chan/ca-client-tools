// Package serviceconfig loads the trusted, versioned multi-service SBI
// configuration used by the SBI transport and the generic operation executor.
//
// The loader is deliberately strict: unknown fields, duplicate keys, YAML
// aliases and merge keys, multiple documents and any literal secret field are
// rejected. Tokens are read either from an absolute secret file or from an
// environment variable; they are never embedded in the configuration file.
package serviceconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/odango-chan/ca-client-tools/internal/sbi"
	"gopkg.in/yaml.v3"
)

// SupportedVersion is the only accepted configuration schema version.
const SupportedVersion = 1

// MaxTokenFileBytes bounds the secret file the loader accepts.
const MaxTokenFileBytes = 4 << 10

const maxConfigFileBytes = 1 << 20

var (
	envNamePattern     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	nfTypePattern      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,31}$`)
	redactedTokenValue = "<redacted>"
)

// Document is the parsed configuration plus trusted metadata that the v1
// shared transport cannot apply per service. Load returns only the sbi.Config
// accepted by the transport; LoadDocument additionally exposes per-service
// metadata for callers that can use it.
type Document struct {
	Config         sbi.Config
	NFType         map[string]string
	RequestTimeout map[string]time.Duration
}

// Load reads, validates and returns the trusted SBI configuration. lookupEnv
// resolves environment-variable token profiles; when nil, os.LookupEnv is
// used. Token values are returned inside the configuration and never logged.
func Load(path string, lookupEnv func(string) (string, bool)) (sbi.Config, error) {
	document, err := LoadDocument(path, lookupEnv)
	if err != nil {
		return sbi.Config{}, err
	}
	return document.Config, nil
}

// LoadDocument behaves like Load and also returns per-service metadata.
func LoadDocument(path string, lookupEnv func(string) (string, bool)) (Document, error) {
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	raw, err := readLimitedFile(path, maxConfigFileBytes)
	if err != nil {
		return Document{}, fmt.Errorf("service configuration: %w", err)
	}
	decoded, err := decodeStrict(raw)
	if err != nil {
		return Document{}, fmt.Errorf("service configuration: %w", err)
	}
	if decoded.Version == nil || *decoded.Version != SupportedVersion {
		return Document{}, fmt.Errorf("service configuration: unsupported or missing version (want %d)", SupportedVersion)
	}
	if len(decoded.Services) == 0 {
		return Document{}, errors.New("service configuration: services must not be empty")
	}

	document := Document{
		Config: sbi.Config{
			Services:            make(map[string]sbi.ServiceConfig, len(decoded.Services)),
			HeaderTokenProfiles: make(map[string]sbi.HeaderTokenProfile, len(decoded.HeaderTokenProfiles)),
			Limits:              limitsFromFile(decoded.Limits),
		},
		NFType:         make(map[string]string, len(decoded.Services)),
		RequestTimeout: make(map[string]time.Duration, len(decoded.Services)),
	}
	for name, profile := range decoded.HeaderTokenProfiles {
		token, err := resolveToken(name, profile, lookupEnv)
		if err != nil {
			return Document{}, fmt.Errorf("service configuration: header token profile %q: %w", name, err)
		}
		document.Config.HeaderTokenProfiles[name] = sbi.HeaderTokenProfile{
			Header: profile.Header,
			Token:  token,
		}
	}
	for name, service := range decoded.Services {
		if service.Protocol != "" && !strings.EqualFold(service.Protocol, "h2c") {
			return Document{}, fmt.Errorf("service configuration: service %q: protocol %q is not supported (h2c only)", name, service.Protocol)
		}
		if service.NFType != "" && !nfTypePattern.MatchString(service.NFType) {
			return Document{}, fmt.Errorf("service configuration: service %q: invalid nfType", name)
		}
		if service.HeaderTokenProfile != "" {
			if _, ok := document.Config.HeaderTokenProfiles[service.HeaderTokenProfile]; !ok {
				return Document{}, fmt.Errorf("service configuration: service %q references unknown header token profile %q", name, service.HeaderTokenProfile)
			}
		}
		if service.RequestTimeout != nil {
			if service.RequestTimeout.value <= 0 || service.RequestTimeout.value > 2*time.Minute {
				return Document{}, fmt.Errorf("service configuration: service %q: requestTimeout is outside the supported bounds", name)
			}
			document.RequestTimeout[name] = service.RequestTimeout.value
		}
		document.Config.Services[name] = sbi.ServiceConfig{
			Origin:             service.Origin,
			APIRoot:            service.APIRoot,
			HeaderTokenProfile: service.HeaderTokenProfile,
		}
		document.NFType[name] = service.NFType
	}
	if _, err := sbi.NewClient(document.Config); err != nil {
		return Document{}, fmt.Errorf("service configuration: invalid SBI configuration: %w", err)
	}
	return document, nil
}

// Redacted returns a diagnostics view of an SBI configuration in which every
// token value is replaced by a fixed placeholder. It is safe to log.
func Redacted(config sbi.Config) map[string]any {
	services := make(map[string]any, len(config.Services))
	for name, service := range config.Services {
		services[name] = map[string]any{
			"origin":             service.Origin,
			"apiRoot":            service.APIRoot,
			"headerTokenProfile": service.HeaderTokenProfile,
		}
	}
	profiles := make(map[string]any, len(config.HeaderTokenProfiles))
	for name, profile := range config.HeaderTokenProfiles {
		profiles[name] = map[string]any{
			"header": profile.Header,
			"token":  redactedTokenValue,
		}
	}
	return map[string]any{
		"services":            services,
		"headerTokenProfiles": profiles,
		"limits": map[string]any{
			"requestTimeout":      config.Limits.RequestTimeout.String(),
			"dialTimeout":         config.Limits.DialTimeout.String(),
			"maxRequestBytes":     config.Limits.MaxRequestBytes,
			"maxResponseBytes":    config.Limits.MaxResponseBytes,
			"maxIdleConns":        config.Limits.MaxIdleConns,
			"maxIdleConnsPerHost": config.Limits.MaxIdleConnsPerHost,
			"maxConnsPerHost":     config.Limits.MaxConnsPerHost,
			"idleConnTimeout":     config.Limits.IdleConnTimeout.String(),
		},
	}
}

// Redacted returns a diagnostics view of the loaded document, including the
// per-service metadata. Token values are replaced by a fixed placeholder.
func (d Document) Redacted() map[string]any {
	view := Redacted(d.Config)
	services := view["services"].(map[string]any)
	for name := range d.Config.Services {
		service := services[name].(map[string]any)
		service["nfType"] = d.NFType[name]
		if timeout, ok := d.RequestTimeout[name]; ok {
			service["requestTimeout"] = timeout.String()
		}
	}
	return view
}

type fileConfig struct {
	Version             *int                        `yaml:"version"`
	Services            map[string]fileService      `yaml:"services"`
	Limits              *fileLimits                 `yaml:"limits"`
	HeaderTokenProfiles map[string]fileTokenProfile `yaml:"headerTokenProfiles"`
}

type fileService struct {
	NFType             string          `yaml:"nfType"`
	Origin             string          `yaml:"origin"`
	APIRoot            string          `yaml:"apiRoot"`
	Protocol           string          `yaml:"protocol"`
	HeaderTokenProfile string          `yaml:"headerTokenProfile"`
	Discovery          *discoveryValue `yaml:"discovery"`
	RequestTimeout     *duration       `yaml:"requestTimeout"`
}

type fileLimits struct {
	RequestTimeout      *duration `yaml:"requestTimeout"`
	DialTimeout         *duration `yaml:"dialTimeout"`
	MaxRequestBytes     int64     `yaml:"maxRequestBytes"`
	MaxResponseBytes    int64     `yaml:"maxResponseBytes"`
	MaxIdleConns        int       `yaml:"maxIdleConns"`
	MaxIdleConnsPerHost int       `yaml:"maxIdleConnsPerHost"`
	MaxConnsPerHost     int       `yaml:"maxConnsPerHost"`
	IdleConnTimeout     *duration `yaml:"idleConnTimeout"`
}

type fileTokenProfile struct {
	Header    string `yaml:"header"`
	TokenFile string `yaml:"tokenFile"`
	TokenEnv  string `yaml:"tokenEnv"`
}

// discoveryValue reserves the discovery field. Only null, the scalar "none" or
// an empty mapping are accepted; discovery is not implemented in v1.
type discoveryValue struct{}

func (d *discoveryValue) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!null" || node.Value == "none" {
			return nil
		}
		return errors.New("discovery must be \"none\", null or an empty mapping")
	case yaml.MappingNode:
		if len(node.Content) != 0 {
			return errors.New("discovery is reserved and must not declare fields")
		}
		return nil
	default:
		return errors.New("discovery must be \"none\", null or an empty mapping")
	}
}

type duration struct {
	value time.Duration
}

func (d *duration) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return errors.New("duration must be a string such as \"10s\"")
	}
	parsed, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q", node.Value)
	}
	d.value = parsed
	return nil
}

func limitsFromFile(input *fileLimits) sbi.Limits {
	if input == nil {
		return sbi.Limits{}
	}
	limits := sbi.Limits{
		MaxRequestBytes:     input.MaxRequestBytes,
		MaxResponseBytes:    input.MaxResponseBytes,
		MaxIdleConns:        input.MaxIdleConns,
		MaxIdleConnsPerHost: input.MaxIdleConnsPerHost,
		MaxConnsPerHost:     input.MaxConnsPerHost,
	}
	if input.RequestTimeout != nil {
		limits.RequestTimeout = input.RequestTimeout.value
	}
	if input.DialTimeout != nil {
		limits.DialTimeout = input.DialTimeout.value
	}
	if input.IdleConnTimeout != nil {
		limits.IdleConnTimeout = input.IdleConnTimeout.value
	}
	return limits
}

func resolveToken(name string, profile fileTokenProfile, lookupEnv func(string) (string, bool)) (string, error) {
	if strings.TrimSpace(profile.Header) == "" {
		return "", errors.New("header must be configured")
	}
	if profile.TokenFile != "" && profile.TokenEnv != "" {
		return "", errors.New("tokenFile and tokenEnv are mutually exclusive")
	}
	switch {
	case profile.TokenFile != "":
		return readTokenFile(profile.TokenFile)
	case profile.TokenEnv != "":
		if !envNamePattern.MatchString(profile.TokenEnv) {
			return "", errors.New("tokenEnv is not a valid environment variable name")
		}
		raw, ok := lookupEnv(profile.TokenEnv)
		if !ok {
			return "", fmt.Errorf("environment variable %q is not set", profile.TokenEnv)
		}
		return normalizeToken(raw)
	default:
		return "", errors.New("exactly one of tokenFile or tokenEnv must be configured")
	}
}

func readTokenFile(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("tokenFile must be an absolute path")
	}
	raw, err := readLimitedFile(path, MaxTokenFileBytes)
	if err != nil {
		return "", fmt.Errorf("cannot read token file: %w", err)
	}
	return normalizeToken(string(raw))
}

func normalizeToken(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", errors.New("token is empty")
	}
	for _, r := range token {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("token contains control characters or multiple lines")
		}
	}
	return token, nil
}

func readLimitedFile(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("file exceeds %d bytes", maximum)
	}
	return data, nil
}

func decodeStrict(raw []byte) (*fileConfig, error) {
	if err := validateYAMLNode(raw); err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	var decoded fileConfig
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("multiple YAML documents are not allowed")
	}
	return &decoded, nil
}

func validateYAMLNode(raw []byte) error {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("multiple YAML documents are not allowed")
	}
	return walkYAMLNode(&document)
}

func walkYAMLNode(node *yaml.Node) error {
	switch node.Kind {
	case yaml.AliasNode:
		return errors.New("YAML aliases are not allowed")
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range node.Content {
			if err := walkYAMLNode(child); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		seen := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode {
				return errors.New("mapping keys must be scalars")
			}
			if key.Value == "<<" {
				return errors.New("YAML merge keys are not allowed")
			}
			if _, duplicate := seen[key.Value]; duplicate {
				return fmt.Errorf("duplicate key %q", key.Value)
			}
			seen[key.Value] = struct{}{}
			if err := walkYAMLNode(node.Content[index+1]); err != nil {
				return err
			}
		}
	}
	return nil
}

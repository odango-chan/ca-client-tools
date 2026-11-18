package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/odango-chan/ca-client-tools/internal/sbi"
)

const (
	defaultTimeout     = 5 * time.Second
	maxTimeout         = 30 * time.Second
	defaultMaxResponse = int64(2 << 20)
	maxAllowedResponse = int64(4 << 20)
	maxPageSize        = 100

	// AMFMetricsServiceKey and SMFMetricsServiceKey are the trusted Catalog
	// serviceKey values of the two metrics listeners.
	AMFMetricsServiceKey = "openamsm-amf-metrics"
	SMFMetricsServiceKey = "openamsm-smf-metrics"
)

type Config struct {
	AMFOrigin        string
	SMFOrigin        string
	Timeout          time.Duration
	MaxResponseBytes int64
}

// Client keeps the source-specific typed metrics handling (paths, paging
// validation, response shape and error semantics) on top of the shared
// transport and its explicit HTTP/1.1 trusted service profile.
type Client struct {
	transport        *sbi.Client
	amfServiceKey    string
	smfServiceKey    string
	timeout          time.Duration
	maxResponseBytes int64
}

type Pager struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Count    int    `json:"count"`
	Next     string `json:"next,omitempty"`
	Prev     string `json:"prev,omitempty"`
}

type Page struct {
	Items []json.RawMessage `json:"items"`
	Pager Pager             `json:"pager"`
}

type InputError struct{ Reason string }

func (e *InputError) Error() string { return "invalid OpenAMSM metrics query: " + e.Reason }

type ConfigurationError struct{ Reason string }

func (e *ConfigurationError) Error() string {
	return "invalid OpenAMSM metrics configuration: " + e.Reason
}

type ProtocolError struct{ Reason string }

func (e *ProtocolError) Error() string { return "invalid OpenAMSM metrics response: " + e.Reason }

type RemoteError struct {
	HTTPStatus int
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("OpenAMSM metrics endpoint returned HTTP %d", e.HTTPStatus)
}

// NewClient builds a metrics client with its own shared transport instance. It
// is the CLI/standalone entry point; the Provider uses NewClientWithSBI so one
// transport instance serves typed MAS, generic SBI and metrics.
func NewClient(cfg Config) (*Client, error) {
	timeout, maxBytes, err := normalizeLimits(cfg)
	if err != nil {
		return nil, err
	}
	services := make(map[string]sbi.ServiceConfig, 2)
	amfKey, smfKey := "", ""
	if cfg.AMFOrigin != "" {
		origin, err := parseOrigin(cfg.AMFOrigin)
		if err != nil {
			return nil, err
		}
		services[AMFMetricsServiceKey] = sbi.ServiceConfig{
			Origin:  origin.String(),
			Profile: sbi.TransportMetricsHTTP1,
		}
		amfKey = AMFMetricsServiceKey
	}
	if cfg.SMFOrigin != "" {
		origin, err := parseOrigin(cfg.SMFOrigin)
		if err != nil {
			return nil, err
		}
		services[SMFMetricsServiceKey] = sbi.ServiceConfig{
			Origin:  origin.String(),
			Profile: sbi.TransportMetricsHTTP1,
		}
		smfKey = SMFMetricsServiceKey
	}
	if len(services) == 0 {
		return nil, &ConfigurationError{Reason: "at least one metrics origin must be configured"}
	}
	transport, err := sbi.NewClient(sbi.Config{
		Services: services,
		Limits: sbi.Limits{
			RequestTimeout:      timeout,
			DialTimeout:         5 * time.Second,
			MaxResponseBytes:    maxBytes,
			MaxIdleConns:        16,
			MaxIdleConnsPerHost: 4,
			MaxConnsPerHost:     8,
			IdleConnTimeout:     90 * time.Second,
		},
	})
	if err != nil {
		return nil, &ConfigurationError{Reason: "shared metrics transport configuration is invalid"}
	}
	return newMetricsClient(transport, amfKey, smfKey, timeout, maxBytes)
}

// NewClientWithSBI adapts an already-built shared transport to the typed
// metrics client. The service keys must resolve to services configured with the
// explicit HTTP/1.1 metrics profile; the transport's lifecycle belongs to the
// caller.
func NewClientWithSBI(
	transport *sbi.Client,
	amfServiceKey, smfServiceKey string,
	cfg Config,
) (*Client, error) {
	timeout, maxBytes, err := normalizeLimits(cfg)
	if err != nil {
		return nil, err
	}
	if transport == nil {
		return nil, &ConfigurationError{Reason: "shared metrics transport is not configured"}
	}
	if amfServiceKey == "" && smfServiceKey == "" {
		return nil, &ConfigurationError{Reason: "at least one metrics service must be configured"}
	}
	for _, key := range []string{amfServiceKey, smfServiceKey} {
		if key == "" {
			continue
		}
		profile, ok := transport.ServiceProfile(key)
		if !ok || profile != sbi.TransportMetricsHTTP1 {
			return nil, &ConfigurationError{Reason: "metrics service is not configured with the HTTP/1.1 profile"}
		}
	}
	return newMetricsClient(transport, amfServiceKey, smfServiceKey, timeout, maxBytes)
}

func newMetricsClient(
	transport *sbi.Client,
	amfServiceKey, smfServiceKey string,
	timeout time.Duration,
	maxResponseBytes int64,
) (*Client, error) {
	if transport == nil {
		return nil, &ConfigurationError{Reason: "shared metrics transport is not configured"}
	}
	return &Client{
		transport:        transport,
		amfServiceKey:    amfServiceKey,
		smfServiceKey:    smfServiceKey,
		timeout:          timeout,
		maxResponseBytes: maxResponseBytes,
	}, nil
}

// normalizeLimits validates the caller-independent metrics bounds.
func normalizeLimits(cfg Config) (time.Duration, int64, error) {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout <= 0 || timeout > maxTimeout {
		return 0, 0, &ConfigurationError{Reason: "timeout must be within (0, 30s]"}
	}
	maxBytes := cfg.MaxResponseBytes
	if maxBytes == 0 {
		maxBytes = defaultMaxResponse
	}
	if maxBytes <= 0 || maxBytes > maxAllowedResponse {
		return 0, 0, &ConfigurationError{Reason: "response limit must be within (0, 4 MiB]"}
	}
	return timeout, maxBytes, nil
}

// CloseIdleConnections releases idle connections of this client's transport.
// When the transport is shared, the owner decides when to close it.
func (c *Client) CloseIdleConnections() {
	if c == nil || c.transport == nil {
		return
	}
	c.transport.CloseIdleConnections()
}

func parseOrigin(raw string) (*url.URL, error) {
	origin, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || origin == nil ||
		(origin.Scheme != "http" && origin.Scheme != "https") ||
		origin.Host == "" || origin.User != nil ||
		(origin.Path != "" && origin.Path != "/") ||
		origin.RawQuery != "" || origin.Fragment != "" {
		return nil, &ConfigurationError{Reason: "metrics origin must be an absolute HTTP(S) origin without credentials, path, query or fragment"}
	}
	origin.Path = ""
	origin.RawPath = ""
	origin.Host = strings.ToLower(origin.Host)
	return origin, nil
}

func (c *Client) GetUEInfo(ctx context.Context, page, pageSize int) (Page, error) {
	if c == nil {
		return Page{}, &ConfigurationError{Reason: "client is not configured"}
	}
	if c.amfServiceKey == "" {
		return Page{}, &ConfigurationError{Reason: "requested NF metrics origin is not configured"}
	}
	return c.get(ctx, c.amfServiceKey, "/ue-info", page, pageSize)
}

func (c *Client) GetGNBInfo(ctx context.Context, page, pageSize int) (Page, error) {
	if c == nil {
		return Page{}, &ConfigurationError{Reason: "client is not configured"}
	}
	if c.amfServiceKey == "" {
		return Page{}, &ConfigurationError{Reason: "requested NF metrics origin is not configured"}
	}
	return c.get(ctx, c.amfServiceKey, "/gnb-info", page, pageSize)
}

func (c *Client) GetPDUInfo(ctx context.Context, page, pageSize int) (Page, error) {
	if c == nil {
		return Page{}, &ConfigurationError{Reason: "client is not configured"}
	}
	if c.smfServiceKey == "" {
		return Page{}, &ConfigurationError{Reason: "requested NF metrics origin is not configured"}
	}
	return c.get(ctx, c.smfServiceKey, "/pdu-info", page, pageSize)
}

func (c *Client) get(ctx context.Context, serviceKey, path string, page, pageSize int) (Page, error) {
	if c == nil || c.transport == nil || serviceKey == "" {
		return Page{}, &ConfigurationError{Reason: "requested NF metrics origin is not configured"}
	}
	if page < 0 {
		return Page{}, &InputError{Reason: "page must be non-negative"}
	}
	if pageSize < 1 || pageSize > maxPageSize {
		return Page{}, &InputError{Reason: "page-size must be in [1, 100]"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// The typed client keeps its own bounded timeout while the shared
	// transport keeps connection, cancellation, redirect and size handling.
	requestContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(pageSize))
	response, err := c.transport.Do(requestContext, serviceKey, sbi.Request{
		Method: http.MethodGet,
		Path:   path,
		Query:  query,
		Accept: "application/json",
	})
	if err != nil {
		return Page{}, fmt.Errorf("OpenAMSM metrics request failed: %w", err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		if errors.Is(err, sbi.ErrResponseTooLarge) {
			return Page{}, &ProtocolError{Reason: "response exceeds configured size limit"}
		}
		return Page{}, &ProtocolError{Reason: "could not read response"}
	}
	if int64(len(raw)) > c.maxResponseBytes {
		return Page{}, &ProtocolError{Reason: "response exceeds configured size limit"}
	}
	if response.StatusCode != http.StatusOK {
		return Page{}, &RemoteError{HTTPStatus: response.StatusCode}
	}
	mediaType, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		return Page{}, &ProtocolError{Reason: "success response is not application/json"}
	}
	return parsePage(raw, path, page, pageSize)
}

func parsePage(raw []byte, path string, requestedPage, requestedSize int) (Page, error) {
	decoded, err := decodeUniqueJSON(raw)
	if err != nil {
		return Page{}, &ProtocolError{Reason: "response is not unique-key JSON"}
	}
	root, ok := decoded.(map[string]any)
	if !ok || len(root) != 2 {
		return Page{}, &ProtocolError{Reason: "response must contain only items and pager"}
	}
	rawItems, ok := root["items"].([]any)
	if !ok {
		return Page{}, &ProtocolError{Reason: "items must be an array"}
	}
	rawPager, ok := root["pager"].(map[string]any)
	if !ok {
		return Page{}, &ProtocolError{Reason: "pager must be an object"}
	}
	if !allowedKeys(rawPager, "page", "page_size", "count", "next", "prev", "truncated") {
		return Page{}, &ProtocolError{Reason: "pager contains an unknown field"}
	}
	page, ok := integerField(rawPager, "page")
	if !ok || page != requestedPage {
		return Page{}, &ProtocolError{Reason: "pager.page does not match the request"}
	}
	pageSize, ok := integerField(rawPager, "page_size")
	if !ok || pageSize != requestedSize {
		return Page{}, &ProtocolError{Reason: "pager.page_size does not match the request"}
	}
	count, ok := integerField(rawPager, "count")
	if !ok || count != len(rawItems) || count > requestedSize {
		return Page{}, &ProtocolError{Reason: "pager.count does not match items"}
	}
	if truncated, exists := rawPager["truncated"]; exists {
		value, ok := truncated.(bool)
		if !ok || value {
			return Page{}, &ProtocolError{Reason: "truncated response is not usable"}
		}
	}

	items := make([]json.RawMessage, 0, len(rawItems))
	for _, item := range rawItems {
		if _, ok := item.(map[string]any); !ok {
			return Page{}, &ProtocolError{Reason: "each item must be an object"}
		}
		encoded, err := json.Marshal(item)
		if err != nil {
			return Page{}, &ProtocolError{Reason: "could not preserve source item"}
		}
		items = append(items, encoded)
	}

	next, err := parsePageLink(rawPager, "next", path, requestedPage+1, false)
	if err != nil {
		return Page{}, err
	}
	prev, err := parsePageLink(rawPager, "prev", path, requestedPage-1, requestedPage > 0)
	if err != nil {
		return Page{}, err
	}
	if next != "" && len(items) != requestedSize {
		return Page{}, &ProtocolError{Reason: "next page exists but current page is not full"}
	}
	return Page{
		Items: items,
		Pager: Pager{Page: page, PageSize: pageSize, Count: count, Next: next, Prev: prev},
	}, nil
}

func parsePageLink(pager map[string]any, field, path string, expectedPage int, required bool) (string, error) {
	raw, exists := pager[field]
	if !exists {
		if required {
			return "", &ProtocolError{Reason: field + " page link is missing"}
		}
		return "", nil
	}
	value, ok := raw.(string)
	if !ok || value == "" {
		return "", &ProtocolError{Reason: field + " page link must be a non-empty string"}
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Opaque != "" ||
		parsed.Host != "" || parsed.User != nil || parsed.Fragment != "" ||
		parsed.Path != path {
		return "", &ProtocolError{Reason: field + " page link is not a local endpoint link"}
	}
	if expectedPage < 0 {
		return "", &ProtocolError{Reason: field + " page link exceeds the client page limit"}
	}
	values := parsed.Query()
	if len(values) != 1 || len(values["page"]) != 1 {
		return "", &ProtocolError{Reason: field + " page link has unexpected query parameters"}
	}
	page, err := strconv.Atoi(values.Get("page"))
	if err != nil || page != expectedPage || values.Get("page") != strconv.Itoa(expectedPage) {
		return "", &ProtocolError{Reason: field + " page link is not sequential"}
	}
	return value, nil
}

func integerField(object map[string]any, key string) (int, bool) {
	number, ok := object[key].(json.Number)
	if !ok {
		return 0, false
	}
	value, err := number.Int64()
	if err != nil || value < 0 || value > int64(^uint(0)>>1) {
		return 0, false
	}
	return int(value), true
}

func allowedKeys(object map[string]any, allowed ...string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		set[key] = struct{}{}
	}
	for key := range object {
		if _, ok := set[key]; !ok {
			return false
		}
	}
	return true
}

func decodeUniqueJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}

func decodeValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return token, nil
	}
	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("object key is not a string")
			}
			if _, duplicate := object[key]; duplicate {
				return nil, fmt.Errorf("duplicate JSON key %q", key)
			}
			value, err := decodeValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return nil, errors.New("unterminated JSON object")
		}
		return object, nil
	case '[':
		values := make([]any, 0)
		for decoder.More() {
			value, err := decodeValue(decoder)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return nil, errors.New("unterminated JSON array")
		}
		return values, nil
	default:
		return nil, errors.New("unexpected JSON delimiter")
	}
}

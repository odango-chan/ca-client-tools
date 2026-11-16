package mas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout     = 5 * time.Second
	maxTimeout         = 30 * time.Second
	defaultMaxResponse = int64(1 << 20)
	maxAllowedResponse = int64(4 << 20)
	queryTokenHeader   = "X-OpenAMSM-Query-Token"
)

var supportedMAS = map[string]struct{}{
	"SC": {}, "PA": {}, "UPN": {}, "SSM": {},
	"IAM": {}, "UPS": {}, "CA": {}, "SAA": {},
}

type Config struct {
	Origin           string
	QueryToken       string
	Timeout          time.Duration
	MaxResponseBytes int64
}

type Client struct {
	origin           *url.URL
	queryToken       string
	httpClient       *http.Client
	maxResponseBytes int64
}

type Query struct {
	MAS           string
	SUPI          string
	IMSI          string
	PDUSessionID  *int
	UEIPv4        string
	UEIPv6        string
}

type Result struct {
	MASID   string         `json:"masId"`
	Status  string         `json:"status"`
	Key     map[string]any `json:"key"`
	Context map[string]any `json:"context"`
}

type InputError struct {
	Reason string
}

func (e *InputError) Error() string {
	return "invalid SMF MAS query: " + e.Reason
}

type ConfigurationError struct {
	Reason string
}

func (e *ConfigurationError) Error() string {
	return "invalid OpenAMSM client configuration: " + e.Reason
}

type ProtocolError struct {
	Reason string
}

func (e *ProtocolError) Error() string {
	return "invalid OpenAMSM response: " + e.Reason
}

type RemoteError struct {
	HTTPStatus int
	Cause      string
}

func (e *RemoteError) Error() string {
	if e.Cause == "" {
		return fmt.Sprintf("OpenAMSM returned HTTP %d", e.HTTPStatus)
	}
	return fmt.Sprintf("OpenAMSM returned HTTP %d (%s)", e.HTTPStatus, e.Cause)
}

func NewClient(cfg Config) (*Client, error) {
	origin, err := parseOrigin(cfg.Origin)
	if err != nil {
		return nil, err
	}
	if cfg.QueryToken == "" || strings.TrimSpace(cfg.QueryToken) != cfg.QueryToken ||
		strings.ContainsAny(cfg.QueryToken, "\r\n") {
		return nil, &ConfigurationError{Reason: "query token is missing or malformed"}
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout <= 0 || timeout > maxTimeout {
		return nil, &ConfigurationError{Reason: "timeout must be within (0, 30s]"}
	}

	maxBytes := cfg.MaxResponseBytes
	if maxBytes == 0 {
		maxBytes = defaultMaxResponse
	}
	if maxBytes <= 0 || maxBytes > maxAllowedResponse {
		return nil, &ConfigurationError{Reason: "response limit must be within (0, 4 MiB]"}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Client{
		origin:           origin,
		queryToken:       cfg.QueryToken,
		httpClient:       httpClient,
		maxResponseBytes: maxBytes,
	}, nil
}

func parseOrigin(raw string) (*url.URL, error) {
	origin, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || origin == nil ||
		(origin.Scheme != "http" && origin.Scheme != "https") ||
		origin.Host == "" || origin.User != nil ||
		(origin.Path != "" && origin.Path != "/") ||
		origin.RawQuery != "" || origin.Fragment != "" {
		return nil, &ConfigurationError{Reason: "SMF SBI origin must be an absolute HTTP(S) origin without credentials, path, query or fragment"}
	}
	origin.Path = ""
	origin.RawPath = ""
	origin.Host = strings.ToLower(origin.Host)
	return origin, nil
}

func (c *Client) Query(ctx context.Context, query Query) (Result, error) {
	if c == nil || c.origin == nil || c.httpClient == nil {
		return Result{}, &ConfigurationError{Reason: "client is not configured"}
	}
	masID, err := normalizeMAS(query.MAS)
	if err != nil {
		return Result{}, err
	}
	body, err := query.requestBody()
	if err != nil {
		return Result{}, err
	}

	endpoint := *c.origin
	endpoint.Path = "/nsmf-" + strings.ToLower(masID) + "-context/v1/query"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return Result{}, &InputError{Reason: "could not construct request"}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(queryTokenHeader, c.queryToken)
	request.Header.Set("User-Agent", "ca-client-tools/1")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return Result{}, fmt.Errorf("OpenAMSM request failed: %w", err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return Result{}, &ProtocolError{Reason: "could not read response"}
	}
	if int64(len(raw)) > c.maxResponseBytes {
		return Result{}, &ProtocolError{Reason: "response exceeds configured size limit"}
	}

	if response.StatusCode != http.StatusOK {
		return Result{}, remoteError(response.StatusCode, raw)
	}

	mediaType, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		return Result{}, &ProtocolError{Reason: "success response is not application/json"}
	}
	decoded, err := decodeUniqueJSON(raw)
	if err != nil {
		return Result{}, &ProtocolError{Reason: "success response is not valid unique-key JSON"}
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return Result{}, &ProtocolError{Reason: "success response must be a JSON object"}
	}
	result, err := parseResult(root, masID)
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func (q Query) requestBody() ([]byte, error) {
	hasIdentity := q.SUPI != "" || q.IMSI != ""
	hasIPv4 := q.UEIPv4 != ""
	hasIPv6 := q.UEIPv6 != ""
	hasIP := hasIPv4 || hasIPv6

	if hasIdentity && hasIP {
		return nil, &InputError{Reason: "identity/PSI and UE-IP lookup keys cannot be combined"}
	}
	if q.SUPI != "" && q.IMSI != "" {
		return nil, &InputError{Reason: "supi and imsi are mutually exclusive"}
	}
	if hasIPv4 && hasIPv6 {
		return nil, &InputError{Reason: "ueIpv4 and ueIpv6 are mutually exclusive"}
	}

	payload := make(map[string]any, 3)
	switch {
	case hasIdentity:
		if q.PDUSessionID == nil || *q.PDUSessionID < 0 || *q.PDUSessionID > 255 {
			return nil, &InputError{Reason: "identity lookup requires pduSessionId in [0, 255]"}
		}
		if q.SUPI != "" {
			if !validSUPI(q.SUPI) {
				return nil, &InputError{Reason: "supi must use imsi- followed by 15 decimal digits"}
			}
			payload["supi"] = q.SUPI
		} else {
			if !validIMSI(q.IMSI) {
				return nil, &InputError{Reason: "imsi must contain exactly 15 decimal digits"}
			}
			payload["imsi"] = q.IMSI
		}
		payload["pduSessionId"] = *q.PDUSessionID
	case hasIPv4:
		if q.PDUSessionID != nil {
			return nil, &InputError{Reason: "UE-IP lookup must not include pduSessionId"}
		}
		ip := net.ParseIP(q.UEIPv4)
		if ip == nil || ip.To4() == nil {
			return nil, &InputError{Reason: "ueIpv4 must be an IPv4 literal"}
		}
		payload["ueIpv4"] = ip.To4().String()
	case hasIPv6:
		if q.PDUSessionID != nil {
			return nil, &InputError{Reason: "UE-IP lookup must not include pduSessionId"}
		}
		ip := net.ParseIP(q.UEIPv6)
		if ip == nil || ip.To4() != nil {
			return nil, &InputError{Reason: "ueIpv6 must be an IPv6 literal"}
		}
		payload["ueIpv6"] = ip.String()
	default:
		return nil, &InputError{Reason: "provide either supi/imsi plus pduSessionId, or one UE-IP key"}
	}
	return json.Marshal(payload)
}

func normalizeMAS(value string) (string, error) {
	masID := strings.ToUpper(strings.TrimSpace(value))
	if _, ok := supportedMAS[masID]; !ok {
		return "", &InputError{Reason: "mas must be one of SC, PA, UPN, SSM, IAM, UPS, CA or SAA"}
	}
	return masID, nil
}

func validSUPI(value string) bool {
	return strings.HasPrefix(value, "imsi-") && validIMSI(strings.TrimPrefix(value, "imsi-"))
}

func validIMSI(value string) bool {
	if len(value) != 15 {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func parseResult(root map[string]any, expectedMAS string) (Result, error) {
	masID, ok := root["masId"].(string)
	if !ok || masID != expectedMAS {
		return Result{}, &ProtocolError{Reason: "masId does not match requested MAS"}
	}
	status, ok := root["status"].(string)
	if !ok || (status != "AVAILABLE" && status != "UNAVAILABLE") {
		return Result{}, &ProtocolError{Reason: "status must be AVAILABLE or UNAVAILABLE"}
	}
	key, ok := root["key"].(map[string]any)
	if !ok {
		return Result{}, &ProtocolError{Reason: "key must be an object"}
	}
	contextValue, ok := root["context"].(map[string]any)
	if !ok {
		return Result{}, &ProtocolError{Reason: "context must be an object"}
	}
	if status == "UNAVAILABLE" && len(contextValue) != 0 {
		return Result{}, &ProtocolError{Reason: "UNAVAILABLE response must contain an empty context object"}
	}
	return Result{MASID: masID, Status: status, Key: key, Context: contextValue}, nil
}

func remoteError(status int, raw []byte) error {
	cause := ""
	if decoded, err := decodeUniqueJSON(raw); err == nil {
		if object, ok := decoded.(map[string]any); ok {
			if value, ok := object["cause"].(string); ok {
				cause = value
			}
		}
	}
	return &RemoteError{HTTPStatus: status, Cause: cause}
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

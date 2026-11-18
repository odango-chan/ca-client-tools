package mas

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/odango-chan/ca-client-tools/internal/jsonx"
	"github.com/odango-chan/ca-client-tools/internal/sbi"
)

const (
	defaultTimeout     = 5 * time.Second
	maxTimeout         = 30 * time.Second
	defaultMaxResponse = int64(1 << 20)
	maxAllowedResponse = int64(4 << 20)
	queryTokenHeader   = "X-OpenAMSM-Query-Token"
	problemMediaType   = "application/problem+json"
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
	transport        queryTransport
	maxResponseBytes int64
}

// queryTransport is the single remote transport used by the typed SMF MAS
// client. The default implementation speaks cleartext HTTP/2 with prior
// knowledge (h2c) directly. The shared internal/sbi transport can implement the
// same interface without changing query, identity, status or ProblemDetails
// semantics.
type queryTransport interface {
	Do(
		ctx context.Context,
		path string,
		contentType, accept string,
		body []byte,
	) (*http.Response, error)
	CloseIdleConnections()
}

type Query struct {
	MAS          string
	SUPI         string
	IMSI         string
	PDUSessionID *int
	UEIPv4       string
	UEIPv6       string
}

// queryKey is the locally validated and canonicalized lookup key. OpenAMSM
// resolves this key and echoes it back in the response envelope. The same
// value therefore drives both the request body and the response binding.
type queryKey struct {
	supi string
	imsi string
	psi  *int
	ipv4 string
	ipv6 string
}

func (k queryKey) hasIdentity() bool { return k.supi != "" || k.imsi != "" }

// resolvedSUPI returns the canonical SUPI expected in the echoed response key.
// IMSI is a request-only alias: OpenAMSM resolves it and echoes the SUPI.
func (k queryKey) resolvedSUPI() string {
	if k.supi != "" {
		return k.supi
	}
	if k.imsi != "" {
		return "imsi-" + k.imsi
	}
	return ""
}

type Result struct {
	MASID   string         `json:"masId"`
	Status  string         `json:"status"`
	Key     map[string]any `json:"key"`
	Context map[string]any `json:"context"`
}

// ProblemDetails is a validated remote RFC 7807-style OpenAMSM error. Only the
// stable HTTP-matching status and safe machine cause are exposed; title, detail
// and any other remote text are never propagated.
type ProblemDetails struct {
	Status int    `json:"status"`
	Cause  string `json:"cause"`
}

// Outcome is the typed remote result of one query. Exactly one of Result or
// Problem is non-nil. A valid remote ProblemDetails response is a successful
// remote outcome; the caller maps it to business semantics.
type Outcome struct {
	HTTPStatus int
	Result     *Result
	Problem    *ProblemDetails
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

	transport, err := newSBITransport(origin, cfg.QueryToken, timeout)
	if err != nil {
		return nil, err
	}

	return &Client{
		transport:        transport,
		maxResponseBytes: maxBytes,
	}, nil
}

// NewClientWithSBI builds the typed SMF MAS client over an already-built shared
// SBI transport and trusted service key. The shared transport owns the origin,
// h2c profile and query-token profile; the typed client keeps its query,
// identity, status and ProblemDetails semantics. This is the Provider path: one
// trusted transport instance serves both typed and generic dispatch.
func NewClientWithSBI(cfg Config, client *sbi.Client, serviceKey string) (*Client, error) {
	transport, err := newSharedSBITransport(client, serviceKey)
	if err != nil {
		return nil, err
	}
	maxBytes := cfg.MaxResponseBytes
	if maxBytes == 0 {
		maxBytes = defaultMaxResponse
	}
	if maxBytes <= 0 || maxBytes > maxAllowedResponse {
		return nil, &ConfigurationError{Reason: "response limit must be within (0, 4 MiB]"}
	}
	return &Client{
		transport:        transport,
		maxResponseBytes: maxBytes,
	}, nil
}

func parseOrigin(raw string) (*url.URL, error) {
	origin, err := url.Parse(strings.TrimSpace(raw))
	// The OpenAMSM SBI listener is cleartext HTTP/2 with prior knowledge (h2c)
	// and this client installs an http2.Transport that dials a raw TCP
	// connection. An https:// origin would be accepted by net/url but cannot be
	// served correctly because no TLS handshake happens, so restrict the
	// deployment origin to http:// and reject everything else up front.
	if err != nil || origin == nil || origin.Scheme != "http" ||
		origin.Host == "" || origin.User != nil ||
		(origin.Path != "" && origin.Path != "/") ||
		origin.RawQuery != "" || origin.Fragment != "" {
		return nil, &ConfigurationError{Reason: "SMF SBI origin must be an absolute http:// origin (h2c) without credentials, path, query or fragment"}
	}
	origin.Path = ""
	origin.RawPath = ""
	origin.Host = strings.ToLower(origin.Host)
	return origin, nil
}

// Query returns the typed 200 response for the query. A non-200 response is a
// *RemoteError. This is the CLI v1 behavior; the Provider uses QueryOutcome so
// a validated ProblemDetails response is returned as a typed remote outcome.
func (c *Client) Query(ctx context.Context, query Query) (Result, error) {
	masID, key, response, err := c.roundTrip(ctx, query)
	if err != nil {
		return Result{}, err
	}
	if response.status != http.StatusOK {
		return Result{}, remoteError(response.status, response.body)
	}
	return parseSuccess(masID, key, response)
}

// QueryOutcome returns the exact typed remote outcome for the query. HTTP 200
// produces a Result; a non-200 response is accepted only as a valid
// application/problem+json ProblemDetails object whose status matches the HTTP
// status and whose cause is a safe machine token. Anything else is a
// *ProtocolError. Transport failures are returned as transport errors, never
// as fabricated remote outcomes.
func (c *Client) QueryOutcome(ctx context.Context, query Query) (Outcome, error) {
	masID, key, response, err := c.roundTrip(ctx, query)
	if err != nil {
		return Outcome{}, err
	}
	if response.status == http.StatusOK {
		result, err := parseSuccess(masID, key, response)
		if err != nil {
			return Outcome{}, err
		}
		return Outcome{HTTPStatus: response.status, Result: &result}, nil
	}
	problem, err := parseProblem(response.status, response.header, response.body)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{HTTPStatus: response.status, Problem: problem}, nil
}

func (c *Client) CloseIdleConnections() {
	if c == nil || c.transport == nil {
		return
	}
	c.transport.CloseIdleConnections()
}

type rawResponse struct {
	status int
	header http.Header
	body   []byte
}

func (c *Client) roundTrip(ctx context.Context, query Query) (string, queryKey, rawResponse, error) {
	if c == nil || c.transport == nil {
		return "", queryKey{}, rawResponse{}, &ConfigurationError{Reason: "client is not configured"}
	}
	masID, err := normalizeMAS(query.MAS)
	if err != nil {
		return "", queryKey{}, rawResponse{}, err
	}
	key, err := query.validate()
	if err != nil {
		return "", queryKey{}, rawResponse{}, err
	}
	body, err := key.requestBody()
	if err != nil {
		return "", queryKey{}, rawResponse{}, err
	}

	response, err := c.transport.Do(
		ctx,
		"/nsmf-"+strings.ToLower(masID)+"-context/v1/query",
		"application/json",
		"application/json",
		body,
	)
	if err != nil {
		return "", queryKey{}, rawResponse{}, err
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return "", queryKey{}, rawResponse{}, &ProtocolError{Reason: "could not read response"}
	}
	if int64(len(raw)) > c.maxResponseBytes {
		return "", queryKey{}, rawResponse{}, &ProtocolError{Reason: "response exceeds configured size limit"}
	}
	return masID, key, rawResponse{status: response.StatusCode, header: response.Header, body: raw}, nil
}

func parseSuccess(masID string, key queryKey, response rawResponse) (Result, error) {
	mediaType, _, mediaErr := mime.ParseMediaType(response.header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		return Result{}, &ProtocolError{Reason: "success response is not application/json"}
	}
	decoded, err := decodeUniqueJSON(response.body)
	if err != nil {
		return Result{}, &ProtocolError{Reason: "success response is not valid unique-key JSON"}
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return Result{}, &ProtocolError{Reason: "success response must be a JSON object"}
	}
	return parseResult(root, masID, key)
}

func parseProblem(status int, header http.Header, body []byte) (*ProblemDetails, error) {
	mediaType, _, mediaErr := mime.ParseMediaType(header.Get("Content-Type"))
	if mediaErr != nil || mediaType != problemMediaType {
		return nil, &ProtocolError{Reason: "error response is not application/problem+json"}
	}
	decoded, err := decodeUniqueJSON(body)
	if err != nil {
		return nil, &ProtocolError{Reason: "error response is not valid unique-key JSON"}
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return nil, &ProtocolError{Reason: "error response must be a JSON object"}
	}
	statusValue, ok := root["status"].(json.Number)
	if !ok {
		return nil, &ProtocolError{Reason: "problem status must be an integer"}
	}
	problemStatus, err := statusValue.Int64()
	if err != nil || problemStatus < 100 || problemStatus > 599 || int(problemStatus) != status {
		return nil, &ProtocolError{Reason: "problem status does not match the HTTP status"}
	}
	cause, ok := root["cause"].(string)
	if !ok || !safeCause(cause) {
		return nil, &ProtocolError{Reason: "problem cause is missing or not a safe machine token"}
	}
	return &ProblemDetails{Status: int(problemStatus), Cause: cause}, nil
}

// validate enforces the OpenAMSM v0.10 key-combination rules before any
// network access and canonicalizes the values used for the response binding.
func (q Query) validate() (queryKey, error) {
	key := queryKey{supi: q.SUPI, imsi: q.IMSI}
	hasIP := q.UEIPv4 != "" || q.UEIPv6 != ""

	if !key.hasIdentity() && !hasIP {
		return queryKey{}, &InputError{Reason: "provide SUPI/IMSI plus pduSessionId, or at least one UE-IP key"}
	}

	if key.hasIdentity() {
		if q.PDUSessionID == nil || *q.PDUSessionID < 0 || *q.PDUSessionID > 255 {
			return queryKey{}, &InputError{Reason: "identity lookup requires pduSessionId in [0, 255]"}
		}
		if q.SUPI != "" && !validSUPI(q.SUPI) {
			return queryKey{}, &InputError{Reason: "supi must use imsi- followed by 1 to 15 decimal digits"}
		}
		if q.IMSI != "" && !validIMSI(q.IMSI) {
			return queryKey{}, &InputError{Reason: "imsi must contain 1 to 15 decimal digits"}
		}
		if q.SUPI != "" && q.IMSI != "" && strings.TrimPrefix(q.SUPI, "imsi-") != q.IMSI {
			return queryKey{}, &InputError{Reason: "supi and imsi must normalize to the same UE"}
		}
		psi := *q.PDUSessionID
		key.psi = &psi
	} else if q.PDUSessionID != nil {
		return queryKey{}, &InputError{Reason: "pduSessionId requires supi or imsi"}
	}

	if q.UEIPv4 != "" {
		ip := net.ParseIP(q.UEIPv4)
		if ip == nil || ip.To4() == nil {
			return queryKey{}, &InputError{Reason: "ueIpv4 must be an IPv4 literal"}
		}
		key.ipv4 = ip.To4().String()
	}
	if q.UEIPv6 != "" {
		ip := net.ParseIP(q.UEIPv6)
		if ip == nil || ip.To4() != nil {
			return queryKey{}, &InputError{Reason: "ueIpv6 must be an IPv6 literal"}
		}
		key.ipv6 = ip.String()
	}

	return key, nil
}

func (k queryKey) requestBody() ([]byte, error) {
	payload := make(map[string]any, 5)
	if k.hasIdentity() {
		if k.supi != "" {
			payload["supi"] = k.supi
		}
		if k.imsi != "" {
			payload["imsi"] = k.imsi
		}
		payload["pduSessionId"] = *k.psi
	}
	if k.ipv4 != "" {
		payload["ueIpv4"] = k.ipv4
	}
	if k.ipv6 != "" {
		payload["ueIpv6"] = k.ipv6
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
	if len(value) < 1 || len(value) > 15 {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func parseResult(root map[string]any, expectedMAS string, expected queryKey) (Result, error) {
	masID, ok := root["masId"].(string)
	if !ok || masID != expectedMAS {
		return Result{}, &ProtocolError{Reason: "masId does not match requested MAS"}
	}
	status, ok := root["status"].(string)
	if !ok || (status != "AVAILABLE" && status != "UNAVAILABLE") {
		return Result{}, &ProtocolError{Reason: "status must be AVAILABLE or UNAVAILABLE"}
	}
	key, ok := root["key"].(map[string]any)
	if !ok || len(key) == 0 {
		return Result{}, &ProtocolError{Reason: "key must be a non-empty object"}
	}
	if err := bindEchoKey(key, expected); err != nil {
		return Result{}, err
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

// bindEchoKey rejects a response whose echoed lookup key does not identify the
// exact UE and PDU Session that were requested. OpenAMSM v0.10 echoes the
// resolved SUPI (never the request-only IMSI alias) together with the requested
// PSI, and echoes each supplied UE-IP lookup key. A missing, extra or different
// field means the response belongs to another lookup and must not be accepted,
// even when it is otherwise well-formed.
func bindEchoKey(key map[string]any, expected queryKey) error {
	matched := make(map[string]struct{}, 4)

	if expected.hasIdentity() {
		supi, ok := key["supi"].(string)
		if !ok || supi != expected.resolvedSUPI() {
			return &ProtocolError{Reason: "key.supi does not match the requested identity"}
		}
		matched["supi"] = struct{}{}

		if expected.psi == nil {
			return &ProtocolError{Reason: "requested PSI is missing"}
		}
		psi, ok := integerKeyField(key["pduSessionId"])
		if !ok || psi != *expected.psi {
			return &ProtocolError{Reason: "key.pduSessionId does not match the requested PSI"}
		}
		matched["pduSessionId"] = struct{}{}
	}

	if expected.ipv4 != "" {
		value, ok := key["ueIpv4"].(string)
		if !ok || !sameIP(value, expected.ipv4, true) {
			return &ProtocolError{Reason: "key.ueIpv4 does not match the requested IPv4 lookup key"}
		}
		matched["ueIpv4"] = struct{}{}
	}
	if expected.ipv6 != "" {
		value, ok := key["ueIpv6"].(string)
		if !ok || !sameIP(value, expected.ipv6, false) {
			return &ProtocolError{Reason: "key.ueIpv6 does not match the requested IPv6 lookup key"}
		}
		matched["ueIpv6"] = struct{}{}
	}

	for field := range key {
		if _, ok := matched[field]; !ok {
			return &ProtocolError{Reason: "key contains an unexpected field"}
		}
	}
	return nil
}

func integerKeyField(value any) (int, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := number.Int64()
	if err != nil || parsed < 0 || parsed > 255 {
		return 0, false
	}
	return int(parsed), true
}

func sameIP(value, expected string, wantIPv4 bool) bool {
	parsed := net.ParseIP(value)
	if parsed == nil {
		return false
	}
	if wantIPv4 {
		if parsed.To4() == nil {
			return false
		}
	} else if parsed.To4() != nil {
		return false
	}
	want := net.ParseIP(expected)
	if want == nil {
		return false
	}
	return parsed.Equal(want)
}

func remoteError(status int, raw []byte) error {
	cause := ""
	if decoded, err := decodeUniqueJSON(raw); err == nil {
		if object, ok := decoded.(map[string]any); ok {
			if value, ok := object["cause"].(string); ok && safeCause(value) {
				cause = value
			}
		}
	}
	return &RemoteError{HTTPStatus: status, Cause: cause}
}

// ProblemDetails.cause is a stable machine identifier, not free-form text.
// Only expose the conventional uppercase token form in the CLI error envelope.
func safeCause(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}

func decodeUniqueJSON(raw []byte) (any, error) {
	return jsonx.Decode(raw)
}

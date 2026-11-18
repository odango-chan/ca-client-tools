package sbi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

var (
	ErrHTTP2Required    = errors.New("SBI endpoint did not negotiate HTTP/2")
	ErrHTTP1Required    = errors.New("metrics endpoint did not use HTTP/1.1")
	ErrRequestTooLarge  = errors.New("SBI request exceeds configured size limit")
	ErrResponseTooLarge = errors.New("SBI response exceeds configured size limit")
	ErrUnknownService   = errors.New("unknown SBI service")
	ErrInvalidRequest   = errors.New("invalid SBI request")
)

type Request struct {
	Method        string
	Path          string
	Query         url.Values
	Headers       http.Header
	ContentType   string
	Accept        string
	Body          io.Reader
	ContentLength int64
}

// Client is the shared transport for every trusted service profile. Each
// profile owns exactly one http.Transport/http.Client so the SBI h2c and
// metrics HTTP/1.1 wire guarantees cannot leak into one another, while
// lifecycle, cancellation, timeout, redirect and size handling stay shared.
type Client struct {
	services   map[string]*serviceClient
	transports map[TransportProfile]*http.Transport
}

type serviceClient struct {
	origin            *url.URL
	apiRoot           string
	headerTokenHeader string
	headerToken       string
	requestTimeout    time.Duration
	maxRequestBytes   int64
	maxResponseBytes  int64
	profile           TransportProfile
	httpClient        *http.Client
}

func NewClient(input Config) (*Client, error) {
	config, err := normalizeConfig(input)
	if err != nil {
		return nil, err
	}

	limits := config.Limits
	client := &Client{
		services:   make(map[string]*serviceClient, len(config.Services)),
		transports: make(map[TransportProfile]*http.Transport, 2),
	}
	httpClients := make(map[TransportProfile]*http.Client, 2)
	for key, service := range config.Services {
		profile := service.Profile
		transport, ok := client.transports[profile]
		if !ok {
			transport = newProfileTransport(limits, profile)
			client.transports[profile] = transport
			httpClients[profile] = &http.Client{
				Transport: transport,
				CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
		}
		origin, _ := parseServiceOriginForProfile(service.Origin, profile)
		var tokenHeader, token string
		if service.HeaderTokenProfile != "" {
			tokenProfile := config.HeaderTokenProfiles[service.HeaderTokenProfile]
			tokenHeader, token = tokenProfile.Header, tokenProfile.Token
		}
		client.services[key] = &serviceClient{
			origin:            origin,
			apiRoot:           service.APIRoot,
			headerTokenHeader: tokenHeader,
			headerToken:       token,
			requestTimeout:    limits.RequestTimeout,
			maxRequestBytes:   limits.MaxRequestBytes,
			maxResponseBytes:  limits.MaxResponseBytes,
			profile:           profile,
			httpClient:        httpClients[profile],
		}
	}
	return client, nil
}

// newProfileTransport builds the one reviewed transport for a profile. Proxy
// inheritance stays disabled, redirects are handled by the client, and every
// connection, size and timeout bound comes from the trusted limits.
func newProfileTransport(limits Limits, profile TransportProfile) *http.Transport {
	protocols := new(http.Protocols)
	switch profile {
	case TransportMetricsHTTP1:
		protocols.SetHTTP1(true)
	default:
		protocols.SetUnencryptedHTTP2(true)
	}
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: limits.DialTimeout, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          limits.MaxIdleConns,
		MaxIdleConnsPerHost:   limits.MaxIdleConnsPerHost,
		MaxConnsPerHost:       limits.MaxConnsPerHost,
		IdleConnTimeout:       limits.IdleConnTimeout,
		ExpectContinueTimeout: time.Second,
		DisableCompression:    true,
		Protocols:             protocols,
	}
}

func (c *Client) Do(ctx context.Context, serviceKey string, input Request) (*http.Response, error) {
	if c == nil {
		return nil, ErrUnknownService
	}
	service, ok := c.services[serviceKey]
	if !ok {
		return nil, ErrUnknownService
	}
	if ctx == nil {
		ctx = context.Background()
	}
	method := strings.ToUpper(input.Method)
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return nil, ErrInvalidRequest
	}
	if err := validateAPIPath(input.Path); err != nil || input.Path == "" {
		return nil, ErrInvalidRequest
	}
	if strings.ContainsAny(input.ContentType, "\r\n") || strings.ContainsAny(input.Accept, "\r\n") {
		return nil, ErrInvalidRequest
	}
	endpoint, err := resolveEndpoint(service.origin, service.apiRoot, input.Path, input.Query)
	if err != nil {
		return nil, ErrInvalidRequest
	}
	if input.ContentLength < -1 || (input.Body == nil && input.ContentLength != 0) {
		return nil, ErrInvalidRequest
	}
	if input.Body != nil {
		if input.ContentLength > service.maxRequestBytes {
			return nil, ErrRequestTooLarge
		}
		if sized, ok := input.Body.(interface{ Len() int }); ok &&
			int64(sized.Len()) > service.maxRequestBytes {
			return nil, ErrRequestTooLarge
		}
	}

	requestContext, cancel := context.WithTimeout(ctx, service.requestTimeout)
	var body io.Reader
	contentLength := int64(0)
	if input.Body != nil {
		body = newLimitedRequestBody(input.Body, service.maxRequestBytes)
		contentLength = input.ContentLength
		if contentLength == 0 {
			contentLength = -1
		}
	}
	request, err := http.NewRequestWithContext(requestContext, method, endpoint.String(), body)
	if err != nil {
		cancel()
		return nil, ErrInvalidRequest
	}
	if input.Body != nil && contentLength >= 0 {
		request.ContentLength = contentLength
	}
	if err := copyRequestHeaders(request.Header, input.Headers); err != nil {
		cancel()
		return nil, ErrInvalidRequest
	}
	if service.headerTokenHeader != "" {
		if hasHeader(request.Header, service.headerTokenHeader) {
			cancel()
			return nil, ErrInvalidRequest
		}
		request.Header.Set(service.headerTokenHeader, service.headerToken)
	}
	if input.ContentType != "" {
		request.Header.Set("Content-Type", input.ContentType)
	}
	if input.Accept != "" {
		request.Header.Set("Accept", input.Accept)
	} else if request.Header.Get("Accept") == "" {
		request.Header.Set("Accept", "application/json")
	}
	request.Header.Set("User-Agent", "ca-client-tools-sbi/1")

	response, err := service.httpClient.Do(request)
	if err != nil {
		cancel()
		if requestContext.Err() != nil {
			return nil, requestContext.Err()
		}
		return nil, err
	}
	if err := requireProfileProtocol(service.profile, response.ProtoMajor); err != nil {
		response.Body.Close()
		cancel()
		return nil, err
	}
	response.Body = &limitedResponseBody{
		ReadCloser: response.Body,
		remaining:  service.maxResponseBytes,
		cancel:     cancel,
	}
	return response, nil
}

func resolveEndpoint(origin *url.URL, apiRoot, operationPath string, query url.Values) (*url.URL, error) {
	if origin == nil || validateAPIPath(operationPath) != nil || operationPath == "" {
		return nil, ErrInvalidRequest
	}
	escapedPath := strings.TrimRight(apiRoot, "/") + operationPath
	decodedPath, err := url.PathUnescape(escapedPath)
	if err != nil {
		return nil, ErrInvalidRequest
	}
	endpoint := *origin
	endpoint.Path = decodedPath
	endpoint.RawPath = escapedPath
	if endpoint.EscapedPath() != escapedPath {
		endpoint.RawPath = ""
	}
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}
	return &endpoint, nil
}

func copyRequestHeaders(destination, source http.Header) error {
	for name, values := range source {
		if !validHeaderName(name) || isReservedHeader(name) {
			return ErrInvalidRequest
		}
		canonical := textproto.CanonicalMIMEHeaderKey(name)
		if canonical == "" {
			return ErrInvalidRequest
		}
		for _, value := range values {
			if strings.ContainsAny(value, "\r\n") {
				return ErrInvalidRequest
			}
			destination.Add(canonical, value)
		}
	}
	return nil
}

func hasHeader(headers http.Header, name string) bool {
	for key := range headers {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

// requireProfileProtocol enforces the reviewed wire protocol of one service
// profile after negotiation. A profile mismatch is a transport failure, never a
// silent fallback.
func requireProfileProtocol(profile TransportProfile, protoMajor int) error {
	switch profile {
	case TransportMetricsHTTP1:
		if protoMajor != 1 {
			return ErrHTTP1Required
		}
		return nil
	default:
		if protoMajor != 2 {
			return ErrHTTP2Required
		}
		return nil
	}
}

// HasService reports whether the trusted configuration resolved the service
// key. It exposes no origin or credential.
func (c *Client) HasService(serviceKey string) bool {
	if c == nil {
		return false
	}
	_, ok := c.services[serviceKey]
	return ok
}

// ServiceProfile returns the trusted transport profile for a resolved service.
func (c *Client) ServiceProfile(serviceKey string) (TransportProfile, bool) {
	if c == nil {
		return "", false
	}
	service, ok := c.services[serviceKey]
	if !ok {
		return "", false
	}
	return service.profile, true
}

func (c *Client) CloseIdleConnections() {
	if c == nil {
		return
	}
	for _, transport := range c.transports {
		transport.CloseIdleConnections()
	}
}

type limitedRequestBody struct {
	reader    io.Reader
	closer    io.Closer
	remaining int64
}

func (b *limitedRequestBody) Read(buffer []byte) (int, error) {
	if b.remaining == 0 {
		var probe [1]byte
		n, err := b.reader.Read(probe[:])
		if n > 0 {
			return 0, ErrRequestTooLarge
		}
		return 0, err
	}
	if int64(len(buffer)) > b.remaining {
		buffer = buffer[:b.remaining]
	}
	n, err := b.reader.Read(buffer)
	b.remaining -= int64(n)
	return n, err
}

func (b *limitedRequestBody) Close() error {
	if b.closer != nil {
		return b.closer.Close()
	}
	return nil
}

type limitedResponseBody struct {
	io.ReadCloser
	remaining int64
	cancel    context.CancelFunc
}

func (b *limitedResponseBody) Read(buffer []byte) (int, error) {
	if b.remaining == 0 {
		var probe [1]byte
		n, err := b.ReadCloser.Read(probe[:])
		if n > 0 {
			return 0, ErrResponseTooLarge
		}
		return 0, err
	}
	if int64(len(buffer)) > b.remaining {
		buffer = buffer[:b.remaining]
	}
	n, err := b.ReadCloser.Read(buffer)
	b.remaining -= int64(n)
	return n, err
}

func (b *limitedResponseBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

func newLimitedRequestBody(input io.Reader, maximum int64) *limitedRequestBody {
	body := &limitedRequestBody{reader: input, remaining: maximum}
	if closer, ok := input.(io.Closer); ok {
		body.closer = closer
	}
	return body
}

var _ io.ReadCloser = (*limitedRequestBody)(nil)
var _ io.ReadCloser = (*limitedResponseBody)(nil)

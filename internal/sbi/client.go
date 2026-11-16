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

type Client struct {
	services   map[string]*serviceClient
	transports map[string]*http.Transport
}

type serviceClient struct {
	origin          *url.URL
	apiRoot         string
	httpClient      *http.Client
	authenticate    authFunc
	requestTimeout  time.Duration
	maxRequestBytes int64
	maxResponseBytes int64
}

func NewClient(input Config) (*Client, error) {
	config, err := normalizeConfig(input)
	if err != nil {
		return nil, err
	}
	client := &Client{
		services:   make(map[string]*serviceClient, len(config.Services)),
		transports: make(map[string]*http.Transport),
	}
	transportFor := func(profileName string) (*http.Transport, error) {
		if transport, ok := client.transports[profileName]; ok {
			return transport, nil
		}
		profile := config.TLSProfiles[profileName]
		tlsConfig, err := buildTLSConfig(profile)
		if err != nil {
			return nil, errInvalidConfiguration
		}
		tlsConfig.NextProtos = []string{"h2", "http/1.1"}
		limits := config.Limits
		transport := &http.Transport{
			Proxy:                 nil,
			DialContext:           (&net.Dialer{Timeout: limits.TLSHandshakeTimeout, KeepAlive: 30 * time.Second}).DialContext,
			TLSClientConfig:       tlsConfig,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          limits.MaxIdleConns,
			MaxIdleConnsPerHost:   limits.MaxIdleConnsPerHost,
			MaxConnsPerHost:       limits.MaxConnsPerHost,
			IdleConnTimeout:       limits.IdleConnTimeout,
			TLSHandshakeTimeout:   limits.TLSHandshakeTimeout,
			ExpectContinueTimeout: time.Second,
			DisableCompression:    true,
		}
		client.transports[profileName] = transport
		return transport, nil
	}

	for key, service := range config.Services {
		origin, _ := parseServiceOrigin(service.Origin)
		transport, err := transportFor(service.TLSProfile)
		if err != nil {
			client.CloseIdleConnections()
			return nil, err
		}
		httpClient := &http.Client{
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		authenticate, err := buildAuthenticator(config, service, transportFor)
		if err != nil {
			client.CloseIdleConnections()
			return nil, err
		}
		client.services[key] = &serviceClient{
			origin:           origin,
			apiRoot:          service.APIRoot,
			httpClient:       httpClient,
			authenticate:     authenticate,
			requestTimeout:   config.Limits.RequestTimeout,
			maxRequestBytes:  config.Limits.MaxRequestBytes,
			maxResponseBytes: config.Limits.MaxResponseBytes,
		}
	}
	return client, nil
}

func buildAuthenticator(
	config Config,
	service ServiceConfig,
	transportFor func(string) (*http.Transport, error),
) (authFunc, error) {
	if service.AuthProfile == "" {
		return nil, nil
	}
	if profile, ok := config.HeaderTokenProfiles[service.AuthProfile]; ok {
		return func(_ context.Context, request *http.Request) error {
			request.Header.Set(profile.Header, profile.Token)
			return nil
		}, nil
	}
	profile, ok := config.OAuthProfiles[service.AuthProfile]
	if !ok {
		return nil, errInvalidConfiguration
	}
	tlsProfile := profile.TLSProfile
	if tlsProfile == "" {
		tlsProfile = service.TLSProfile
	}
	transport, err := transportFor(tlsProfile)
	if err != nil {
		return nil, err
	}
	tokenClient := &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	source := newOAuthTokenSource(profile, tokenClient, config.Limits.RequestTimeout)
	return func(ctx context.Context, request *http.Request) error {
		token, err := source.bearerToken(ctx)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+token)
		return nil
	}, nil
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
	if input.ContentLength < -1 || (input.Body == nil && input.ContentLength > 0) {
		return nil, ErrInvalidRequest
	}
	if input.Body != nil && input.ContentLength > service.maxRequestBytes {
		return nil, ErrRequestTooLarge
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
	if input.ContentType != "" {
		request.Header.Set("Content-Type", input.ContentType)
	}
	if input.Accept != "" {
		request.Header.Set("Accept", input.Accept)
	} else if request.Header.Get("Accept") == "" {
		request.Header.Set("Accept", "application/json")
	}
	request.Header.Set("User-Agent", "ca-client-tools-sbi/1")
	if service.authenticate != nil {
		if err := service.authenticate(requestContext, request); err != nil {
			cancel()
			return nil, err
		}
	}

	response, err := service.httpClient.Do(request)
	if err != nil {
		cancel()
		if requestContext.Err() != nil {
			return nil, requestContext.Err()
		}
		return nil, err
	}
	if response.ProtoMajor != 2 {
		response.Body.Close()
		cancel()
		return nil, ErrHTTP2Required
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

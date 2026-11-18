// Package artifactchannel implements the Provider side of the pinned Host
// Artifact Store cross-process channel (`ca-host.artifact-store/v1`, ca-agent
// commit 3a0d140401109d19152e253cb0b89b59feef7bfb).
//
// The channel is a Host-owned loopback HTTP/1.1 listener. The Provider resolves
// Host-issued opaque references with GET requests and stages output artifacts
// with POST requests. Bytes never travel in NDJSON, never touch a Provider
// chosen filesystem path and are never logged. Channel errors are mapped to the
// two Provider fault codes fixed by the contract: ARTIFACT_REFERENCE_INVALID
// for an unusable reference and ARTIFACT_TRANSFER_FAILED for a transfer or
// quota failure. Caller cancellation/deadline keeps its context error so the
// protocol layer can report CALL_CANCELLED / CALL_DEADLINE_EXCEEDED.
package artifactchannel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/odango-chan/ca-client-tools/internal/sbi/payload"
)

// Provider protocol fault codes produced by a channel failure. They are the
// only channel-derived codes; Host channel codes never cross to the caller.
const (
	FaultReferenceInvalid = "ARTIFACT_REFERENCE_INVALID"
	FaultTransferFailed   = "ARTIFACT_TRANSFER_FAILED"
)

const (
	// MaximumErrorBodyBytes bounds a Host channel error body. It is never
	// projected, only drained so the connection can be reused.
	maximumErrorBodyBytes = 8 << 10
	// MaximumResponseBodyBytes bounds the 201 staging result body.
	maximumResponseBodyBytes = 8 << 10
)

var (
	artifactTokenPattern = regexp.MustCompile(`^art_[A-Za-z0-9_-]{43}$`)
	artifactSlotPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	baseURLPattern       = regexp.MustCompile(`^http://127\.0\.0\.1:([1-9][0-9]{0,4})$`)
	sha256Pattern        = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Fault is a channel failure mapped to one Provider protocol fault code.
// Its text carries no reference, token, path, header or byte value.
type Fault struct {
	Code string
}

func (f *Fault) Error() string {
	if f == nil {
		return FaultTransferFailed
	}
	return f.Code
}

func fault(code string) *Fault { return &Fault{Code: code} }

// NewFault builds a channel fault with a validated Provider code so callers in
// higher layers can fail closed with the contract mapping.
func NewFault(code string) *Fault {
	if code == FaultReferenceInvalid || code == FaultTransferFailed {
		return &Fault{Code: code}
	}
	return &Fault{Code: FaultTransferFailed}
}

// FaultCode extracts the Provider fault code from a channel failure.
func FaultCode(err error) (string, bool) {
	var channelFault *Fault
	if errors.As(err, &channelFault) && (channelFault.Code == FaultReferenceInvalid || channelFault.Code == FaultTransferFailed) {
		return channelFault.Code, true
	}
	return "", false
}

// ValidReferenceToken reports the contract token grammar
// `^art_[A-Za-z0-9_-]{43}$`.
func ValidReferenceToken(token string) bool { return artifactTokenPattern.MatchString(token) }

// ReferenceValue validates the closed reference JSON value: exactly one string
// member `artifactRef` whose token matches the contract grammar.
func ReferenceValue(value any) (string, bool) {
	object, ok := value.(map[string]any)
	if !ok || len(object) != 1 {
		return "", false
	}
	rawToken, ok := object["artifactRef"]
	if !ok {
		return "", false
	}
	token, ok := rawToken.(string)
	if !ok || !ValidReferenceToken(token) {
		return "", false
	}
	return token, true
}

// Reference returns the canonical reference JSON value for a staged token.
func Reference(token string) map[string]any {
	return map[string]any{"artifactRef": token}
}

// Config is the validated bootstrap of one artifact channel.
type Config struct {
	BaseURL           string
	ProcessToken      string
	RequestTimeout    time.Duration
	MaxArtifactBytes  int64
	MaxAggregateBytes int64
	MaxAttachments    int
}

// Validate applies the contract bootstrap rules without logging the token.
func (c Config) Validate() error {
	match := baseURLPattern.FindStringSubmatch(c.BaseURL)
	if match == nil {
		return errors.New("artifact channel base URL is invalid")
	}
	port, err := strconv.Atoi(match[1])
	if err != nil || port < 1 || port > 65535 {
		return errors.New("artifact channel base URL port is invalid")
	}
	if len(c.ProcessToken) < 1 || len(c.ProcessToken) > 512 {
		return errors.New("artifact channel process token is invalid")
	}
	for index := 0; index < len(c.ProcessToken); index++ {
		if c.ProcessToken[index] < 0x20 || c.ProcessToken[index] > 0x7e {
			return errors.New("artifact channel process token is invalid")
		}
	}
	if c.RequestTimeout <= 0 || c.RequestTimeout > 2*time.Minute {
		return errors.New("artifact channel request timeout is invalid")
	}
	if c.MaxArtifactBytes < 1 || c.MaxArtifactBytes > 1<<30 ||
		c.MaxAggregateBytes < 1 || c.MaxAggregateBytes > 1<<30 ||
		c.MaxAttachments < 1 || c.MaxAttachments > 1<<20 {
		return errors.New("artifact channel bounds are invalid")
	}
	return nil
}

// Channel is one Host-issued artifact channel client. It carries the process
// token in memory only; it never exposes it, logs it or compares it.
type Channel struct {
	baseURL           string
	processToken      string
	requestTimeout    time.Duration
	maxArtifactBytes  int64
	maxAggregateBytes int64
	maxAttachments    int
	client            *http.Client
}

// New validates the bootstrap configuration and creates one channel client.
func New(config Config) (*Channel, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: config.RequestTimeout, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   4,
		MaxConnsPerHost:       4,
		IdleConnTimeout:       30 * time.Second,
		DisableCompression:    true,
		ExpectContinueTimeout: time.Second,
	}
	return &Channel{
		baseURL:           strings.TrimRight(config.BaseURL, "/"),
		processToken:      config.ProcessToken,
		requestTimeout:    config.RequestTimeout,
		maxArtifactBytes:  config.MaxArtifactBytes,
		maxAggregateBytes: config.MaxAggregateBytes,
		maxAttachments:    config.MaxAttachments,
		client: &http.Client{
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// MaxArtifactBytes is the effective per-artifact bound of this channel.
func (c *Channel) MaxArtifactBytes() int64 { return c.maxArtifactBytes }

// MaxAggregateBytes is the effective per-invocation aggregate bound.
func (c *Channel) MaxAggregateBytes() int64 { return c.maxAggregateBytes }

// MaxAttachments is the effective per-invocation attachment count bound.
func (c *Channel) MaxAttachments() int { return c.maxAttachments }

// CloseIdleConnections releases idle channel connections.
func (c *Channel) CloseIdleConnections() {
	if c == nil || c.client == nil {
		return
	}
	if transport, ok := c.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

// Input is one resolved input artifact stream. The metadata has already been
// verified against the channel control headers; the streamed bytes are verified
// against the advertised size and SHA-256 when the caller reads the body to
// EOF. Body must always be closed.
type Input struct {
	Body        io.ReadCloser
	MediaType   string
	ContentType string
	Size        int64
	SHA256      string
}

// Open resolves one Host input reference through the channel. It performs one
// GET with the contract headers and verifies status, media type, Content-Length
// and the advertised digest. The returned body verifies the streamed bytes'
// size and SHA-256 at EOF and fails closed with ARTIFACT_TRANSFER_FAILED on any
// mismatch. Reference/token/path/byte values never appear in errors.
func (c *Channel) Open(ctx context.Context, reference, invocationID, slot string) (*Input, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fault(FaultTransferFailed)
	}
	if !ValidReferenceToken(reference) {
		return nil, fault(FaultReferenceInvalid)
	}
	if !validHeaderToken(invocationID) || !artifactSlotPattern.MatchString(slot) {
		return nil, fault(FaultTransferFailed)
	}

	requestURL := c.baseURL + "/v1/artifacts/" + url.PathEscape(reference)
	requestContext, cancel := context.WithTimeout(ctx, c.requestTimeout)
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, requestURL, nil)
	if err != nil {
		cancel()
		return nil, fault(FaultTransferFailed)
	}
	request.Header.Set("Authorization", "Bearer "+c.processToken)
	request.Header.Set("X-CA-Invocation", invocationID)
	request.Header.Set("X-CA-Slot", slot)
	request.Header.Set("Accept", "*/*")

	response, err := c.client.Do(request)
	if err != nil {
		cancel()
		return nil, classifyRequestError(ctx, requestContext, err)
	}
	if response.StatusCode != http.StatusOK {
		drainAndClose(response.Body, maximumErrorBodyBytes)
		cancel()
		return nil, mapStatus(response.StatusCode)
	}

	contentType := strings.TrimSpace(response.Header.Get("Content-Type"))
	mediaType, _, mediaTypeErr := mime.ParseMediaType(contentType)
	if mediaTypeErr != nil || mediaType == "" {
		drainAndClose(response.Body, maximumErrorBodyBytes)
		cancel()
		return nil, fault(FaultTransferFailed)
	}
	size := response.ContentLength
	if headerLength := strings.TrimSpace(response.Header.Get("Content-Length")); headerLength != "" {
		declared, parseErr := strconv.ParseInt(headerLength, 10, 64)
		if parseErr != nil || declared != size {
			drainAndClose(response.Body, maximumErrorBodyBytes)
			cancel()
			return nil, fault(FaultTransferFailed)
		}
	}
	if size < 0 || size > c.maxArtifactBytes {
		drainAndClose(response.Body, maximumErrorBodyBytes)
		cancel()
		return nil, fault(FaultTransferFailed)
	}
	digest := strings.ToLower(strings.TrimSpace(response.Header.Get("X-CA-Artifact-Sha256")))
	if !sha256Pattern.MatchString(digest) {
		drainAndClose(response.Body, maximumErrorBodyBytes)
		cancel()
		return nil, fault(FaultTransferFailed)
	}

	body := &verifyingBody{
		ctx:            ctx,
		body:           response.Body,
		cancel:         cancel,
		expectedSize:   size,
		expectedSHA256: digest,
		hasher:         sha256.New(),
	}
	return &Input{
		Body:        body,
		MediaType:   strings.ToLower(mediaType),
		ContentType: contentType,
		Size:        size,
		SHA256:      digest,
	}, nil
}

// Sink is one invocation-scoped output artifact sink. It stages each declared
// output part through one POST and records the staged reference per declared
// slot. Nothing is published; the Host publishes only an accepted result.
type Sink struct {
	channel      *Channel
	invocationID string
	quota        *Quota

	mutex sync.Mutex
	refs  map[string]string
}

// Quota tracks the per-invocation aggregate byte and attachment-count bounds
// across input resolution and output staging. A nil Quota disables local quota
// accounting (the channel bounds still apply).
type Quota struct {
	mutex    sync.Mutex
	maxBytes int64
	maxCount int
	bytes    int64
	count    int
}

// NewQuota builds one invocation quota from effective channel limits.
func NewQuota(maxBytes int64, maxCount int) *Quota {
	return &Quota{maxBytes: maxBytes, maxCount: maxCount}
}

// Reserve accounts one artifact of the given size against the quota.
func (q *Quota) Reserve(size int64) error {
	if q == nil {
		return nil
	}
	if size < 0 {
		return fault(FaultTransferFailed)
	}
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.maxCount > 0 && q.count+1 > q.maxCount {
		return fault(FaultTransferFailed)
	}
	if q.maxBytes > 0 && (size > q.maxBytes || q.bytes > q.maxBytes-size) {
		return fault(FaultTransferFailed)
	}
	q.count++
	q.bytes += size
	return nil
}

// Release returns a reservation that did not become a staged artifact.
func (q *Quota) Release(size int64) {
	if q == nil {
		return
	}
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.count > 0 {
		q.count--
	}
	if size > 0 && size <= q.bytes {
		q.bytes -= size
	}
}

// NewSink builds one invocation-scoped sink. quota may be nil.
func (c *Channel) NewSink(invocationID string, quota *Quota) *Sink {
	return &Sink{
		channel:      c,
		invocationID: invocationID,
		quota:        quota,
		refs:         make(map[string]string),
	}
}

// Store exists only to satisfy payload.AttachmentSink. The Provider always
// supplies the slot-aware path through StoreSlot; a slotless upload fails
// closed because a Host reference is location-bound.
func (s *Sink) Store(context.Context, string, string, io.Reader, int64) (payload.Attachment, error) {
	return payload.Attachment{}, fault(FaultTransferFailed)
}

// StoreSlot stages one response part into the Host store and returns its
// non-sensitive metadata. The staged reference is recorded for the declared
// slot so the Provider can project it into the result only after the whole
// invocation succeeds.
func (s *Sink) StoreSlot(
	ctx context.Context,
	slot string,
	contentID string,
	contentType string,
	source io.Reader,
	maximum int64,
) (payload.Attachment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return payload.Attachment{}, err
	}
	if s == nil || s.channel == nil || source == nil {
		return payload.Attachment{}, fault(FaultTransferFailed)
	}
	if !artifactSlotPattern.MatchString(slot) {
		return payload.Attachment{}, fault(FaultTransferFailed)
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || mediaType == "" {
		return payload.Attachment{}, fault(FaultTransferFailed)
	}
	if maximum < 0 {
		return payload.Attachment{}, fault(FaultTransferFailed)
	}
	limit := s.channel.maxArtifactBytes
	if maximum < limit {
		limit = maximum
	}
	data, err := readBounded(ctx, source, limit)
	if err != nil {
		return payload.Attachment{}, err
	}
	size := int64(len(data))
	digest := sha256.Sum256(data)
	digestText := hex.EncodeToString(digest[:])
	if err := s.quota.Reserve(size); err != nil {
		return payload.Attachment{}, err
	}
	release := func() { s.quota.Release(size) }

	s.mutex.Lock()
	if _, duplicate := s.refs[slot]; duplicate {
		s.mutex.Unlock()
		release()
		return payload.Attachment{}, fault(FaultTransferFailed)
	}
	s.mutex.Unlock()

	requestContext, cancel := context.WithTimeout(ctx, s.channel.requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, s.channel.baseURL+"/v1/artifacts", bytes.NewReader(data))
	if err != nil {
		release()
		return payload.Attachment{}, fault(FaultTransferFailed)
	}
	request.ContentLength = size
	request.Header.Set("Authorization", "Bearer "+s.channel.processToken)
	request.Header.Set("X-CA-Invocation", s.invocationID)
	request.Header.Set("X-CA-Slot", slot)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-CA-Artifact-Sha256", digestText)
	request.Header.Set("Accept", "application/json")

	response, err := s.channel.client.Do(request)
	if err != nil {
		release()
		return payload.Attachment{}, classifyRequestError(ctx, requestContext, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		drainAndClose(response.Body, maximumErrorBodyBytes)
		release()
		return payload.Attachment{}, mapStatus(response.StatusCode)
	}
	raw, err := readBounded(ctx, response.Body, maximumResponseBodyBytes)
	if err != nil {
		release()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return payload.Attachment{}, ctxErr
		}
		return payload.Attachment{}, fault(FaultTransferFailed)
	}
	var staged struct {
		ArtifactRef string `json:"artifactRef"`
		Size        int64  `json:"size"`
		SHA256      string `json:"sha256"`
	}
	if err := json.Unmarshal(raw, &staged); err != nil || !ValidReferenceToken(staged.ArtifactRef) ||
		staged.Size != size || staged.SHA256 != digestText {
		release()
		return payload.Attachment{}, fault(FaultTransferFailed)
	}

	s.mutex.Lock()
	s.refs[slot] = staged.ArtifactRef
	s.mutex.Unlock()
	return payload.Attachment{
		ContentID:    contentID,
		RelativePath: staged.ArtifactRef,
		ContentType:  contentType,
		Size:         size,
		SHA256:       digestText,
	}, nil
}

// Refs returns a copy of the staged reference per declared slot.
func (s *Sink) Refs() map[string]string {
	if s == nil {
		return nil
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	refs := make(map[string]string, len(s.refs))
	for slot, reference := range s.refs {
		refs[slot] = reference
	}
	return refs
}

func mapStatus(status int) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound,
		http.StatusConflict, http.StatusGone:
		return fault(FaultReferenceInvalid)
	default:
		return fault(FaultTransferFailed)
	}
}

func classifyRequestError(callerContext, requestContext context.Context, err error) error {
	if callerContext != nil {
		if ctxErr := callerContext.Err(); ctxErr != nil {
			return ctxErr
		}
	}
	if requestContext != nil && errors.Is(requestContext.Err(), context.DeadlineExceeded) {
		return fault(FaultTransferFailed)
	}
	return fault(FaultTransferFailed)
}

func validHeaderToken(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func drainAndClose(body io.ReadCloser, maximum int64) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maximum))
	_ = body.Close()
}

func readBounded(ctx context.Context, source io.Reader, maximum int64) ([]byte, error) {
	if source == nil || maximum < 0 {
		return nil, fault(FaultTransferFailed)
	}
	limited := io.LimitReader(&contextReader{ctx: ctx, reader: source}, maximum+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fault(FaultTransferFailed)
	}
	if int64(len(data)) > maximum {
		return nil, fault(FaultTransferFailed)
	}
	return data, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if r.ctx != nil {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
	}
	return r.reader.Read(buffer)
}

// verifyingBody streams the stored artifact bytes and verifies the advertised
// size and SHA-256 before reporting EOF.
type verifyingBody struct {
	ctx            context.Context
	body           io.ReadCloser
	cancel         context.CancelFunc
	expectedSize   int64
	expectedSHA256 string
	hasher         hash.Hash
	read           int64
	finished       bool
}

func (b *verifyingBody) Read(buffer []byte) (int, error) {
	if b.ctx != nil {
		if err := b.ctx.Err(); err != nil {
			return 0, err
		}
	}
	if b.finished {
		return 0, io.EOF
	}
	n, err := b.body.Read(buffer)
	if n > 0 {
		b.read += int64(n)
		if b.read > b.expectedSize {
			return n, fault(FaultTransferFailed)
		}
		_, _ = b.hasher.Write(buffer[:n])
	}
	if err != nil && !errors.Is(err, io.EOF) {
		// A transport read failure or a truncated stream is an incomplete
		// transfer; the caller cancellation/deadline keeps its own error.
		if b.ctx != nil {
			if ctxErr := b.ctx.Err(); ctxErr != nil {
				return n, ctxErr
			}
		}
		return n, fault(FaultTransferFailed)
	}
	if errors.Is(err, io.EOF) {
		b.finished = true
		if b.read != b.expectedSize {
			return n, fault(FaultTransferFailed)
		}
		if hex.EncodeToString(b.hasher.Sum(nil)) != b.expectedSHA256 {
			return n, fault(FaultTransferFailed)
		}
	}
	return n, err
}

func (b *verifyingBody) Close() error {
	if b.cancel != nil {
		b.cancel()
	}
	if b.body == nil {
		return nil
	}
	return b.body.Close()
}

var _ io.ReadCloser = (*verifyingBody)(nil)
var _ payload.AttachmentSink = (*Sink)(nil)
var _ payload.SlotAttachmentSink = (*Sink)(nil)

// FormatFault is a diagnostics helper that returns only the stable code.
func FormatFault(err error) string {
	if code, ok := FaultCode(err); ok {
		return code
	}
	return "UNKNOWN"
}

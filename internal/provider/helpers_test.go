package provider

// Test harness for the resident Provider process protocol.
//
// The harness starts Serve in a goroutine behind an io.Pipe for stdin and a
// synchronized line sink for stdout. Every emitted stdout line is decoded as a
// strict protocol frame by a dedicated reader goroutine and dispatched by
// requestId, so tests can await correlated responses regardless of the order in
// which concurrent invocations complete.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testProviderVersion = "1.0.0"
	testCatalogVersion  = "2026.11.17"

	// awaitTimeout bounds every response wait so a buggy server fails a test
	// instead of hanging the suite.
	awaitTimeout = 5 * time.Second
	// serveTimeout bounds shutdown waits for the same reason.
	serveTimeout = 5 * time.Second
)

var (
	testBuildDigest   = "sha256:" + strings.Repeat("0a", 32)
	testCatalogDigest = "sha256:" + strings.Repeat("1b", 32)
)

func defaultInfo() ProviderInfo {
	return ProviderInfo{
		ProviderID:      ProviderID,
		ProviderVersion: testProviderVersion,
		BuildDigest:     testBuildDigest,
	}
}

// ---------------------------------------------------------------------------
// Fake Runtime
// ---------------------------------------------------------------------------

type fakeInvocation struct {
	OperationID string
	Arguments   map[string]any
}

// fakeRuntime is a deterministic, concurrency-safe Runtime used in place of
// the real OpenAMSM backend. It counts calls/closes and captures arguments.
type fakeRuntime struct {
	catalogVersion string
	catalogDigest  string
	bindingIDs     []string
	bindings       map[string]Binding
	invokeFn       func(ctx context.Context, operationID string, arguments map[string]any) (Result, error)

	calls  atomic.Int64
	closes atomic.Int64

	mu       sync.Mutex
	captured []fakeInvocation
	closed   bool
}

func int64Ptr(value int64) *int64 { return &value }

func allBindingIDs() []string {
	return []string{
		"openamsm.smf.mas.sc.query",
		"openamsm.smf.mas.pa.query",
		"openamsm.smf.mas.upn.query",
		"openamsm.smf.mas.ssm.query",
		"openamsm.smf.mas.iam.query",
		"openamsm.smf.mas.ups.query",
		"openamsm.smf.mas.ca.query",
		"openamsm.smf.mas.saa.query",
		"openamsm.amf.ue-info.list",
		"openamsm.amf.gnb-info.list",
		"openamsm.smf.pdu-info.list",
	}
}

func smfQueryArgumentSet() []Argument {
	return []Argument{
		{Name: "supi", Type: "string", Required: true, Pattern: regexp.MustCompile(`^imsi-[0-9]{15}$`)},
		{Name: "pduSessionId", Type: "integer", Required: true, Minimum: int64Ptr(0), Maximum: int64Ptr(255)},
	}
}

// listArgumentSet is the documented v2 list argument profile: both arguments
// optional, with trusted catalog defaults applied by the Provider.
func listArgumentSet() []Argument {
	return []Argument{
		{Name: "page", Type: "integer", Required: false, Minimum: int64Ptr(0), Default: int64(0)},
		{Name: "pageSize", Type: "integer", Required: false, Minimum: int64Ptr(1), Maximum: int64Ptr(100), Default: int64(100)},
	}
}

func testBinding(id string) Binding {
	if strings.HasSuffix(id, ".list") {
		sourceID := "openamsm-amf-init-c"
		sourceOperationID := "listAmfUEInfo"
		switch id {
		case "openamsm.amf.gnb-info.list":
			sourceOperationID = "listAmfGnBInfo"
		case "openamsm.smf.pdu-info.list":
			sourceID = "openamsm-smf-metrics"
			sourceOperationID = "queryPduInfo"
		}
		return Binding{
			BackendOperationID: id,
			Effect:             "READ",
			HandlerProfile:     "openamsmMetricsList",
			SourceID:           sourceID,
			SourceOperationID:  sourceOperationID,
			RequestMediaType:   "application/json",
			ResponseMediaTypes: []string{"application/json"},
			Arguments:          listArgumentSet(),
			Available:          true,
		}
	}
	return Binding{
		BackendOperationID: id,
		Effect:             "READ",
		HandlerProfile:     "openamsmSMFContextQuery",
		SourceID:           "openamsm-smf-context",
		SourceOperationID:  "queryContext",
		RequestMediaType:   "application/json",
		ResponseMediaTypes: []string{"application/json", "application/problem+json"},
		Arguments:          smfQueryArgumentSet(),
		Available:          true,
	}
}

// newFakeCatalog returns a fake Runtime exposing all 11 test bindings.
func newFakeCatalog() *fakeRuntime {
	ids := allBindingIDs()
	bindings := make(map[string]Binding, len(ids))
	for _, id := range ids {
		bindings[id] = testBinding(id)
	}
	return &fakeRuntime{
		catalogVersion: testCatalogVersion,
		catalogDigest:  testCatalogDigest,
		bindingIDs:     ids,
		bindings:       bindings,
	}
}

func (r *fakeRuntime) CatalogVersion() string { return r.catalogVersion }

func (r *fakeRuntime) CatalogDigest() string { return r.catalogDigest }

func (r *fakeRuntime) BindingIDs() []string {
	return append([]string(nil), r.bindingIDs...)
}

func (r *fakeRuntime) Binding(id string) (Binding, bool) {
	binding, ok := r.bindings[id]
	return binding, ok
}

func (r *fakeRuntime) Invoke(ctx context.Context, operationID string, arguments map[string]any) (Result, error) {
	r.calls.Add(1)
	r.mu.Lock()
	r.captured = append(r.captured, fakeInvocation{OperationID: operationID, Arguments: cloneArguments(arguments)})
	r.mu.Unlock()
	if r.invokeFn != nil {
		return r.invokeFn(ctx, operationID, arguments)
	}
	return Result{HTTPStatus: 200, Response: map[string]any{"ok": true, "operationId": operationID}}, nil
}

func (r *fakeRuntime) Close() {
	r.closes.Add(1)
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
}

func (r *fakeRuntime) callCount() int64 { return r.calls.Load() }

func (r *fakeRuntime) closeCount() int64 { return r.closes.Load() }

func (r *fakeRuntime) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

func (r *fakeRuntime) capturedInvocations() []fakeInvocation {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]fakeInvocation, len(r.captured))
	copy(result, r.captured)
	return result
}

func (r *fakeRuntime) onlyInvocation(t *testing.T) fakeInvocation {
	t.Helper()
	captured := r.capturedInvocations()
	if len(captured) != 1 {
		t.Fatalf("fake runtime saw %d invocations, want exactly 1", len(captured))
	}
	return captured[0]
}

func cloneArguments(arguments map[string]any) map[string]any {
	if arguments == nil {
		return nil
	}
	cloned := make(map[string]any, len(arguments))
	for key, value := range arguments {
		cloned[key] = value
	}
	return cloned
}

// runtimeFactory records the protocol versions it was asked to build and the
// runtimes it returned.
// ProtocolV2 and legacyProtocolVersion are test identifiers. ProtocolV2 mirrors
// the sole accepted production identity; legacyProtocolVersion is the removed
// v1 identity, kept only to prove it is rejected.
const (
	ProtocolV2            = ProtocolVersion
	legacyProtocolVersion = "ca-client-tools.provider/v1"
)

type runtimeFactory struct {
	mu       sync.Mutex
	calls    int
	runtimes []Runtime

	// build is called for every initialize attempt. When nil a fresh
	// newFakeCatalog is returned.
	build func() (Runtime, error)
	// failErr, when set, is returned instead of a runtime.
	failErr error
}

func (f *runtimeFactory) newRuntime() (Runtime, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.failErr != nil {
		return nil, f.failErr
	}
	var runtime Runtime
	if f.build != nil {
		built, err := f.build()
		if err != nil {
			return nil, err
		}
		runtime = built
	} else {
		runtime = newFakeCatalog()
	}
	f.mu.Lock()
	f.runtimes = append(f.runtimes, runtime)
	f.mu.Unlock()
	return runtime, nil
}

func (f *runtimeFactory) initializeCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *runtimeFactory) recordedRuntimes() []Runtime {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Runtime(nil), f.runtimes...)
}

func fixedRuntimeFactory(fake *fakeRuntime) NewRuntimeFunc {
	return func() (Runtime, error) { return fake, nil }
}

func buildRuntimeFactory(build func() (Runtime, error)) NewRuntimeFunc {
	factory := &runtimeFactory{build: build}
	return factory.newRuntime
}

// ---------------------------------------------------------------------------
// stdout line sink and wire frame decoding
// ---------------------------------------------------------------------------

// lineSink is a concurrency-safe io.Writer that records the exact stdout bytes
// and emits one channel message per LF-terminated protocol line.
type lineSink struct {
	mu      sync.Mutex
	raw     bytes.Buffer
	pending []byte
	lines   chan string
	closed  bool
}

func newLineSink() *lineSink {
	return &lineSink{lines: make(chan string, 4096)}
}

func (s *lineSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, io.ErrClosedPipe
	}
	s.raw.Write(p)
	s.pending = append(s.pending, p...)
	for {
		index := bytes.IndexByte(s.pending, '\n')
		if index < 0 {
			break
		}
		line := string(s.pending[:index])
		s.pending = append([]byte(nil), s.pending[index+1:]...)
		s.lines <- line
	}
	return len(p), nil
}

func (s *lineSink) rawBytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.raw.Bytes()...)
}

func (s *lineSink) pendingBytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.pending...)
}

func (s *lineSink) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.lines)
	}
}

// wireFrame is one decoded Provider response envelope.
type wireFrame struct {
	line            string
	raw             map[string]any
	protocolVersion string
	frameType       string
	requestID       string
	ok              bool
	hasResult       bool
	hasError        bool
	result          any
	errorCode       string
	errorMessage    string
	errorDetail     map[string]any
}

// parseWireFrame strictly decodes exactly one response frame line and validates
// the common response envelope.
func parseWireFrame(line string) (*wireFrame, error) {
	if strings.TrimSpace(line) == "" {
		return nil, errors.New("blank stdout line")
	}
	decoder := json.NewDecoder(strings.NewReader(line))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("stdout line is not JSON: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("stdout line contains trailing JSON content")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("stdout line is not a JSON object")
	}
	allowed := map[string]struct{}{
		"protocolVersion": {}, "type": {}, "requestId": {}, "ok": {},
		"result": {}, "error": {},
	}
	for key := range object {
		if _, known := allowed[key]; !known {
			return nil, fmt.Errorf("unknown response envelope field %q", key)
		}
	}
	frame := &wireFrame{line: line, raw: object}
	var typeOK bool
	frame.protocolVersion, typeOK = object["protocolVersion"].(string)
	if !typeOK {
		return nil, errors.New("response protocolVersion is missing or not a string")
	}
	frame.frameType, typeOK = object["type"].(string)
	if !typeOK {
		return nil, errors.New("response type is missing or not a string")
	}
	frame.requestID, typeOK = object["requestId"].(string)
	if !typeOK {
		return nil, errors.New("response requestId is missing or not a string")
	}
	frame.ok, typeOK = object["ok"].(bool)
	if !typeOK {
		return nil, errors.New("response ok is missing or not a boolean")
	}
	if result, present := object["result"]; present {
		frame.hasResult = true
		frame.result = result
	}
	if errorObject, present := object["error"]; present {
		frame.hasError = true
		errorMap, ok := errorObject.(map[string]any)
		if !ok {
			return nil, errors.New("response error is not an object")
		}
		for key := range errorMap {
			if key != "code" && key != "message" && key != "detail" {
				return nil, fmt.Errorf("unknown response error field %q", key)
			}
		}
		code, ok := errorMap["code"].(string)
		if !ok || code == "" {
			return nil, errors.New("response error code is missing or not a string")
		}
		message, ok := errorMap["message"].(string)
		if !ok || message == "" {
			return nil, errors.New("response error message is missing or not a string")
		}
		frame.errorCode = code
		frame.errorMessage = message
		if detail, present := errorMap["detail"]; present && detail != nil {
			detailMap, ok := detail.(map[string]any)
			if !ok {
				return nil, errors.New("response error detail is not an object")
			}
			frame.errorDetail = detailMap
		}
	}
	if frame.hasResult == frame.hasError {
		return nil, errors.New("response must have exactly one of result or error")
	}
	return frame, nil
}

func (f *wireFrame) requireEnvelope(t *testing.T, protocolVersion, requestID string) {
	t.Helper()
	if f == nil {
		t.Fatal("missing response frame")
	}
	if f.frameType != "response" {
		t.Fatalf("response type = %q, want %q (frame: %s)", f.frameType, "response", f.line)
	}
	if f.protocolVersion != protocolVersion {
		t.Fatalf("response protocolVersion = %q, want %q (frame: %s)", f.protocolVersion, protocolVersion, f.line)
	}
	if f.requestID != requestID {
		t.Fatalf("response requestId = %q, want %q (frame: %s)", f.requestID, requestID, f.line)
	}
	if f.hasResult == f.hasError {
		t.Fatalf("response does not have exactly one of result/error (frame: %s)", f.line)
	}
}

// expectOK validates a successful response envelope and returns its result
// object.
func (f *wireFrame) expectOK(t *testing.T, protocolVersion, requestID string) map[string]any {
	t.Helper()
	f.requireEnvelope(t, protocolVersion, requestID)
	if !f.ok {
		t.Fatalf("response ok = false, want true (frame: %s)", f.line)
	}
	if !f.hasResult {
		t.Fatalf("successful response is missing result (frame: %s)", f.line)
	}
	result, ok := f.result.(map[string]any)
	if !ok {
		t.Fatalf("response result is not a JSON object (frame: %s)", f.line)
	}
	return result
}

// expectError validates a fault response envelope and asserts its stable code.
// It returns the optional safe detail object.
func (f *wireFrame) expectError(t *testing.T, protocolVersion, requestID, wantCode string) map[string]any {
	t.Helper()
	f.requireEnvelope(t, protocolVersion, requestID)
	if f.ok {
		t.Fatalf("response ok = true, want false with code %s (frame: %s)", wantCode, f.line)
	}
	if !f.hasError {
		t.Fatalf("fault response is missing error object (frame: %s)", f.line)
	}
	if f.errorCode != wantCode {
		t.Fatalf("response error code = %q, want %q (frame: %s)", f.errorCode, wantCode, f.line)
	}
	if f.errorMessage == "" {
		t.Fatalf("response error message is empty (frame: %s)", f.line)
	}
	return f.errorDetail
}

func requireExactKeys(t *testing.T, what string, object map[string]any, keys ...string) {
	t.Helper()
	if len(object) != len(keys) {
		t.Fatalf("%s has %d members %v, want exactly %v", what, len(object), sortedKeys(object), keys)
	}
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			t.Fatalf("%s is missing member %q (has %v)", what, key, sortedKeys(object))
		}
	}
}

func sortedKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func requireNumber(t *testing.T, what string, value any) int64 {
	t.Helper()
	number, ok := value.(json.Number)
	if !ok {
		t.Fatalf("%s = %#v, want a JSON number", what, value)
	}
	parsed, err := number.Int64()
	if err != nil {
		t.Fatalf("%s = %q, want an integer", what, number.String())
	}
	return parsed
}

func requireString(t *testing.T, what string, value any) string {
	t.Helper()
	text, ok := value.(string)
	if !ok {
		t.Fatalf("%s = %#v, want a string", what, value)
	}
	return text
}

// requireStringSlice asserts a non-nil JSON array of strings.
func requireStringSlice(t *testing.T, what string, value any) []string {
	t.Helper()
	rawItems, ok := value.([]any)
	if !ok {
		t.Fatalf("%s = %#v, want a JSON array", what, value)
	}
	items := make([]string, len(rawItems))
	for index, item := range rawItems {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("%s[%d] = %#v, want a string", what, index, item)
		}
		items[index] = text
	}
	return items
}

// requireObjectSlice asserts a non-nil JSON array of objects.
func requireObjectSlice(t *testing.T, what string, value any) []map[string]any {
	t.Helper()
	rawItems, ok := value.([]any)
	if !ok {
		t.Fatalf("%s = %#v, want a JSON array", what, value)
	}
	items := make([]map[string]any, len(rawItems))
	for index, item := range rawItems {
		object, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("%s[%d] = %#v, want an object", what, index, item)
		}
		items[index] = object
	}
	return items
}

func requireEqualStrings(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Serve harness
// ---------------------------------------------------------------------------

type harness struct {
	t          *testing.T
	ctx        context.Context
	cancelFunc context.CancelFunc
	inW        *io.PipeWriter
	sink       *lineSink

	serveDone    chan struct{}
	serveErrOnce sync.Once
	serveErrVal  error

	readerDone chan struct{}

	mu         sync.Mutex
	order      []*wireFrame
	arrived    map[string][]*wireFrame
	waiters    map[string]chan *wireFrame
	decodeErrs []error
	anyIndex   int
	newFrame   chan struct{}
	consumed   map[*wireFrame]struct{}

	closeOnce sync.Once
}

func newHarness(t *testing.T, factory NewRuntimeFunc) *harness {
	t.Helper()
	return newHarnessWithInfo(t, defaultInfo(), factory)
}

func newHarnessWithInfo(t *testing.T, info ProviderInfo, factory NewRuntimeFunc) *harness {
	t.Helper()
	return newHarnessWithOptions(t, Options{
		Info:       info,
		NewRuntime: factory,
	})
}

func newHarnessWithOptions(t *testing.T, options Options) *harness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	inReader, inWriter := io.Pipe()
	sink := newLineSink()
	h := &harness{
		t:          t,
		ctx:        ctx,
		cancelFunc: cancel,
		inW:        inWriter,
		sink:       sink,
		serveDone:  make(chan struct{}),
		readerDone: make(chan struct{}),
		arrived:    make(map[string][]*wireFrame),
		waiters:    make(map[string]chan *wireFrame),
		newFrame:   make(chan struct{}, 1),
		consumed:   make(map[*wireFrame]struct{}),
	}
	options.In = inReader
	options.Out = sink
	if options.Err == nil {
		options.Err = io.Discard
	}
	go h.readLoop()
	go func() {
		err := Serve(ctx, options)
		h.serveErrOnce.Do(func() { h.serveErrVal = err })
		close(h.serveDone)
	}()
	t.Cleanup(h.cleanup)
	return h
}

func (h *harness) readLoop() {
	defer close(h.readerDone)
	for line := range h.sink.lines {
		frame, err := parseWireFrame(line)
		h.mu.Lock()
		if err != nil {
			h.decodeErrs = append(h.decodeErrs, fmt.Errorf("stdout line %q: %w", line, err))
			h.mu.Unlock()
			continue
		}
		h.order = append(h.order, frame)
		select {
		case h.newFrame <- struct{}{}:
		default:
		}
		if waiter, ok := h.waiters[frame.requestID]; ok {
			delete(h.waiters, frame.requestID)
			h.mu.Unlock()
			waiter <- frame
			continue
		}
		h.arrived[frame.requestID] = append(h.arrived[frame.requestID], frame)
		h.mu.Unlock()
	}
}

func (h *harness) cleanup() {
	h.cancelFunc()
	_ = h.inW.Close()
	select {
	case <-h.serveDone:
	case <-time.After(serveTimeout):
		h.t.Errorf("Serve did not return after context cancellation and stdin close")
	}
	h.sink.close()
	select {
	case <-h.readerDone:
	case <-time.After(serveTimeout):
		h.t.Errorf("stdout reader did not finish")
	}
}

// await waits for the next response frame correlated to requestID.
func (h *harness) await(t *testing.T, requestID string) *wireFrame {
	t.Helper()
	h.mu.Lock()
	if list := h.arrived[requestID]; len(list) > 0 {
		frame := list[0]
		h.arrived[requestID] = list[1:]
		h.consumed[frame] = struct{}{}
		h.mu.Unlock()
		return frame
	}
	waiter := make(chan *wireFrame, 1)
	h.waiters[requestID] = waiter
	h.mu.Unlock()
	select {
	case frame := <-waiter:
		h.mu.Lock()
		h.consumed[frame] = struct{}{}
		h.mu.Unlock()
		return frame
	case <-time.After(awaitTimeout):
		h.mu.Lock()
		seen := make([]string, 0, len(h.order))
		for _, frame := range h.order {
			seen = append(seen, frame.requestID)
		}
		decodeErrs := append([]error(nil), h.decodeErrs...)
		h.mu.Unlock()
		t.Fatalf("timed out waiting for response to requestId %q; responses seen: %v; decode errors: %v", requestID, seen, decodeErrs)
		return nil
	}
}

// awaitAnyFrame waits for the next stdout frame that has not yet been consumed
// by await/awaitAnyFrame, regardless of its requestId. It is used for
// uncorrelated protocol faults such as PROTOCOL_FRAME_TOO_LARGE.
func (h *harness) awaitAnyFrame(t *testing.T) *wireFrame {
	t.Helper()
	deadline := time.After(awaitTimeout)
	for {
		h.mu.Lock()
		for h.anyIndex < len(h.order) {
			frame := h.order[h.anyIndex]
			h.anyIndex++
			if _, alreadyConsumed := h.consumed[frame]; alreadyConsumed {
				continue
			}
			h.consumed[frame] = struct{}{}
			h.mu.Unlock()
			return frame
		}
		h.mu.Unlock()
		select {
		case <-h.newFrame:
		case <-deadline:
			h.mu.Lock()
			seen := make([]string, 0, len(h.order))
			for _, frame := range h.order {
				seen = append(seen, frame.requestID)
			}
			decodeErrs := append([]error(nil), h.decodeErrs...)
			h.mu.Unlock()
			t.Fatalf("timed out waiting for any stdout frame; responses seen: %v; decode errors: %v", seen, decodeErrs)
			return nil
		}
	}
}

func (h *harness) framesFor(requestID string) []*wireFrame {
	h.mu.Lock()
	defer h.mu.Unlock()
	frames := make([]*wireFrame, 0)
	for _, frame := range h.order {
		if frame.requestID == requestID {
			frames = append(frames, frame)
		}
	}
	return frames
}

func (h *harness) allFrames() []*wireFrame {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*wireFrame(nil), h.order...)
}

func (h *harness) serveResult() error {
	<-h.serveDone
	h.serveErrOnce.Do(func() {})
	return h.serveErrVal
}

// waitServe blocks until Serve returns and asserts the error contract.
// A nil wantErr means a normal (nil) return is expected.
func (h *harness) waitServe(t *testing.T, wantErr error) error {
	t.Helper()
	select {
	case <-h.serveDone:
	case <-time.After(serveTimeout):
		t.Fatalf("Serve did not return within %s", serveTimeout)
	}
	err := h.serveErrVal
	if wantErr == nil {
		if err != nil {
			t.Fatalf("Serve returned %v, want nil", err)
		}
	} else if !errors.Is(err, wantErr) {
		t.Fatalf("Serve returned %v, want %v", err, wantErr)
	}
	return err
}

// serveReturned reports whether Serve has already returned.
func (h *harness) serveReturned() bool {
	select {
	case <-h.serveDone:
		return true
	default:
		return false
	}
}

func (h *harness) send(t *testing.T, line string) {
	h.t.Helper()
	h.sendRaw(t, line+"\n")
}

func (h *harness) sendBytes(t *testing.T, raw []byte) {
	t.Helper()
	h.sendRaw(t, string(raw))
}

func (h *harness) sendRaw(t *testing.T, raw string) {
	t.Helper()
	if _, err := io.WriteString(h.inW, raw); err != nil {
		t.Fatalf("write request frame: %v", err)
	}
}

func (h *harness) closeInput(t *testing.T) {
	t.Helper()
	h.closeOnce.Do(func() {
		if err := h.inW.Close(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("close stdin: %v", err)
		}
	})
}

// assertStdoutProtocolOnly proves that every byte on stdout is a strict
// single-line response frame: no banners, no blank lines, no pretty printing.
func (h *harness) assertStdoutProtocolOnly(t *testing.T) {
	t.Helper()
	raw := h.sink.rawBytes()
	if len(raw) == 0 {
		t.Fatal("stdout contains no protocol frames")
	}
	if raw[len(raw)-1] != '\n' {
		t.Fatalf("stdout does not end with a newline: %q", raw)
	}
	if bytes.ContainsRune(raw, '\r') {
		t.Fatalf("stdout contains a carriage return: %q", raw)
	}
	lines := strings.Split(string(raw), "\n")
	if lines[len(lines)-1] != "" {
		t.Fatalf("stdout has trailing data after the final newline: %q", raw)
	}
	for index, line := range lines[:len(lines)-1] {
		if line == "" {
			t.Fatalf("stdout line %d is blank", index+1)
		}
		if _, err := parseWireFrame(line); err != nil {
			t.Fatalf("stdout line %d is not a valid protocol frame: %v", index+1, err)
		}
	}
	if pending := h.sink.pendingBytes(); len(pending) != 0 {
		t.Fatalf("stdout has an unterminated trailing frame: %q", pending)
	}
	h.mu.Lock()
	decodeErrs := append([]error(nil), h.decodeErrs...)
	h.mu.Unlock()
	if len(decodeErrs) != 0 {
		t.Fatalf("stdout contained invalid frames: %v", decodeErrs)
	}
}

// ---------------------------------------------------------------------------
// Request builders
// ---------------------------------------------------------------------------

func wireRequest(protocolVersion, requestID, method string, params map[string]any) string {
	raw, err := json.Marshal(map[string]any{
		"protocolVersion": protocolVersion,
		"type":            "request",
		"requestId":       requestID,
		"method":          method,
		"params":          params,
	})
	if err != nil {
		panic(fmt.Sprintf("marshal request frame: %v", err))
	}
	return string(raw)
}

const (
	defaultMaxFrameBytes   = 1 << 20
	defaultMaxInFlight     = 8
	defaultMaxCallDuration = 5000
)

func initParams(maxFrameBytes, maxInFlight, maxCallDurationMS int64) map[string]any {
	return map[string]any{
		"hostInstanceId":     "host-instance-01",
		"providerInstanceId": ProviderID,
		"limits": map[string]any{
			"maxFrameBytes":     maxFrameBytes,
			"maxInFlight":       maxInFlight,
			"maxCallDurationMs": maxCallDurationMS,
		},
	}
}

func defaultInitParams() map[string]any {
	return initParams(defaultMaxFrameBytes, defaultMaxInFlight, defaultMaxCallDuration)
}

// initialize sends an initialize request with the supplied params and returns
// the raw response frame.
func (h *harness) initializeRequest(t *testing.T, protocolVersion, requestID string, params map[string]any) *wireFrame {
	t.Helper()
	h.send(t, wireRequest(protocolVersion, requestID, "initialize", params))
	return h.await(t, requestID)
}

// mustInitialize sends a successful initialize and returns its result object.
func (h *harness) mustInitialize(t *testing.T, protocolVersion, requestID string) map[string]any {
	t.Helper()
	return h.initializeRequest(t, protocolVersion, requestID, defaultInitParams()).expectOK(t, protocolVersion, requestID)
}

// mustInitializeWithLimits sends a successful initialize with custom limits.
func (h *harness) mustInitializeWithLimits(t *testing.T, protocolVersion, requestID string, maxFrameBytes, maxInFlight, maxCallDurationMS int64) map[string]any {
	t.Helper()
	params := initParams(maxFrameBytes, maxInFlight, maxCallDurationMS)
	return h.initializeRequest(t, protocolVersion, requestID, params).expectOK(t, protocolVersion, requestID)
}

// catalog sends a catalog request.
func (h *harness) catalog(t *testing.T, protocolVersion, requestID string, ids []string) *wireFrame {
	t.Helper()
	rawIDs := make([]any, 0, len(ids))
	for _, id := range ids {
		rawIDs = append(rawIDs, id)
	}
	h.send(t, wireRequest(protocolVersion, requestID, "catalog", map[string]any{
		"backendOperationIds": rawIDs,
	}))
	return h.await(t, requestID)
}

func invokeParams(operationID string, arguments map[string]any, invocationID, producerInvocationID, deadline string) map[string]any {
	return map[string]any{
		"backendOperationId": operationID,
		"arguments":          arguments,
		"invocation": map[string]any{
			"invocationId":         invocationID,
			"producerInvocationId": producerInvocationID,
			"deadline":             deadline,
		},
	}
}

// standardArguments returns valid SMF query arguments.
func standardArguments() map[string]any {
	return map[string]any{
		"supi":         "imsi-001010000000001",
		"pduSessionId": 5,
	}
}

// futureDeadline returns an RFC 3339 UTC deadline far enough in the future to
// avoid flakiness while staying within recorded-time formats.
func futureDeadline() string {
	return time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
}

// pastDeadline returns an RFC 3339 UTC deadline already in the past.
func pastDeadline() string {
	return time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)
}

// invoke sends an invoke request for the standard SMF binding.
func (h *harness) invoke(t *testing.T, protocolVersion, requestID string, arguments map[string]any) *wireFrame {
	t.Helper()
	h.invokeOperation(t, protocolVersion, requestID, "openamsm.smf.mas.sc.query", arguments, futureDeadline())
	return h.await(t, requestID)
}

// invokeWithDeadline sends an invoke request with an explicit deadline.
func (h *harness) invokeWithDeadline(t *testing.T, protocolVersion, requestID string, arguments map[string]any, deadline string) *wireFrame {
	t.Helper()
	h.send(t, wireRequest(protocolVersion, requestID, "invoke", invokeParams(
		"openamsm.smf.mas.sc.query", arguments, "runtime-invocation-"+requestID, "producer-"+requestID, deadline)))
	return h.await(t, requestID)
}

// invokeOperation sends an invoke request for an explicit backend operation ID.
func (h *harness) invokeOperation(t *testing.T, protocolVersion, requestID, operationID string, arguments map[string]any, deadline string) {
	t.Helper()
	h.send(t, wireRequest(protocolVersion, requestID, "invoke", invokeParams(
		operationID, arguments, "runtime-invocation-"+requestID, "producer-"+requestID, deadline)))
}

// cancel sends a cancel request.
func (h *harness) cancel(t *testing.T, protocolVersion, requestID, targetRequestID string) *wireFrame {
	t.Helper()
	h.send(t, wireRequest(protocolVersion, requestID, "cancel", map[string]any{
		"targetRequestId": targetRequestID,
	}))
	return h.await(t, requestID)
}

// expectCancelDisposition sends a cancel for targetRequestID and asserts the
// acknowledged disposition.
func (h *harness) expectCancelDisposition(t *testing.T, protocolVersion, requestID, targetRequestID, wantDisposition string) map[string]any {
	t.Helper()
	frame := h.cancel(t, protocolVersion, requestID, targetRequestID)
	result := frame.expectOK(t, protocolVersion, requestID)
	requireExactKeys(t, "cancel result", result, "disposition")
	disposition := requireString(t, "cancel disposition", result["disposition"])
	if disposition != wantDisposition {
		t.Fatalf("cancel disposition = %q, want %q", disposition, wantDisposition)
	}
	return result
}

// blockingInvoke returns an Invoke implementation that signals when it starts,
// then blocks until the call context is cancelled/expired or the release
// channel is closed. It always honors ctx.
func blockingInvoke(started chan<- string) func(context.Context, string, map[string]any) (Result, error) {
	return func(ctx context.Context, operationID string, arguments map[string]any) (Result, error) {
		select {
		case started <- operationID:
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
		<-ctx.Done()
		return Result{}, ctx.Err()
	}
}

// invokeOperationV2 sends a v2 invoke and returns its successful result object.
func (h *harness) invokeOperationV2(t *testing.T, requestID, operationID string, arguments map[string]any) map[string]any {
	t.Helper()
	h.invokeOperation(t, ProtocolV2, requestID, operationID, arguments, futureDeadline())
	return h.await(t, requestID).expectOK(t, ProtocolV2, requestID)
}

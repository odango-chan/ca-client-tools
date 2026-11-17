package provider

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Options configures one resident Provider process. In/Out carry protocol
// frames; Err carries safe diagnostics only and must never receive credentials,
// remote bodies or subscriber identifiers.
type Options struct {
	In         io.Reader
	Out        io.Writer
	Err        io.Writer
	Info       ProviderInfo
	NewRuntime NewRuntimeFunc
	// Now is injectable for deadline tests; nil means time.Now.
	Now func() time.Time
}

type invocationState struct {
	cancel   context.CancelFunc
	finished bool
}

type server struct {
	options Options
	now     func() time.Time

	outMu sync.Mutex
	// writeErr is the first stdout write failure; it terminates the process.
	writeErr error

	mu              sync.Mutex
	initialized     bool
	protocolVersion string
	limits          Limits
	runtime         Runtime
	seen            map[string]struct{}
	invocations     map[string]*invocationState
	shuttingDown    bool

	maxFrame atomic.Int64
	sem      chan struct{}
	wg       sync.WaitGroup

	rootCtx    context.Context
	rootCancel context.CancelFunc
}

// Serve runs one resident Provider process until stdin reaches EOF after a
// clean drain, or until ctx is cancelled. It returns nil for a normal EOF
// shutdown. A protocol error never terminates the process; only a stdout write
// failure or an unrecoverable read failure does.
func Serve(ctx context.Context, options Options) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if options.In == nil || options.Out == nil {
		return errors.New("provider: missing process streams")
	}
	if options.Err == nil {
		options.Err = io.Discard
	}
	if options.NewRuntime == nil {
		return errors.New("provider: missing runtime factory")
	}
	rootCtx, rootCancel := context.WithCancel(ctx)
	defer rootCancel()

	s := &server{
		options:     options,
		now:         options.Now,
		seen:        make(map[string]struct{}),
		invocations: make(map[string]*invocationState),
		maxFrame:    atomic.Int64{},
		sem:         make(chan struct{}, HardMaxInFlight),
		rootCtx:     rootCtx,
		rootCancel:  rootCancel,
	}
	if s.now == nil {
		s.now = time.Now
	}
	s.maxFrame.Store(DefaultMaxFrameBytes)

	frames := make(chan frameRead)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer close(frames)
		reader := bufio.NewReaderSize(options.In, 64*1024)
		for {
			// Read up to the hard maximum; the negotiated per-process limit is
			// enforced in the main loop because the reader may already be blocked
			// on the next line when initialize updates the negotiated limit.
			raw, err := readFrameLine(reader, HardMaxFrameBytes)
			select {
			case frames <- frameRead{raw: raw, err: err}:
			case <-rootCtx.Done():
				return
			}
			if err != nil && !errors.Is(err, errFrameTooLarge) {
				return
			}
		}
	}()

	for {
		select {
		case <-rootCtx.Done():
			s.beginShutdown()
			s.drain()
			if s.writeError() != nil {
				return s.writeError()
			}
			return rootCtx.Err()
		case frame, ok := <-frames:
			if !ok {
				// Reader stopped after a terminal read error; inspect frames for
				// a final malformed-frame report before draining.
				s.beginShutdown()
				s.drain()
				return s.writeError()
			}
			if frame.err != nil {
				if errors.Is(frame.err, io.EOF) {
					<-readerDone
					s.beginShutdown()
					s.drain()
					return s.writeError()
				}
				if errors.Is(frame.err, errFrameTooLarge) {
					// The line exceeded even the hard maximum; the reader has
					// already drained it, so report and keep frame synchronization.
					s.respondFault("", s.responseProtocolVersion(), fault(CodeProtocolFrameTooLarge))
					continue
				}
				// Unterminated final frame: report, then treat as EOF.
				s.respondFault("", s.responseProtocolVersion(), fault(CodeProtocolFrameInvalid))
				<-readerDone
				s.beginShutdown()
				s.drain()
				return s.writeError()
			}
			if int64(len(frame.raw))+1 > s.maxFrame.Load() {
				requestID, protocolVersion := bestEffortCorrelation(frame.raw)
				if protocolVersion == "" {
					protocolVersion = s.responseProtocolVersion()
				}
				s.respondFault(requestID, protocolVersion, fault(CodeProtocolFrameTooLarge))
				continue
			}
			s.handleFrame(frame.raw)
			if err := s.writeError(); err != nil {
				rootCancel()
				s.beginShutdown()
				s.drain()
				return err
			}
		}
	}
}

type frameRead struct {
	raw []byte
	err error
}

// readFrameLine reads exactly one LF-terminated frame without buffering more
// than the negotiated maximum. An over-limit line is drained so the process
// stays frame-synchronized.
func readFrameLine(reader *bufio.Reader, maximum int64) ([]byte, error) {
	collected := make([]byte, 0, 256)
	for {
		chunk, err := reader.ReadSlice('\n')
		total := int64(len(collected) + len(chunk))
		if total > maximum {
			if err == nil {
				return nil, errFrameTooLarge
			}
			if errors.Is(err, bufio.ErrBufferFull) {
				if err := drainLine(reader); err != nil {
					return nil, err
				}
				return nil, errFrameTooLarge
			}
			return nil, err
		}
		collected = append(collected, chunk...)
		switch {
		case err == nil:
			// Drop the single terminating LF; keep any other bytes exact.
			return collected[:len(collected)-1], nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if len(collected) == 0 {
				return nil, io.EOF
			}
			return nil, errUnexpectedEOF
		default:
			return nil, err
		}
	}
}

func drainLine(reader *bufio.Reader) error {
	for {
		_, err := reader.ReadSlice('\n')
		switch {
		case err == nil:
			return nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			return nil
		default:
			return err
		}
	}
}

func (s *server) responseProtocolVersion() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.protocolVersion != "" {
		return s.protocolVersion
	}
	return ProtocolV1
}

func (s *server) handleFrame(raw []byte) {
	frame, parseFailure := parseRequestFrame(raw)
	if parseFailure != nil {
		requestID, protocolVersion := bestEffortCorrelation(raw)
		if protocolVersion == "" {
			protocolVersion = s.responseProtocolVersion()
		}
		s.respondFault(requestID, protocolVersion, parseFailure)
		return
	}

	s.mu.Lock()
	initialized := s.initialized
	negotiated := s.protocolVersion
	s.mu.Unlock()

	if !initialized {
		if frame.Method != "initialize" {
			s.respondFault(frame.RequestID, responseVersionFor(frame.ProtocolVersion), fault(CodeProviderNotInitialized))
			return
		}
		if !s.markRequestID(frame.RequestID) {
			s.respondFault(frame.RequestID, responseVersionFor(frame.ProtocolVersion), fault(CodeProtocolRequestIDInvalid))
			return
		}
		s.handleInitialize(frame)
		return
	}

	if frame.ProtocolVersion != negotiated {
		s.respondFault(frame.RequestID, negotiated, fault(CodeProtocolVersionUnsupported))
		return
	}
	if !s.markRequestID(frame.RequestID) {
		s.respondFault(frame.RequestID, negotiated, fault(CodeProtocolRequestIDInvalid))
		return
	}

	switch frame.Method {
	case "initialize":
		s.respondFault(frame.RequestID, negotiated, fault(CodeProviderAlreadyInitialized))
	case "catalog":
		s.handleCatalog(frame)
	case "invoke":
		s.handleInvoke(frame)
	case "cancel":
		s.handleCancel(frame)
	default:
		s.respondFault(frame.RequestID, negotiated, fault(CodeProtocolMethodUnsupported))
	}
}

func responseVersionFor(requested string) string {
	if requested == ProtocolV1 || requested == ProtocolV2 {
		return requested
	}
	return ProtocolV1
}

// bestEffortCorrelation extracts a syntactically safe requestId and protocol
// version from an otherwise invalid frame so the Host can correlate the error
// when possible. A malformed frame without them still receives a response with
// an empty requestId; the Host treats that as a protocol failure.
func bestEffortCorrelation(raw []byte) (string, string) {
	object, err := decodeStrictObject(raw)
	if err != nil {
		return "", ""
	}
	requestID, _ := object["requestId"].(string)
	if !validOpaqueID(requestID) {
		requestID = ""
	}
	protocolVersion, _ := object["protocolVersion"].(string)
	if protocolVersion != ProtocolV1 && protocolVersion != ProtocolV2 {
		protocolVersion = ""
	}
	return requestID, protocolVersion
}

func (s *server) markRequestID(requestID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, duplicate := s.seen[requestID]; duplicate {
		return false
	}
	s.seen[requestID] = struct{}{}
	return true
}

func (s *server) handleInitialize(frame *requestFrame) {
	hostID, providerInstanceID, limits, initFailure := parseInitializeParams(frame.Params)
	if initFailure != nil {
		s.respondFault(frame.RequestID, frame.ProtocolVersion, initFailure)
		return
	}
	_ = hostID
	_ = providerInstanceID

	runtime, err := s.options.NewRuntime(frame.ProtocolVersion)
	if err != nil {
		switch {
		case errors.As(err, new(*ConfigError)):
			s.respondFault(frame.RequestID, frame.ProtocolVersion, fault(CodeProviderConfigurationInvalid))
		case errors.As(err, new(*CatalogError)):
			s.respondFault(frame.RequestID, frame.ProtocolVersion, fault(CodeProviderBindingCatalogInvalid))
		default:
			s.respondFault(frame.RequestID, frame.ProtocolVersion, fault(CodeProviderInternal))
		}
		return
	}
	if !validDigest(s.options.Info.BuildDigest) {
		runtime.Close()
		s.respondFault(frame.RequestID, frame.ProtocolVersion, fault(CodeProviderConfigurationInvalid))
		return
	}
	if !validDigest(runtime.CatalogDigest()) {
		runtime.Close()
		s.respondFault(frame.RequestID, frame.ProtocolVersion, fault(CodeProviderBindingCatalogInvalid))
		return
	}

	result := map[string]any{
		"providerId":                  s.options.Info.ProviderID,
		"providerVersion":             s.options.Info.ProviderVersion,
		"buildDigest":                 s.options.Info.BuildDigest,
		"backendBindingCatalogDigest": runtime.CatalogDigest(),
		"limits":                      limits.wire(),
	}
	if frame.ProtocolVersion == ProtocolV2 {
		result["catalogVersion"] = runtime.CatalogVersion()
	}

	s.mu.Lock()
	s.initialized = true
	s.protocolVersion = frame.ProtocolVersion
	s.limits = limits
	s.runtime = runtime
	s.sem = make(chan struct{}, limits.MaxInFlight)
	s.mu.Unlock()
	s.maxFrame.Store(limits.MaxFrameBytes)
	s.respondResult(frame.RequestID, frame.ProtocolVersion, result)
}

func validDigest(value string) bool {
	const prefix = "sha256:"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+64 {
		return false
	}
	_, err := hex.DecodeString(value[len(prefix):])
	return err == nil && strings.ToLower(value) == value
}

func (s *server) handleCatalog(frame *requestFrame) {
	requested, catalogFailure := parseCatalogParams(frame.Params)
	if catalogFailure != nil {
		s.respondFault(frame.RequestID, frame.ProtocolVersion, catalogFailure)
		return
	}
	s.mu.Lock()
	runtime := s.runtime
	negotiated := s.protocolVersion
	s.mu.Unlock()

	supported := make([]string, 0, len(requested))
	unsupported := make([]map[string]any, 0)
	descriptors := make([]map[string]any, 0, len(requested))
	for _, id := range requested {
		binding, ok := runtime.Binding(id)
		if !ok {
			unsupported = append(unsupported, map[string]any{
				"backendOperationId": id,
				"reason":             "NOT_IMPLEMENTED",
			})
			continue
		}
		if !binding.Available {
			reason := binding.UnavailableReason
			if reason == "" {
				reason = "NOT_CONFIGURED"
			}
			unsupported = append(unsupported, map[string]any{
				"backendOperationId": id,
				"reason":             reason,
			})
			continue
		}
		supported = append(supported, id)
		descriptors = append(descriptors, describeBinding(binding))
	}
	result := map[string]any{
		"supported":   supported,
		"unsupported": unsupported,
	}
	if negotiated == ProtocolV2 {
		result["catalogVersion"] = runtime.CatalogVersion()
		result["bindings"] = descriptors
	}
	s.respondResult(frame.RequestID, frame.ProtocolVersion, result)
}

func (s *server) handleInvoke(frame *requestFrame) {
	operationID, rawArguments, invocation, invokeFailure := parseInvokeParams(frame.Params)
	if invokeFailure != nil {
		s.respondFault(frame.RequestID, frame.ProtocolVersion, invokeFailure)
		return
	}
	s.mu.Lock()
	runtime := s.runtime
	limits := s.limits
	s.mu.Unlock()

	binding, ok := runtime.Binding(operationID)
	if !ok {
		s.respondFault(frame.RequestID, frame.ProtocolVersion, fault(CodeBackendOperationUnsupported))
		return
	}
	if !binding.Available {
		reason := binding.UnavailableReason
		if reason == "" {
			reason = "NOT_CONFIGURED"
		}
		s.respondFault(frame.RequestID, frame.ProtocolVersion, faultDetail(CodeBackendOperationUnsupported, "reason", reason))
		return
	}
	arguments, argumentFault := validateArguments(binding, rawArguments)
	if argumentFault != nil {
		s.respondFault(frame.RequestID, frame.ProtocolVersion, argumentFault)
		return
	}

	invocationContext, cancel := context.WithCancel(s.rootCtx)
	state := &invocationState{cancel: cancel}
	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		cancel()
		s.respondFault(frame.RequestID, frame.ProtocolVersion, fault(CodeProviderShuttingDown))
		return
	}
	s.invocations[frame.RequestID] = state
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
		case <-invocationContext.Done():
			s.finish(frame.RequestID, frame.ProtocolVersion, nil, faultFromContext(invocationContext))
			return
		}
		if !s.finishIfActive(frame.RequestID) {
			return
		}

		remaining := invocation.Deadline.Sub(s.now())
		if remaining <= 0 {
			s.completeInvocation(frame.RequestID, frame.ProtocolVersion, nil, fault(CodeCallDeadlineExceeded))
			return
		}
		if limits.MaxCallDuration > 0 && remaining > limits.MaxCallDuration {
			remaining = limits.MaxCallDuration
		}
		callContext, callCancel := context.WithTimeout(invocationContext, remaining)
		defer callCancel()

		result, err := runtime.Invoke(callContext, operationID, arguments)
		if err != nil {
			s.completeInvocation(frame.RequestID, frame.ProtocolVersion, nil, faultFromContext(callContext, err))
			return
		}
		s.completeInvocation(frame.RequestID, frame.ProtocolVersion, &result, nil)
	}()
}

// finishIfActive ensures a queued invocation has not been cancelled or already
// finished before it starts remote work.
func (s *server) finishIfActive(requestID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.invocations[requestID]
	if !ok || state.finished {
		return false
	}
	return true
}

func faultFromContext(ctx context.Context, errs ...error) *Fault {
	if ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fault(CodeCallDeadlineExceeded)
	}
	if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
		return fault(CodeCallCancelled)
	}
	for _, err := range errs {
		if err == nil {
			continue
		}
		var providerFault *Fault
		if errors.As(err, &providerFault) {
			return &Fault{Code: providerFault.Code, Detail: providerFault.Detail}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return fault(CodeCallDeadlineExceeded)
		}
		if errors.Is(err, context.Canceled) {
			return fault(CodeCallCancelled)
		}
	}
	for _, err := range errs {
		if err != nil {
			return fault(CodeProviderInternal)
		}
	}
	return fault(CodeProviderInternal)
}

func (s *server) completeInvocation(requestID, protocolVersion string, result *Result, f *Fault) {
	s.mu.Lock()
	state, ok := s.invocations[requestID]
	if !ok || state.finished {
		s.mu.Unlock()
		return
	}
	state.finished = true
	state.cancel()
	s.mu.Unlock()
	if f != nil {
		s.respondFault(requestID, protocolVersion, f)
		return
	}
	if result == nil {
		s.respondFault(requestID, protocolVersion, fault(CodeProviderInternal))
		return
	}
	wire := map[string]any{"httpStatus": result.HTTPStatus}
	if result.Problem != nil {
		wire["problem"] = result.Problem
	} else {
		wire["response"] = result.Response
	}
	s.respondResult(requestID, protocolVersion, wire)
}

func (s *server) finish(requestID, protocolVersion string, _ *Result, f *Fault) {
	s.completeInvocation(requestID, protocolVersion, nil, f)
}

func (s *server) handleCancel(frame *requestFrame) {
	target, cancelFailure := parseCancelParams(frame.Params)
	if cancelFailure != nil {
		s.respondFault(frame.RequestID, frame.ProtocolVersion, cancelFailure)
		return
	}
	disposition := "NOT_FOUND"
	s.mu.Lock()
	if state, ok := s.invocations[target]; ok {
		if state.finished {
			disposition = "ALREADY_COMPLETED"
		} else {
			state.cancel()
			disposition = "CANCEL_REQUESTED"
		}
	}
	s.mu.Unlock()
	s.respondResult(frame.RequestID, frame.ProtocolVersion, map[string]any{"disposition": disposition})
}

func (s *server) respondResult(requestID, protocolVersion string, result any) {
	s.writeResponse(responseFrame{
		ProtocolVersion: protocolVersion,
		Type:            "response",
		RequestID:       requestID,
		OK:              true,
		Result:          result,
	})
}

func (s *server) respondFault(requestID, protocolVersion string, f *Fault) {
	s.writeResponse(responseFrame{
		ProtocolVersion: protocolVersion,
		Type:            "response",
		RequestID:       requestID,
		OK:              false,
		Error:           errorResponseBody(f),
	})
}

func (s *server) writeResponse(frame responseFrame) {
	encoded, err := marshalResponse(frame)
	if err != nil {
		s.setWriteError(err)
		return
	}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	if s.writeErr != nil {
		return
	}
	encoded = append(encoded, '\n')
	if _, err := s.options.Out.Write(encoded); err != nil {
		s.writeErr = err
		s.rootCancel()
	}
}

func (s *server) setWriteError(err error) {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	if s.writeErr == nil {
		s.writeErr = err
	}
	s.rootCancel()
}

func (s *server) writeError() error {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	return s.writeErr
}

func (s *server) beginShutdown() {
	s.mu.Lock()
	s.shuttingDown = true
	s.mu.Unlock()
}

func (s *server) drain() {
	s.mu.Lock()
	states := make([]*invocationState, 0, len(s.invocations))
	for _, state := range s.invocations {
		states = append(states, state)
	}
	s.mu.Unlock()
	// Normal EOF: the Host drains before closing stdin, so in-flight work is
	// allowed to complete. A cancelled process context instead aborts it.
	if s.rootCtx.Err() != nil {
		for _, state := range states {
			state.cancel()
		}
	}
	s.wg.Wait()
	if s.runtime != nil {
		s.runtime.Close()
	}
}

var _ = sha256.Sum256

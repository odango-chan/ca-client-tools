package executor

import (
	"fmt"
)

// The executor distinguishes four failure classes so a Host can map them to
// stable machine outcomes:
//
//   - InputError: caller argument or Host-staged input problem.
//   - ConfigurationError: the trusted operation descriptor is invalid.
//   - ProtocolError: the remote response or wire framing violated the contract.
//   - RemoteError: a well-formed remote error response with a machine cause.
//   - TransportError: the request did not complete (network, cancellation,
//     timeout, protocol negotiation). Cause wraps the underlying error so
//     errors.Is(err, context.Canceled) and errors.Is(err, context.DeadlineExceeded)
//     keep working.

// InputError reports invalid caller arguments or a Host-staged input problem.
type InputError struct {
	Message string
	Cause   error
}

func (e *InputError) Error() string {
	if e.Cause != nil {
		return "invalid operation input: " + e.Message + ": " + e.Cause.Error()
	}
	return "invalid operation input: " + e.Message
}

func (e *InputError) Unwrap() error { return e.Cause }

// ConfigurationError reports an invalid trusted operation descriptor.
type ConfigurationError struct {
	Message string
	Cause   error
}

func (e *ConfigurationError) Error() string {
	if e.Cause != nil {
		return "invalid operation configuration: " + e.Message + ": " + e.Cause.Error()
	}
	return "invalid operation configuration: " + e.Message
}

func (e *ConfigurationError) Unwrap() error { return e.Cause }

// ProtocolError reports a response or wire contract violation.
type ProtocolError struct {
	Message string
	Cause   error
}

func (e *ProtocolError) Error() string {
	if e.Cause != nil {
		return "SBI protocol violation: " + e.Message + ": " + e.Cause.Error()
	}
	return "SBI protocol violation: " + e.Message
}

func (e *ProtocolError) Unwrap() error { return e.Cause }

// ProblemDetails is the subset of RFC 7807 / 3GPP ProblemDetails the executor
// parses. Title, Detail and Instance are retained for the caller but are never
// included in Error() output; remote text is not logged.
type ProblemDetails struct {
	Type     string
	Title    string
	Status   int
	Detail   string
	Instance string
	Cause    string
}

// RemoteError reports a remote non-2xx response that carried a well-formed
// application/problem+json body whose status matched the HTTP status.
type RemoteError struct {
	HTTPStatus int
	Problem    *ProblemDetails
	Cause      string
}

func (e *RemoteError) Error() string {
	if e.Cause != "" {
		return fmt.Sprintf("remote SBI error: HTTP %d cause %s", e.HTTPStatus, e.Cause)
	}
	return fmt.Sprintf("remote SBI error: HTTP %d", e.HTTPStatus)
}

// TransportError reports that the request did not complete.
type TransportError struct {
	Message string
	Cause   error
}

func (e *TransportError) Error() string {
	if e.Cause != nil {
		return "SBI transport failure: " + e.Message + ": " + e.Cause.Error()
	}
	return "SBI transport failure: " + e.Message
}

func (e *TransportError) Unwrap() error { return e.Cause }

func inputError(cause error, format string, arguments ...any) error {
	return &InputError{Message: fmt.Sprintf(format, arguments...), Cause: cause}
}

func protocolError(cause error, format string, arguments ...any) error {
	return &ProtocolError{Message: fmt.Sprintf(format, arguments...), Cause: cause}
}

func configError(cause error, format string, arguments ...any) error {
	return &ConfigurationError{Message: fmt.Sprintf(format, arguments...), Cause: cause}
}

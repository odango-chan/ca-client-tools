package providerapp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"strconv"
	"strings"

	"github.com/odango-chan/ca-client-tools/internal/artifactchannel"
	"github.com/odango-chan/ca-client-tools/internal/provider"
	"github.com/odango-chan/ca-client-tools/internal/sbi/executor"
)

// artifactSlotsForOperation derives the closed artifactSlots descriptor
// extension from the trusted, catalog-derived operation. It carries only the
// catalog part name, the argument/result JSON Pointer and the declared media
// type: never an origin, path, token or Content-ID.
//
// Contract (ca-host.artifact-store/v1 §2/§3):
//   - input binary body: slot "body" at argument pointer /body;
//   - input multipart part: slot = part name at argument pointer
//     /payloads/<escaped part name>;
//   - output multipart part: slot = declared response part name; the published
//     pointer is the location inside response of the decoded part's
//     RefToBinaryData object, which is the parent of the private content-ID
//     pointer declared by the catalog. The catalog content-ID pointer itself is
//     never published;
//   - output binary body: slot "body" at the empty pointer (whole response).
func artifactSlotsForOperation(operation executor.Operation) *provider.ArtifactSlots {
	slots := &provider.ArtifactSlots{}
	if operation.Body != nil {
		switch operation.Body.Kind {
		case executor.PayloadBinary:
			slots.Inputs = append(slots.Inputs, provider.ArtifactSlot{
				Slot:      "body",
				Pointer:   "/body",
				MediaType: operation.Body.MediaType,
				Required:  true,
			})
		case executor.PayloadMultipart:
			for _, part := range operation.Body.Parts {
				slots.Inputs = append(slots.Inputs, provider.ArtifactSlot{
					Slot:      part.Name,
					Pointer:   "/payloads/" + escapePointerSegment(part.Name),
					MediaType: part.ContentType,
					Required:  part.Required,
				})
			}
		}
	}
	switch {
	case len(operation.ResponseParts) > 0:
		for _, part := range operation.ResponseParts {
			slots.Outputs = append(slots.Outputs, provider.ArtifactSlot{
				Slot:      part.Name,
				Pointer:   publicationPointer(part.JSONPointerField),
				MediaType: part.ContentType,
				Required:  part.Required,
			})
		}
	case len(operation.ResponseMediaTypes) == 1 && !jsonMediaType(operation.ResponseMediaTypes[0]):
		slots.Outputs = append(slots.Outputs, provider.ArtifactSlot{
			Slot:      "body",
			Pointer:   "",
			MediaType: operation.ResponseMediaTypes[0],
			Required:  true,
		})
	}
	if len(slots.Inputs) == 0 && len(slots.Outputs) == 0 {
		return nil
	}
	return slots
}

// publicationPointer derives the result location from the private catalog
// content-ID pointer. The catalog responseParts pointer locates the MIME
// Content-ID string (for example /messages/0/n2MessageContent/contentId); the
// published location is the containing RefToBinaryData object
// (/messages/0/n2MessageContent). A pointer that does not end in a contentId
// segment is already treated as the publication location.
func publicationPointer(contentIDPointer string) string {
	const suffix = "/contentId"
	if strings.HasSuffix(contentIDPointer, suffix) && len(contentIDPointer) > len(suffix) {
		return strings.TrimSuffix(contentIDPointer, suffix)
	}
	return contentIDPointer
}

func jsonMediaType(mediaType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(mediaType))
	return normalized == "application/json" || strings.HasSuffix(normalized, "+json")
}

// validateArtifactArguments enforces the contract §2.4/§5 input validation
// before any remote I/O: a reference value is valid only at a declared artifact
// slot, every required declared input slot holds exactly a closed reference
// object with a grammar-valid token, and no reference value appears at an
// undeclared location or at a non-slot value. A binding that declares no
// artifact slots has an empty declared set, so any reference in its arguments is
// an undeclared-location violation. Reference/token/path values are never
// echoed in the fault.
func validateArtifactArguments(slots *provider.ArtifactSlots, arguments map[string]any) *provider.Fault {
	var inputs []provider.ArtifactSlot
	if slots != nil {
		inputs = slots.Inputs
	}
	declared := make(map[string]struct{}, len(inputs))
	for _, slot := range inputs {
		declared[slot.Pointer] = struct{}{}
		value, found := resolveArgumentPointer(arguments, slot.Pointer)
		if !found || value == nil {
			if slot.Required {
				return &provider.Fault{Code: provider.CodeBackendArgumentsInvalid}
			}
			continue
		}
		if _, ok := artifactchannel.ReferenceValue(value); !ok {
			return &provider.Fault{Code: provider.CodeBackendArgumentsInvalid}
		}
	}
	if referenceAtUndeclaredLocation(arguments, "", declared) {
		return &provider.Fault{Code: provider.CodeBackendArgumentsInvalid}
	}
	return nil
}

func referenceAtUndeclaredLocation(node any, pointer string, declared map[string]struct{}) bool {
	switch typed := node.(type) {
	case map[string]any:
		if _, hasReference := typed["artifactRef"]; hasReference {
			if _, ok := declared[pointer]; !ok {
				return true
			}
			return false
		}
		for key, value := range typed {
			if referenceAtUndeclaredLocation(value, pointer+"/"+escapePointerSegment(key), declared) {
				return true
			}
		}
	case []any:
		for index, value := range typed {
			if referenceAtUndeclaredLocation(value, pointer+"/"+strconv.Itoa(index), declared) {
				return true
			}
		}
	}
	return false
}

// resolveArtifactInputs resolves every declared input reference through the
// Host channel before any remote I/O and returns the verified bytes per slot.
// Any channel failure or digest/size mismatch fails the invocation before the
// upstream sees a request.
func (r *providerRuntime) resolveArtifactInputs(
	ctx context.Context,
	invocationID string,
	slots *provider.ArtifactSlots,
	arguments map[string]any,
	quota *artifactchannel.Quota,
) (map[string][]byte, error) {
	resolved := make(map[string][]byte, len(slots.Inputs))
	for _, slot := range slots.Inputs {
		value, found := resolveArgumentPointer(arguments, slot.Pointer)
		if !found || value == nil {
			if slot.Required {
				return nil, artifactchannel.NewFault(artifactchannel.FaultReferenceInvalid)
			}
			continue
		}
		token, ok := artifactchannel.ReferenceValue(value)
		if !ok {
			return nil, artifactchannel.NewFault(artifactchannel.FaultReferenceInvalid)
		}
		input, err := r.artifact.Open(ctx, token, invocationID, slot.Slot)
		if err != nil {
			return nil, err
		}
		if !sameMediaType(input.MediaType, slot.MediaType) {
			_ = input.Body.Close()
			return nil, artifactchannel.NewFault(artifactchannel.FaultTransferFailed)
		}
		if err := quota.Reserve(input.Size); err != nil {
			_ = input.Body.Close()
			return nil, err
		}
		data, readErr := io.ReadAll(input.Body)
		closeErr := input.Body.Close()
		if readErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if _, ok := artifactchannel.FaultCode(readErr); ok {
				return nil, readErr
			}
			return nil, artifactchannel.NewFault(artifactchannel.FaultTransferFailed)
		}
		if closeErr != nil || int64(len(data)) != input.Size {
			return nil, artifactchannel.NewFault(artifactchannel.FaultTransferFailed)
		}
		resolved[slot.Slot] = data
	}
	return resolved, nil
}

// executorArgumentsForArtifacts rewrites the validated invocation arguments into
// the executor's resolver-name form: the Host reference object at a declared
// input slot is replaced by the slot name the per-invocation resolver serves.
func executorArgumentsForArtifacts(slots *provider.ArtifactSlots, arguments map[string]any) map[string]any {
	rewritten := make(map[string]any, len(arguments))
	for key, value := range arguments {
		rewritten[key] = value
	}
	if !slots.Configured() {
		return rewritten
	}
	payloads := make(map[string]any)
	for _, slot := range slots.Inputs {
		// Only a declared slot that actually carries a reference value is
		// rewritten. An omitted optional part must stay absent so the executor
		// does not try to open a resolver entry that was never resolved.
		value, found := resolveArgumentPointer(arguments, slot.Pointer)
		if !found || value == nil {
			continue
		}
		switch {
		case slot.Pointer == "/body":
			rewritten["body"] = slot.Slot
		case strings.HasPrefix(slot.Pointer, "/payloads/"):
			payloads[slot.Slot] = slot.Slot
		}
	}
	if len(payloads) > 0 {
		rewritten["payloads"] = payloads
	}
	return rewritten
}

// artifactInputResolver serves the already verified input bytes by the declared
// slot name. It implements executor.ArgumentResolver.
type artifactInputResolver struct {
	inputs map[string][]byte
	used   map[string]struct{}
}

func (r *artifactInputResolver) Open(_ context.Context, name string) (io.ReadCloser, int64, error) {
	if r == nil {
		return nil, 0, artifactchannel.NewFault(artifactchannel.FaultTransferFailed)
	}
	data, ok := r.inputs[name]
	if !ok {
		return nil, 0, artifactchannel.NewFault(artifactchannel.FaultTransferFailed)
	}
	if _, duplicate := r.used[name]; duplicate {
		return nil, 0, artifactchannel.NewFault(artifactchannel.FaultTransferFailed)
	}
	if r.used == nil {
		r.used = make(map[string]struct{}, len(r.inputs))
	}
	r.used[name] = struct{}{}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

// projectArtifactResult projects a successfully executed artifact-bearing
// response. A binary response body is exactly the reference object; a
// multipart/related response keeps its JSON root and replaces each declared
// output slot value with its reference object, preserving nested objects and
// array indexes. httpStatus is unchanged. No reference is projected on failure.
func projectArtifactResult(
	executed executor.Result,
	slots *provider.ArtifactSlots,
	sink *artifactchannel.Sink,
) (provider.Result, error) {
	result := provider.Result{HTTPStatus: executed.HTTPStatus}
	if !slots.Configured() || len(slots.Outputs) == 0 {
		return result, nil
	}
	refs := sink.Refs()
	if len(slots.Outputs) == 1 && slots.Outputs[0].Pointer == "" {
		slot := slots.Outputs[0]
		reference := refs[slot.Slot]
		if !artifactchannel.ValidReferenceToken(reference) {
			if slot.Required {
				return provider.Result{}, &provider.Fault{Code: provider.CodeRemoteResponseInvalid}
			}
			return result, nil
		}
		result.Response = artifactchannel.Reference(reference)
		return result, nil
	}
	root, ok := executed.JSON.(map[string]any)
	if !ok {
		return provider.Result{}, &provider.Fault{Code: provider.CodeRemoteResponseInvalid}
	}
	for _, slot := range slots.Outputs {
		reference := refs[slot.Slot]
		if !artifactchannel.ValidReferenceToken(reference) {
			if slot.Required {
				return provider.Result{}, &provider.Fault{Code: provider.CodeRemoteResponseInvalid}
			}
			continue
		}
		if err := setArgumentPointer(root, slot.Pointer, artifactchannel.Reference(reference)); err != nil {
			return provider.Result{}, &provider.Fault{Code: provider.CodeRemoteResponseInvalid}
		}
	}
	result.Response = root
	return result, nil
}

func sameMediaType(left, right string) bool {
	leftType, _, leftErr := mime.ParseMediaType(strings.TrimSpace(left))
	rightType, _, rightErr := mime.ParseMediaType(strings.TrimSpace(right))
	return leftErr == nil && rightErr == nil && strings.EqualFold(leftType, rightType)
}

func escapePointerSegment(segment string) string {
	segment = strings.ReplaceAll(segment, "~", "~0")
	return strings.ReplaceAll(segment, "/", "~1")
}

func pointerSegments(pointer string) ([]string, bool) {
	if pointer == "" {
		return nil, true
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	parts := strings.Split(pointer[1:], "/")
	segments := make([]string, len(parts))
	for index, part := range parts {
		var builder strings.Builder
		for position := 0; position < len(part); position++ {
			if part[position] != '~' {
				builder.WriteByte(part[position])
				continue
			}
			if position+1 >= len(part) {
				return nil, false
			}
			position++
			switch part[position] {
			case '0':
				builder.WriteByte('~')
			case '1':
				builder.WriteByte('/')
			default:
				return nil, false
			}
		}
		segments[index] = builder.String()
	}
	return segments, true
}

func resolveArgumentPointer(root any, pointer string) (any, bool) {
	segments, ok := pointerSegments(pointer)
	if !ok {
		return nil, false
	}
	current := root
	for _, segment := range segments {
		switch node := current.(type) {
		case map[string]any:
			value, found := node[segment]
			if !found {
				return nil, false
			}
			current = value
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			current = node[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func setArgumentPointer(root any, pointer string, value any) error {
	segments, ok := pointerSegments(pointer)
	if !ok || len(segments) == 0 {
		return errors.New("invalid publication pointer")
	}
	current := root
	for _, segment := range segments[:len(segments)-1] {
		switch node := current.(type) {
		case map[string]any:
			next, found := node[segment]
			if !found {
				return errors.New("missing publication pointer parent")
			}
			current = next
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(node) {
				return errors.New("missing publication pointer parent")
			}
			current = node[index]
		default:
			return errors.New("invalid publication pointer parent")
		}
	}
	last := segments[len(segments)-1]
	switch node := current.(type) {
	case map[string]any:
		if _, found := node[last]; !found {
			return errors.New("missing publication pointer member")
		}
		node[last] = value
	case []any:
		index, err := strconv.Atoi(last)
		if err != nil || index < 0 || index >= len(node) {
			return errors.New("missing publication pointer index")
		}
		node[index] = value
	default:
		return errors.New("invalid publication pointer container")
	}
	return nil
}

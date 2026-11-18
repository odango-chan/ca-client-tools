package providerapp

import (
	"errors"
	"strings"

	"github.com/odango-chan/ca-client-tools/internal/catalog"
	"github.com/odango-chan/ca-client-tools/internal/sbi/executor"
	"github.com/odango-chan/ca-client-tools/internal/schemacheck"
)

// responseProjectionSupported reports whether the Provider can project the
// operation's declared response contract. P0 supports successful JSON roots
// (and empty 2xx responses). Binary bodies and multipart/related parts remain
// P1 because they require the published Host Artifact Store contract; the
// executor itself keeps its independent binary/multipart capability for that
// later integration.
func responseProjectionSupported(operation executor.Operation) bool {
	if len(operation.ResponseParts) > 0 {
		return false
	}
	if len(operation.ResponseMediaTypes) == 0 {
		return true
	}
	for _, mediaType := range operation.ResponseMediaTypes {
		normalized := strings.ToLower(strings.TrimSpace(mediaType))
		if normalized == "application/json" || strings.HasSuffix(normalized, "+json") {
			continue
		}
		return false
	}
	return true
}

// buildGenericOperation converts one trusted catalog genericSBI binding into
// the generic executor's fully bound operation. This is the catalog ->
// executor bridge: method, path template, typed parameters, trusted headers,
// payload contract, response media types and schemas all come from trusted
// catalog content, never from a caller.
//
// No production genericSBI binding is shipped in catalog v2: a standardized
// 3GPP operation requires a pinned release-specific OpenAPI source and
// source-verified deployment support first. The bridge is implemented and
// tested so that onboarding such an operation does not require a new client.
func buildGenericOperation(cat *catalog.Catalog, binding catalog.Binding) (executor.Operation, error) {
	if binding.Generic == nil {
		return executor.Operation{}, errors.New("binding has no generic operation")
	}
	if binding.Handler != catalog.HandlerGenericSBI {
		return executor.Operation{}, errors.New("binding handler is not genericSBI")
	}
	spec := binding.Generic
	operation := executor.Operation{
		OperationID:        binding.BackendOperationID,
		Effect:             executor.Effect(binding.Effect),
		ServiceKey:         binding.ServiceKey,
		Method:             spec.Method,
		PathTemplate:       spec.Path,
		ResponseMediaTypes: spec.ResponseMediaTypes,
	}
	if len(operation.ResponseMediaTypes) == 0 {
		operation.ResponseMediaTypes = binding.ResponseMediaTypes
	}
	for _, parameter := range spec.PathParameters {
		operation.PathParameters = append(operation.PathParameters, genericParameter(parameter))
	}
	for _, parameter := range spec.QueryParameters {
		operation.QueryParameters = append(operation.QueryParameters, genericParameter(parameter))
	}
	for _, header := range spec.HeaderBindings {
		operation.HeaderBindings = append(operation.HeaderBindings, executor.HeaderBinding{
			Name:  header.Name,
			Value: header.Value,
		})
	}
	if spec.Body != nil {
		body := &executor.BodySpec{
			Kind:             executor.PayloadKind(spec.Body.Kind),
			MediaType:        spec.Body.MediaType,
			MultipartRoot:    spec.Body.MultipartRoot,
			MaxParts:         spec.Body.MaxParts,
			ContentTypeParam: spec.Body.ContentTypeParam,
		}
		for _, part := range spec.Body.Parts {
			body.Parts = append(body.Parts, executor.MultipartPart{
				Name:             part.Name,
				ContentType:      part.ContentType,
				JSONPointerField: part.JSONPointerField,
				Required:         part.Required,
			})
		}
		if len(spec.Body.RequestSchema) > 0 {
			schema, err := schemacheck.Compile(spec.Body.RequestSchema)
			if err != nil {
				return executor.Operation{}, err
			}
			body.Schema = schema
		}
		operation.Body = body
	}
	responseSchema := spec.ResponseSchema
	if len(responseSchema) == 0 && binding.ResponseSchemaRef != "" {
		resolved, ok := cat.Schemas[binding.ResponseSchemaRef]
		if !ok {
			return executor.Operation{}, errors.New("unknown response schema reference")
		}
		responseSchema = resolved
	}
	if len(responseSchema) > 0 {
		schema, err := schemacheck.Compile(responseSchema)
		if err != nil {
			return executor.Operation{}, err
		}
		operation.ResponseSchema = schema
	}
	for _, part := range spec.ResponseParts {
		operation.ResponseParts = append(operation.ResponseParts, executor.MultipartPart{
			Name:             part.Name,
			ContentType:      part.ContentType,
			JSONPointerField: part.JSONPointerField,
			Required:         part.Required,
		})
	}
	if spec.Idempotency != nil {
		mode := spec.Idempotency.Mode
		keyArgument := ""
		switch mode {
		case "NONE":
			// Explicitly no idempotency key contract.
		case "IDEMPOTENCY_KEY":
			// The catalog names a declared argument; the executor resolves
			// that value through a JSON Pointer into the invocation arguments.
			mode = "REQUIRED"
			keyArgument = "/" + spec.Idempotency.KeyArgument
		}
		operation.Idempotency = &executor.Idempotency{
			Mode:        mode,
			KeyArgument: keyArgument,
		}
	}
	return operation, nil
}

func genericParameter(argument catalog.Argument) executor.Parameter {
	return executor.Parameter{
		Name:      argument.Name,
		Type:      argument.Type,
		Required:  argument.Required,
		Minimum:   argument.Minimum,
		Maximum:   argument.Maximum,
		MinLength: argument.MinLength,
		MaxLength: argument.MaxLength,
		Pattern:   argument.Pattern,
		Default:   argument.Default,
	}
}

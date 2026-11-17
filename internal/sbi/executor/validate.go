package executor

import (
	"fmt"
	"mime"
	"net/textproto"
	"regexp"
	"strings"
)

var (
	requestMethodNames = map[string]struct{}{
		"GET": {}, "POST": {}, "PUT": {}, "PATCH": {}, "DELETE": {},
	}

	// reservedRequestHeaders can never be supplied by an operation binding.
	// The transport itself owns framing, connection and credential headers.
	reservedRequestHeaders = map[string]struct{}{
		"authorization":       {},
		"proxy-authorization": {},
		"host":                {},
		"content-length":      {},
		"content-type":        {},
		"connection":          {},
		"proxy-connection":    {},
		"keep-alive":          {},
		"transfer-encoding":   {},
		"upgrade":             {},
		"te":                  {},
		"trailer":             {},
		"cookie":              {},
		"set-cookie":          {},
	}

	argumentNamePattern = regexp.MustCompile(`^[a-z][A-Za-z0-9_]{0,63}$`)
	operationIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	serviceKeyPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	jsonPointerPattern  = regexp.MustCompile(`^/(?:[^~/]|~[01])*(?:/(?:[^~/]|~[01])*)*$`)
)

// validateOperation rejects any operation descriptor that could let a caller
// influence the destination, method, headers or framing.
func validateOperation(op *Operation) error {
	if op == nil {
		return configError(nil, "operation is nil")
	}
	if !operationIDPattern.MatchString(op.OperationID) {
		return configError(nil, "invalid operation ID %q", op.OperationID)
	}
	switch op.Effect {
	case EffectRead, EffectMutation:
	default:
		return configError(nil, "operation %s: unsupported effect %q", op.OperationID, op.Effect)
	}
	if !serviceKeyPattern.MatchString(op.ServiceKey) {
		return configError(nil, "operation %s: invalid service key %q", op.OperationID, op.ServiceKey)
	}
	method := strings.ToUpper(op.Method)
	if _, ok := requestMethodNames[method]; !ok {
		return configError(nil, "operation %s: unsupported method %q", op.OperationID, op.Method)
	}
	pathParameters, err := validatePathTemplate(op)
	if err != nil {
		return err
	}
	if err := validateParameters(op, pathParameters); err != nil {
		return err
	}
	if err := validateHeaders(op); err != nil {
		return err
	}
	if err := validateBody(op); err != nil {
		return err
	}
	if err := validateResponses(op); err != nil {
		return err
	}
	return validateIdempotency(op)
}

func validatePathTemplate(op *Operation) (map[string]struct{}, error) {
	template := op.PathTemplate
	if template == "" || !strings.HasPrefix(template, "/") || strings.HasPrefix(template, "//") {
		return nil, configError(nil, "operation %s: path template must be an absolute path", op.OperationID)
	}
	if strings.ContainsAny(template, "?#\\%") {
		return nil, configError(nil, "operation %s: path template contains forbidden characters", op.OperationID)
	}
	for _, r := range template {
		if r < 0x20 || r == 0x7f {
			return nil, configError(nil, "operation %s: path template contains control characters", op.OperationID)
		}
	}
	tokens := make(map[string]struct{})
	segments := strings.Split(template, "/")[1:]
	if len(segments) == 0 {
		return nil, configError(nil, "operation %s: path template has no segments", op.OperationID)
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return nil, configError(nil, "operation %s: path template has an empty or relative segment", op.OperationID)
		}
		rest := segment
		for {
			open := strings.IndexByte(rest, '{')
			if open < 0 {
				if strings.ContainsRune(rest, '}') {
					return nil, configError(nil, "operation %s: path template has an unmatched '}'", op.OperationID)
				}
				break
			}
			if strings.ContainsRune(rest[:open], '}') {
				return nil, configError(nil, "operation %s: path template has an unmatched '}'", op.OperationID)
			}
			closing := strings.IndexByte(rest[open:], '}')
			if closing < 0 {
				return nil, configError(nil, "operation %s: path template has an unterminated '{'", op.OperationID)
			}
			name := rest[open+1 : open+closing]
			if !argumentNamePattern.MatchString(name) {
				return nil, configError(nil, "operation %s: path template token %q is not a valid argument name", op.OperationID, name)
			}
			if _, duplicate := tokens[name]; duplicate {
				return nil, configError(nil, "operation %s: path template token %q is repeated", op.OperationID, name)
			}
			tokens[name] = struct{}{}
			rest = rest[open+closing+1:]
		}
	}
	return tokens, nil
}

func validateParameters(op *Operation, tokens map[string]struct{}) error {
	seen := make(map[string]struct{}, len(op.PathParameters)+len(op.QueryParameters))
	for _, parameter := range op.PathParameters {
		if err := validateParameter(parameter, true); err != nil {
			return fmt.Errorf("%w: operation %s path parameter", err, op.OperationID)
		}
		if _, duplicate := seen[parameter.Name]; duplicate {
			return configError(nil, "operation %s: duplicate parameter %q", op.OperationID, parameter.Name)
		}
		seen[parameter.Name] = struct{}{}
		if _, declared := tokens[parameter.Name]; !declared {
			return configError(nil, "operation %s: path parameter %q does not appear in the path template", op.OperationID, parameter.Name)
		}
	}
	for name := range tokens {
		if _, declared := seen[name]; !declared {
			return configError(nil, "operation %s: path template token %q has no path parameter", op.OperationID, name)
		}
	}
	for _, parameter := range op.QueryParameters {
		if err := validateParameter(parameter, false); err != nil {
			return fmt.Errorf("%w: operation %s query parameter", err, op.OperationID)
		}
		if _, duplicate := seen[parameter.Name]; duplicate {
			return configError(nil, "operation %s: duplicate parameter %q", op.OperationID, parameter.Name)
		}
		seen[parameter.Name] = struct{}{}
	}
	return nil
}

func validateParameter(parameter Parameter, path bool) error {
	if !argumentNamePattern.MatchString(parameter.Name) {
		return configError(nil, "invalid parameter name %q", parameter.Name)
	}
	if path && parameter.Type != "string" {
		return configError(nil, "path parameter %q must have type string", parameter.Name)
	}
	switch parameter.Type {
	case "string":
		if parameter.Minimum != nil || parameter.Maximum != nil {
			return configError(nil, "string parameter %q must not declare minimum/maximum", parameter.Name)
		}
		if parameter.MinLength != nil && *parameter.MinLength < 0 {
			return configError(nil, "parameter %q has a negative minLength", parameter.Name)
		}
		if parameter.MaxLength != nil && *parameter.MaxLength < 0 {
			return configError(nil, "parameter %q has a negative maxLength", parameter.Name)
		}
		if parameter.MinLength != nil && parameter.MaxLength != nil && *parameter.MinLength > *parameter.MaxLength {
			return configError(nil, "parameter %q minLength exceeds maxLength", parameter.Name)
		}
		if parameter.Pattern != "" {
			if _, err := regexp.Compile(parameter.Pattern); err != nil {
				return configError(err, "parameter %q has an invalid pattern", parameter.Name)
			}
		}
	case "integer":
		if parameter.MinLength != nil || parameter.MaxLength != nil || parameter.Pattern != "" || len(parameter.Enum) > 0 {
			return configError(nil, "integer parameter %q declares string constraints", parameter.Name)
		}
		if parameter.Minimum != nil && parameter.Maximum != nil && *parameter.Minimum > *parameter.Maximum {
			return configError(nil, "parameter %q minimum exceeds maximum", parameter.Name)
		}
	case "boolean":
		if parameter.Minimum != nil || parameter.Maximum != nil ||
			parameter.MinLength != nil || parameter.MaxLength != nil ||
			parameter.Pattern != "" || len(parameter.Enum) > 0 {
			return configError(nil, "boolean parameter %q declares numeric or string constraints", parameter.Name)
		}
	default:
		return configError(nil, "parameter %q has unsupported type %q", parameter.Name, parameter.Type)
	}
	if parameter.Default != nil {
		if err := validateParameterDefault(parameter); err != nil {
			return err
		}
	}
	return nil
}

func validateParameterDefault(parameter Parameter) error {
	switch parameter.Type {
	case "string":
		value, ok := parameter.Default.(string)
		if !ok {
			return configError(nil, "parameter %q default must be a string", parameter.Name)
		}
		if err := validateStringConstraints(parameter, value); err != nil {
			return configError(err, "parameter %q default violates its constraints", parameter.Name)
		}
	case "integer":
		value, err := integerArgument(parameter.Default)
		if err != nil {
			return configError(err, "parameter %q default must be an integer", parameter.Name)
		}
		if parameter.Minimum != nil && value < *parameter.Minimum {
			return configError(nil, "parameter %q default is below minimum", parameter.Name)
		}
		if parameter.Maximum != nil && value > *parameter.Maximum {
			return configError(nil, "parameter %q default is above maximum", parameter.Name)
		}
	case "boolean":
		if _, ok := parameter.Default.(bool); !ok {
			return configError(nil, "parameter %q default must be a boolean", parameter.Name)
		}
	}
	return nil
}

func validateHeaders(op *Operation) error {
	seen := make(map[string]struct{}, len(op.HeaderBindings))
	for _, binding := range op.HeaderBindings {
		if !validHeaderName(binding.Name) {
			return configError(nil, "operation %s: invalid header name %q", op.OperationID, binding.Name)
		}
		canonical := textproto.CanonicalMIMEHeaderKey(binding.Name)
		if _, reserved := reservedRequestHeaders[strings.ToLower(canonical)]; reserved {
			return configError(nil, "operation %s: header %q is reserved by the transport", op.OperationID, binding.Name)
		}
		if _, duplicate := seen[strings.ToLower(canonical)]; duplicate {
			return configError(nil, "operation %s: duplicate header binding %q", op.OperationID, binding.Name)
		}
		seen[strings.ToLower(canonical)] = struct{}{}
		if binding.Value == "" || !validHeaderValue(binding.Value) {
			return configError(nil, "operation %s: header %q has an invalid value", op.OperationID, binding.Name)
		}
	}
	return nil
}

func validateBody(op *Operation) error {
	if op.Body == nil {
		return nil
	}
	body := op.Body
	switch body.Kind {
	case PayloadJSON:
		if !isJSONMediaType(body.MediaType) {
			return configError(nil, "operation %s: JSON body requires a JSON media type", op.OperationID)
		}
		if len(body.Parts) > 0 {
			return configError(nil, "operation %s: JSON body must not declare multipart parts", op.OperationID)
		}
	case PayloadBinary:
		if _, err := parseMediaType(body.MediaType); err != nil {
			return configError(err, "operation %s: binary body has an invalid media type", op.OperationID)
		}
		if body.Schema != nil {
			return configError(nil, "operation %s: binary body must not declare a JSON schema", op.OperationID)
		}
		if len(body.Parts) > 0 {
			return configError(nil, "operation %s: binary body must not declare multipart parts", op.OperationID)
		}
	case PayloadMultipart:
		if !body.MultipartRoot {
			return configError(nil, "operation %s: multipart body must declare a JSON root", op.OperationID)
		}
		if isJSONMediaType(body.MediaType) {
			return configError(nil, "operation %s: multipart body must not use a JSON media type", op.OperationID)
		}
		mediaType, err := parseMediaType(body.MediaType)
		if err != nil || mediaType != "multipart/related" {
			return configError(nil, "operation %s: multipart body media type must be multipart/related", op.OperationID)
		}
		if len(body.Parts) == 0 {
			return configError(nil, "operation %s: multipart body must declare at least one part", op.OperationID)
		}
		// ContentTypeParam is reserved by the catalog schema. This slice always
		// emits the 3GPP multipart/related root type parameter
		// (type=application/json) from the payload engine, so a configured value
		// carries no additional behavior and is ignored.
		names := make(map[string]struct{}, len(body.Parts))
		pointers := make(map[string]struct{}, len(body.Parts))
		for _, part := range body.Parts {
			if !argumentNamePattern.MatchString(part.Name) {
				return configError(nil, "operation %s: multipart part has invalid name %q", op.OperationID, part.Name)
			}
			if _, duplicate := names[part.Name]; duplicate {
				return configError(nil, "operation %s: duplicate multipart part %q", op.OperationID, part.Name)
			}
			names[part.Name] = struct{}{}
			if _, err := parseMediaType(part.ContentType); err != nil {
				return configError(err, "operation %s: multipart part %q has an invalid content type", op.OperationID, part.Name)
			}
			if part.JSONPointerField == "" || !validJSONPointer(part.JSONPointerField) {
				return configError(nil, "operation %s: multipart part %q has an invalid JSON pointer", op.OperationID, part.Name)
			}
			if _, duplicate := pointers[part.JSONPointerField]; duplicate {
				return configError(nil, "operation %s: multipart part %q repeats a JSON pointer", op.OperationID, part.Name)
			}
			pointers[part.JSONPointerField] = struct{}{}
		}
		if body.MaxParts > 0 && body.MaxParts < len(body.Parts)+1 {
			return configError(nil, "operation %s: MaxParts cannot be smaller than the declared part count plus the JSON root", op.OperationID)
		}
	default:
		return configError(nil, "operation %s: unsupported body kind %q", op.OperationID, body.Kind)
	}
	return nil
}

func validateResponses(op *Operation) error {
	seen := make(map[string]struct{}, len(op.ResponseMediaTypes))
	hasJSON := false
	hasMultipart := false
	for _, raw := range op.ResponseMediaTypes {
		mediaType, err := parseMediaType(raw)
		if err != nil {
			return configError(err, "operation %s: invalid response media type", op.OperationID)
		}
		if _, duplicate := seen[mediaType]; duplicate {
			return configError(nil, "operation %s: duplicate response media type %q", op.OperationID, mediaType)
		}
		seen[mediaType] = struct{}{}
		if isJSONMediaType(mediaType) {
			hasJSON = true
		}
		if mediaType == "multipart/related" {
			hasMultipart = true
		}
	}
	if op.ResponseSchema != nil && !hasJSON && !hasMultipart {
		return configError(nil, "operation %s: response schema requires a declared JSON or multipart/related response media type", op.OperationID)
	}
	if len(op.ResponseParts) > 0 && !hasMultipart {
		return configError(nil, "operation %s: response parts require a declared multipart/related response media type", op.OperationID)
	}
	if hasMultipart && len(op.ResponseParts) == 0 {
		return configError(nil, "operation %s: multipart/related response requires declared response parts", op.OperationID)
	}
	names := make(map[string]struct{}, len(op.ResponseParts))
	pointers := make(map[string]struct{}, len(op.ResponseParts))
	for _, part := range op.ResponseParts {
		if !argumentNamePattern.MatchString(part.Name) {
			return configError(nil, "operation %s: response part has invalid name %q", op.OperationID, part.Name)
		}
		if _, duplicate := names[part.Name]; duplicate {
			return configError(nil, "operation %s: duplicate response part %q", op.OperationID, part.Name)
		}
		names[part.Name] = struct{}{}
		if _, err := parseMediaType(part.ContentType); err != nil {
			return configError(err, "operation %s: response part %q has an invalid content type", op.OperationID, part.Name)
		}
		if part.JSONPointerField == "" || !validJSONPointer(part.JSONPointerField) {
			return configError(nil, "operation %s: response part %q has an invalid JSON pointer", op.OperationID, part.Name)
		}
		if _, duplicate := pointers[part.JSONPointerField]; duplicate {
			return configError(nil, "operation %s: response part %q repeats a JSON pointer", op.OperationID, part.Name)
		}
		pointers[part.JSONPointerField] = struct{}{}
	}
	return nil
}

func validateIdempotency(op *Operation) error {
	if op.Effect != EffectMutation {
		if op.Idempotency != nil {
			return configError(nil, "operation %s: idempotency is only valid for a MUTATION", op.OperationID)
		}
		return nil
	}
	if op.Idempotency == nil {
		return configError(nil, "operation %s: MUTATION requires an idempotency contract", op.OperationID)
	}
	switch op.Idempotency.Mode {
	case "NONE":
		if op.Idempotency.KeyArgument != "" {
			return configError(nil, "operation %s: idempotency mode NONE must not declare a key argument", op.OperationID)
		}
	case "REQUIRED":
		if op.Idempotency.KeyArgument == "" || !validJSONPointer(op.Idempotency.KeyArgument) {
			return configError(nil, "operation %s: idempotency mode REQUIRED needs a valid key argument JSON pointer", op.OperationID)
		}
	default:
		return configError(nil, "operation %s: unsupported idempotency mode %q", op.OperationID, op.Idempotency.Mode)
	}
	return nil
}

func validHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func validHeaderValue(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func validJSONPointer(pointer string) bool {
	return jsonPointerPattern.MatchString(pointer)
}

func parseMediaType(raw string) (string, error) {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(raw))
	if err != nil || mediaType == "" || strings.Contains(mediaType, "*") {
		return "", fmt.Errorf("invalid media type %q", raw)
	}
	return strings.ToLower(mediaType), nil
}

func isJSONMediaType(raw string) bool {
	mediaType, err := parseMediaType(raw)
	if err != nil {
		return false
	}
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

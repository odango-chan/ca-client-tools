package executor

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// resolvedArguments is the validated, fully-bound form of caller arguments.
// After resolveArguments returns, no caller value can affect the request
// destination, method, header names/values or media types.
type resolvedArguments struct {
	path    string
	query   url.Values
	headers http.Header
	body    any    // JSON value for json/multipart bodies
	binary  string // resolver key for a binary body
	parts   map[string]string
}

func resolveArguments(op *Operation, args map[string]any) (*resolvedArguments, error) {
	if args == nil {
		args = map[string]any{}
	}
	allowed := make(map[string]struct{}, len(op.PathParameters)+len(op.QueryParameters)+2)
	for _, parameter := range op.PathParameters {
		allowed[parameter.Name] = struct{}{}
	}
	for _, parameter := range op.QueryParameters {
		allowed[parameter.Name] = struct{}{}
	}
	if op.Body != nil {
		allowed["body"] = struct{}{}
		if op.Body.Kind == PayloadMultipart {
			allowed["payloads"] = struct{}{}
		}
	}
	if op.Effect == EffectMutation && op.Idempotency != nil && op.Idempotency.Mode == "REQUIRED" {
		if segments, err := jsonPointerSegments(op.Idempotency.KeyArgument); err == nil && len(segments) > 0 {
			allowed[segments[0]] = struct{}{}
		}
	}
	for name := range args {
		if _, ok := allowed[name]; !ok {
			return nil, inputError(nil, "unknown argument %q", name)
		}
	}

	resolved := &resolvedArguments{
		query:   url.Values{},
		headers: http.Header{},
	}
	pathValues := make(map[string]string, len(op.PathParameters))
	for _, parameter := range op.PathParameters {
		raw, present := args[parameter.Name]
		if !present || raw == nil {
			return nil, inputError(nil, "missing required path parameter %q", parameter.Name)
		}
		value, ok := raw.(string)
		if !ok {
			return nil, inputError(nil, "path parameter %q must be a string", parameter.Name)
		}
		if err := validatePathValue(parameter, value); err != nil {
			return nil, err
		}
		pathValues[parameter.Name] = value
	}
	path := op.PathTemplate
	for name, value := range pathValues {
		path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(value))
	}
	resolved.path = path

	for _, parameter := range op.QueryParameters {
		raw, present := args[parameter.Name]
		if !present || raw == nil {
			if parameter.Required {
				return nil, inputError(nil, "missing required query parameter %q", parameter.Name)
			}
			continue
		}
		text, err := queryValue(parameter, raw)
		if err != nil {
			return nil, err
		}
		resolved.query.Set(parameter.Name, text)
	}

	for _, binding := range op.HeaderBindings {
		resolved.headers.Set(textproto.CanonicalMIMEHeaderKey(binding.Name), binding.Value)
	}

	if op.Body != nil {
		if err := resolveBody(op, args, resolved); err != nil {
			return nil, err
		}
	}
	if op.Effect == EffectMutation && op.Idempotency != nil && op.Idempotency.Mode == "REQUIRED" {
		key, found, err := resolveJSONPointer(args, op.Idempotency.KeyArgument)
		if err != nil {
			return nil, inputError(err, "cannot resolve idempotency key argument %q", op.Idempotency.KeyArgument)
		}
		if !found {
			return nil, inputError(nil, "missing required idempotency key argument %q", op.Idempotency.KeyArgument)
		}
		text, ok := key.(string)
		if !ok || text == "" {
			return nil, inputError(nil, "idempotency key argument %q must be a non-empty string", op.Idempotency.KeyArgument)
		}
	}
	return resolved, nil
}

func resolveBody(op *Operation, args map[string]any, resolved *resolvedArguments) error {
	body := op.Body
	raw, present := args["body"]
	switch body.Kind {
	case PayloadJSON:
		if !present {
			return inputError(nil, "missing required JSON body")
		}
		if body.Schema != nil {
			if err := body.Schema.Validate(raw); err != nil {
				return inputError(err, "body does not match the operation schema")
			}
		}
		resolved.body = raw
	case PayloadBinary:
		if !present {
			return inputError(nil, "missing required binary body argument")
		}
		key, ok := raw.(string)
		if !ok || key == "" {
			return inputError(nil, "binary body argument must be a non-empty Host payload name")
		}
		resolved.binary = key
	case PayloadMultipart:
		if !present {
			return inputError(nil, "missing required multipart JSON root body")
		}
		object, ok := raw.(map[string]any)
		if !ok {
			return inputError(nil, "multipart root body must be a JSON object")
		}
		if body.Schema != nil {
			if err := body.Schema.Validate(object); err != nil {
				return inputError(err, "multipart root body does not match the operation schema")
			}
		}
		resolved.body = object
		parts, err := resolvePayloads(op, args["payloads"])
		if err != nil {
			return err
		}
		resolved.parts = parts
	default:
		return configError(nil, "operation %s: unsupported body kind %q", op.OperationID, body.Kind)
	}
	return nil
}

func resolvePayloads(op *Operation, raw any) (map[string]string, error) {
	result := make(map[string]string)
	switch typed := raw.(type) {
	case nil:
	case map[string]any:
		for name, value := range typed {
			text, ok := value.(string)
			if !ok || text == "" {
				return nil, inputError(nil, "payload part %q must be a non-empty Host payload name", name)
			}
			result[name] = text
		}
	case map[string]string:
		for name, value := range typed {
			if value == "" {
				return nil, inputError(nil, "payload part %q must be a non-empty Host payload name", name)
			}
			result[name] = value
		}
	default:
		return nil, inputError(nil, "payloads must be an object mapping part names to Host payload names")
	}
	declared := make(map[string]struct{}, len(op.Body.Parts))
	for _, part := range op.Body.Parts {
		declared[part.Name] = struct{}{}
	}
	for name := range result {
		if _, ok := declared[name]; !ok {
			return nil, inputError(nil, "unknown payload part %q", name)
		}
	}
	for _, part := range op.Body.Parts {
		if !part.Required {
			continue
		}
		if _, present := result[part.Name]; !present {
			return nil, inputError(nil, "missing required payload part %q", part.Name)
		}
	}
	return result, nil
}

func validatePathValue(parameter Parameter, value string) error {
	if value == "" {
		return inputError(nil, "path parameter %q must not be empty", parameter.Name)
	}
	if strings.ContainsAny(value, "/?#\\") {
		return inputError(nil, "path parameter %q contains a forbidden character", parameter.Name)
	}
	if value == "." || value == ".." {
		return inputError(nil, "path parameter %q must not be a dot segment", parameter.Name)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return inputError(nil, "path parameter %q contains control characters", parameter.Name)
		}
	}
	return validateStringConstraints(parameter, value)
}

func queryValue(parameter Parameter, raw any) (string, error) {
	switch parameter.Type {
	case "string":
		value, ok := raw.(string)
		if !ok {
			return "", inputError(nil, "query parameter %q must be a string", parameter.Name)
		}
		if err := validateStringConstraints(parameter, value); err != nil {
			return "", err
		}
		return value, nil
	case "integer":
		value, err := integerArgument(raw)
		if err != nil {
			return "", inputError(err, "query parameter %q must be an integer", parameter.Name)
		}
		if parameter.Minimum != nil && value < *parameter.Minimum {
			return "", inputError(nil, "query parameter %q is below minimum", parameter.Name)
		}
		if parameter.Maximum != nil && value > *parameter.Maximum {
			return "", inputError(nil, "query parameter %q is above maximum", parameter.Name)
		}
		return strconv.FormatInt(value, 10), nil
	case "boolean":
		value, ok := raw.(bool)
		if !ok {
			return "", inputError(nil, "query parameter %q must be a boolean", parameter.Name)
		}
		return strconv.FormatBool(value), nil
	default:
		return "", configError(nil, "query parameter %q has unsupported type %q", parameter.Name, parameter.Type)
	}
}

func validateStringConstraints(parameter Parameter, value string) error {
	length := len([]rune(value))
	if parameter.MinLength != nil && length < *parameter.MinLength {
		return inputError(nil, "parameter %q is shorter than %d characters", parameter.Name, *parameter.MinLength)
	}
	if parameter.MaxLength != nil && length > *parameter.MaxLength {
		return inputError(nil, "parameter %q is longer than %d characters", parameter.Name, *parameter.MaxLength)
	}
	if parameter.Pattern != "" {
		pattern, err := regexp.Compile(parameter.Pattern)
		if err != nil {
			return configError(err, "parameter %q has an invalid pattern", parameter.Name)
		}
		if !pattern.MatchString(value) {
			return inputError(nil, "parameter %q does not match the required pattern", parameter.Name)
		}
	}
	if len(parameter.Enum) > 0 {
		for _, allowed := range parameter.Enum {
			if value == allowed {
				return nil
			}
		}
		return inputError(nil, "parameter %q is not one of the allowed values", parameter.Name)
	}
	return nil
}

func integerArgument(value any) (int64, error) {
	switch number := value.(type) {
	case json.Number:
		return strconv.ParseInt(number.String(), 10, 64)
	case int:
		return int64(number), nil
	case int8:
		return int64(number), nil
	case int16:
		return int64(number), nil
	case int32:
		return int64(number), nil
	case int64:
		return number, nil
	case uint:
		if uint64(number) > uint64(^uint64(0)>>1) {
			return 0, strconv.ErrRange
		}
		return int64(number), nil
	case uint8:
		return int64(number), nil
	case uint16:
		return int64(number), nil
	case uint32:
		return int64(number), nil
	case uint64:
		if number > uint64(^uint64(0)>>1) {
			return 0, strconv.ErrRange
		}
		return int64(number), nil
	case float32:
		return integralFloat(float64(number))
	case float64:
		return integralFloat(number)
	default:
		return 0, errNotAnInteger
	}
}

func integralFloat(value float64) (int64, error) {
	if value != float64(int64(value)) {
		return 0, errNotAnInteger
	}
	return int64(value), nil
}

var (
	errNotAnInteger   = errors.New("value is not an integer")
	errInvalidPointer = errors.New("invalid JSON pointer")
)

// resolveJSONPointer resolves an RFC 6901 JSON pointer against a decoded
// argument object.
func resolveJSONPointer(root any, pointer string) (any, bool, error) {
	if pointer == "" {
		return root, true, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, false, errInvalidPointer
	}
	segments, err := jsonPointerSegments(pointer)
	if err != nil {
		return nil, false, err
	}
	current := root
	for _, segment := range segments {
		switch node := current.(type) {
		case map[string]any:
			value, ok := node[segment]
			if !ok {
				return nil, false, nil
			}
			current = value
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false, nil
			}
			current = node[index]
		default:
			return nil, false, nil
		}
	}
	return current, true, nil
}

func jsonPointerSegments(pointer string) ([]string, error) {
	parts := strings.Split(pointer[1:], "/")
	for index, part := range parts {
		var builder strings.Builder
		for position := 0; position < len(part); position++ {
			if part[position] != '~' {
				builder.WriteByte(part[position])
				continue
			}
			if position+1 >= len(part) {
				return nil, errInvalidPointer
			}
			position++
			switch part[position] {
			case '0':
				builder.WriteByte('~')
			case '1':
				builder.WriteByte('/')
			default:
				return nil, errInvalidPointer
			}
		}
		parts[index] = builder.String()
	}
	return parts, nil
}

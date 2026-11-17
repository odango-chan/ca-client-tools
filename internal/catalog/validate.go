package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Strict-syntax patterns shared by the catalog validators.
var (
	commitPattern             = regexp.MustCompile(`^[0-9a-f]{40}$`)
	serviceKeyPattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	operationIDPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	argumentNamePattern       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
	multipartPartNamePattern  = regexp.MustCompile(`^[a-z][A-Za-z0-9_]{0,63}$`)
	idempotencyModePattern    = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	mediaTypePattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9!#$&^_.+-]*/[A-Za-z0-9][A-Za-z0-9!#$&^_.+-]*$`)
	headerTokenPattern        = regexp.MustCompile("^[!#$%&'*+\\-.^_`|~0-9A-Za-z]+$")
)

// allowedSchemaKeywords is the closed JSON Schema subset permitted by the
// catalog. Unknown keywords fail closed.
var allowedSchemaKeywords = map[string]struct{}{
	"type":                 {},
	"properties":           {},
	"required":             {},
	"additionalProperties": {},
	"items":                {},
	"enum":                 {},
	"const":                {},
	"minimum":              {},
	"maximum":              {},
	"minLength":            {},
	"maxLength":            {},
	"pattern":              {},
	"oneOf":                {},
}

var allowedSchemaTypes = map[string]struct{}{
	"object":  {},
	"array":   {},
	"string":  {},
	"integer": {},
	"number":  {},
	"boolean": {},
	"null":    {},
}

// validateDocument parses exactly one YAML document and enforces the catalog's
// strict syntax rules: valid UTF-8, non-empty, one document, an object root,
// no anchors/aliases/merge keys, string mapping keys and no duplicate keys at
// any nesting level.
func validateDocument(raw []byte) (*yaml.Node, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("catalog: artifact is not valid UTF-8")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("catalog: artifact is empty")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("catalog: artifact is empty")
		}
		return nil, fmt.Errorf("catalog: invalid YAML: %w", err)
	}
	var extra yaml.Node
	switch err := decoder.Decode(&extra); {
	case errors.Is(err, io.EOF):
	case err != nil:
		return nil, fmt.Errorf("catalog: invalid YAML: %w", err)
	default:
		return nil, errors.New("catalog: artifact must contain exactly one YAML document")
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, errors.New("catalog: artifact must be exactly one YAML document with one value")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("catalog: artifact root must be a mapping object")
	}
	if err := checkYAMLNode(root, "$"); err != nil {
		return nil, err
	}
	return root, nil
}

// checkYAMLNode rejects the YAML features the catalog contract forbids at any
// depth. It runs on the parsed node tree before strict typed decoding so that
// aliases and merge keys cannot hide duplicate or unknown fields.
func checkYAMLNode(node *yaml.Node, path string) error {
	if node.Anchor != "" {
		return fmt.Errorf("%s: YAML anchors are not allowed", path)
	}
	switch node.Kind {
	case yaml.AliasNode:
		return fmt.Errorf("%s: YAML aliases are not allowed", path)
	case yaml.ScalarNode:
		if node.Tag == "!!merge" {
			return fmt.Errorf("%s: YAML merge keys are not allowed", path)
		}
		return nil
	case yaml.SequenceNode:
		for position, child := range node.Content {
			if err := checkYAMLNode(child, fmt.Sprintf("%s[%d]", path, position)); err != nil {
				return err
			}
		}
		return nil
	case yaml.MappingNode:
		seen := make(map[string]struct{}, len(node.Content)/2)
		for position := 0; position+1 < len(node.Content); position += 2 {
			key := node.Content[position]
			if key.Kind != yaml.ScalarNode {
				return fmt.Errorf("%s: mapping keys must be scalar strings", path)
			}
			if key.Tag == "!!merge" {
				return fmt.Errorf("%s: YAML merge keys are not allowed", path)
			}
			if key.Tag != "!!str" {
				return fmt.Errorf("%s: mapping keys must be strings", path)
			}
			if _, duplicate := seen[key.Value]; duplicate {
				return fmt.Errorf("%s: duplicate mapping key %q", path, key.Value)
			}
			seen[key.Value] = struct{}{}
			if err := checkYAMLNode(node.Content[position+1], path+"."+key.Value); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%s: unsupported YAML node kind %d", path, node.Kind)
	}
}

// decodeStrict decodes one already syntax-checked artifact with unknown-field
// rejection enabled at every typed struct level.
func decodeStrict(raw []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("catalog: invalid artifact: %w", err)
	}
	return nil
}

// apiVersionFromRoot returns the root apiVersion scalar.
func apiVersionFromRoot(root *yaml.Node) (string, error) {
	for position := 0; position+1 < len(root.Content); position += 2 {
		if root.Content[position].Value != "apiVersion" {
			continue
		}
		value := root.Content[position+1]
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || value.Value == "" {
			return "", errors.New("catalog: apiVersion must be a non-empty string")
		}
		return value.Value, nil
	}
	return "", errors.New("catalog: missing apiVersion")
}

// detectAPIVersion parses and syntax-checks the artifact and returns its
// apiVersion.
func detectAPIVersion(raw []byte) (string, error) {
	root, err := validateDocument(raw)
	if err != nil {
		return "", err
	}
	return apiVersionFromRoot(root)
}

// validateSchemaValue validates one trusted JSON schema definition decoded from
// YAML and returns its canonical JSON encoding.
func validateSchemaValue(value any, where string) (json.RawMessage, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: schema must be an object", where)
	}
	if len(object) == 0 {
		return nil, fmt.Errorf("%s: schema must not be empty", where)
	}
	for keyword, entry := range object {
		if _, allowed := allowedSchemaKeywords[keyword]; !allowed {
			return nil, fmt.Errorf("%s: unknown schema keyword %q", where, keyword)
		}
		keywordWhere := where + "." + keyword
		switch keyword {
		case "type":
			if err := validateSchemaType(entry, keywordWhere); err != nil {
				return nil, err
			}
		case "properties":
			properties, ok := entry.(map[string]any)
			if !ok || len(properties) == 0 {
				return nil, fmt.Errorf("%s: must be a non-empty object", keywordWhere)
			}
			for name, child := range properties {
				if name == "" {
					return nil, fmt.Errorf("%s: property names must not be empty", keywordWhere)
				}
				if _, err := validateSchemaValue(child, keywordWhere+"."+name); err != nil {
					return nil, err
				}
			}
		case "required":
			if err := validateRequiredList(entry, keywordWhere); err != nil {
				return nil, err
			}
		case "additionalProperties":
			switch additional := entry.(type) {
			case bool:
				_ = additional
			case map[string]any:
				if _, err := validateSchemaValue(additional, keywordWhere); err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("%s: must be a boolean or a schema object", keywordWhere)
			}
		case "items":
			if _, err := validateSchemaValue(entry, keywordWhere); err != nil {
				return nil, err
			}
		case "enum":
			entries, ok := entry.([]any)
			if !ok || len(entries) == 0 {
				return nil, fmt.Errorf("%s: must be a non-empty array", keywordWhere)
			}
			for _, candidate := range entries {
				if err := validateJSONValue(candidate, keywordWhere); err != nil {
					return nil, err
				}
			}
		case "const":
			if err := validateJSONValue(entry, keywordWhere); err != nil {
				return nil, err
			}
		case "minimum", "maximum":
			if !isJSONNumber(entry) {
				return nil, fmt.Errorf("%s: must be a finite number", keywordWhere)
			}
		case "minLength", "maxLength":
			length, ok := integerValue(entry)
			if !ok || length < 0 {
				return nil, fmt.Errorf("%s: must be a non-negative integer", keywordWhere)
			}
		case "pattern":
			pattern, ok := entry.(string)
			if !ok {
				return nil, fmt.Errorf("%s: must be a string", keywordWhere)
			}
			if _, err := regexp.Compile(pattern); err != nil {
				return nil, fmt.Errorf("%s: %q is not a valid regular expression", keywordWhere, pattern)
			}
		case "oneOf":
			entries, ok := entry.([]any)
			if !ok || len(entries) == 0 {
				return nil, fmt.Errorf("%s: must be a non-empty array of schemas", keywordWhere)
			}
			for _, candidate := range entries {
				if _, err := validateSchemaValue(candidate, keywordWhere); err != nil {
					return nil, err
				}
			}
		}
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("%s: schema is not JSON-encodable: %w", where, err)
	}
	return encoded, nil
}

func validateSchemaType(value any, where string) error {
	validate := func(name any) error {
		text, ok := name.(string)
		if !ok {
			return fmt.Errorf("%s: type names must be strings", where)
		}
		if _, allowed := allowedSchemaTypes[text]; !allowed {
			return fmt.Errorf("%s: unsupported type %q", where, text)
		}
		return nil
	}
	switch typed := value.(type) {
	case string:
		return validate(typed)
	case []any:
		if len(typed) == 0 {
			return fmt.Errorf("%s: type array must not be empty", where)
		}
		seen := make(map[string]struct{}, len(typed))
		for _, name := range typed {
			if err := validate(name); err != nil {
				return err
			}
			text := name.(string)
			if _, duplicate := seen[text]; duplicate {
				return fmt.Errorf("%s: duplicate type %q", where, text)
			}
			seen[text] = struct{}{}
		}
		return nil
	default:
		return fmt.Errorf("%s: must be a string or array of strings", where)
	}
}

func validateRequiredList(value any, where string) error {
	entries, ok := value.([]any)
	if !ok || len(entries) == 0 {
		return fmt.Errorf("%s: must be a non-empty array of strings", where)
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name, ok := entry.(string)
		if !ok || name == "" {
			return fmt.Errorf("%s: entries must be non-empty strings", where)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("%s: duplicate entry %q", where, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// validateJSONValue rejects YAML-native values that are not JSON values.
func validateJSONValue(value any, where string) error {
	switch typed := value.(type) {
	case nil, bool, string:
		return nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return nil
	case float64:
		if math.IsInf(typed, 0) || math.IsNaN(typed) {
			return fmt.Errorf("%s: non-finite number is not JSON", where)
		}
		return nil
	case []any:
		for _, entry := range typed {
			if err := validateJSONValue(entry, where); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for _, entry := range typed {
			if err := validateJSONValue(entry, where); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%s: value is not JSON-encodable", where)
	}
}

func isJSONNumber(value any) bool {
	switch typed := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return !math.IsInf(typed, 0) && !math.IsNaN(typed)
	default:
		return false
	}
}

// integerValue converts a YAML integer scalar to int64.
func integerValue(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		if uint64(typed) > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	case float64:
		if math.IsInf(typed, 0) || math.IsNaN(typed) || typed != math.Trunc(typed) {
			return 0, false
		}
		if typed < math.MinInt64 || typed > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	default:
		return 0, false
	}
}

// validateArgument checks one typed backend argument declaration.
func validateArgument(argument v2Argument, where string, pathParameter bool) error {
	if !argumentNamePattern.MatchString(argument.Name) {
		return fmt.Errorf("%s: invalid argument name %q", where, argument.Name)
	}
	if pathParameter && !argument.Required {
		return fmt.Errorf("%s: path parameters must be required", where)
	}
	switch argument.Type {
	case "string", "integer", "boolean":
	default:
		return fmt.Errorf("%s: unsupported argument type %q", where, argument.Type)
	}
	if argument.Minimum != nil || argument.Maximum != nil {
		if argument.Type != "integer" {
			return fmt.Errorf("%s: minimum/maximum require an integer argument", where)
		}
	}
	if argument.Minimum != nil && argument.Maximum != nil && *argument.Minimum > *argument.Maximum {
		return fmt.Errorf("%s: minimum must not exceed maximum", where)
	}
	if argument.MinLength != nil || argument.MaxLength != nil || argument.Pattern != "" {
		if argument.Type != "string" {
			return fmt.Errorf("%s: minLength/maxLength/pattern require a string argument", where)
		}
	}
	if argument.MinLength != nil && *argument.MinLength < 0 {
		return fmt.Errorf("%s: minLength must be non-negative", where)
	}
	if argument.MaxLength != nil && *argument.MaxLength < 0 {
		return fmt.Errorf("%s: maxLength must be non-negative", where)
	}
	if argument.MinLength != nil && argument.MaxLength != nil && *argument.MinLength > *argument.MaxLength {
		return fmt.Errorf("%s: minLength must not exceed maxLength", where)
	}
	if argument.Pattern != "" {
		if _, err := regexp.Compile(argument.Pattern); err != nil {
			return fmt.Errorf("%s: pattern %q is not a valid regular expression", where, argument.Pattern)
		}
	}
	if argument.Default != nil {
		if err := validateArgumentDefault(argument, where); err != nil {
			return err
		}
	}
	return nil
}

func validateArgumentDefault(argument v2Argument, where string) error {
	switch argument.Type {
	case "string":
		text, ok := argument.Default.(string)
		if !ok {
			return fmt.Errorf("%s: default must be a string", where)
		}
		length := len([]rune(text))
		if argument.MinLength != nil && length < *argument.MinLength {
			return fmt.Errorf("%s: default is shorter than minLength", where)
		}
		if argument.MaxLength != nil && length > *argument.MaxLength {
			return fmt.Errorf("%s: default is longer than maxLength", where)
		}
		if argument.Pattern != "" {
			compiled, err := regexp.Compile(argument.Pattern)
			if err != nil {
				return fmt.Errorf("%s: pattern %q is not a valid regular expression", where, argument.Pattern)
			}
			if !compiled.MatchString(text) {
				return fmt.Errorf("%s: default does not match pattern", where)
			}
		}
		return nil
	case "integer":
		value, ok := integerValue(argument.Default)
		if !ok {
			return fmt.Errorf("%s: default must be an integer", where)
		}
		if argument.Minimum != nil && value < *argument.Minimum {
			return fmt.Errorf("%s: default is below minimum", where)
		}
		if argument.Maximum != nil && value > *argument.Maximum {
			return fmt.Errorf("%s: default is above maximum", where)
		}
		return nil
	case "boolean":
		if _, ok := argument.Default.(bool); !ok {
			return fmt.Errorf("%s: default must be a boolean", where)
		}
		return nil
	default:
		return fmt.Errorf("%s: unsupported argument type %q", where, argument.Type)
	}
}

// validateArgumentList validates a homogeneous argument list and returns the
// declared names in order.
func validateArgumentList(arguments []v2Argument, where string, pathParameters bool) ([]string, error) {
	names := make([]string, 0, len(arguments))
	seen := make(map[string]struct{}, len(arguments))
	for position := range arguments {
		argumentWhere := fmt.Sprintf("%s[%d]", where, position)
		if err := validateArgument(arguments[position], argumentWhere, pathParameters); err != nil {
			return nil, err
		}
		name := arguments[position].Name
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("%s: duplicate argument name %q", where, name)
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names, nil
}

// validateHeaderBindings validates trusted fixed header bindings.
func validateHeaderBindings(bindings []v2HeaderBinding, where string) ([]HeaderBinding, error) {
	if len(bindings) == 0 {
		return nil, nil
	}
	converted := make([]HeaderBinding, 0, len(bindings))
	seen := make(map[string]struct{}, len(bindings))
	for position, binding := range bindings {
		bindingWhere := fmt.Sprintf("%s[%d]", where, position)
		if !headerTokenPattern.MatchString(binding.Name) {
			return nil, fmt.Errorf("%s: invalid header name %q", bindingWhere, binding.Name)
		}
		if strings.TrimSpace(binding.Value) != binding.Value || binding.Value == "" {
			return nil, fmt.Errorf("%s: header value must be non-empty and trimmed", bindingWhere)
		}
		if strings.ContainsAny(binding.Value, "\r\n\x00") {
			return nil, fmt.Errorf("%s: header value contains an illegal character", bindingWhere)
		}
		name := strings.ToLower(binding.Name)
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("%s: duplicate header binding %q", where, binding.Name)
		}
		seen[name] = struct{}{}
		converted = append(converted, HeaderBinding{Name: binding.Name, Value: binding.Value})
	}
	return converted, nil
}

// validateGenericSpec validates and converts a generic SBI binding spec.
func validateGenericSpec(spec *v2Generic, bindingArguments []v2Argument, where string) (*GenericSpec, error) {
	if !validMethod(spec.Method) {
		return nil, fmt.Errorf("%s.method: unsupported HTTP method %q", where, spec.Method)
	}
	if err := validateAbsolutePath(spec.Path, where+".path"); err != nil {
		return nil, err
	}
	pathParameters, err := validateArgumentList(spec.PathParameters, where+".pathParameters", true)
	if err != nil {
		return nil, err
	}
	queryParameters, err := validateArgumentList(spec.QueryParameters, where+".queryParameters", false)
	if err != nil {
		return nil, err
	}
	queryNames := make(map[string]struct{}, len(queryParameters))
	for _, name := range queryParameters {
		if _, duplicate := queryNames[name]; duplicate {
			return nil, fmt.Errorf("%s: duplicate query parameter %q", where, name)
		}
		queryNames[name] = struct{}{}
	}
	if err := validateParameterAgreement(spec.Path, pathParameters, queryParameters, where); err != nil {
		return nil, err
	}
	headers, err := validateHeaderBindings(spec.HeaderBindings, where+".headerBindings")
	if err != nil {
		return nil, err
	}
	body, err := validateGenericBody(spec.Body, where+".body")
	if err != nil {
		return nil, err
	}
	if len(spec.ResponseMediaTypes) == 0 {
		return nil, fmt.Errorf("%s.responseMediaTypes: must not be empty", where)
	}
	hasJSONResponse := false
	hasMultipartResponse := false
	seenResponseTypes := make(map[string]struct{}, len(spec.ResponseMediaTypes))
	for position, mediaType := range spec.ResponseMediaTypes {
		if !mediaTypePattern.MatchString(mediaType) {
			return nil, fmt.Errorf("%s.responseMediaTypes[%d]: invalid media type %q", where, position, mediaType)
		}
		normalized := strings.ToLower(mediaType)
		if _, duplicate := seenResponseTypes[normalized]; duplicate {
			return nil, fmt.Errorf("%s.responseMediaTypes: duplicate media type %q", where, mediaType)
		}
		seenResponseTypes[normalized] = struct{}{}
		if normalized == "application/json" || strings.HasSuffix(normalized, "+json") {
			hasJSONResponse = true
		}
		if normalized == "multipart/related" {
			hasMultipartResponse = true
		} else if strings.HasPrefix(normalized, "multipart/") {
			return nil, fmt.Errorf("%s.responseMediaTypes[%d]: only multipart/related is supported", where, position)
		}
	}
	responseParts, err := validateGenericParts(spec.ResponseParts, where+".responseParts")
	if err != nil {
		return nil, err
	}
	if hasMultipartResponse && len(responseParts) == 0 {
		return nil, fmt.Errorf("%s.responseParts: multipart/related responses require declared parts", where)
	}
	if !hasMultipartResponse && len(responseParts) > 0 {
		return nil, fmt.Errorf("%s.responseParts: parts require multipart/related in responseMediaTypes", where)
	}
	var responseSchema json.RawMessage
	if spec.ResponseSchema != nil {
		if !hasJSONResponse && !hasMultipartResponse {
			return nil, fmt.Errorf("%s.responseSchema: requires a JSON or multipart/related response media type", where)
		}
		responseSchema, err = validateSchemaValue(spec.ResponseSchema, where+".responseSchema")
		if err != nil {
			return nil, err
		}
	}
	var idempotency *Idempotency
	if spec.Idempotency != nil {
		mode := spec.Idempotency.Mode
		if !idempotencyModePattern.MatchString(mode) {
			return nil, fmt.Errorf("%s.idempotency.mode: must be an uppercase contract token", where)
		}
		switch mode {
		case "NONE":
			if spec.Idempotency.KeyArgument != "" {
				return nil, fmt.Errorf("%s.idempotency.keyArgument: mode NONE must not declare a key argument", where)
			}
		case "IDEMPOTENCY_KEY":
			if spec.Idempotency.KeyArgument == "" {
				return nil, fmt.Errorf("%s.idempotency.keyArgument: mode IDEMPOTENCY_KEY requires a declared argument", where)
			}
			if !declaresArgument(spec.Idempotency.KeyArgument, bindingArguments, spec.PathParameters, spec.QueryParameters) {
				return nil, fmt.Errorf("%s.idempotency.keyArgument: %q is not a declared argument", where, spec.Idempotency.KeyArgument)
			}
		default:
			return nil, fmt.Errorf("%s.idempotency.mode: unsupported mode %q", where, mode)
		}
		idempotency = &Idempotency{
			Mode:        mode,
			KeyArgument: spec.Idempotency.KeyArgument,
		}
	}
	return &GenericSpec{
		Method:             spec.Method,
		Path:               spec.Path,
		PathParameters:     convertArguments(spec.PathParameters),
		QueryParameters:    convertArguments(spec.QueryParameters),
		HeaderBindings:     headers,
		Body:               body,
		ResponseMediaTypes: append([]string(nil), spec.ResponseMediaTypes...),
		ResponseSchema:     responseSchema,
		ResponseParts:      responseParts,
		Idempotency:        idempotency,
	}, nil
}

// declaresArgument reports whether name is declared in any of the argument
// lists.
func declaresArgument(name string, lists ...[]v2Argument) bool {
	for _, list := range lists {
		for _, argument := range list {
			if argument.Name == name {
				return true
			}
		}
	}
	return false
}

// validateParameterAgreement checks path template parameters against the
// declared path parameters and rejects ambiguous parameter names.
func validateParameterAgreement(path string, pathParameters, queryParameters []string, where string) error {
	templateNames, err := pathTemplateParameters(path, where+".path")
	if err != nil {
		return err
	}
	if len(templateNames) != len(pathParameters) {
		return fmt.Errorf("%s.pathParameters: declared path parameters do not match the path template", where)
	}
	declared := make(map[string]struct{}, len(pathParameters))
	for _, name := range pathParameters {
		declared[name] = struct{}{}
	}
	for _, name := range templateNames {
		if _, ok := declared[name]; !ok {
			return fmt.Errorf("%s.pathParameters: path template parameter %q is not declared", where, name)
		}
	}
	for _, name := range queryParameters {
		if _, duplicate := declared[name]; duplicate {
			return fmt.Errorf("%s: parameter %q is declared as both path and query parameter", where, name)
		}
	}
	for _, name := range templateNames {
		delete(declared, name)
	}
	for name := range declared {
		return fmt.Errorf("%s.pathParameters: declared parameter %q does not appear in the path template", where, name)
	}
	return nil
}

// pathTemplateParameters extracts the {name} placeholders of a normalized
// absolute path.
func pathTemplateParameters(path, where string) ([]string, error) {
	names := make([]string, 0)
	seen := make(map[string]struct{})
	for position := 0; position < len(path); position++ {
		switch path[position] {
		case '{':
			end := strings.IndexByte(path[position:], '}')
			if end < 0 {
				return nil, fmt.Errorf("%s: unterminated path parameter", where)
			}
			name := path[position+1 : position+end]
			if !argumentNamePattern.MatchString(name) {
				return nil, fmt.Errorf("%s: invalid path parameter %q", where, name)
			}
			if _, duplicate := seen[name]; duplicate {
				return nil, fmt.Errorf("%s: duplicate path parameter %q", where, name)
			}
			seen[name] = struct{}{}
			names = append(names, name)
			position += end
		case '}':
			return nil, fmt.Errorf("%s: stray closing brace", where)
		}
	}
	return names, nil
}

// validateGenericBody validates and converts a generic request body.
func validateGenericBody(body *v2GenericBody, where string) (*GenericBody, error) {
	if body == nil {
		return nil, nil
	}
	if !mediaTypePattern.MatchString(body.MediaType) {
		return nil, fmt.Errorf("%s.mediaType: invalid media type %q", where, body.MediaType)
	}
	parts, err := validateGenericParts(body.Parts, where+".parts")
	if err != nil {
		return nil, err
	}
	converted := &GenericBody{Kind: body.Kind, MediaType: body.MediaType}
	switch body.Kind {
	case "json":
		if body.MediaType != "application/json" {
			return nil, fmt.Errorf("%s.mediaType: json bodies must use application/json", where)
		}
		if body.RequestSchema == nil {
			return nil, fmt.Errorf("%s.requestSchema: json bodies require a schema", where)
		}
		schema, err := validateSchemaValue(body.RequestSchema, where+".requestSchema")
		if err != nil {
			return nil, err
		}
		converted.RequestSchema = schema
		if body.MultipartRoot || body.MaxParts != 0 || body.ContentTypeParam != "" || len(parts) > 0 {
			return nil, fmt.Errorf("%s: multipart-only fields are not allowed for a json body", where)
		}
	case "binary":
		if body.RequestSchema != nil {
			return nil, fmt.Errorf("%s.requestSchema: binary bodies must not declare a JSON schema", where)
		}
		if body.MultipartRoot || body.MaxParts != 0 || body.ContentTypeParam != "" || len(parts) > 0 {
			return nil, fmt.Errorf("%s: multipart-only fields are not allowed for a binary body", where)
		}
	case "multipart":
		if body.MediaType != "multipart/related" {
			return nil, fmt.Errorf("%s.mediaType: multipart bodies must use multipart/related", where)
		}
		if !body.MultipartRoot {
			return nil, fmt.Errorf("%s.multipartRoot: multipart bodies require multipartRoot", where)
		}
		if len(parts) == 0 {
			return nil, fmt.Errorf("%s.parts: multipart bodies require at least one declared part", where)
		}
		if body.MaxParts < len(parts)+1 {
			return nil, fmt.Errorf("%s.maxParts: must include the JSON root and every declared part", where)
		}
		if body.ContentTypeParam == "" {
			return nil, fmt.Errorf("%s.contentTypeParam: multipart bodies require contentTypeParam", where)
		}
		if !headerTokenPattern.MatchString(body.ContentTypeParam) {
			return nil, fmt.Errorf("%s.contentTypeParam: invalid parameter name %q", where, body.ContentTypeParam)
		}
		if body.RequestSchema == nil {
			return nil, fmt.Errorf("%s.requestSchema: multipart bodies require a JSON root schema", where)
		}
		schema, err := validateSchemaValue(body.RequestSchema, where+".requestSchema")
		if err != nil {
			return nil, err
		}
		converted.RequestSchema = schema
		converted.MultipartRoot = true
		converted.MaxParts = body.MaxParts
		converted.ContentTypeParam = body.ContentTypeParam
		converted.Parts = parts
	default:
		return nil, fmt.Errorf("%s.kind: unsupported body kind %q", where, body.Kind)
	}
	return converted, nil
}

// validateGenericParts validates the closed per-part metadata used by request
// and response multipart/related bindings.
func validateGenericParts(parts []v2GenericPart, where string) ([]GenericPart, error) {
	if len(parts) == 0 {
		return nil, nil
	}
	converted := make([]GenericPart, 0, len(parts))
	seenNames := make(map[string]struct{}, len(parts))
	seenPointers := make(map[string]struct{}, len(parts))
	for position, part := range parts {
		partWhere := fmt.Sprintf("%s[%d]", where, position)
		if !multipartPartNamePattern.MatchString(part.Name) {
			return nil, fmt.Errorf("%s.name: invalid multipart part name %q", partWhere, part.Name)
		}
		if _, duplicate := seenNames[part.Name]; duplicate {
			return nil, fmt.Errorf("%s.name: duplicate multipart part %q", partWhere, part.Name)
		}
		seenNames[part.Name] = struct{}{}
		if !mediaTypePattern.MatchString(part.ContentType) {
			return nil, fmt.Errorf("%s.contentType: invalid media type %q", partWhere, part.ContentType)
		}
		if !validCatalogJSONPointer(part.JSONPointerField) {
			return nil, fmt.Errorf("%s.jsonPointerField: invalid JSON pointer", partWhere)
		}
		if _, duplicate := seenPointers[part.JSONPointerField]; duplicate {
			return nil, fmt.Errorf("%s.jsonPointerField: duplicate JSON pointer %q", partWhere, part.JSONPointerField)
		}
		seenPointers[part.JSONPointerField] = struct{}{}
		converted = append(converted, GenericPart{
			Name:             part.Name,
			ContentType:      part.ContentType,
			JSONPointerField: part.JSONPointerField,
			Required:         part.Required,
		})
	}
	return converted, nil
}

func validCatalogJSONPointer(pointer string) bool {
	if pointer == "" || pointer[0] != '/' {
		return false
	}
	for position := 0; position < len(pointer); position++ {
		if pointer[position] != '~' {
			continue
		}
		if position+1 >= len(pointer) || (pointer[position+1] != '0' && pointer[position+1] != '1') {
			return false
		}
		position++
	}
	return true
}

// validateAbsolutePath enforces the normalized absolute path rule for generic
// bindings.
func validateAbsolutePath(path, where string) error {
	if path == "" || path[0] != '/' {
		return fmt.Errorf("%s: path must be absolute and start with '/'", where)
	}
	if strings.ContainsAny(path, "?#\\") {
		return fmt.Errorf("%s: path must not contain query, fragment or backslash characters", where)
	}
	for position := 0; position < len(path); position++ {
		if path[position] < 0x21 || path[position] > 0x7e {
			return fmt.Errorf("%s: path must be printable ASCII", where)
		}
	}
	segments := strings.Split(path, "/")
	if len(segments) < 2 {
		return fmt.Errorf("%s: path must not be empty", where)
	}
	for _, segment := range segments[1:] {
		if segment == "" {
			return fmt.Errorf("%s: path must not contain empty segments", where)
		}
		if segment == "." || segment == ".." {
			return fmt.Errorf("%s: path must not contain dot segments", where)
		}
	}
	return nil
}

// validateRelativePath enforces the normalized repository-relative path rule
// for source pins.
func validateRelativePath(path, where string) error {
	if path == "" {
		return fmt.Errorf("%s: path must not be empty", where)
	}
	if strings.ContainsRune(path, '\\') || path[0] == '/' {
		return fmt.Errorf("%s: path must be repository-relative with '/' separators", where)
	}
	segments := strings.Split(path, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("%s: path is not normalized", where)
		}
	}
	for position := 0; position < len(path); position++ {
		if path[position] < 0x21 || path[position] > 0x7e {
			return fmt.Errorf("%s: path must be printable ASCII", where)
		}
	}
	return nil
}

func validMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

// convertArguments copies raw arguments into the public model.
func convertArguments(arguments []v2Argument) []Argument {
	if len(arguments) == 0 {
		return nil
	}
	converted := make([]Argument, 0, len(arguments))
	for _, argument := range arguments {
		entry := Argument{
			Name:     argument.Name,
			Type:     argument.Type,
			Required: argument.Required,
			Pattern:  argument.Pattern,
			Default:  argument.Default,
		}
		if argument.Minimum != nil {
			minimum := *argument.Minimum
			entry.Minimum = &minimum
		}
		if argument.Maximum != nil {
			maximum := *argument.Maximum
			entry.Maximum = &maximum
		}
		if argument.MinLength != nil {
			minimumLength := *argument.MinLength
			entry.MinLength = &minimumLength
		}
		if argument.MaxLength != nil {
			maximumLength := *argument.MaxLength
			entry.MaxLength = &maximumLength
		}
		converted = append(converted, entry)
	}
	return converted
}

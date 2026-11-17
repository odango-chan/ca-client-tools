// Package schemacheck implements a small, dependency-free JSON Schema subset
// validator for trusted catalog content.
//
// The supported subset is deliberately closed: Compile rejects every keyword it
// does not implement so a trusted loader fails closed instead of silently
// ignoring a constraint. Values are decoded with encoding/json and
// json.Number, and validation never coerces strings or numbers.
package schemacheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	// ErrInvalidSchema reports a schema document that is not valid JSON or uses
	// an unsupported or inconsistent keyword.
	ErrInvalidSchema = errors.New("invalid JSON schema")
	// ErrInvalidJSON reports a JSON value that is malformed, contains duplicate
	// object keys or has trailing data.
	ErrInvalidJSON = errors.New("invalid JSON document")
)

// Schema is a compiled JSON Schema subset. It is immutable after Compile and
// safe for concurrent Validate calls.
type Schema struct {
	types []string // nil means no "type" constraint

	properties           map[string]*Schema
	required             []string
	additionalProperties *bool // nil or true permits extra properties
	items                *Schema

	hasEnum  bool
	enum     []any
	hasConst bool
	constant any

	minimum *big.Rat
	maximum *big.Rat

	minLength *int
	maxLength *int
	pattern   *regexp.Regexp

	oneOf []*Schema
}

// ValidationError describes one failed schema assertion. Path is a JSON-style
// location such as "$.items[0].name"; "$" denotes the validated root value.
type ValidationError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Path == "" || e.Path == "$" {
		return "schema violation at $: " + e.Message
	}
	return "schema violation at " + e.Path + ": " + e.Message
}

// Compile parses and compiles one JSON Schema document. The document must be a
// single JSON object value; duplicate object keys and trailing data are
// rejected.
func Compile(raw []byte) (*Schema, error) {
	value, err := DecodeJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSchema, err)
	}
	schema, err := compileSchema(value)
	if err != nil {
		return nil, err
	}
	return schema, nil
}

func compileSchema(value any) (*Schema, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: schema must be a JSON object", ErrInvalidSchema)
	}
	schema := &Schema{}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, keyword := range keys {
		raw := object[keyword]
		switch keyword {
		case "type":
			types, err := compileTypes(raw)
			if err != nil {
				return nil, err
			}
			schema.types = types
		case "properties":
			properties, ok := raw.(map[string]any)
			if !ok {
				return nil, schemaError("properties must be an object")
			}
			schema.properties = make(map[string]*Schema, len(properties))
			names := make([]string, 0, len(properties))
			for name := range properties {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				child, err := compileSchema(properties[name])
				if err != nil {
					return nil, fmt.Errorf("%w: properties.%s", err, name)
				}
				schema.properties[name] = child
			}
		case "required":
			required, ok := raw.([]any)
			if !ok || len(required) == 0 {
				return nil, schemaError("required must be a non-empty array of strings")
			}
			seen := make(map[string]struct{}, len(required))
			for _, entry := range required {
				name, ok := entry.(string)
				if !ok || name == "" {
					return nil, schemaError("required must contain non-empty strings")
				}
				if _, duplicate := seen[name]; duplicate {
					return nil, schemaError("required contains duplicate name %q", name)
				}
				seen[name] = struct{}{}
				schema.required = append(schema.required, name)
			}
		case "additionalProperties":
			flag, ok := raw.(bool)
			if !ok {
				return nil, schemaError("additionalProperties must be a boolean")
			}
			schema.additionalProperties = &flag
		case "items":
			items, err := compileSchema(raw)
			if err != nil {
				return nil, fmt.Errorf("%w: items", err)
			}
			schema.items = items
		case "enum":
			entries, ok := raw.([]any)
			if !ok || len(entries) == 0 {
				return nil, schemaError("enum must be a non-empty array")
			}
			schema.hasEnum = true
			schema.enum = append([]any(nil), entries...)
		case "const":
			schema.hasConst = true
			schema.constant = raw
		case "minimum":
			number, err := compileNumber(raw, "minimum")
			if err != nil {
				return nil, err
			}
			schema.minimum = number
		case "maximum":
			number, err := compileNumber(raw, "maximum")
			if err != nil {
				return nil, err
			}
			schema.maximum = number
		case "minLength":
			length, err := compileLength(raw, "minLength")
			if err != nil {
				return nil, err
			}
			schema.minLength = &length
		case "maxLength":
			length, err := compileLength(raw, "maxLength")
			if err != nil {
				return nil, err
			}
			schema.maxLength = &length
		case "pattern":
			pattern, ok := raw.(string)
			if !ok {
				return nil, schemaError("pattern must be a string")
			}
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				return nil, schemaError("pattern %q is not a valid Go regular expression", pattern)
			}
			schema.pattern = compiled
		case "oneOf":
			entries, ok := raw.([]any)
			if !ok || len(entries) == 0 {
				return nil, schemaError("oneOf must be a non-empty array of schemas")
			}
			schema.oneOf = make([]*Schema, 0, len(entries))
			for index, entry := range entries {
				child, err := compileSchema(entry)
				if err != nil {
					return nil, fmt.Errorf("%w: oneOf[%d]", err, index)
				}
				schema.oneOf = append(schema.oneOf, child)
			}
		case "title", "description":
			// Annotation keywords carry no validation semantics.
			if _, ok := raw.(string); !ok {
				return nil, schemaError("%s must be a string", keyword)
			}
		default:
			return nil, schemaError("unsupported keyword %q", keyword)
		}
	}
	if err := schema.checkConsistency(); err != nil {
		return nil, err
	}
	return schema, nil
}

func compileTypes(raw any) ([]string, error) {
	var types []string
	switch value := raw.(type) {
	case string:
		types = []string{value}
	case []any:
		if len(value) == 0 {
			return nil, schemaError("type array must not be empty")
		}
		seen := make(map[string]struct{}, len(value))
		for _, entry := range value {
			name, ok := entry.(string)
			if !ok {
				return nil, schemaError("type array must contain strings")
			}
			if _, duplicate := seen[name]; duplicate {
				return nil, schemaError("type array contains duplicate %q", name)
			}
			seen[name] = struct{}{}
			types = append(types, name)
		}
	default:
		return nil, schemaError("type must be a string or array of strings")
	}
	for _, name := range types {
		switch name {
		case "object", "array", "string", "integer", "number", "boolean", "null":
		default:
			return nil, schemaError("unsupported type %q", name)
		}
	}
	return types, nil
}

func compileNumber(raw any, keyword string) (*big.Rat, error) {
	number, ok := raw.(json.Number)
	if !ok {
		return nil, schemaError("%s must be a number", keyword)
	}
	rational, ok := new(big.Rat).SetString(number.String())
	if !ok || rational == nil {
		return nil, schemaError("%s is not a finite JSON number", keyword)
	}
	return rational, nil
}

func compileLength(raw any, keyword string) (int, error) {
	number, ok := raw.(json.Number)
	if !ok {
		return 0, schemaError("%s must be a non-negative integer", keyword)
	}
	value, err := strconv.ParseInt(number.String(), 10, 32)
	if err != nil || value < 0 {
		return 0, schemaError("%s must be a non-negative integer", keyword)
	}
	return int(value), nil
}

func (s *Schema) checkConsistency() error {
	if s.minimum != nil && s.maximum != nil && s.minimum.Cmp(s.maximum) > 0 {
		return schemaError("minimum must not exceed maximum")
	}
	if s.minLength != nil && s.maxLength != nil && *s.minLength > *s.maxLength {
		return schemaError("minLength must not exceed maxLength")
	}
	if s.minimum != nil || s.maximum != nil {
		if s.types != nil && !s.allowsAnyType("integer", "number") {
			return schemaError("minimum/maximum require type integer or number")
		}
	}
	if s.minLength != nil || s.maxLength != nil || s.pattern != nil {
		if s.types != nil && !s.allowsAnyType("string") {
			return schemaError("minLength/maxLength/pattern require type string")
		}
	}
	if s.properties != nil || s.required != nil || s.additionalProperties != nil {
		if s.types != nil && !s.allowsAnyType("object") {
			return schemaError("properties/required/additionalProperties require type object")
		}
	}
	if s.items != nil {
		if s.types != nil && !s.allowsAnyType("array") {
			return schemaError("items requires type array")
		}
	}
	return nil
}

func (s *Schema) allowsAnyType(names ...string) bool {
	for _, allowed := range names {
		for _, declared := range s.types {
			if declared == allowed {
				return true
			}
		}
	}
	return false
}

func schemaError(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSchema, fmt.Sprintf(format, arguments...))
}

// Validate checks value against the compiled schema. Values are expected to be
// the result of DecodeJSON (maps, slices, strings, bool, nil and json.Number),
// but ordinary Go numbers are accepted as a convenience.
func (s *Schema) Validate(value any) error {
	if s == nil {
		return nil
	}
	return s.validate(value, "$")
}

func (s *Schema) validate(value any, path string) error {
	if s.types != nil && !matchesAnyType(value, s.types) {
		return &ValidationError{Path: path, Message: fmt.Sprintf("expected type %s", strings.Join(s.types, " or "))}
	}
	if s.hasConst && !equalJSON(s.constant, value) {
		return &ValidationError{Path: path, Message: "value does not match const"}
	}
	if s.hasEnum && !matchesEnum(s.enum, value) {
		return &ValidationError{Path: path, Message: "value is not one of the enum values"}
	}
	if s.minimum != nil || s.maximum != nil {
		number, err := numberValue(value)
		if err != nil {
			return &ValidationError{Path: path, Message: "numeric bound applied to a non-number"}
		}
		if s.minimum != nil && number.Cmp(s.minimum) < 0 {
			return &ValidationError{Path: path, Message: "number is below minimum"}
		}
		if s.maximum != nil && number.Cmp(s.maximum) > 0 {
			return &ValidationError{Path: path, Message: "number is above maximum"}
		}
	}
	if s.minLength != nil || s.maxLength != nil || s.pattern != nil {
		text, isString := value.(string)
		if !isString {
			return &ValidationError{Path: path, Message: "string bound applied to a non-string"}
		}
		length := len([]rune(text))
		if s.minLength != nil && length < *s.minLength {
			return &ValidationError{Path: path, Message: fmt.Sprintf("string is shorter than %d characters", *s.minLength)}
		}
		if s.maxLength != nil && length > *s.maxLength {
			return &ValidationError{Path: path, Message: fmt.Sprintf("string is longer than %d characters", *s.maxLength)}
		}
		if s.pattern != nil && !s.pattern.MatchString(text) {
			return &ValidationError{Path: path, Message: "string does not match pattern"}
		}
	}
	if object, isObject := value.(map[string]any); isObject {
		for _, name := range s.required {
			if _, present := object[name]; !present {
				return &ValidationError{Path: path, Message: fmt.Sprintf("required property %q is missing", name)}
			}
		}
		for name, child := range s.properties {
			entry, present := object[name]
			if !present {
				continue
			}
			if err := child.validate(entry, propertyPath(path, name)); err != nil {
				return err
			}
		}
		if s.additionalProperties != nil && !*s.additionalProperties {
			if err := s.rejectAdditional(path, object); err != nil {
				return err
			}
		}
	}
	if array, isArray := value.([]any); isArray && s.items != nil {
		for index, entry := range array {
			if err := s.items.validate(entry, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}
	if len(s.oneOf) > 0 {
		matches := 0
		for _, candidate := range s.oneOf {
			if candidate.validate(value, path) == nil {
				matches++
			}
		}
		if matches != 1 {
			return &ValidationError{Path: path, Message: fmt.Sprintf("value matches %d oneOf schemas; exactly one is required", matches)}
		}
	}
	return nil
}

func (s *Schema) rejectAdditional(path string, object map[string]any) error {
	names := make([]string, 0, len(object))
	for name := range object {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, declared := s.properties[name]; !declared {
			return &ValidationError{Path: path, Message: fmt.Sprintf("additional property %q is not allowed", name)}
		}
	}
	return nil
}

func propertyPath(path, name string) string {
	if isIdentifier(name) {
		return path + "." + name
	}
	return path + "[" + strconv.Quote(name) + "]"
}

func isIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for index, r := range name {
		if index == 0 {
			if !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
				return false
			}
			continue
		}
		if !(r == '_' || r == '-' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func matchesAnyType(value any, types []string) bool {
	for _, name := range types {
		if matchesType(value, name) {
			return true
		}
	}
	return false
}

func matchesType(value any, name string) bool {
	switch name {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	case "integer":
		return isInteger(value)
	case "number":
		_, err := numberValue(value)
		return err == nil
	default:
		return false
	}
}

func isInteger(value any) bool {
	switch number := value.(type) {
	case json.Number:
		text := number.String()
		if text == "" {
			return false
		}
		if text[0] == '-' {
			text = text[1:]
		}
		if text == "" {
			return false
		}
		for _, r := range text {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return float64(number) == math.Trunc(float64(number)) && !math.IsInf(float64(number), 0) && !math.IsNaN(float64(number))
	case float64:
		return number == math.Trunc(number) && !math.IsInf(number, 0) && !math.IsNaN(number)
	default:
		return false
	}
}

func numberValue(value any) (*big.Rat, error) {
	var text string
	switch number := value.(type) {
	case json.Number:
		text = number.String()
	case int:
		text = strconv.FormatInt(int64(number), 10)
	case int8:
		text = strconv.FormatInt(int64(number), 10)
	case int16:
		text = strconv.FormatInt(int64(number), 10)
	case int32:
		text = strconv.FormatInt(int64(number), 10)
	case int64:
		text = strconv.FormatInt(number, 10)
	case uint:
		text = strconv.FormatUint(uint64(number), 10)
	case uint8:
		text = strconv.FormatUint(uint64(number), 10)
	case uint16:
		text = strconv.FormatUint(uint64(number), 10)
	case uint32:
		text = strconv.FormatUint(uint64(number), 10)
	case uint64:
		text = strconv.FormatUint(number, 10)
	case float32:
		value := float64(number)
		if math.IsInf(value, 0) || math.IsNaN(value) {
			return nil, ErrInvalidJSON
		}
		text = strconv.FormatFloat(value, 'g', -1, 32)
	case float64:
		if math.IsInf(number, 0) || math.IsNaN(number) {
			return nil, ErrInvalidJSON
		}
		text = strconv.FormatFloat(number, 'g', -1, 64)
	default:
		return nil, ErrInvalidJSON
	}
	rational, ok := new(big.Rat).SetString(text)
	if !ok || rational == nil {
		return nil, ErrInvalidJSON
	}
	return rational, nil
}

func matchesEnum(enum []any, value any) bool {
	for _, candidate := range enum {
		if equalJSON(candidate, value) {
			return true
		}
	}
	return false
}

func equalJSON(left, right any) bool {
	switch leftValue := left.(type) {
	case nil:
		return right == nil
	case bool:
		rightValue, ok := right.(bool)
		return ok && leftValue == rightValue
	case string:
		rightValue, ok := right.(string)
		return ok && leftValue == rightValue
	case json.Number:
		rightNumber, err := numberValue(right)
		if err != nil {
			return false
		}
		leftNumber, err := numberValue(leftValue)
		return err == nil && leftNumber.Cmp(rightNumber) == 0
	case []any:
		rightValue, ok := right.([]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for index := range leftValue {
			if !equalJSON(leftValue[index], rightValue[index]) {
				return false
			}
		}
		return true
	case map[string]any:
		rightValue, ok := right.(map[string]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for name, entry := range leftValue {
			other, present := rightValue[name]
			if !present || !equalJSON(entry, other) {
				return false
			}
		}
		return true
	default:
		// Only JSON-decoded values are expected; treat other Go values as
		// unequal rather than relying on reflection semantics.
		return false
	}
}

// DecodeJSON decodes exactly one JSON value with json.Number. Duplicate object
// keys at any nesting level, malformed input and trailing data are rejected.
func DecodeJSON(raw []byte) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("%w: empty document", ErrInvalidJSON)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeValue(decoder)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing data after JSON value", ErrInvalidJSON)
	}
	return value, nil
}

func decodeValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("object key is not a string")
			}
			if _, duplicate := object[key]; duplicate {
				return nil, fmt.Errorf("duplicate object key %q", key)
			}
			value, err := decodeValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		end, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if end != json.Delim('}') {
			return nil, errors.New("unterminated object")
		}
		return object, nil
	case '[':
		array := make([]any, 0)
		for decoder.More() {
			value, err := decodeValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		end, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if end != json.Delim(']') {
			return nil, errors.New("unterminated array")
		}
		return array, nil
	default:
		return nil, errors.New("unexpected JSON delimiter")
	}
}

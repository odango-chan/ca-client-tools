package schemacheck

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

func mustCompile(t *testing.T, raw string) *Schema {
	t.Helper()
	schema, err := Compile([]byte(raw))
	if err != nil {
		t.Fatalf("Compile(%s): %v", raw, err)
	}
	return schema
}

func mustDecode(t *testing.T, raw string) any {
	t.Helper()
	value, err := DecodeJSON([]byte(raw))
	if err != nil {
		t.Fatalf("DecodeJSON(%s): %v", raw, err)
	}
	return value
}

func TestValidateAcceptsDeclaredValues(t *testing.T) {
	schema := mustCompile(t, `{
		"type": "object",
		"additionalProperties": false,
		"required": ["name", "count", "tags", "nested"],
		"properties": {
			"name":   {"type": "string", "minLength": 2, "maxLength": 8, "pattern": "^[a-z]+$"},
			"count":  {"type": "integer", "minimum": 1, "maximum": 10},
			"ratio":  {"type": "number", "minimum": 0, "maximum": 1},
			"tags":   {"type": "array", "items": {"type": "string", "enum": ["a", "b"]}},
			"nested": {"type": "object", "properties": {"flag": {"type": "boolean"}}},
			"choice": {"oneOf": [{"type": "string"}, {"type": "integer"}]},
			"fixed":  {"const": "only"}
		}
	}`)
	value := mustDecode(t, `{
		"name": "abc",
		"count": 3,
		"ratio": 0.5,
		"tags": ["a", "b"],
		"nested": {"flag": true},
		"choice": 7,
		"fixed": "only"
	}`)
	if err := schema.Validate(value); err != nil {
		t.Fatalf("expected valid value, got %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name   string
		schema string
		value  string
	}{
		{"type mismatch", `{"type":"object"}`, `[]`},
		{"missing required", `{"type":"object","required":["a"]}`, `{}`},
		{"additional property", `{"type":"object","additionalProperties":false,"properties":{"a":{"type":"string"}}}`, `{"a":"x","b":1}`},
		{"enum miss", `{"type":"string","enum":["a","b"]}`, `"c"`},
		{"const miss", `{"const":1}`, `2`},
		{"below minimum", `{"type":"integer","minimum":2}`, `1`},
		{"above maximum", `{"type":"number","maximum":1.5}`, `1.51`},
		{"short string", `{"type":"string","minLength":3}`, `"ab"`},
		{"long string", `{"type":"string","maxLength":2}`, `"abc"`},
		{"pattern miss", `{"type":"string","pattern":"^[0-9]+$"}`, `"12a"`},
		{"items miss", `{"type":"array","items":{"type":"integer"}}`, `[1,"2"]`},
		{"nested miss", `{"type":"object","properties":{"a":{"type":"string"}}}`, `{"a":1}`},
		{"oneOf none", `{"oneOf":[{"type":"string"},{"type":"boolean"}]}`, `1`},
		{"oneOf two", `{"oneOf":[{"type":"number"},{"type":"integer"}]}`, `1`},
		{"null rejected", `{"type":"object"}`, `null`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			schema := mustCompile(t, testCase.schema)
			value := mustDecode(t, testCase.value)
			err := schema.Validate(value)
			if err == nil {
				t.Fatalf("expected validation error for %s against %s", testCase.value, testCase.schema)
			}
			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf("expected *ValidationError, got %T: %v", err, err)
			}
		})
	}
}

func TestIntegerRejectsDecimalAndExponent(t *testing.T) {
	schema := mustCompile(t, `{"type":"integer"}`)
	for _, raw := range []string{`1.0`, `1e2`, `1E2`, `-1.5`, `0.0`} {
		value := mustDecode(t, raw)
		if err := schema.Validate(value); err == nil {
			t.Fatalf("expected integer rejection for %s", raw)
		}
	}
	for _, raw := range []string{`0`, `-1`, `9007199254740993`} {
		value := mustDecode(t, raw)
		if err := schema.Validate(value); err != nil {
			t.Fatalf("expected integer acceptance for %s: %v", raw, err)
		}
	}
	number := mustCompile(t, `{"type":"number"}`)
	for _, raw := range []string{`1.0`, `1e2`, `-0.5`} {
		if err := number.Validate(mustDecode(t, raw)); err != nil {
			t.Fatalf("expected number acceptance for %s: %v", raw, err)
		}
	}
}

func TestEnumComparesNumbersNumerically(t *testing.T) {
	schema := mustCompile(t, `{"enum":[1,2.5]}`)
	if err := schema.Validate(mustDecode(t, `1.0`)); err != nil {
		t.Fatalf("enum should compare numbers numerically: %v", err)
	}
	if err := schema.Validate(mustDecode(t, `2.50`)); err != nil {
		t.Fatalf("enum should compare numbers numerically: %v", err)
	}
	if err := schema.Validate(mustDecode(t, `3`)); err == nil {
		t.Fatal("expected enum miss")
	}
}

func TestCompileRejectsUnsupportedOrInconsistentKeywords(t *testing.T) {
	cases := []string{
		`{"$ref":"#/foo"}`,
		`{"type":"object","allOf":[]}`,
		`{"type":"object","additionalProperties":{"type":"string"}}`,
		`{"type":"string","minimum":1}`,
		`{"type":"integer","minLength":1}`,
		`{"type":"object","properties":[]}`,
		`{"type":"integer","enum":[]}`,
		`{"type":"bogus"}`,
		`{"type":"string","pattern":"("}`,
		`{"type":"string","minLength":-1}`,
		`{"type":"integer","minimum":5,"maximum":1}`,
		`{"type":"string","minLength":5,"maxLength":1}`,
		`{"type":"array","items":[]}`,
		`{"oneOf":[]}`,
		`[]`,
		`{"type":"object","properties":{"a":{"unknown":"x"}}}`,
		`{"type":"integer","minimum":"1"}`,
	}
	for _, raw := range cases {
		if _, err := Compile([]byte(raw)); err == nil {
			t.Fatalf("expected Compile to reject %s", raw)
		} else if !errors.Is(err, ErrInvalidSchema) {
			t.Fatalf("expected ErrInvalidSchema for %s, got %v", raw, err)
		}
	}
}

func TestCompileRejectsDuplicateSchemaKeys(t *testing.T) {
	if _, err := Compile([]byte(`{"type":"string","type":"integer"}`)); err == nil {
		t.Fatal("expected duplicate JSON key rejection")
	}
	if _, err := Compile([]byte(`{"type":"object","properties":{"a":{"type":"string"},"a":{"type":"integer"}}}`)); err == nil {
		t.Fatal("expected nested duplicate JSON key rejection")
	}
}

func TestCompileAcceptsAnnotations(t *testing.T) {
	schema := mustCompile(t, `{"title":"A","description":"B","type":"string"}`)
	if err := schema.Validate("x"); err != nil {
		t.Fatalf("annotations should be ignored: %v", err)
	}
}

func TestPatternIsCompiledOnceAndReusable(t *testing.T) {
	schema := mustCompile(t, `{"type":"string","pattern":"^[A-Z][A-Z0-9_]{0,63}$"}`)
	if schema.pattern == nil {
		t.Fatal("expected compiled pattern")
	}
	var waitGroup sync.WaitGroup
	for range 32 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for range 100 {
				if err := schema.Validate("CAUSE_1"); err != nil {
					t.Errorf("valid value rejected: %v", err)
					return
				}
				if err := schema.Validate("bad cause"); err == nil {
					t.Error("invalid value accepted")
					return
				}
			}
		}()
	}
	waitGroup.Wait()
}

func TestDecodeJSONRejectsTrailingDataAndDuplicates(t *testing.T) {
	cases := []string{
		`{"a":1}{"b":2}`,
		`{"a":1} extra`,
		`{"a":1,"a":2}`,
		`{"a":{"b":1,"b":2}}`,
		``,
		`   `,
		`{`,
	}
	for _, raw := range cases {
		if _, err := DecodeJSON([]byte(raw)); err == nil {
			t.Fatalf("expected DecodeJSON to reject %q", raw)
		} else if !errors.Is(err, ErrInvalidJSON) {
			t.Fatalf("expected ErrInvalidJSON for %q, got %v", raw, err)
		}
	}
	value, err := DecodeJSON([]byte(`{"n":1,"value":1.5}`))
	if err != nil {
		t.Fatal(err)
	}
	object := value.(map[string]any)
	if _, ok := object["n"].(json.Number); !ok {
		t.Fatalf("expected json.Number, got %T", object["n"])
	}
	if object["n"].(json.Number).String() != "1" {
		t.Fatalf("unexpected number encoding: %s", object["n"])
	}
}

func TestValidateNilSchemaIsNoOp(t *testing.T) {
	var schema *Schema
	if err := schema.Validate("anything"); err != nil {
		t.Fatalf("nil schema should accept any value: %v", err)
	}
}

func TestValidationErrorPath(t *testing.T) {
	schema := mustCompile(t, `{
		"type":"object",
		"properties":{"items":{"type":"array","items":{"type":"object","required":["name"]}}}
	}`)
	err := schema.Validate(mustDecode(t, `{"items":[{"name":"ok"},{}]}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "$.items[1]") {
		t.Fatalf("expected path in error, got %v", err)
	}
}

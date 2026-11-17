package jsonx

import (
	"errors"
	"testing"
)

func TestDecodeRejectsDuplicateKeysAndTrailingData(t *testing.T) {
	for _, raw := range []string{
		`{"a":1,"a":2}`,
		`{"a":{"b":1,"b":2}}`,
		`{"a":1} {"b":2}`,
		`{"a":1} trailing`,
		`{"a":`,
		`{"a":1}]`,
	} {
		if _, err := Decode([]byte(raw)); err == nil {
			t.Fatalf("Decode(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestDecodePreservesNumbersAndNestedValues(t *testing.T) {
	value, err := Decode([]byte(`{"n":5,"f":1.5,"a":[1,{"b":true}],"s":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value has type %T", value)
	}
	if _, ok := object["n"].(interface{ Int64() (int64, error) }); !ok {
		t.Fatalf("integer token is not json.Number: %T", object["n"])
	}
	items, ok := object["a"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("nested array not preserved: %#v", object["a"])
	}
}

func TestDecodeObjectRequiresObjectRoot(t *testing.T) {
	if _, err := DecodeObject([]byte(`[1]`)); err == nil {
		t.Fatal("array root unexpectedly accepted")
	}
	if _, err := DecodeObject([]byte(`{"a":1}`)); err != nil {
		t.Fatalf("object root rejected: %v", err)
	}
}

func TestDecodeReportsDuplicateKeySentinel(t *testing.T) {
	_, err := Decode([]byte(`{"a":1,"a":2}`))
	if !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("error = %v, want ErrDuplicateKey", err)
	}
}

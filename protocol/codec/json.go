package codec

import (
	"bytes"
	stdjson "encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/free5gc/ngap/aper"
)

var (
	bitStringType  = reflect.TypeOf(aper.BitString{})
	octetStringTyp = reflect.TypeOf(aper.OctetString(nil))
)

// ProjectToJSON converts a typed Free5GC value into the CA canonical JSON
// projection. The projection preserves the distinctions that encoding/json
// alone loses: CHOICE alternatives carry their concrete type name, BIT STRING
// carries its bit length, and binary values are hex strings.
//
// The projection is not a second protocol model. It is a reversible value view
// over the pinned upstream Go types; if a type is not reversible, callers must
// treat it as decode-projection-only (see the capability manifest).
func ProjectToJSON(value any) ([]byte, error) {
	projected, err := projectValue(reflect.ValueOf(value))
	if err != nil {
		return nil, err
	}
	return stdjson.Marshal(projected)
}

// ProjectFromJSON builds a typed Free5GC value from typeName (for example
// nas.message.PDUSessEstReq) and a canonical projection.
func ProjectFromJSON(typeName string, data []byte) (any, error) {
	ctor, ok := upstreamTypes[typeName]
	if !ok {
		return nil, codecError("JSON_TYPE_UNSUPPORTED", "no generated Free5GC type named %q", typeName)
	}
	value := reflect.ValueOf(ctor())
	decoder := stdjson.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var node any
	if err := decoder.Decode(&node); err != nil {
		return nil, codecError("JSON_INVALID", "invalid canonical JSON: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, codecError("JSON_INVALID", "canonical JSON must contain exactly one value")
	}
	if err := populateValue(value.Elem(), node); err != nil {
		return nil, err
	}
	return value.Interface(), nil
}

func projectValue(v reflect.Value) (any, error) {
	if !v.IsValid() {
		return nil, nil
	}
	for v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, nil
		}
		value, err := projectValue(v.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"_choice": qualifiedGoType(v.Interface(), ""),
			"value":   value,
		}, nil
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil
		}
		return projectValue(v.Elem())
	}
	if v.Type() == bitStringType {
		bs := v.Interface().(aper.BitString)
		return map[string]any{"hex": fmt.Sprintf("%x", bs.Bytes), "bitLength": bs.BitLength}, nil
	}
	if v.Type() == octetStringTyp {
		if v.IsNil() {
			return nil, nil
		}
		return fmt.Sprintf("%x", []byte(v.Interface().(aper.OctetString))), nil
	}

	switch v.Kind() {
	case reflect.Bool:
		return v.Bool(), nil
	case reflect.String:
		return v.String(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint(), nil
	case reflect.Float32, reflect.Float64:
		return v.Float(), nil
	case reflect.Slice:
		if v.IsNil() {
			return nil, nil
		}
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return fmt.Sprintf("%x", v.Bytes()), nil
		}
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			item, err := projectValue(v.Index(i))
			if err != nil {
				return nil, err
			}
			out[i] = item
		}
		return out, nil
	case reflect.Array:
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			item, err := projectValue(v.Index(i))
			if err != nil {
				return nil, err
			}
			out[i] = item
		}
		return out, nil
	case reflect.Map:
		out := map[string]any{}
		iter := v.MapRange()
		for iter.Next() {
			item, err := projectValue(iter.Value())
			if err != nil {
				return nil, err
			}
			out[fmt.Sprint(iter.Key().Interface())] = item
		}
		return out, nil
	case reflect.Struct:
		out := map[string]any{}
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			if tag := field.Tag.Get("json"); tag == "-" {
				continue
			}
			item, err := projectValue(v.Field(i))
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", t.Name(), field.Name, err)
			}
			out[field.Name] = item
		}
		return out, nil
	default:
		return nil, codecError("JSON_TYPE_UNSUPPORTED", "cannot project Go kind %s", v.Kind())
	}
}

func populateValue(target reflect.Value, node any) error {
	if !target.IsValid() || !target.CanSet() {
		return codecError("JSON_TARGET_INVALID", "target is not settable")
	}
	if node == nil {
		switch target.Kind() {
		case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map:
			target.SetZero()
			return nil
		default:
			return codecError("JSON_TYPE_MISMATCH", "null is not valid for %s", target.Type())
		}
	}
	if target.Kind() == reflect.Interface {
		obj, ok := node.(map[string]any)
		if !ok {
			return codecError("JSON_CHOICE_INVALID", "interface value must be a CHOICE object")
		}
		choiceName, _ := obj["_choice"].(string)
		if choiceName == "" {
			return codecError("JSON_CHOICE_INVALID", "CHOICE object is missing _choice")
		}
		ctor, ok := upstreamTypes[choiceName]
		if !ok {
			return codecError("JSON_TYPE_UNSUPPORTED", "no generated Free5GC type named %q", choiceName)
		}
		value := reflect.ValueOf(ctor())
		if !value.Type().AssignableTo(target.Type()) {
			return codecError("JSON_CHOICE_INVALID", "%s does not implement %s", choiceName, target.Type())
		}
		if err := populateValue(value.Elem(), obj["value"]); err != nil {
			return err
		}
		target.Set(value)
		return nil
	}
	if target.Kind() == reflect.Pointer {
		ptr := reflect.New(target.Type().Elem())
		if err := populateValue(ptr.Elem(), node); err != nil {
			return err
		}
		target.Set(ptr)
		return nil
	}
	if target.Type() == bitStringType {
		obj, ok := node.(map[string]any)
		if !ok {
			return codecError("JSON_BITSTRING_INVALID", "BIT STRING must be an object")
		}
		hexValue, _ := obj["hex"].(string)
		raw, err := decodeHex(hexValue)
		if err != nil {
			return codecError("JSON_BITSTRING_INVALID", "%v", err)
		}
		bits, err := numberToInt(obj["bitLength"])
		if err != nil || bits < 0 {
			return codecError("JSON_BITSTRING_INVALID", "invalid bitLength")
		}
		if bits > int64(len(raw))*8 {
			return codecError("JSON_BITSTRING_INVALID", "bitLength %d exceeds the %d-bit payload", bits, len(raw)*8)
		}
		target.Set(reflect.ValueOf(aper.BitString{Bytes: raw, BitLength: uint64(bits)}))
		return nil
	}

	switch target.Kind() {
	case reflect.Bool:
		value, ok := node.(bool)
		if !ok {
			return codecError("JSON_TYPE_MISMATCH", "expected boolean")
		}
		target.SetBool(value)
	case reflect.String:
		value, ok := node.(string)
		if !ok {
			return codecError("JSON_TYPE_MISMATCH", "expected string")
		}
		target.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := numberToInt(node)
		if err != nil {
			return err
		}
		if target.OverflowInt(value) {
			return codecError("JSON_TYPE_MISMATCH", "integer %d does not fit %s", value, target.Type())
		}
		target.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, err := numberToUint(node)
		if err != nil {
			return err
		}
		if target.OverflowUint(value) {
			return codecError("JSON_TYPE_MISMATCH", "integer %d does not fit %s", value, target.Type())
		}
		target.SetUint(value)
	case reflect.Float32, reflect.Float64:
		value, ok := node.(stdjson.Number)
		if !ok {
			return codecError("JSON_TYPE_MISMATCH", "expected number")
		}
		f, err := value.Float64()
		if err != nil {
			return codecError("JSON_TYPE_MISMATCH", "%v", err)
		}
		if target.OverflowFloat(f) {
			return codecError("JSON_TYPE_MISMATCH", "number does not fit %s", target.Type())
		}
		target.SetFloat(f)
	case reflect.Slice:
		if target.Type().Elem().Kind() == reflect.Uint8 {
			switch typed := node.(type) {
			case string:
				raw, err := decodeHex(typed)
				if err != nil {
					return codecError("JSON_BINARY_INVALID", "%v", err)
				}
				target.SetBytes(raw)
			case []any:
				raw := make([]byte, len(typed))
				for i, item := range typed {
					value, err := numberToUint(item)
					if err != nil || value > 255 {
						return codecError("JSON_BINARY_INVALID", "byte value out of range")
					}
					raw[i] = byte(value)
				}
				target.SetBytes(raw)
			default:
				return codecError("JSON_BINARY_INVALID", "binary value must be a hex string")
			}
			return nil
		}
		items, ok := node.([]any)
		if !ok {
			return codecError("JSON_TYPE_MISMATCH", "expected array")
		}
		slice := reflect.MakeSlice(target.Type(), len(items), len(items))
		for i, item := range items {
			if err := populateValue(slice.Index(i), item); err != nil {
				return err
			}
		}
		target.Set(slice)
	case reflect.Array:
		items, ok := node.([]any)
		if !ok || len(items) != target.Len() {
			return codecError("JSON_TYPE_MISMATCH", "expected array of length %d", target.Len())
		}
		for i, item := range items {
			if err := populateValue(target.Index(i), item); err != nil {
				return err
			}
		}
	case reflect.Map:
		obj, ok := node.(map[string]any)
		if !ok {
			return codecError("JSON_TYPE_MISMATCH", "expected object")
		}
		m := reflect.MakeMapWithSize(target.Type(), len(obj))
		for key, item := range obj {
			keyValue := reflect.New(target.Type().Key()).Elem()
			if keyValue.Kind() != reflect.String {
				return codecError("JSON_TYPE_UNSUPPORTED", "only string map keys are supported")
			}
			keyValue.SetString(key)
			value := reflect.New(target.Type().Elem()).Elem()
			if err := populateValue(value, item); err != nil {
				return err
			}
			m.SetMapIndex(keyValue, value)
		}
		target.Set(m)
	case reflect.Struct:
		obj, ok := node.(map[string]any)
		if !ok {
			return codecError("JSON_TYPE_MISMATCH", "expected object")
		}
		t := target.Type()
		for key, item := range obj {
			fieldIndex := fieldIndexByName(t, key)
			if fieldIndex < 0 {
				return codecError("JSON_FIELD_UNKNOWN", "%s has no exported field %q", t.Name(), key)
			}
			if err := populateValue(target.Field(fieldIndex), item); err != nil {
				return fmt.Errorf("%s.%s: %w", t.Name(), key, err)
			}
		}
	default:
		return codecError("JSON_TYPE_UNSUPPORTED", "cannot populate Go kind %s", target.Kind())
	}
	return nil
}

func fieldIndexByName(t reflect.Type, name string) int {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		if field.Name == name {
			return i
		}
	}
	return -1
}

func decodeHex(value string) ([]byte, error) {
	value = strings.TrimPrefix(strings.TrimPrefix(value, "0x"), "0X")
	if len(value)%2 != 0 {
		return nil, fmt.Errorf("hex string has odd length")
	}
	out := make([]byte, len(value)/2)
	for i := 0; i < len(out); i++ {
		high, ok := hexNibble(value[i*2])
		if !ok {
			return nil, fmt.Errorf("invalid hex digit")
		}
		low, ok := hexNibble(value[i*2+1])
		if !ok {
			return nil, fmt.Errorf("invalid hex digit")
		}
		out[i] = high<<4 | low
	}
	return out, nil
}

func hexNibble(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

func numberToInt(value any) (int64, error) {
	number, ok := value.(stdjson.Number)
	if !ok {
		return 0, codecError("JSON_TYPE_MISMATCH", "expected integer")
	}
	parsed, err := number.Int64()
	if err != nil {
		return 0, codecError("JSON_TYPE_MISMATCH", "invalid integer: %v", err)
	}
	return parsed, nil
}

func numberToUint(value any) (uint64, error) {
	number, ok := value.(stdjson.Number)
	if !ok {
		return 0, codecError("JSON_TYPE_MISMATCH", "expected unsigned integer")
	}
	parsed, err := number.Int64()
	if err != nil || parsed < 0 {
		return 0, codecError("JSON_TYPE_MISMATCH", "invalid unsigned integer")
	}
	return uint64(parsed), nil
}

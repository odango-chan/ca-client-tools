// Package jsonx provides the strict JSON decoding rules shared by the CLI,
// the typed OpenAMSM clients and the Provider protocol. It rejects duplicate
// object keys, trailing data and non-object roots where required, and it
// preserves numeric tokens as json.Number so integer-only wire fields are not
// silently accepted as floats.
package jsonx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// ErrDuplicateKey reports a JSON object containing the same key more than once.
var ErrDuplicateKey = errors.New("duplicate JSON object key")

// ErrMultipleValues reports trailing non-whitespace JSON content.
var ErrMultipleValues = errors.New("multiple JSON values")

// Decode decodes exactly one strict JSON value. Object keys must be unique and
// no value may follow the first one.
func Decode(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, ErrMultipleValues
		}
		return nil, err
	}
	return value, nil
}

// DecodeObject decodes exactly one strict JSON object.
func DecodeObject(raw []byte) (map[string]any, error) {
	value, err := Decode(raw)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("JSON value is not an object")
	}
	return object, nil
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
				return nil, errors.New("JSON object key is not a string")
			}
			if _, duplicate := object[key]; duplicate {
				return nil, ErrDuplicateKey
			}
			value, err := decodeValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return nil, errors.New("unterminated JSON object")
		}
		return object, nil
	case '[':
		values := make([]any, 0)
		for decoder.More() {
			value, err := decodeValue(decoder)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return nil, errors.New("unterminated JSON array")
		}
		return values, nil
	default:
		return nil, errors.New("unexpected JSON delimiter")
	}
}

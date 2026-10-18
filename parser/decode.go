package parser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// orderedObject is a decoded JSON object that remembers key order. Go maps
// iterate randomly, so order must be captured to keep generation deterministic.
type orderedObject struct {
	keys   []string
	values map[string]any
}

func (o *orderedObject) get(key string) (any, bool) {
	value, ok := o.values[key]
	return value, ok
}

// decodeDocument decodes JSON into *orderedObject, []any, json.Number,
// string, bool or nil values.
func decodeDocument(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	value, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected data after top-level value")
	}
	return value, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return token, nil
	}
	switch delim {
	case '{':
		return decodeObject(dec)
	case '[':
		return decodeArray(dec)
	}
	return nil, fmt.Errorf("unexpected delimiter %q", delim)
}

func decodeObject(dec *json.Decoder) (*orderedObject, error) {
	obj := &orderedObject{values: map[string]any{}}
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, isString := token.(string)
		if !isString {
			return nil, fmt.Errorf("unexpected object key %v", token)
		}
		value, err := decodeValue(dec)
		if err != nil {
			return nil, err
		}
		if _, seen := obj.values[key]; !seen {
			obj.keys = append(obj.keys, key)
		}
		obj.values[key] = value
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return obj, nil
}

func decodeArray(dec *json.Decoder) ([]any, error) {
	items := []any{}
	for dec.More() {
		item, err := decodeValue(dec)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return items, nil
}

// plainValue converts decoded values into the types the generator emits:
// map[string]any, []any, int64, float64, string, bool and nil.
func plainValue(raw any) any {
	switch value := raw.(type) {
	case *orderedObject:
		plain := make(map[string]any, len(value.keys))
		for _, key := range value.keys {
			plain[key] = plainValue(value.values[key])
		}
		return plain
	case []any:
		plain := make([]any, len(value))
		for i, item := range value {
			plain[i] = plainValue(item)
		}
		return plain
	case json.Number:
		if integer, err := value.Int64(); err == nil {
			return integer
		}
		float, _ := value.Float64()
		return float
	}
	return raw
}

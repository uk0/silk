package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

var (
	ErrJSONPointer  = errors.New("httpx: invalid JSON pointer")
	ErrMissingField = errors.New("httpx: JSON field not found")
	ErrType         = errors.New("httpx: unexpected JSON value type")
)

// JSON selects an RFC 6901 JSON Pointer (not JSONPath): "" is the root,
// /items/0/name selects an array entry, ~0 escapes ~ and ~1 escapes /.
// Each call returns independent values, preserving json.Number precision.
func (r *Response) JSON(pointer string) (any, error) {
	d := json.NewDecoder(bytes.NewReader(r.body))
	d.UseNumber()
	var value any
	if err := d.Decode(&value); err != nil {
		return nil, fmt.Errorf("httpx: decode JSON: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, errors.New("httpx: response must contain exactly one JSON value")
	}
	if pointer == "" {
		return value, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, ErrJSONPointer
	}
	for _, part := range strings.Split(pointer[1:], "/") {
		var key strings.Builder
		for i := 0; i < len(part); i++ {
			if part[i] != '~' {
				key.WriteByte(part[i])
				continue
			}
			i++
			if i >= len(part) {
				return nil, ErrJSONPointer
			}
			switch part[i] {
			case '0':
				key.WriteByte('~')
			case '1':
				key.WriteByte('/')
			default:
				return nil, ErrJSONPointer
			}
		}
		token := key.String()
		switch v := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = v[token]
			if !ok {
				return nil, ErrMissingField
			}
		case []any:
			if token == "" || (len(token) > 1 && token[0] == '0') {
				return nil, ErrJSONPointer
			}
			for _, c := range token {
				if c < '0' || c > '9' {
					return nil, ErrJSONPointer
				}
			}
			n, err := strconv.ParseUint(token, 10, 64)
			if err != nil || n >= uint64(len(v)) {
				return nil, ErrMissingField
			}
			value = v[n]
		default:
			return nil, ErrMissingField
		}
	}
	return value, nil
}
func (r *Response) Str(pointer string) (string, error) {
	v, err := r.JSON(pointer)
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", ErrType
	}
	return s, nil
}

// Int returns int64, never truncating fractions or rounding through float64.
func (r *Response) Int(pointer string) (int64, error) {
	v, err := r.JSON(pointer)
	if err != nil {
		return 0, err
	}
	n, ok := v.(json.Number)
	if !ok {
		return 0, ErrType
	}
	return n.Int64()
}
func (r *Response) Float(pointer string) (float64, error) {
	v, err := r.JSON(pointer)
	if err != nil {
		return 0, err
	}
	n, ok := v.(json.Number)
	if !ok {
		return 0, ErrType
	}
	f, err := n.Float64()
	if err != nil {
		return 0, err
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, ErrType
	}
	return f, nil
}
func (r *Response) Bool(pointer string) (bool, error) {
	v, err := r.JSON(pointer)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, ErrType
	}
	return b, nil
}

// Extract is the small-tool / designer adapter. str with an empty pointer is
// raw response text; str with a pointer requires a JSON string. json returns
// compact valid JSON text; int, float and bool return typed Go values.
func (r *Response) Extract(kind, pointer string) (any, error) {
	switch kind {
	case "str":
		if pointer == "" {
			return r.Text(), nil
		}
		return r.Str(pointer)
	case "json":
		v, err := r.JSON(pointer)
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(v)
		return string(b), err
	case "int":
		return r.Int(pointer)
	case "float":
		return r.Float(pointer)
	case "bool":
		return r.Bool(pointer)
	default:
		return nil, fmt.Errorf("httpx: unknown result kind %q", kind)
	}
}

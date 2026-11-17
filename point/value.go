// Package point provides strict scalar conversion and bounded HMI expressions.
// It deliberately does not execute Go code, access files or expose functions.
package point

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type Kind string

const (
	Float  Kind = "float"
	Int    Kind = "int"
	Bool   Kind = "bool"
	String Kind = "string"
)

func (k Kind) Valid() bool { return k == Float || k == Int || k == Bool || k == String }

// Convert never silently replaces a malformed value with zero. Integers must
// be exact, finite and in range; string conversions are explicit and trimmed.
func Convert(v any, k Kind) (any, error) {
	switch k {
	case String:
		switch x := v.(type) {
		case string:
			return x, nil
		case bool:
			return strconv.FormatBool(x), nil
		case int:
			return strconv.Itoa(x), nil
		case int64:
			return strconv.FormatInt(x, 10), nil
		case json.Number:
			// Formatting preserves the original token, not a rounded float.
			// Validation need not impose the float conversion's integer limit.
			if n, e := strconv.ParseFloat(x.String(), 64); e != nil || !finite(n) {
				return nil, fmt.Errorf("point: invalid finite JSON number")
			}
			return x.String(), nil
		case float64:
			if finite(x) {
				return strconv.FormatFloat(x, 'g', -1, 64), nil
			}
		}
	case Bool:
		switch x := v.(type) {
		case bool:
			return x, nil
		case string:
			s := strings.TrimSpace(x)
			if s == "true" || s == "1" {
				return true, nil
			}
			if s == "false" || s == "0" {
				return false, nil
			}
		}
		if n, e := number(v); e == nil && (n == 0 || n == 1) {
			return n == 1, nil
		}
	case Float:
		return number(v)
	case Int:
		switch x := v.(type) {
		case int64:
			return x, nil
		case int:
			return int64(x), nil
		case string:
			n, e := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
			if e != nil {
				return nil, fmt.Errorf("point: invalid exact integer string")
			}
			return n, nil
		case json.Number:
			n, e := x.Int64()
			if e != nil {
				return nil, fmt.Errorf("point: invalid exact integer JSON number")
			}
			return n, nil
		}
		n, e := number(v)
		if e == nil && math.Trunc(n) == n && n >= -0x1p63 && n < 0x1p63 {
			return int64(n), nil
		}
	}
	return nil, fmt.Errorf("point: cannot convert %T to %s", v, k)
}
func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func number(v any) (float64, error) {
	var n float64
	var err error
	switch x := v.(type) {
	case float64:
		n = x
	case float32:
		n = float64(x)
	case int:
		return number(int64(x))
	case int64:
		n = float64(x)
		if x > 1<<53 || x < -(1<<53) {
			return 0, fmt.Errorf("point: integer loses float precision")
		}
	case bool:
		if x {
			n = 1
		}
	case string:
		n, err = strconv.ParseFloat(strings.TrimSpace(x), 64)
	case json.Number:
		if i, integer, e := jsonInteger(x); integer {
			if e != nil {
				return 0, e
			}
			return number(i)
		}
		n, err = x.Float64()
	default:
		err = fmt.Errorf("unsupported scalar")
	}
	if err != nil || !finite(n) {
		return 0, fmt.Errorf("point: invalid finite number")
	}
	return n, nil
}

// Definition separates naming/category (DI/AI/BI) from actual scalar type.
// Category is project-defined. Define never silently changes an existing type.
type Definition struct {
	Name        string `json:"name"`
	Kind        Kind   `json:"kind"`
	Category    string `json:"category,omitempty"`
	Unit        string `json:"unit,omitempty"`
	Description string `json:"description,omitempty"`
}

func (d Definition) Validate() error {
	if d.Name == "" || strings.TrimSpace(d.Name) != d.Name || len(d.Name) > 256 || !d.Kind.Valid() || len(d.Category) > 128 || len(d.Unit) > 128 || len(d.Description) > 4096 {
		return fmt.Errorf("point: invalid definition")
	}
	return nil
}

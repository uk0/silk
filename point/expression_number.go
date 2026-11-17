package point

import (
	"encoding/json"
	"fmt"
	"go/token"
	"math"
	"strings"
)

type expressionNumber struct {
	i       int64
	f       float64
	integer bool
}

func expressionNumeric(v any) (expressionNumber, error) {
	switch n := v.(type) {
	case int:
		return expressionNumber{i: int64(n), integer: true}, nil
	case int64:
		return expressionNumber{i: n, integer: true}, nil
	case json.Number:
		if i, integer, err := jsonInteger(n); integer {
			return expressionNumber{i: i, integer: true}, err
		}
	}
	f, err := number(v)
	return expressionNumber{f: f}, err
}

// Integral JSON tokens are signed int64 values, not silently rounded floats.
// Decimal/exponent tokens and explicit string-to-float casts retain float semantics.
func jsonInteger(n json.Number) (int64, bool, error) {
	if strings.ContainsAny(string(n), ".eE") {
		return 0, false, nil
	}
	i, err := n.Int64()
	if err != nil {
		return 0, true, fmt.Errorf("point: JSON integer outside int64 range")
	}
	return i, true, nil
}

func (n expressionNumber) floatValue() (float64, error) {
	if n.integer {
		// Keep the SDK's existing conservative exact-float conversion contract.
		return number(n.i)
	}
	return n.f, nil
}

// CompareNumbers returns -1, 0 or 1 for finite numeric values. Signed integers
// (int/int64 and integral JSON tokens) compare exactly, even against float64
// boundaries outside the exact-integer float range. It never rounds an integer
// to float before comparing. Other scalar coercions match expression arithmetic;
// use Convert(..., Int) explicitly for exact integer strings.
func CompareNumbers(a, b any) (int, error) {
	x, err := expressionNumeric(a)
	if err != nil {
		return 0, err
	}
	y, err := expressionNumeric(b)
	if err != nil {
		return 0, err
	}
	if x.integer && y.integer {
		if x.i < y.i {
			return -1, nil
		}
		if x.i > y.i {
			return 1, nil
		}
		return 0, nil
	}
	if x.integer {
		return compareIntegerFloat(x.i, y.f), nil
	}
	if y.integer {
		return -compareIntegerFloat(y.i, x.f), nil
	}
	if x.f < y.f {
		return -1, nil
	}
	if x.f > y.f {
		return 1, nil
	}
	return 0, nil
}

func compareIntegerFloat(i int64, f float64) int {
	if f < -0x1p63 {
		return 1
	}
	if f >= 0x1p63 {
		return -1
	}
	// Bounds above make the truncating float-to-int conversion portable. If the
	// integer parts match, only the represented fractional part decides order.
	n := int64(f)
	if i < n {
		return -1
	}
	if i > n {
		return 1
	}
	if float64(n) < f {
		return -1
	}
	if float64(n) > f {
		return 1
	}
	return 0
}

func unaryNumber(op token.Token, v any) (any, error) {
	n, err := expressionNumeric(v)
	if err != nil {
		return nil, err
	}
	if op == token.XOR {
		if !n.integer {
			return nil, fmt.Errorf("point: bit operations require int values")
		}
		return ^n.i, nil
	}
	if n.integer {
		if op == token.SUB {
			if n.i == math.MinInt64 {
				return nil, fmt.Errorf("point: integer arithmetic overflow")
			}
			return -n.i, nil
		}
		return n.i, nil
	}
	if op == token.SUB {
		return -n.f, nil
	}
	return n.f, nil
}

func binaryNumber(op token.Token, a, b any) (any, error) {
	x, err := expressionNumeric(a)
	if err != nil {
		return nil, err
	}
	y, err := expressionNumeric(b)
	if err != nil {
		return nil, err
	}
	switch op {
	case token.REM, token.AND, token.OR, token.XOR, token.AND_NOT, token.SHL, token.SHR:
		if !x.integer || !y.integer {
			return nil, fmt.Errorf("point: bit/remainder operations require int values")
		}
	}
	if x.integer && y.integer && op != token.QUO {
		return binaryInteger(op, x.i, y.i)
	}
	f, err := x.floatValue()
	if err != nil {
		return nil, err
	}
	g, err := y.floatValue()
	if err != nil {
		return nil, err
	}
	var out float64
	switch op {
	case token.ADD:
		out = f + g
	case token.SUB:
		out = f - g
	case token.MUL:
		out = f * g
	case token.QUO:
		if g == 0 {
			return nil, fmt.Errorf("point: division by zero")
		}
		out = f / g
	default:
		return nil, fmt.Errorf("point: invalid numeric operation")
	}
	if !finite(out) {
		return nil, fmt.Errorf("point: arithmetic overflow")
	}
	return out, nil
}

func binaryInteger(op token.Token, a, b int64) (any, error) {
	var n int64
	switch op {
	case token.ADD:
		n = a + b
		if (a^b) >= 0 && (a^n) < 0 {
			break
		}
		return n, nil
	case token.SUB:
		n = a - b
		if (a^b) < 0 && (a^n) < 0 {
			break
		}
		return n, nil
	case token.MUL:
		if a == 0 || b == 0 {
			return int64(0), nil
		}
		if a == math.MinInt64 && b == -1 || b == math.MinInt64 && a == -1 {
			break
		}
		n = a * b
		if n/b == a {
			return n, nil
		}
	case token.REM:
		if b == 0 {
			return nil, fmt.Errorf("point: remainder by zero")
		}
		return a % b, nil
	case token.AND:
		return a & b, nil
	case token.OR:
		return a | b, nil
	case token.XOR:
		return a ^ b, nil
	case token.AND_NOT:
		return a &^ b, nil
	case token.SHL, token.SHR:
		if b < 0 || b > 63 {
			return nil, fmt.Errorf("point: shift count must be within 0..63")
		}
		if op == token.SHR {
			return a >> uint(b), nil
		}
		n = a << uint(b)
		if n>>uint(b) == a {
			return n, nil
		}
	default:
		return nil, fmt.Errorf("point: invalid integer operation")
	}
	return nil, fmt.Errorf("point: integer arithmetic overflow")
}

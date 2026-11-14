package point

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

// Expression is immutable and safe to evaluate concurrently. Only x, scalar
// constants, parentheses, arithmetic, comparisons and short-circuit boolean
// operators are accepted. No calls, indexing, allocation or loops are allowed.
type Expression struct{ root ast.Expr }

func Compile(source string) (*Expression, error) {
	if len(source) > 1024 {
		return nil, fmt.Errorf("point: expression exceeds 1024 bytes")
	}
	root, e := parser.ParseExpr(source)
	if e != nil {
		return nil, fmt.Errorf("point: expression syntax: %w", e)
	}
	nodes := 0
	var check func(ast.Expr) error
	check = func(n ast.Expr) error {
		nodes++
		if nodes > 128 {
			return fmt.Errorf("point: expression exceeds 128 nodes")
		}
		switch x := n.(type) {
		case *ast.ParenExpr:
			return check(x.X)
		case *ast.Ident:
			if x.Name == "x" || x.Name == "true" || x.Name == "false" {
				return nil
			}
		case *ast.BasicLit:
			if x.Kind == token.INT || x.Kind == token.FLOAT || x.Kind == token.STRING {
				_, e := literal(x)
				return e
			}
		case *ast.UnaryExpr:
			if x.Op == token.ADD || x.Op == token.SUB || x.Op == token.NOT {
				return check(x.X)
			}
		case *ast.BinaryExpr:
			switch x.Op {
			case token.ADD, token.SUB, token.MUL, token.QUO, token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ, token.LAND, token.LOR:
				if e := check(x.X); e != nil {
					return e
				}
				return check(x.Y)
			}
		}
		return fmt.Errorf("point: expression operation is not allowed")
	}
	if e = check(root); e != nil {
		return nil, e
	}
	return &Expression{root}, nil
}
func literal(x *ast.BasicLit) (any, error) {
	if x.Kind == token.STRING {
		return strconv.Unquote(x.Value)
	}
	n, e := strconv.ParseFloat(x.Value, 64)
	if e != nil || !finite(n) {
		return nil, fmt.Errorf("point: invalid numeric literal")
	}
	return n, nil
}
func (p *Expression) Evaluate(value any) (any, error) {
	if p == nil || p.root == nil {
		return nil, fmt.Errorf("point: expression not compiled")
	}
	var eval func(ast.Expr) (any, error)
	eval = func(n ast.Expr) (any, error) {
		switch x := n.(type) {
		case *ast.ParenExpr:
			return eval(x.X)
		case *ast.BasicLit:
			return literal(x)
		case *ast.Ident:
			switch x.Name {
			case "x":
				return value, nil
			case "true":
				return true, nil
			case "false":
				return false, nil
			}
		case *ast.UnaryExpr:
			v, e := eval(x.X)
			if e != nil {
				return nil, e
			}
			if x.Op == token.NOT {
				if b, ok := v.(bool); ok {
					return !b, nil
				}
				return nil, fmt.Errorf("point: ! requires bool")
			}
			f, e := number(v)
			if e != nil {
				return nil, e
			}
			if x.Op == token.SUB {
				f = -f
			}
			return f, nil
		case *ast.BinaryExpr:
			a, e := eval(x.X)
			if e != nil {
				return nil, e
			}
			if x.Op == token.LAND || x.Op == token.LOR {
				b, ok := a.(bool)
				if !ok {
					return nil, fmt.Errorf("point: logical operand requires bool")
				}
				if x.Op == token.LAND && !b || x.Op == token.LOR && b {
					return b, nil
				}
				v, e := eval(x.Y)
				if e != nil {
					return nil, e
				}
				if b, ok := v.(bool); ok {
					return b, nil
				}
				return nil, fmt.Errorf("point: logical operand requires bool")
			}
			b, e := eval(x.Y)
			if e != nil {
				return nil, e
			}
			if x.Op == token.EQL || x.Op == token.NEQ {
				eq := false
				switch v := a.(type) {
				case string:
					w, ok := b.(string)
					if !ok {
						return nil, fmt.Errorf("point: equality type mismatch")
					}
					eq = v == w
				case bool:
					w, ok := b.(bool)
					if !ok {
						return nil, fmt.Errorf("point: equality type mismatch")
					}
					eq = v == w
				default:
					f, e := number(a)
					if e != nil {
						return nil, e
					}
					g, e := number(b)
					if e != nil {
						return nil, e
					}
					eq = f == g
				}
				if x.Op == token.NEQ {
					eq = !eq
				}
				return eq, nil
			}
			f, e := number(a)
			if e != nil {
				return nil, e
			}
			g, e := number(b)
			if e != nil {
				return nil, e
			}
			var out float64
			switch x.Op {
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
			case token.REM:
				return nil, fmt.Errorf("point: remainder is not supported; use arithmetic conversion")
			case token.LSS:
				return f < g, nil
			case token.LEQ:
				return f <= g, nil
			case token.GTR:
				return f > g, nil
			case token.GEQ:
				return f >= g, nil
			}
			if !finite(out) {
				return nil, fmt.Errorf("point: arithmetic overflow")
			}
			return out, nil
		}
		return nil, fmt.Errorf("point: invalid expression")
	}
	return eval(p.root)
}

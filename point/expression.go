package point

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

// Expression is immutable and safe to evaluate concurrently. Only x, scalar
// constants, parentheses, arithmetic, integer bit operations, comparisons and short-circuit boolean
// operators are accepted. No calls, indexing, allocation or loops are allowed.
type Expression struct {
	root      ast.Expr
	constants map[ast.Expr]any
}

func Compile(source string) (*Expression, error) {
	if len(source) > 1024 {
		return nil, fmt.Errorf("point: expression exceeds 1024 bytes")
	}
	root, e := parser.ParseExpr(source)
	if e != nil {
		return nil, fmt.Errorf("point: expression syntax: %w", e)
	}
	nodes := 0
	constants := make(map[ast.Expr]any)
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
				v, e := literal(x)
				if e == nil {
					constants[x] = v
				}
				return e
			}
		case *ast.UnaryExpr:
			if x.Op == token.SUB {
				// Fold a signed literal before checking its positive magnitude:
				// MinInt64 has no representable positive int64 counterpart.
				lit, count := signedIntegerOperand(x.X)
				if lit != nil {
					nodes += count
					if nodes > 128 {
						return fmt.Errorf("point: expression exceeds 128 nodes")
					}
					v, err := strconv.ParseInt("-"+lit.Value, 0, 64)
					if err != nil {
						return fmt.Errorf("point: integer literal outside int64 range")
					}
					constants[x] = v
					return nil
				}
			}
			if x.Op == token.ADD || x.Op == token.SUB || x.Op == token.NOT || x.Op == token.XOR {
				return check(x.X)
			}
		case *ast.BinaryExpr:
			switch x.Op {
			case token.ADD, token.SUB, token.MUL, token.QUO, token.REM, token.AND, token.OR, token.XOR, token.AND_NOT, token.SHL, token.SHR, token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ, token.LAND, token.LOR:
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
	return &Expression{root: root, constants: constants}, nil
}

func signedIntegerOperand(n ast.Expr) (*ast.BasicLit, int) {
	count := 1
	for {
		p, ok := n.(*ast.ParenExpr)
		if !ok {
			break
		}
		n = p.X
		count++
	}
	if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.INT {
		return lit, count
	}
	return nil, 0
}
func literal(x *ast.BasicLit) (any, error) {
	if x.Kind == token.STRING {
		return strconv.Unquote(x.Value)
	}
	if x.Kind == token.INT {
		n, e := strconv.ParseInt(x.Value, 0, 64)
		if e != nil {
			return nil, fmt.Errorf("point: integer literal outside int64 range")
		}
		return n, nil
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
		if v, ok := p.constants[n]; ok {
			return v, nil
		}
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
			return unaryNumber(x.Op, v)
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
					cmp, e := CompareNumbers(a, b)
					if e != nil {
						return nil, e
					}
					eq = cmp == 0
				}
				if x.Op == token.NEQ {
					eq = !eq
				}
				return eq, nil
			}
			switch x.Op {
			case token.LSS, token.LEQ, token.GTR, token.GEQ:
				cmp, err := CompareNumbers(a, b)
				if err != nil {
					return nil, err
				}
				switch x.Op {
				case token.LSS:
					return cmp < 0, nil
				case token.LEQ:
					return cmp <= 0, nil
				case token.GTR:
					return cmp > 0, nil
				default:
					return cmp >= 0, nil
				}
			}
			return binaryNumber(x.Op, a, b)
		}
		return nil, fmt.Errorf("point: invalid expression")
	}
	return eval(p.root)
}

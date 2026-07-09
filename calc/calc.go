// Package calc implements 公式/计算变量 (calc / formula tags): a derived tag
// whose live value is an expression evaluated over other tags.
//
// A CalcTag names an Output tag, an Expr, and the Input tags the expression
// reads. Each Input name is used verbatim as an identifier inside Expr, so the
// Inputs MUST be simple identifiers — no dots, no paths (e.g. "a", "level", not
// "plant.a") — because a dot inside an expr expression means field access, not
// a tag name. The expression is compiled once; whenever any Input changes, the
// Output tag is recomputed.
//
// Example:
//
//	e := calc.NewEngine(db)
//	e.Add(calc.CalcTag{Output: "avg", Expr: "(a + b) / 2", Inputs: []string{"a", "b"}})
//	db.SetValue("a", 10.0)
//	db.SetValue("b", 20.0) // -> tag "avg" == 15
//
// Expressions use github.com/expr-lang/expr. Inputs are fed to the expression as
// float64 (via core.Value.Float); the result is coerced back to float64 for the
// Output tag.
package calc

import (
	"sync"

	"github.com/expr-lang/expr"

	"github.com/uk0/silk/core"
)

// CalcTag declares one derived tag: Output = Expr evaluated over Inputs.
//
// Inputs are referenced by name as bare identifiers in Expr (e.g.
// Expr:"(a + b) / 2", Inputs:[]string{"a","b"}) and therefore MUST be simple
// identifiers without dots.
type CalcTag struct {
	Output string   // name of the tag that receives the computed value
	Expr   string   // expression over the Input identifiers
	Inputs []string // input tag names, each a simple (dot-free) identifier
}

// Engine wires CalcTags into a core.TagDB: it compiles each expression and keeps
// the Output tags recomputed as their Inputs change. Add and RemoveAll are safe
// for concurrent use.
type Engine struct {
	tags *core.TagDB

	mu      sync.Mutex
	cancels []core.CancelFunc
}

// NewEngine returns an Engine backed by tags.
func NewEngine(tags *core.TagDB) *Engine {
	return &Engine{tags: tags}
}

// Add compiles ct.Expr once, subscribes to every Input tag, and recomputes the
// Output tag on Add and on any Input change. It returns the compile error for a
// malformed expression, in which case nothing is registered.
func (e *Engine) Add(ct CalcTag) error {
	// Env template: every input is presented as a float64 so the expression
	// type-checks against the values fed at run time.
	tmpl := make(map[string]any, len(ct.Inputs))
	for _, name := range ct.Inputs {
		tmpl[name] = float64(0)
	}
	program, err := expr.Compile(ct.Expr, expr.Env(tmpl))
	if err != nil {
		return err
	}

	out := e.tags.GetOrCreate(ct.Output, core.Meta{})
	inputs := make([]*core.Tag, len(ct.Inputs))
	for i, name := range ct.Inputs {
		inputs[i] = e.tags.GetOrCreate(name, core.Meta{})
	}

	eval := func() {
		env := make(map[string]any, len(ct.Inputs))
		for i, name := range ct.Inputs {
			env[name] = inputs[i].Value().Float()
		}
		res, err := expr.Run(program, env)
		if err != nil {
			return
		}
		out.SetValue(toFloat(res))
	}

	// Subscribe to each input. Subscribe primes the callback with the current
	// value, so registering the first input already computes Output once.
	e.mu.Lock()
	for _, t := range inputs {
		e.cancels = append(e.cancels, t.Subscribe(func(core.Value) { eval() }))
	}
	e.mu.Unlock()

	// Guarantee at least one evaluation even for a constant expression with no
	// Inputs, where no subscription primes it.
	eval()
	return nil
}

// RemoveAll unsubscribes every calc tag registered on this Engine.
func (e *Engine) RemoveAll() {
	e.mu.Lock()
	cancels := e.cancels
	e.cancels = nil
	e.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

// toFloat coerces an expr result (float64/int/bool, plus int64 for integer
// arithmetic) to the float64 that a tag stores.
func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	}
	return 0
}

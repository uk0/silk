package point

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Step is either an explicit conversion (type) or a bounded expression (expr).
// Expressions use x as the preceding result. The list is applied in order.
type Step struct {
	Type Kind   `json:"type,omitempty"`
	Expr string `json:"expr,omitempty"`
}
type Pipeline struct {
	steps       []Step
	expressions []*Expression
}

func NewPipeline(steps []Step) (*Pipeline, error) {
	if len(steps) > 16 {
		return nil, fmt.Errorf("point: at most 16 conversion steps")
	}
	p := &Pipeline{steps: append([]Step(nil), steps...), expressions: make([]*Expression, len(steps))}
	for i, s := range steps {
		if (s.Type != "") == (s.Expr != "") {
			return nil, fmt.Errorf("point: step %d requires exactly one of type or expr", i+1)
		}
		if s.Type != "" && !s.Type.Valid() {
			return nil, fmt.Errorf("point: invalid conversion type")
		}
		if s.Expr != "" {
			e, err := Compile(s.Expr)
			if err != nil {
				return nil, err
			}
			p.expressions[i] = e
		}
	}
	return p, nil
}
func ParsePipeline(text string) (*Pipeline, error) {
	if text == "" {
		return NewPipeline(nil)
	}
	var steps []Step
	if e := DecodeConfig(text, &steps); e != nil {
		return nil, e
	}
	return NewPipeline(steps)
}
func (p *Pipeline) Apply(v any) (any, error) {
	if p == nil {
		return v, nil
	}
	for i, s := range p.steps {
		var e error
		if s.Type != "" {
			v, e = Convert(v, s.Type)
		} else {
			v, e = p.expressions[i].Evaluate(v)
		}
		if e != nil {
			return nil, fmt.Errorf("point: step %d: %w", i+1, e)
		}
	}
	return v, nil
}

// DecodeConfig rejects unknown fields, trailing values and oversized designs.
func DecodeConfig(text string, dst any) error {
	if len(text) > 65536 {
		return fmt.Errorf("point: config exceeds 64 KiB")
	}
	d := json.NewDecoder(bytes.NewBufferString(text))
	d.DisallowUnknownFields()
	if e := d.Decode(dst); e != nil {
		return e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return fmt.Errorf("point: expected exactly one JSON value")
	}
	return nil
}

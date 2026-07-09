// Package script runs Go scripts inside silk at runtime (via a yaegi
// interpreter) with the tag API pre-bound, so designer-authored logic — derived
// tags, alarm actions, button handlers — can be edited and run without
// recompiling the app.
//
// The exposed surface is deliberately small and safe: the "silk" package (tag
// read/write + log) plus a curated subset of the standard library (fmt, math,
// strings, strconv, time). Filesystem, network and process packages are NOT
// exposed, so a screen script cannot reach outside the process.
package script

import (
	"reflect"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"github.com/uk0/silk/core"
)

// Engine runs scripts against a tag database.
type Engine struct {
	i    *interp.Interpreter
	tags *core.TagDB
}

// NewEngine builds an engine whose scripts drive the given tag database. The
// "silk" package is pre-imported, so a script body can call silk.SetTag/GetTag
// directly; scripts may additionally import the whitelisted stdlib packages.
func NewEngine(tags *core.TagDB) *Engine {
	e := &Engine{tags: tags}
	e.i = interp.New(interp.Options{})
	e.i.Use(safeStdlib())
	e.i.Use(e.silkSymbols())
	// Pre-import the tag API so short scripts need no import line.
	_, _ = e.i.Eval(`import "silk"`)
	return e
}

// Run evaluates src (Go statements). Returns the script's error, if any.
func (e *Engine) Run(src string) error {
	_, err := e.i.Eval(src)
	return err
}

// Eval evaluates src and returns the value of its final expression (nil when
// the script yields no value).
func (e *Engine) Eval(src string) (interface{}, error) {
	v, err := e.i.Eval(src)
	if err != nil {
		return nil, err
	}
	if !v.IsValid() || !v.CanInterface() {
		return nil, nil
	}
	return v.Interface(), nil
}

// RunOnTagChange runs src every time the named tag changes, so a script can
// react to live data (compute a derived tag, raise an alarm). The returned func
// unsubscribes and is idempotent.
func (e *Engine) RunOnTagChange(tagName, src string) core.CancelFunc {
	tag := e.tags.GetOrCreate(tagName, core.Meta{})
	return tag.Subscribe(func(core.Value) {
		if err := e.Run(src); err != nil {
			core.Warn("script on tag ", tagName, ": ", err)
		}
	})
}

// silkSymbols exposes the tag API to scripts as the "silk" package.
func (e *Engine) silkSymbols() interp.Exports {
	return interp.Exports{
		"silk/silk": {
			"SetTag":     reflect.ValueOf(func(name string, v float64) { e.tags.GetOrCreate(name, core.Meta{}).SetValue(v) }),
			"GetTag":     reflect.ValueOf(func(name string) float64 { return e.tags.GetOrCreate(name, core.Meta{}).Value().Float() }),
			"SetTagBool": reflect.ValueOf(func(name string, b bool) { e.tags.GetOrCreate(name, core.Meta{}).SetValue(b) }),
			"GetTagBool": reflect.ValueOf(func(name string) bool { return e.tags.GetOrCreate(name, core.Meta{}).Value().Bool() }),
			"Log":        reflect.ValueOf(func(args ...interface{}) { core.Log(args...) }),
		},
	}
}

// safeStdlib returns the subset of the Go standard library scripts may import.
func safeStdlib() interp.Exports {
	allow := map[string]bool{
		"fmt/fmt": true, "math/math": true, "strings/strings": true,
		"strconv/strconv": true, "time/time": true,
	}
	out := interp.Exports{}
	for pkg, syms := range stdlib.Symbols {
		if allow[pkg] {
			out[pkg] = syms
		}
	}
	return out
}

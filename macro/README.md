# Silk compile-time macros (experimental)

An **opt-in source generator**, not a modified Go compiler or a runtime scripting
engine. `.silkgo` input expands to ordinary, formatted Go that the existing Go
toolchain can build, test and debug. No macro body is executed by the generator.

```go
package main

//silk:macro Accessor(Owner, Field, Type)
func (s *${Owner}) Set${Field}(value ${Type}) { s.${Field} = value }
//silk:end

type Sensor struct { Pressure float64; Enabled bool }
//silk:expand Accessor(Sensor, Pressure, float64)
//silk:expand Accessor(Sensor, Enabled, bool)
```

From the repository root:

```sh
go run ./cmd/silkgen -macro -o examples/silk-macros/sensor_gen.go examples/silk-macros/sensor.silkgo
go run ./examples/silk-macros
# pressure=42.5 enabled=true
```

In SilkIDE, open and save a `.silkgo` file, then choose **Tools → Expand Silk
Macros** (also available in the command palette). The IDE creates/refreshes the
adjacent `<stem>_gen.go` and opens it. Generation is explicit, not triggered by
each keystroke. An unsaved source, stale in-flight result, handwritten output,
symlink output or unsaved generated editor is not overwritten.

[Actual Windows Tools-action result](../screenshots/silkide-silk-macros-windows.png)
(debug outlines enabled; generated file was read back and compiled in a project
test, not just displayed).

## Contract

- Directives must occupy their own `//silk:` comment line. Strings and block
  comments containing directive-looking text are inert.
- Parameters are distinct Go identifiers; `${Name}` performs explicit textual
  substitution, including inside identifiers and literals. Use ASCII parameter
  names. Arguments must parse as Go expressions/types; commas inside composite
  literals/calls are supported.
- Definitions may appear after their uses. Nested definitions, recursive
  expansion, duplicate definitions, unknown parameters/macros and wrong arity
  are errors. This is **not a hygienic macro system**: choose names deliberately
  to avoid capture and do not substitute untrusted text into literals.
- Each result must parse as a complete Go file, not merely a `gofmt`-acceptable
  fragment. Go build constraints are preserved. Normal Go type checking still
  happens at `go build` / `go test`, not during textual expansion.
- Input limit: 1 MiB; output limit: 4 MiB; at most 2,048 expansions per file.
- Output writes use a temporary file and rename. Only `.go` files carrying this
  generator's exact pre-package marker can be replaced. Errors leave previous
  output untouched. Do not run multiple writers against the same output path.
- There is no automatic dependency tracking, cross-file macro import, or gopls
  server for macro syntax. The local analysis API provides macro/parameter
  symbols, references, highlighting and diagnostics. Expanded syntax failures
  map back to their invocation (or original source); the underlying generated
  Go parser error is retained. This is not a full debugger source map after
  gofmt. Debug generated Go and rerun generation after changing `.silkgo`.

For reproducible builds, check generated Go into source control and use
`//go:generate` as shown in [the runnable example](../examples/silk-macros).
Do not assume ordinary `go build` regenerates macros for you.

## Macro editing and SDK analysis (development)

SilkIDE now analyzes unsaved `.silkgo` buffers in a coalescing 90 ms worker.
Directive names, macro names and `${Parameter}` references get semantic colors;
macro declarations appear in the outline. F12 jumps to a declaration and
Shift+F12 lists same-file references. Ctrl+Space after `//silk:expand ` offers
macro names without requiring gopls. These features use the current buffer,
not the generated file. Snippet argument tab stops and cross-file imports are
not implemented.

SDK consumers can call `macro.Analyze(filename, source)` off the UI thread.
`Analysis.DefinitionAt` uses original UTF-8 byte offsets; highlights/references
are half-open spans. The generator and analyzer share the Go scanner, so
apparent directives in literals/block comments are inert. Analysis returns the
first expansion failure while retaining usable symbols from incomplete source;
it does not execute code or write output.

Errors have stable [diagnostic codes](../diagnostic/README.md), source locations,
remediation hints and wrapped causes. For automation:

```sh
go build -o ./bin/silkgen ./cmd/silkgen
./bin/silkgen -macro -diagnostics-json -o sensor_gen.go sensor.silkgo
```

Failure diagnostics are JSON on stderr (nonzero exit); success preserves the
normal generated-Go output contract. Native desktop validation for this new
iteration is tracked in [the active checklist](../EDITOR_SDK_ITERATION_20261021.md).

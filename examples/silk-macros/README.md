# Generated Go accessor example

```sh
go generate ./examples/silk-macros
go run ./examples/silk-macros
```

Run from the repository root. `sensor.silkgo` is the input; `sensor_gen.go` is
the checked-in ordinary Go output. Expected output:

```text
pressure=42.5 enabled=true
```

See [macro syntax and limitations](../../macro/README.md). The Go compiler itself
is unchanged; `go generate` must be rerun explicitly after source changes.

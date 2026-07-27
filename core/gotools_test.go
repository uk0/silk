package core

import (
	"reflect"
	"strings"
	"testing"
)

// Everything in this file is hermetic: the builders are asserted on their
// argv, the parsers on canned output. The only PATH contact is
// exec.LookPath inside DetectTool, which stats but never executes, and
// the availability assertions below are written so they hold whether or
// not the machine has the toolchain installed.

// --- Builders ---

// TestGoToolsBuildersArgv pins the exact argv of every workflow builder,
// including the default "./..." package and the flag order inside a
// profiling test run. Anything that reorders or renames a flag has to
// break here first.
func TestGoToolsBuildersArgv(t *testing.T) {
	cases := []struct {
		name string
		got  ToolCommand
		tool string
		want []string
	}{
		{
			name: "go vet default packages",
			got:  GoVetCommand("/w"),
			tool: ToolGoVet,
			want: []string{"go", "vet", "./..."},
		},
		{
			name: "go vet explicit packages",
			got:  GoVetCommand("/w", "./core/", "./ged/"),
			tool: ToolGoVet,
			want: []string{"go", "vet", "./core/", "./ged/"},
		},
		{
			name: "go test -race default packages",
			got:  GoTestRaceCommand("/w"),
			tool: ToolGoTestRace,
			want: []string{"go", "test", "-race", "./..."},
		},
		{
			name: "go test -race one package",
			got:  GoTestRaceCommand("/w", "./core/"),
			tool: ToolGoTestRace,
			want: []string{"go", "test", "-race", "./core/"},
		},
		{
			name: "go test cpu profile only",
			got:  GoTestProfileCommand("/w", ProfileOptions{CPUProfile: "/tmp/cpu.out"}),
			tool: ToolGoTestProf,
			want: []string{"go", "test", "-cpuprofile=/tmp/cpu.out", "./..."},
		},
		{
			name: "go test mem profile only",
			got:  GoTestProfileCommand("/w", ProfileOptions{MemProfile: "/tmp/mem.out"}),
			tool: ToolGoTestProf,
			want: []string{"go", "test", "-memprofile=/tmp/mem.out", "./..."},
		},
		{
			name: "go test cpu+mem+trace with bench and packages",
			got: GoTestProfileCommand("/w", ProfileOptions{
				CPUProfile: "/tmp/cpu.out",
				MemProfile: "/tmp/mem.out",
				TraceFile:  "/tmp/trace.out",
				Bench:      ".",
				Packages:   []string{"./gui/"},
			}),
			tool: ToolGoTestProf,
			want: []string{"go", "test", "-bench=.", "-cpuprofile=/tmp/cpu.out",
				"-memprofile=/tmp/mem.out", "-trace=/tmp/trace.out", "./gui/"},
		},
		{
			name: "go test no profile flags still runs the packages",
			got:  GoTestProfileCommand("/w", ProfileOptions{}),
			tool: ToolGoTestProf,
			want: []string{"go", "test", "./..."},
		},
		{
			name: "pprof top minimal",
			got:  PprofTopCommand("/w", "", "/tmp/cpu.out", 0),
			tool: ToolPprofTop,
			want: []string{"go", "tool", "pprof", "-top", "/tmp/cpu.out"},
		},
		{
			name: "pprof top with binary and nodecount",
			got:  PprofTopCommand("/w", "/tmp/silk.test", "/tmp/cpu.out", 25),
			tool: ToolPprofTop,
			want: []string{"go", "tool", "pprof", "-top", "-nodecount=25", "/tmp/silk.test", "/tmp/cpu.out"},
		},
		{
			name: "pprof top negative nodecount omits the flag",
			got:  PprofTopCommand("/w", "", "/tmp/cpu.out", -3),
			tool: ToolPprofTop,
			want: []string{"go", "tool", "pprof", "-top", "/tmp/cpu.out"},
		},
		{
			name: "go tool trace without http addr",
			got:  GoToolTraceCommand("/w", "/tmp/trace.out", ""),
			tool: ToolGoToolTrace,
			want: []string{"go", "tool", "trace", "/tmp/trace.out"},
		},
		{
			name: "go tool trace with http addr",
			got:  GoToolTraceCommand("/w", "/tmp/trace.out", "127.0.0.1:0"),
			tool: ToolGoToolTrace,
			want: []string{"go", "tool", "trace", "-http=127.0.0.1:0", "/tmp/trace.out"},
		},
		{
			name: "govulncheck default packages",
			got:  GovulncheckCommand("/w"),
			tool: ToolGovulncheck,
			want: []string{"govulncheck", "./..."},
		},
		{
			name: "govulncheck explicit packages",
			got:  GovulncheckCommand("/w", "./cmd/silkide/"),
			tool: ToolGovulncheck,
			want: []string{"govulncheck", "./cmd/silkide/"},
		},
		{
			name: "staticcheck default checks",
			got:  StaticcheckCommand("/w", ""),
			tool: ToolStaticcheck,
			want: []string{"staticcheck", "./..."},
		},
		{
			name: "staticcheck with check list",
			got:  StaticcheckCommand("/w", "SA*,ST1005", "./core/"),
			tool: ToolStaticcheck,
			want: []string{"staticcheck", "-checks=SA*,ST1005", "./core/"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !reflect.DeepEqual(c.got.Argv, c.want) {
				t.Errorf("Argv = %#v, want %#v", c.got.Argv, c.want)
			}
			if c.got.Tool != c.tool {
				t.Errorf("Tool = %q, want %q", c.got.Tool, c.tool)
			}
			if c.got.Dir != "/w" {
				t.Errorf("Dir = %q, want %q", c.got.Dir, "/w")
			}
			if c.got.Env != nil {
				t.Errorf("Env = %#v, want nil for a default build", c.got.Env)
			}
		})
	}
}

// TestGoToolsBuildersDoNotAliasPackages checks a builder copies the
// caller's package slice: mutating it afterwards must not rewrite an
// already-built argv.
func TestGoToolsBuildersDoNotAliasPackages(t *testing.T) {
	pkgs := []string{"./core/"}
	cmd := GoVetCommand("/w", pkgs...)
	pkgs[0] = "./MUTATED/"
	if got := cmd.Argv[len(cmd.Argv)-1]; got != "./core/" {
		t.Errorf("argv package = %q after mutating the caller's slice, want %q", got, "./core/")
	}
}

// TestToolCommandWithEnv checks WithEnv appends and copies: the receiver
// keeps its own Env, so one cached ToolCommand can be specialised twice.
func TestToolCommandWithEnv(t *testing.T) {
	base := GoTestRaceCommand("/w")
	a := base.WithEnv("GORACE=halt_on_error=1")
	b := a.WithEnv("CGO_ENABLED=1")

	if base.Env != nil {
		t.Errorf("base.Env = %#v, want nil (WithEnv must not touch the receiver)", base.Env)
	}
	if want := []string{"GORACE=halt_on_error=1"}; !reflect.DeepEqual(a.Env, want) {
		t.Errorf("a.Env = %#v, want %#v", a.Env, want)
	}
	if want := []string{"GORACE=halt_on_error=1", "CGO_ENABLED=1"}; !reflect.DeepEqual(b.Env, want) {
		t.Errorf("b.Env = %#v, want %#v", b.Env, want)
	}
	if got := base.WithEnv(); got.Env != nil {
		t.Errorf("WithEnv() with no entries set Env = %#v, want nil", got.Env)
	}
	// Argv must survive the copy untouched.
	if !reflect.DeepEqual(b.Argv, base.Argv) {
		t.Errorf("WithEnv changed Argv: %#v vs %#v", b.Argv, base.Argv)
	}
}

// --- Availability detection ---

// TestDetectToolMissing verifies a tool that is not on PATH comes back as
// unavailable with an empty Path instead of an error or a panic.
func TestDetectToolMissing(t *testing.T) {
	const missing = "silk-no-such-analyzer-9f3a1c"
	got := DetectTool(missing)
	if got.Name != missing {
		t.Errorf("Name = %q, want %q", got.Name, missing)
	}
	if got.Available {
		t.Errorf("Available = true for %q, want false", missing)
	}
	if got.Path != "" {
		t.Errorf("Path = %q, want empty for a missing tool", got.Path)
	}
	if got.Version != "" {
		t.Errorf("Version = %q, want empty (DetectTool must not run the tool)", got.Version)
	}
	// An empty name must not be looked up at all.
	if empty := DetectTool(""); empty.Available || empty.Path != "" {
		t.Errorf("DetectTool(\"\") = %+v, want an unavailable zero tool", empty)
	}
}

// TestDetectGoToolsShape checks the detection set covers exactly the
// binaries the workflows need, in GoToolBinaries order, and that
// Available and Path always agree. Deliberately makes no claim about
// whether any given tool is installed.
func TestDetectGoToolsShape(t *testing.T) {
	bins := GoToolBinaries()
	want := []string{"go", "govulncheck", "staticcheck"}
	if !reflect.DeepEqual(bins, want) {
		t.Fatalf("GoToolBinaries() = %#v, want %#v", bins, want)
	}
	tools := DetectGoTools()
	if len(tools) != len(bins) {
		t.Fatalf("DetectGoTools() returned %d tools, want %d: %+v", len(tools), len(bins), tools)
	}
	for i, tool := range tools {
		if tool.Name != bins[i] {
			t.Errorf("tool[%d].Name = %q, want %q", i, tool.Name, bins[i])
		}
		if tool.Available != (tool.Path != "") {
			t.Errorf("tool[%d] = %+v: Available and Path disagree", i, tool)
		}
		if tool.Version != "" {
			t.Errorf("tool[%d].Version = %q, want empty from DetectGoTools", i, tool.Version)
		}
	}
}

// TestGoToolBinaryMapping checks every workflow id maps to the binary it
// shells out to, and that an unknown id maps to "" (treated as
// unavailable by the picker).
func TestGoToolBinaryMapping(t *testing.T) {
	cases := map[string]string{
		ToolGoVet:       "go",
		ToolGoTestRace:  "go",
		ToolGoTestProf:  "go",
		ToolPprofTop:    "go",
		ToolGoToolTrace: "go",
		ToolGovulncheck: "govulncheck",
		ToolStaticcheck: "staticcheck",
		"nonsense":      "",
	}
	for id, want := range cases {
		if got := GoToolBinary(id); got != want {
			t.Errorf("GoToolBinary(%q) = %q, want %q", id, got, want)
		}
	}
}

// TestGoToolWorkflowsOrderAndCopy pins the picker order and checks the
// returned slice is a copy.
func TestGoToolWorkflowsOrderAndCopy(t *testing.T) {
	want := []string{
		ToolGoVet, ToolGoTestRace, ToolGoTestProf,
		ToolPprofTop, ToolGoToolTrace, ToolGovulncheck, ToolStaticcheck,
	}
	got := GoToolWorkflows()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GoToolWorkflows() = %#v, want %#v", got, want)
	}
	got[0] = "MUTATED"
	if again := GoToolWorkflows(); again[0] != ToolGoVet {
		t.Errorf("GoToolWorkflows() returned an aliased slice: %q", again[0])
	}
}

// TestToolVersionArgs checks each tool's version flag.
func TestToolVersionArgs(t *testing.T) {
	cases := []struct {
		name string
		want []string
	}{
		{"go", []string{"go", "version"}},
		{"govulncheck", []string{"govulncheck", "-version"}},
		{"staticcheck", []string{"staticcheck", "-version"}},
		{"gopls", []string{"gopls", "--version"}},
	}
	for _, c := range cases {
		if got := ToolVersionArgs(c.name); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ToolVersionArgs(%q) = %#v, want %#v", c.name, got, c.want)
		}
	}
}

// TestParseToolVersion checks the banner is the first non-empty trimmed
// line, and that empty output yields "".
func TestParseToolVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"go version go1.25.0 darwin/arm64\n", "go version go1.25.0 darwin/arm64"},
		{"\n\n  staticcheck 2025.1 (v0.6.1)  \n\n", "staticcheck 2025.1 (v0.6.1)"},
		{"Go: go1.25.0\r\nScanner: govulncheck@v1.1.4\r\n", "Go: go1.25.0"},
		{"", ""},
		{"\n \t\n", ""},
	}
	for _, c := range cases {
		if got := ParseToolVersion(c.in); got != c.want {
			t.Errorf("ParseToolVersion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- Vet-style parser ---

// TestParseVetFindings runs canned `go vet` output through the vet-style
// parser: the "# package" header is dropped and each diagnostic keeps its
// location and message.
func TestParseVetFindings(t *testing.T) {
	in := strings.Join([]string{
		"# github.com/uk0/silk/core",
		"core/gotools.go:12:2: imported and not used: \"fmt\"",
		"core/tdoc.go:450:4: unreachable code",
		"ok  \tgithub.com/uk0/silk/geom\t0.201s",
	}, "\n")
	want := []Finding{
		{Tool: ToolGoVet, File: "core/gotools.go", Line: 12, Col: 2,
			Severity: FindingError, Message: `imported and not used: "fmt"`},
		{Tool: ToolGoVet, File: "core/tdoc.go", Line: 450, Col: 4,
			Severity: FindingError, Message: "unreachable code"},
	}
	if got := ParseVetFindings(ToolGoVet, in); !reflect.DeepEqual(got, want) {
		t.Errorf("ParseVetFindings = %+v, want %+v", got, want)
	}
	if got := ParseVetFindings(ToolGoVet, ""); got != nil {
		t.Errorf("ParseVetFindings(\"\") = %+v, want nil", got)
	}
}

// TestParseVetFindingsStaticcheckCode checks a trailing "(SA4006)" check
// id is lifted out of the message into Code, while a vet message with a
// parenthesised tail that is not a check id is left alone.
func TestParseVetFindingsStaticcheckCode(t *testing.T) {
	in := strings.Join([]string{
		"core/gotools.go:120:2: this value of err is never used (SA4006)",
		"core/tdoc.go:44:1: error strings should not be capitalized (ST1005)",
		"core/git.go:9:6: func runGit is unused (some note)",
	}, "\n")
	want := []Finding{
		{Tool: ToolStaticcheck, File: "core/gotools.go", Line: 120, Col: 2,
			Severity: FindingError, Message: "this value of err is never used", Code: "SA4006"},
		{Tool: ToolStaticcheck, File: "core/tdoc.go", Line: 44, Col: 1,
			Severity: FindingError, Message: "error strings should not be capitalized", Code: "ST1005"},
		{Tool: ToolStaticcheck, File: "core/git.go", Line: 9, Col: 6,
			Severity: FindingError, Message: "func runGit is unused (some note)"},
	}
	if got := ParseVetFindings(ToolStaticcheck, in); !reflect.DeepEqual(got, want) {
		t.Errorf("ParseVetFindings = %+v, want %+v", got, want)
	}
}

// --- govulncheck parser ---

// govulncheckSample is a govulncheck v1.1 text report with both result
// sections: a reachable stdlib symbol (with an example trace) and a
// module-level finding (no trace).
const govulncheckSample = `govulncheck is an experimental tool. Share feedback at https://go.dev/s/govulncheck-feedback.

Scanning your code and 245 packages across 34 dependent modules for known vulnerabilities...

=== Symbol Results ===

Vulnerability #1: GO-2024-2687
    HTTP/2 CONTINUATION flood in net/http
  More info: https://pkg.go.dev/vuln/GO-2024-2687
  Standard library
    Found in: net/http@go1.22.1
    Fixed in: net/http@go1.22.2
    Example traces found:
      #1: cmd/silkide/main.go:24:11: main.main calls http.ListenAndServe
      #2: cmd/silkide/main.go:99:3: main.serve calls http.Serve

=== Module Results ===

Vulnerability #1: GO-2024-2611
    Infinite loop in QUIC connection handling in golang.org/x/net
  More info: https://pkg.go.dev/vuln/GO-2024-2611
  Module: golang.org/x/net
    Found in: golang.org/x/net@v0.22.0
    Fixed in: golang.org/x/net@v0.23.0

Your code is affected by 1 vulnerability from the Go standard library.
`

// TestParseGovulncheck parses the canned report: one finding per
// vulnerability block, the first example trace as the location, the
// affected/fixed versions folded into the message, and error vs warning
// decided by the result section.
func TestParseGovulncheck(t *testing.T) {
	want := []Finding{
		{
			Tool:     ToolGovulncheck,
			File:     "cmd/silkide/main.go",
			Line:     24,
			Col:      11,
			Severity: FindingError,
			Message:  "HTTP/2 CONTINUATION flood in net/http (found in net/http@go1.22.1, fixed in net/http@go1.22.2)",
			Code:     "GO-2024-2687",
		},
		{
			Tool:     ToolGovulncheck,
			Severity: FindingWarning,
			Message:  "Infinite loop in QUIC connection handling in golang.org/x/net (found in golang.org/x/net@v0.22.0, fixed in golang.org/x/net@v0.23.0)",
			Code:     "GO-2024-2611",
		},
	}
	got := ParseGovulncheck(govulncheckSample)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseGovulncheck =\n%+v\nwant\n%+v", got, want)
	}
}

// TestParseGovulncheckWrappedTitleNoSections covers the pre-1.1 layout:
// no "=== ... Results ===" header (so everything is an error) and a title
// wrapped over two lines, which must be rejoined with a single space.
func TestParseGovulncheckWrappedTitleNoSections(t *testing.T) {
	in := `Vulnerability #1: GO-2023-1988
    Improper handling of HTML-like comments in template/html in
    html/template
  More info: https://pkg.go.dev/vuln/GO-2023-1988
  Standard library
    Found in: html/template@go1.20.5
    Fixed in: html/template@go1.20.7
`
	want := []Finding{{
		Tool:     ToolGovulncheck,
		Severity: FindingError,
		Message: "Improper handling of HTML-like comments in template/html in html/template " +
			"(found in html/template@go1.20.5, fixed in html/template@go1.20.7)",
		Code: "GO-2023-1988",
	}}
	if got := ParseGovulncheck(in); !reflect.DeepEqual(got, want) {
		t.Errorf("ParseGovulncheck =\n%+v\nwant\n%+v", got, want)
	}
}

// TestParseGovulncheckClean checks the no-vulnerabilities report and
// empty input both yield no findings.
func TestParseGovulncheckClean(t *testing.T) {
	clean := `Scanning your code and 245 packages across 34 dependent modules for known vulnerabilities...

No vulnerabilities found.
`
	if got := ParseGovulncheck(clean); len(got) != 0 {
		t.Errorf("ParseGovulncheck(clean) = %+v, want none", got)
	}
	if got := ParseGovulncheck(""); len(got) != 0 {
		t.Errorf("ParseGovulncheck(\"\") = %+v, want none", got)
	}
}

// --- pprof -top parser ---

// pprofTopSample is a `go tool pprof -top` capture: the preamble, the
// column header, four rows, and a trailing blank line.
const pprofTopSample = `File: silk.test
Type: cpu
Time: Jul 27, 2026 at 10:04am (CST)
Duration: 1.21s, Total samples = 980ms (81.02%)
Showing nodes accounting for 900ms, 91.84% of 980ms total
Dropped 42 nodes (cum <= 4.90ms)
      flat  flat%   sum%        cum   cum%
     300ms 30.61% 30.61%      400ms 40.82%  runtime.mallocgc
     200ms 20.41% 51.02%      200ms 20.41%  github.com/uk0/silk/paint.(*Font).TextExtents
      40ms  4.08% 55.10%      500ms 51.02%  github.com/uk0/silk/gui.(*Widget).Draw
         0     0% 55.10%      120ms 12.24%  github.com/uk0/silk/ged.(*CodePanel).Draw

`

// TestParsePprofTop parses the canned table: preamble skipped, one
// finding per row in pprof's order, the numbers folded into the message,
// flat% as the code, and the hot-spot threshold deciding warning vs info.
func TestParsePprofTop(t *testing.T) {
	want := []Finding{
		{Tool: ToolPprofTop, Severity: FindingWarning, Code: "30.61%",
			Message: "runtime.mallocgc (flat 300ms 30.61%, cum 400ms 40.82%)"},
		{Tool: ToolPprofTop, Severity: FindingWarning, Code: "20.41%",
			Message: "github.com/uk0/silk/paint.(*Font).TextExtents (flat 200ms 20.41%, cum 200ms 20.41%)"},
		{Tool: ToolPprofTop, Severity: FindingInfo, Code: "4.08%",
			Message: "github.com/uk0/silk/gui.(*Widget).Draw (flat 40ms 4.08%, cum 500ms 51.02%)"},
		{Tool: ToolPprofTop, Severity: FindingInfo, Code: "0%",
			Message: "github.com/uk0/silk/ged.(*CodePanel).Draw (flat 0 0%, cum 120ms 12.24%)"},
	}
	got := ParsePprofTop(pprofTopSample)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParsePprofTop =\n%+v\nwant\n%+v", got, want)
	}
}

// TestParsePprofTopLinesGranularity checks the trailing "file.go:123"
// column that `-top -lines` adds becomes File/Line and is stripped out of
// the frame name, so the pane can jump to a hot line.
func TestParsePprofTopLinesGranularity(t *testing.T) {
	in := "      flat  flat%   sum%        cum   cum%\n" +
		"     300ms 30.61% 30.61%      400ms 40.82%  runtime.mallocgc /usr/local/go/src/runtime/malloc.go:1234\n"
	want := []Finding{{
		Tool:     ToolPprofTop,
		File:     "/usr/local/go/src/runtime/malloc.go",
		Line:     1234,
		Severity: FindingWarning,
		Message:  "runtime.mallocgc (flat 300ms 30.61%, cum 400ms 40.82%)",
		Code:     "30.61%",
	}}
	if got := ParsePprofTop(in); !reflect.DeepEqual(got, want) {
		t.Errorf("ParsePprofTop =\n%+v\nwant\n%+v", got, want)
	}
}

// TestParsePprofTopNoTable checks output without the column header (an
// error from pprof, say) yields no findings rather than garbage rows.
func TestParsePprofTopNoTable(t *testing.T) {
	in := "profile: could not open /tmp/cpu.out: no such file or directory\n"
	if got := ParsePprofTop(in); len(got) != 0 {
		t.Errorf("ParsePprofTop(no table) = %+v, want none", got)
	}
	if got := ParsePprofTop(""); len(got) != 0 {
		t.Errorf("ParsePprofTop(\"\") = %+v, want none", got)
	}
}

// --- race parser ---

// raceSample is a `go test -race` capture: one data-race report followed
// by the test-runner failure detail.
const raceSample = `==================
WARNING: DATA RACE
Write at 0x00c0000b4010 by goroutine 8:
  github.com/uk0/silk/core.(*Bus).Publish()
      /Users/x/silk/core/bus.go:42 +0x64

Previous read at 0x00c0000b4010 by main goroutine:
  github.com/uk0/silk/core.(*Bus).Len()
      /Users/x/silk/core/bus.go:31 +0x2c

Goroutine 8 (running) created at:
  github.com/uk0/silk/core.NewBus()
      /Users/x/silk/core/bus.go:18 +0x88
==================
--- FAIL: TestBusPublish (0.01s)
    bus_test.go:20: race detected during execution of test
FAIL
exit status 1
FAIL	github.com/uk0/silk/core	0.412s
`

// TestParseRaceFindings checks both passes: the vet-style test-failure
// detail first, then one finding per DATA RACE report, located at the
// first frame of the first access and described by that access line.
func TestParseRaceFindings(t *testing.T) {
	want := []Finding{
		{Tool: ToolGoTestRace, File: "bus_test.go", Line: 20,
			Severity: FindingError, Message: "race detected during execution of test"},
		{Tool: ToolGoTestRace, File: "/Users/x/silk/core/bus.go", Line: 42,
			Severity: FindingError, Message: "Write at 0x00c0000b4010 by goroutine 8", Code: "DATA RACE"},
	}
	got := ParseRaceFindings(raceSample)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseRaceFindings =\n%+v\nwant\n%+v", got, want)
	}
}

// TestParseRaceFindingsTwoReports checks each WARNING block becomes its
// own finding, including a final block that ends at EOF without a closing
// separator.
func TestParseRaceFindingsTwoReports(t *testing.T) {
	in := `==================
WARNING: DATA RACE
Read at 0x00c000010000 by goroutine 7:
  a.F()
      /w/a.go:10 +0x11
==================
==================
WARNING: DATA RACE
Atomic write at 0x00c000010008 by goroutine 9:
  b.G()
      /w/b.go:20 +0x22
`
	want := []Finding{
		{Tool: ToolGoTestRace, File: "/w/a.go", Line: 10, Severity: FindingError,
			Message: "Read at 0x00c000010000 by goroutine 7", Code: "DATA RACE"},
		{Tool: ToolGoTestRace, File: "/w/b.go", Line: 20, Severity: FindingError,
			Message: "Atomic write at 0x00c000010008 by goroutine 9", Code: "DATA RACE"},
	}
	got := ParseRaceFindings(in)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseRaceFindings =\n%+v\nwant\n%+v", got, want)
	}
}

// TestParseRaceFindingsCleanRun checks a passing race run reports nothing.
func TestParseRaceFindingsCleanRun(t *testing.T) {
	in := "ok  \tgithub.com/uk0/silk/core\t2.104s\n"
	if got := ParseRaceFindings(in); len(got) != 0 {
		t.Errorf("ParseRaceFindings(clean) = %+v, want none", got)
	}
}

// --- trace URL + dispatch ---

// TestParseTraceServerURL checks the viewer address is taken from the
// last URL `go tool trace` logs.
func TestParseTraceServerURL(t *testing.T) {
	in := strings.Join([]string{
		"2026/07/27 10:04:01 Parsing trace...",
		"2026/07/27 10:04:02 Splitting trace...",
		"2026/07/27 10:04:03 Opening browser. Trace viewer is listening on http://127.0.0.1:57263",
	}, "\n")
	if got, want := ParseTraceServerURL(in), "http://127.0.0.1:57263"; got != want {
		t.Errorf("ParseTraceServerURL = %q, want %q", got, want)
	}
	if got := ParseTraceServerURL("2026/07/27 10:04:01 Parsing trace...\n"); got != "" {
		t.Errorf("ParseTraceServerURL(no url) = %q, want empty", got)
	}
	if got := ParseTraceServerURL(""); got != "" {
		t.Errorf("ParseTraceServerURL(\"\") = %q, want empty", got)
	}
}

// TestParseToolOutputDispatch checks each workflow id routes to its own
// parser: govulncheck / pprof / race get their specialised parsers, go
// tool trace yields nothing, and everything else falls through to the
// vet-style parser with the id carried into Finding.Tool.
func TestParseToolOutputDispatch(t *testing.T) {
	if got := ParseToolOutput(ToolGovulncheck, govulncheckSample); len(got) != 2 ||
		got[0].Code != "GO-2024-2687" {
		t.Errorf("ParseToolOutput(govulncheck) = %+v, want the 2 govulncheck findings", got)
	}
	if got := ParseToolOutput(ToolPprofTop, pprofTopSample); len(got) != 4 ||
		got[0].Code != "30.61%" {
		t.Errorf("ParseToolOutput(pprof) = %+v, want the 4 pprof rows", got)
	}
	if got := ParseToolOutput(ToolGoTestRace, raceSample); len(got) != 2 ||
		got[1].Code != "DATA RACE" {
		t.Errorf("ParseToolOutput(race) = %+v, want the failure detail + the race", got)
	}
	if got := ParseToolOutput(ToolGoToolTrace, "Serving web UI on http://127.0.0.1:1\n"); got != nil {
		t.Errorf("ParseToolOutput(trace) = %+v, want nil", got)
	}
	vet := "core/gotools.go:12:2: undefined: foo\n"
	for _, id := range []string{ToolGoVet, ToolStaticcheck, ToolGoTestProf, "unknown-tool"} {
		got := ParseToolOutput(id, vet)
		if len(got) != 1 {
			t.Fatalf("ParseToolOutput(%q) = %+v, want 1 finding", id, got)
		}
		if got[0].Tool != id {
			t.Errorf("ParseToolOutput(%q) tagged Tool = %q", id, got[0].Tool)
		}
		if got[0].File != "core/gotools.go" || got[0].Line != 12 || got[0].Col != 2 {
			t.Errorf("ParseToolOutput(%q) location = %+v", id, got[0])
		}
	}
}

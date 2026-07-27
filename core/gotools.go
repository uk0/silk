package core

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/uk0/silk/buildissues"
)

// Static-analyzer and profiler workflows for the IDE's "Go Tools" pane
// (`go vet`, `go test -race`, cpu/mem profiling, `go tool pprof -top`,
// `go tool trace`, `govulncheck`, `staticcheck`).
//
// The plumbing is deliberately split into three layers so no UI code has
// to own toolchain trivia:
//
//	detection — DetectTool / DetectGoTools ask PATH whether a binary
//	            exists. LookPath only: nothing is executed, so the tool
//	            picker can grey out what is not installed without ever
//	            blocking on a subprocess.
//	builders  — GoVetCommand, GoTestRaceCommand, GoTestProfileCommand,
//	            PprofTopCommand, GoToolTraceCommand, GovulncheckCommand
//	            and StaticcheckCommand return a ToolCommand (argv + env +
//	            dir) and never run it. The host runs it off the UI thread
//	            (the panel is a pure view) and streams the text back.
//	parsers   — ParseToolOutput and its per-tool siblings fold captured
//	            output into a neutral []Finding, so the pane renders and
//	            jumps the same way no matter which tool produced the row.
//
// Nothing here holds state and nothing panics; malformed output yields
// fewer findings, never an error.

// Tool is an external analyzer binary as found (or not found) on PATH.
// Version is filled only by DetectToolVersion — plain DetectTool leaves
// it empty because reading a version means starting a process.
type Tool struct {
	Name      string
	Path      string
	Available bool
	Version   string
}

// ToolCommand is a built-but-unexecuted analyzer invocation. Argv[0] is
// the executable name (resolved through PATH by os/exec, which is why the
// builders emit the bare name rather than Tool.Path — Tool.Path exists
// for display and availability, not for spawning). Env holds extra
// "KEY=value" entries the host layers on top of os.Environ(); it is nil
// for every default build. Tool is the logical workflow id, carried so
// the findings a run produces can be grouped under the row that started
// it.
type ToolCommand struct {
	Tool string
	Argv []string
	Env  []string
	Dir  string
}

// WithEnv returns a copy of c with extra "KEY=value" entries appended to
// Env. The copy is deep enough that the receiver's Env is never aliased,
// so a cached ToolCommand can be specialised per run — this is how
// RunConfigPanel's env rows reach an analyzer process.
func (c ToolCommand) WithEnv(extra ...string) ToolCommand {
	if len(extra) == 0 {
		return c
	}
	env := make([]string, 0, len(c.Env)+len(extra))
	env = append(env, c.Env...)
	env = append(env, extra...)
	c.Env = env
	return c
}

// Workflow ids. They double as ToolCommand.Tool tags, as Finding.Tool
// tags, and as the ids the Go Tools picker reports through its run
// callback, so a run's findings always group under the row that fired it.
const (
	ToolGoVet       = "go vet"
	ToolGoTestRace  = "go test -race"
	ToolGoTestProf  = "go test -profile"
	ToolPprofTop    = "go tool pprof"
	ToolGoToolTrace = "go tool trace"
	ToolGovulncheck = "govulncheck"
	ToolStaticcheck = "staticcheck"
)

// Finding.Severity values. Strings rather than an enum because they come
// from three unrelated sources (buildissues.Severity.String(), a
// govulncheck section header, a pprof flat% threshold) and the pane only
// ever switches a colour on them.
const (
	FindingError   = "error"
	FindingWarning = "warning"
	FindingInfo    = "info"
)

// Finding is one row in the Go Tools pane: a diagnostic, a vulnerability
// or a profile hot spot, normalised across every tool. File is empty
// (and Line/Col zero) when the tool reported no source location — a
// `pprof -top` row at function granularity, for instance — in which case
// the pane must not offer a jump. Code is the tool's own identifier for
// the finding when it has one: a staticcheck check id ("SA4006"), a Go
// vulnerability id ("GO-2024-2687"), a pprof flat percentage ("30.61%").
type Finding struct {
	Tool     string
	File     string
	Line     int
	Col      int
	Severity string
	Message  string
	Code     string
}

// --- Availability detection ---

// goToolWorkflows is the picker order: the go-subcommand workflows
// first (they only need the toolchain), then the two optional binaries.
var goToolWorkflows = []string{
	ToolGoVet,
	ToolGoTestRace,
	ToolGoTestProf,
	ToolPprofTop,
	ToolGoToolTrace,
	ToolGovulncheck,
	ToolStaticcheck,
}

// GoToolWorkflows returns the workflow ids in picker order. The slice is
// a copy, so a caller reordering it cannot reorder anyone else's picker.
func GoToolWorkflows() []string {
	out := make([]string, len(goToolWorkflows))
	copy(out, goToolWorkflows)
	return out
}

// GoToolBinary returns the executable a workflow needs on PATH: "go" for
// everything the toolchain ships, the tool's own name for the two
// third-party analyzers. An unknown id yields "" — treat that as
// unavailable rather than as "no binary needed".
func GoToolBinary(id string) string {
	switch id {
	case ToolGoVet, ToolGoTestRace, ToolGoTestProf, ToolPprofTop, ToolGoToolTrace:
		return "go"
	case ToolGovulncheck:
		return "govulncheck"
	case ToolStaticcheck:
		return "staticcheck"
	default:
		return ""
	}
}

// GoToolBinaries returns the distinct executables the workflows need, in
// first-use order: "go", "govulncheck", "staticcheck".
func GoToolBinaries() []string {
	var out []string
	seen := make(map[string]bool)
	for _, id := range goToolWorkflows {
		bin := GoToolBinary(id)
		if bin == "" || seen[bin] {
			continue
		}
		seen[bin] = true
		out = append(out, bin)
	}
	return out
}

// DetectTool locates name on PATH. It never executes the binary, so it
// is safe on the UI thread and in tests; a missing tool is reported as
// Available false with an empty Path, never as an error.
func DetectTool(name string) Tool {
	t := Tool{Name: name}
	if name == "" {
		return t
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return t
	}
	t.Path = path
	t.Available = true
	return t
}

// DetectGoTools reports availability for every binary the workflows
// need, in GoToolBinaries order. Versions are left empty — see
// DetectToolVersion for the variant that pays for a subprocess.
func DetectGoTools() []Tool {
	bins := GoToolBinaries()
	out := make([]Tool, 0, len(bins))
	for _, bin := range bins {
		out = append(out, DetectTool(bin))
	}
	return out
}

// toolVersionTimeout caps a version probe. A version flag that hangs
// (a wrapper script waiting on stdin, say) must not wedge the caller.
const toolVersionTimeout = 5 * time.Second

// ToolVersionArgs returns the argv that makes name print its version.
// The toolchain uses the `version` subcommand; the two analyzers both
// take `-version`. Anything else gets the conventional `--version`.
func ToolVersionArgs(name string) []string {
	switch name {
	case "go":
		return []string{"go", "version"}
	case "govulncheck", "staticcheck":
		return []string{name, "-version"}
	default:
		return []string{name, "--version"}
	}
}

// ParseToolVersion picks the version banner out of a version probe's
// output: the first non-empty line, trimmed. Every tool here prints its
// identity there ("go version go1.25.0 darwin/arm64", "staticcheck
// 2025.1 (v0.6.1)"), and the line is only ever shown to the user, so it
// is kept whole instead of being dissected per tool.
func ParseToolVersion(output string) string {
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line != "" {
			return line
		}
	}
	return ""
}

// DetectToolVersion is DetectTool plus a version probe: it runs
// ToolVersionArgs under a toolVersionTimeout context and stores the
// banner in Version. It starts a process, so callers must keep it off
// the UI thread; a missing tool short-circuits without spawning
// anything, and a probe that fails or times out leaves Version empty
// rather than reporting the tool as unavailable.
func DetectToolVersion(name string) Tool {
	t := DetectTool(name)
	if !t.Available {
		return t
	}
	args := ToolVersionArgs(name)
	ctx, cancel := context.WithTimeout(context.Background(), toolVersionTimeout)
	defer cancel()
	// CombinedOutput: `go version` writes to stdout, some wrappers to
	// stderr; either way the banner is what we want.
	out, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return t
	}
	t.Version = ParseToolVersion(string(out))
	return t
}

// --- Command builders ---

// goToolPkgs defaults an omitted package list to "./..." — every
// workflow here means "the whole module" unless the user narrowed the
// scope — and copies what it is given so a builder never aliases the
// caller's slice.
func goToolPkgs(pkgs []string) []string {
	if len(pkgs) == 0 {
		return []string{"./..."}
	}
	out := make([]string, len(pkgs))
	copy(out, pkgs)
	return out
}

// GoVetCommand builds `go vet <pkgs>`.
func GoVetCommand(dir string, pkgs ...string) ToolCommand {
	return ToolCommand{
		Tool: ToolGoVet,
		Dir:  dir,
		Argv: append([]string{"go", "vet"}, goToolPkgs(pkgs)...),
	}
}

// GoTestRaceCommand builds `go test -race <pkgs>`.
func GoTestRaceCommand(dir string, pkgs ...string) ToolCommand {
	return ToolCommand{
		Tool: ToolGoTestRace,
		Dir:  dir,
		Argv: append([]string{"go", "test", "-race"}, goToolPkgs(pkgs)...),
	}
}

// ProfileOptions selects what a profiling `go test` run should write.
// Every path is optional: an empty field omits its flag, so the same
// builder covers "cpu only", "cpu + mem", or "just capture a trace".
// Paths should be absolute — `go test` resolves profile paths against
// the package directory, not against ToolCommand.Dir.
type ProfileOptions struct {
	CPUProfile string   // -cpuprofile
	MemProfile string   // -memprofile
	TraceFile  string   // -trace, the input GoToolTraceCommand later views
	Bench      string   // -bench pattern; omitted when empty
	Packages   []string // defaults to ./...
}

// GoTestProfileCommand builds the profiling test run:
// `go test [-bench=P] [-cpuprofile=F] [-memprofile=F] [-trace=F] <pkgs>`.
// Flag order is fixed so the argv is reproducible across runs.
func GoTestProfileCommand(dir string, opt ProfileOptions) ToolCommand {
	argv := []string{"go", "test"}
	if opt.Bench != "" {
		argv = append(argv, "-bench="+opt.Bench)
	}
	if opt.CPUProfile != "" {
		argv = append(argv, "-cpuprofile="+opt.CPUProfile)
	}
	if opt.MemProfile != "" {
		argv = append(argv, "-memprofile="+opt.MemProfile)
	}
	if opt.TraceFile != "" {
		argv = append(argv, "-trace="+opt.TraceFile)
	}
	argv = append(argv, goToolPkgs(opt.Packages)...)
	return ToolCommand{Tool: ToolGoTestProf, Dir: dir, Argv: argv}
}

// PprofTopCommand builds `go tool pprof -top [-nodecount=N] [binary] <profile>`.
// binary may be empty: profiles written by the Go runtime already carry
// their symbol table, so the test binary is only needed when it does not
// (or when the user wants pprof to re-symbolise). nodeCount <= 0 omits
// -nodecount and lets pprof pick.
func PprofTopCommand(dir, binary, profile string, nodeCount int) ToolCommand {
	argv := []string{"go", "tool", "pprof", "-top"}
	if nodeCount > 0 {
		argv = append(argv, "-nodecount="+strconv.Itoa(nodeCount))
	}
	if binary != "" {
		argv = append(argv, binary)
	}
	argv = append(argv, profile)
	return ToolCommand{Tool: ToolPprofTop, Dir: dir, Argv: argv}
}

// GoToolTraceCommand builds `go tool trace [-http=addr] <traceFile>`.
// The command serves a web UI instead of printing findings; the host
// takes the address out of its output with ParseTraceServerURL. An empty
// httpAddr lets go pick a port on localhost.
func GoToolTraceCommand(dir, traceFile, httpAddr string) ToolCommand {
	argv := []string{"go", "tool", "trace"}
	if httpAddr != "" {
		argv = append(argv, "-http="+httpAddr)
	}
	argv = append(argv, traceFile)
	return ToolCommand{Tool: ToolGoToolTrace, Dir: dir, Argv: argv}
}

// GovulncheckCommand builds `govulncheck <pkgs>` (text output, which is
// what ParseGovulncheck consumes).
func GovulncheckCommand(dir string, pkgs ...string) ToolCommand {
	return ToolCommand{
		Tool: ToolGovulncheck,
		Dir:  dir,
		Argv: append([]string{"govulncheck"}, goToolPkgs(pkgs)...),
	}
}

// StaticcheckCommand builds `staticcheck [-checks=LIST] <pkgs>`. An empty
// checks omits the flag and leaves staticcheck on its default check set.
func StaticcheckCommand(dir, checks string, pkgs ...string) ToolCommand {
	argv := []string{"staticcheck"}
	if checks != "" {
		argv = append(argv, "-checks="+checks)
	}
	argv = append(argv, goToolPkgs(pkgs)...)
	return ToolCommand{Tool: ToolStaticcheck, Dir: dir, Argv: argv}
}

// --- Parsers ---

// ParseToolOutput folds a workflow's captured output into findings,
// dispatching on the workflow id. go tool trace yields none — it serves
// a web UI, see ParseTraceServerURL. Unknown ids fall through to the
// vet-style parser, which is the safe default: it only reports lines
// that carry a real "file:line: message" locator.
func ParseToolOutput(tool, output string) []Finding {
	switch tool {
	case ToolGovulncheck:
		return ParseGovulncheck(output)
	case ToolPprofTop:
		return ParsePprofTop(output)
	case ToolGoTestRace:
		return ParseRaceFindings(output)
	case ToolGoToolTrace:
		return nil
	default:
		return ParseVetFindings(tool, output)
	}
}

// staticcheckCodeRe matches a trailing check id, e.g. the "(SA4006)" in
// "staticcheck: a.go:3:2: value never used (SA4006)". Anchored at the end
// and shaped exactly two letters + four digits so an ordinary
// parenthesised message tail is not mistaken for a code.
var staticcheckCodeRe = regexp.MustCompile(`\s*\(([A-Z]{2}\d{4})\)$`)

// splitCheckCode peels a trailing "(SA4006)"-style check id off a
// message, returning the message without it plus the bare code. vet
// messages have no such suffix and pass through untouched.
func splitCheckCode(message string) (string, string) {
	m := staticcheckCodeRe.FindStringSubmatch(message)
	if m == nil {
		return message, ""
	}
	return strings.TrimSpace(message[:len(message)-len(m[0])]), m[1]
}

// ParseVetFindings converts vet-style output — "file:line[:col]: message"
// diagnostics, as emitted by go build, go vet, staticcheck and the
// indented details under a `--- FAIL` header — into findings tagged with
// tool. buildissues.Parse does the line recognition and severity call
// (its rules already skip package headers, test-runner status lines and
// stack frames); the only thing added here is lifting a staticcheck
// check id out of the message into Code.
func ParseVetFindings(tool, output string) []Finding {
	issues := buildissues.Parse(output)
	if len(issues) == 0 {
		return nil
	}
	out := make([]Finding, 0, len(issues))
	for _, iss := range issues {
		msg, code := splitCheckCode(iss.Message)
		out = append(out, Finding{
			Tool:     tool,
			File:     iss.File,
			Line:     iss.Line,
			Col:      iss.Col,
			Severity: iss.Severity.String(),
			Message:  msg,
			Code:     code,
		})
	}
	return out
}

// vulnHeaderRe matches a govulncheck block header, e.g.
// "Vulnerability #2: GO-2024-2687", capturing the vulnerability id.
var vulnHeaderRe = regexp.MustCompile(`^Vulnerability #\d+:\s*(\S+)`)

// vulnTraceRe matches the "#1: " ordinal that prefixes each example
// trace, so the rest of the line ("main.go:24:11: main.run calls ...")
// can go straight through buildissues.Parse.
var vulnTraceRe = regexp.MustCompile(`^#\d+:\s*`)

// ParseGovulncheck parses govulncheck's text report. The shape it walks:
//
//	=== Symbol Results ===
//
//	Vulnerability #1: GO-2024-2687
//	    HTTP/2 CONTINUATION flood in net/http
//	  More info: https://pkg.go.dev/vuln/GO-2024-2687
//	  Standard library
//	    Found in: net/http@go1.22.1
//	    Fixed in: net/http@go1.22.2
//	    Example traces found:
//	      #1: cmd/main.go:24:11: main.main calls http.ListenAndServe
//
// One Finding per "Vulnerability #N" block, in report order:
//
//	Code     — the GO-YYYY-NNNN id.
//	Message  — the (possibly line-wrapped) title, with the affected and
//	           fixed versions appended, because a vulnerability row
//	           without "fixed in" tells the user nothing actionable.
//	File/Line/Col — from the FIRST example trace, so a click lands on the
//	           call site in the user's own code. Blocks without traces
//	           (module-level results) carry no location.
//	Severity — error under "=== Symbol Results ===" (govulncheck proved
//	           the vulnerable symbol is reachable) and warning under the
//	           module/package sections (the dependency is vulnerable but
//	           the call was not observed). Reports with no section header
//	           at all — the pre-1.1 layout — are all errors.
//
// Trailing prose ("Your code is affected by 1 vulnerability...") is
// ignored: title capture stops at the first labelled line or blank line
// inside a block, so only the real title is collected.
func ParseGovulncheck(output string) []Finding {
	var out []Finding
	var cur *Finding
	var title []string
	var found, fixed string
	titleOpen := false
	severity := FindingError

	flush := func() {
		if cur == nil {
			return
		}
		msg := strings.Join(title, " ")
		if msg == "" {
			msg = cur.Code
		}
		var extra []string
		if found != "" {
			extra = append(extra, "found in "+found)
		}
		if fixed != "" {
			extra = append(extra, "fixed in "+fixed)
		}
		if len(extra) > 0 {
			msg += " (" + strings.Join(extra, ", ") + ")"
		}
		cur.Message = msg
		out = append(out, *cur)
		cur, title, titleOpen, found, fixed = nil, nil, false, "", ""
	}

	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))

		// Section header: closes the open block and re-aims severity.
		if strings.HasPrefix(line, "===") && strings.HasSuffix(line, "===") && len(line) > 5 {
			flush()
			if strings.Contains(line, "Symbol") {
				severity = FindingError
			} else {
				severity = FindingWarning
			}
			continue
		}
		if m := vulnHeaderRe.FindStringSubmatch(line); m != nil {
			flush()
			cur = &Finding{Tool: ToolGovulncheck, Severity: severity, Code: m[1]}
			titleOpen = true
			continue
		}
		if cur == nil {
			continue
		}
		if line == "" {
			// A wrapped title never contains a blank line, so this ends
			// title capture without ending the block.
			titleOpen = false
			continue
		}
		switch {
		case strings.HasPrefix(line, "Found in:"):
			titleOpen = false
			found = strings.TrimSpace(strings.TrimPrefix(line, "Found in:"))
		case strings.HasPrefix(line, "Fixed in:"):
			titleOpen = false
			fixed = strings.TrimSpace(strings.TrimPrefix(line, "Fixed in:"))
		case vulnTraceRe.MatchString(line):
			titleOpen = false
			if cur.File == "" {
				if iss := buildissues.Parse(vulnTraceRe.ReplaceAllString(line, "")); len(iss) > 0 {
					cur.File = iss[0].File
					cur.Line = iss[0].Line
					cur.Col = iss[0].Col
				}
			}
		case strings.HasPrefix(line, "More info:"),
			strings.HasPrefix(line, "Module:"),
			strings.HasPrefix(line, "Package:"),
			strings.HasPrefix(line, "Platforms:"),
			strings.HasPrefix(line, "Example traces found:"),
			line == "Standard library":
			titleOpen = false
		default:
			if titleOpen {
				title = append(title, line)
			}
		}
	}
	flush()
	return out
}

// pprofTopHeader is the exact column header `go tool pprof -top` prints
// above its rows; the table starts on the line after it.
var pprofTopHeader = []string{"flat", "flat%", "sum%", "cum", "cum%"}

// pprofLocRe matches a trailing "<file>.<ext>:<line>" column, which
// `-top` only emits at -lines granularity.
var pprofLocRe = regexp.MustCompile(`^(.+\.\w+):(\d+)$`)

// pprofHotFlatPercent is the flat% at or above which a row is called out
// as a warning rather than plain info. Ten percent of a profile in one
// frame is the point at which a hot spot is worth a second look; below
// it the rows are just the ranking.
const pprofHotFlatPercent = 10.0

// ParsePprofTop parses the `go tool pprof -top` table:
//
//	 flat  flat%   sum%        cum   cum%
//	300ms 30.61% 30.61%      400ms 40.82%  runtime.mallocgc
//	200ms 20.41% 51.02%      200ms 20.41%  paint.(*Font).TextExtents
//
// One Finding per row, in pprof's own (descending flat) order:
//
//	Message  — "<frame> (flat 300ms 30.61%, cum 400ms 40.82%)", i.e. the
//	           frame plus the numbers that justify its position.
//	Code     — the flat percentage.
//	Severity — warning at or above pprofHotFlatPercent flat, else info.
//	           Profile rows are not defects, so nothing here is an error.
//	File/Line— set only when pprof was asked for -lines granularity and
//	           appended a "file.go:123" column; otherwise empty, and the
//	           pane offers no jump for the row.
//
// Everything before the column header (File/Type/Duration/Showing/
// Dropped preamble) is skipped, and the table ends at the first line
// with fewer than six columns.
func ParsePprofTop(output string) []Finding {
	var out []Finding
	inTable := false
	for _, raw := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimRight(raw, "\r"))
		if !inTable {
			if isPprofTopHeader(fields) {
				inTable = true
			}
			continue
		}
		if len(fields) < 6 {
			break
		}
		flat, flatPct, cum, cumPct := fields[0], fields[1], fields[3], fields[4]
		name := strings.Join(fields[5:], " ")
		file, lineNo := "", 0
		if len(fields) >= 7 {
			if m := pprofLocRe.FindStringSubmatch(fields[len(fields)-1]); m != nil {
				file = m[1]
				lineNo, _ = strconv.Atoi(m[2])
				name = strings.Join(fields[5:len(fields)-1], " ")
			}
		}
		severity := FindingInfo
		if parsePprofPercent(flatPct) >= pprofHotFlatPercent {
			severity = FindingWarning
		}
		out = append(out, Finding{
			Tool:     ToolPprofTop,
			File:     file,
			Line:     lineNo,
			Severity: severity,
			Message:  name + " (flat " + flat + " " + flatPct + ", cum " + cum + " " + cumPct + ")",
			Code:     flatPct,
		})
	}
	return out
}

// isPprofTopHeader reports whether a line's columns are exactly pprof's
// "flat flat% sum% cum cum%" header.
func isPprofTopHeader(fields []string) bool {
	if len(fields) != len(pprofTopHeader) {
		return false
	}
	for i, want := range pprofTopHeader {
		if fields[i] != want {
			return false
		}
	}
	return true
}

// parsePprofPercent reads a pprof percentage cell ("30.61%", "100%", or
// a bare "0"). Anything unparsable counts as 0, which only ever demotes
// a row to info.
func parsePprofPercent(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "%"), 64)
	if err != nil {
		return 0
	}
	return v
}

// raceOpRe matches the access line that opens a race report section,
// e.g. "Write at 0x00c0000b4010 by goroutine 8:". The detector varies the
// wording ("Previous write", "Atomic write", "Previous atomic read") and
// only capitalises the first word, hence the case-insensitive alternation.
var raceOpRe = regexp.MustCompile(`(?i)^(?:previous )?(?:atomic )?(?:write|read) at 0x[0-9a-fA-F]+ by .+:$`)

// raceFrameRe matches a stack frame under such an access,
// e.g. "/Users/x/silk/core/bus.go:42 +0x64". The mandatory "+0x" offset
// is what separates a frame from any other "file:line" text.
var raceFrameRe = regexp.MustCompile(`^(.+\.\w+):(\d+)\s+\+0x[0-9a-fA-F]+$`)

// raceSeparator is the "=====" rule the detector prints around a report.
const raceSeparator = "=================="

// ParseRaceFindings parses `go test -race` output. Two kinds of finding
// come back, in this order:
//
//  1. every vet-style "file:line: message" line (build errors and the
//     indented details under a `--- FAIL` header), via ParseVetFindings;
//
//  2. one Finding per "WARNING: DATA RACE" report, located at the first
//     stack frame under the first access:
//
//     ==================
//     WARNING: DATA RACE
//     Write at 0x00c0000b4010 by goroutine 8:
//     core.(*Bus).Publish()
//     /Users/x/silk/core/bus.go:42 +0x64
//     ...
//     ==================
//
//     Code is "DATA RACE" and Message is the access line without its
//     trailing colon ("Write at 0x00c0000b4010 by goroutine 8"), which is
//     what identifies the report in a one-line row.
//
// The two passes cannot double-count: a race report's frames carry a
// "+0x" offset and no ": " after the line number, so buildissues.Parse
// skips every line of them.
func ParseRaceFindings(output string) []Finding {
	out := ParseVetFindings(ToolGoTestRace, output)

	var cur *Finding
	flush := func() {
		if cur == nil {
			return
		}
		if cur.Message == "" {
			cur.Message = "data race"
		}
		out = append(out, *cur)
		cur = nil
	}
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "WARNING: DATA RACE" {
			flush()
			cur = &Finding{Tool: ToolGoTestRace, Severity: FindingError, Code: "DATA RACE"}
			continue
		}
		if cur == nil {
			continue
		}
		if strings.HasPrefix(line, raceSeparator) {
			flush()
			continue
		}
		if raceOpRe.MatchString(line) {
			if cur.Message == "" {
				cur.Message = strings.TrimSuffix(line, ":")
			}
			continue
		}
		if m := raceFrameRe.FindStringSubmatch(line); m != nil && cur.File == "" {
			cur.File = m[1]
			cur.Line, _ = strconv.Atoi(m[2])
		}
	}
	flush()
	return out
}

// traceURLRe matches the viewer address `go tool trace` prints.
var traceURLRe = regexp.MustCompile(`https?://[^\s]+`)

// ParseTraceServerURL returns the web-UI address from `go tool trace`
// output, or "" when it has not printed one yet. The last URL in the
// output wins: the tool logs its parsing progress first and announces
// the listening address last ("Serving web UI on http://127.0.0.1:57263").
// The host opens that URL in a browser — the trace viewer produces no
// findings to fold into the pane.
func ParseTraceServerURL(output string) string {
	matches := traceURLRe.FindAllString(output, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimRight(matches[len(matches)-1], ".,)")
}

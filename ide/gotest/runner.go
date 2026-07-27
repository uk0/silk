// Package gotest runs `go test -json` and folds its event stream into the
// aggregated model in core (core.Aggregator / core.PackageResult).
//
// Two things it buys over capturing `go test -v` console text:
//
//   - Attribution. Every JSON event carries its own Package/Test, so results
//     stay correct when several packages run in parallel and their output
//     interleaves. Nothing is guessed after the fact.
//   - Unambiguous re-runs. A run is always package-qualified: RerunTest takes
//     the package the test actually lives in and an anchored -run pattern, so
//     two packages holding a test of the same name no longer collide the way a
//     bare name across `./...` does.
//
// The package is UI-agnostic: it starts a process and calls back with parsed
// events, nothing else.
package gotest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"

	"github.com/uk0/silk/core"
)

// Options describes one `go test -json` invocation.
type Options struct {
	// Packages are the package arguments: import paths ("github.com/uk0/silk/gui")
	// or relative patterns ("./gui/"). Empty means "./..." — the whole module.
	Packages []string
	// Run is the -run regexp. Empty omits the flag. Build it with RunRegexp so
	// the anchoring matches go test's per-segment matching.
	Run string
	// Count1 adds -count=1. A re-run needs it: without it the test cache
	// replays the previous result instead of executing the test again.
	Count1 bool
}

// Args returns the argv (minus the go binary itself) for o. Pure — it starts
// nothing — so the exact command line can be echoed into a build log and
// asserted in tests.
func Args(o Options) []string {
	args := []string{"test", "-json"}
	if o.Count1 {
		args = append(args, "-count=1")
	}
	if o.Run != "" {
		args = append(args, "-run", o.Run)
	}
	if len(o.Packages) == 0 {
		args = append(args, "./...")
	} else {
		args = append(args, o.Packages...)
	}
	return args
}

// Update is one streaming notification, delivered once per line of the event
// stream after that line has been folded into Agg.
//
// Event.Action is empty when the line was not a JSON event (a build error from
// an older toolchain, or a diagnostic the go command wrote to stderr); such
// lines land in Agg.BuildOutput().
//
// Agg is the live aggregator, not a copy. Callbacks are serialized and run on
// the runner's reader goroutine, so reading it inside the callback is safe;
// handing state to another thread (a GUI thread, say) requires Agg.Clone().
type Update struct {
	Event core.TestEvent
	Raw   string
	Agg   *core.Aggregator
}

// Runner runs `go test -json` in one module directory.
type Runner struct {
	Dir string   // module directory; "" uses the process working directory
	Go  string   // go binary; "" uses "go" from PATH
	Env []string // full child environment; nil inherits the parent's
}

// Run executes `go test -json` for o, streaming an Update per line, and
// returns the aggregator holding the folded results. It blocks until the
// toolchain exits (or ctx is cancelled) and never returns a nil aggregator, so
// partial results from a cancelled or crashed run are still inspectable.
//
// Failing tests are NOT an error: `go test` exits non-zero for them, and that
// exit status is swallowed here. Judge the outcome from the aggregator —
// Counts(), a package Status, or a non-empty BuildOutput(). A non-nil error
// means the run itself did not complete: the binary could not start, ctx was
// cancelled (errors.Is(err, context.Canceled) holds), or the stream could not
// be read.
func (r *Runner) Run(ctx context.Context, o Options, onUpdate func(Update)) (*core.Aggregator, error) {
	agg := core.NewAggregator()

	bin := r.Go
	if bin == "" {
		bin = "go"
	}
	cmd := exec.CommandContext(ctx, bin, Args(o)...)
	cmd.Dir = r.Dir
	if len(r.Env) > 0 {
		cmd.Env = r.Env
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return agg, fmt.Errorf("go test -json: stdout pipe: %w", err)
	}
	// stderr carries the go command's own diagnostics (an old toolchain's
	// compile errors, "no packages to test", a bad flag) — plain text, never
	// JSON. Buffering it instead of scanning it in a second goroutine keeps a
	// single writer on the aggregator: no lock, and no chance of a stderr
	// write splicing itself into the middle of a JSON line.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return agg, fmt.Errorf("go test -json: %w", err)
	}

	streamErr := stream(agg, stdout, onUpdate)
	if streamErr != nil {
		// Parsing stopped early (an absurdly long line). The pipe still has to
		// be drained: a child blocked writing into a full pipe would never
		// exit, and Wait would never return.
		_, _ = io.Copy(io.Discard, stdout)
	}
	waitErr := cmd.Wait()

	// Fold the buffered stderr afterwards so its lines still reach
	// BuildOutput (and the callback) in the order they can be attributed:
	// after the events, since nothing links them to a test.
	for _, line := range splitLines(stderr.String()) {
		ev := agg.AddLine(line)
		if onUpdate != nil {
			onUpdate(Update{Event: ev, Raw: line, Agg: agg})
		}
	}

	// Cancellation wins over the "signal: killed" that Wait reports for it.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return agg, fmt.Errorf("go test -json: %w", ctxErr)
	}
	if streamErr != nil {
		return agg, streamErr
	}
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		return agg, fmt.Errorf("go test -json: %w", waitErr)
	}
	return agg, nil
}

// stream folds every line of r into agg, reporting each one through onUpdate.
// Split out from Run so the streaming contract can be tested against a canned
// stream without starting a process.
func stream(agg *core.Aggregator, r io.Reader, onUpdate func(Update)) error {
	return agg.Feed(r, func(ev core.TestEvent, raw string) {
		if onUpdate != nil {
			onUpdate(Update{Event: ev, Raw: raw, Agg: agg})
		}
	})
}

// RunRegexp builds an anchored -run pattern for one test or subtest path.
//
// `go test -run` splits its pattern on "/" and matches each element against
// the corresponding level of the test name, so a subtest path has to be
// anchored element by element: "TestA/sub" becomes `^TestA$/^sub$`. Each
// element is quoted, so a table-driven subtest named "a+b" matches literally
// instead of as a regexp.
func RunRegexp(test string) string {
	if test == "" {
		return ""
	}
	parts := strings.Split(test, "/")
	for i, p := range parts {
		parts[i] = "^" + regexp.QuoteMeta(p) + "$"
	}
	return strings.Join(parts, "/")
}

// RerunTest returns Options that re-run exactly one test in exactly one
// package — the package-qualified form. An empty pkg falls back to the whole
// module, which is the ambiguous case this exists to avoid; pass the Package
// from the test's own core.PackageResult.
func RerunTest(pkg, test string) Options {
	o := Options{Run: RunRegexp(test), Count1: true}
	if pkg != "" {
		o.Packages = []string{pkg}
	}
	return o
}

// RerunFailed returns one Options per package that needs re-running after a
// run, in the packages' first-seen order. Each entry is package-qualified and
// carries -count=1 so cached passes cannot mask the re-run.
//
// Failing subtests are folded into their top-level test: -run's element-wise
// matching cannot express "these particular subtests under those particular
// parents" in a single pattern, and a subtest failure always fails its parent
// too, so collapsing to the root name loses no failure. A package that failed
// without any failing test — a build failure, or a panic that took the binary
// down before any result — is re-run whole, with no -run filter.
func RerunFailed(agg *core.Aggregator) []Options {
	if agg == nil {
		return nil
	}
	var out []Options
	for _, pkg := range agg.Packages() {
		var roots []string
		seen := make(map[string]bool)
		for _, t := range pkg.TestList() {
			if t.Status != core.TestStatusFail {
				continue
			}
			root := t.Name
			if i := strings.IndexByte(root, '/'); i > 0 {
				root = root[:i]
			}
			if seen[root] {
				continue
			}
			seen[root] = true
			roots = append(roots, regexp.QuoteMeta(root))
		}
		switch {
		case len(roots) > 0:
			out = append(out, Options{
				Packages: []string{pkg.Package},
				Run:      "^(" + strings.Join(roots, "|") + ")$",
				Count1:   true,
			})
		case pkg.Status == core.TestStatusFail:
			out = append(out, Options{
				Packages: []string{pkg.Package},
				Count1:   true,
			})
		}
	}
	return out
}

// splitLines splits captured text into lines, dropping the trailing empty
// element a final newline produces. Returns nil for empty input.
func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

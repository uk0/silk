package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// The gutter is the only place a user sets a breakpoint once the debugger is
// already up, so a toggle made during a live session has to reach dlv itself —
// the launch-time snapshot in runProjectInDebugger can only ever carry what
// existed before the session started, and a file opened afterwards was never in
// it at all. These tests therefore watch the wire rather than the editor: a
// recording fake dlv sits on the far end of a real core.DebugSession and every
// assertion is about the request that arrived there, at which file and line.
//
// Harness: core.LaunchDebug insists on a `dlv` on PATH and dials the port it
// picked itself, so a stub binary takes that name, publishes the --listen
// address it was handed, and blocks; the test process binds that address and
// answers the RPCs in-process, which is what keeps the traffic assertable as
// values instead of as log lines.

// fakeDlvStubSrc stands in for the dlv binary silkide launches. All it has to
// do is hand the port back — the protocol lives in the test process.
const fakeDlvStubSrc = `package main

import (
	"os"
	"strings"
	"time"
)

func main() {
	out := os.Getenv("FAKE_DLV_LISTEN")
	for _, a := range os.Args[1:] {
		if !strings.HasPrefix(a, "--listen=") {
			continue
		}
		// Write-then-rename: the poller on the other side must never read a
		// half-written address.
		if err := os.WriteFile(out+".tmp", []byte(strings.TrimPrefix(a, "--listen=")), 0o644); err == nil {
			_ = os.Rename(out+".tmp", out)
		}
	}
	time.Sleep(2 * time.Minute) // suicide timer: a wedged run must not leak us
}
`

// dlvReq is one RPC as it arrived at the fake dlv. File/Line carry the location
// the request is about — the spec for CreateBreakpoint, the location dlv had
// remembered under that id for ClearBreakpoint — so a test can assert where the
// debugger was told to put (or drop) a breakpoint without reaching back into
// editor state.
type dlvReq struct {
	Method string
	ID     int
	File   string
	Line   int
}

type fakeDlv struct {
	reqs chan dlvReq // every RPC that reached the endpoint, in wire order
	fail chan error  // harness failure from the serving goroutine

	mu    sync.Mutex
	bps   map[int]dlvReq // id → location: dlv's own breakpoint table
	next  int
	delay time.Duration // stall before answering (see the UI-thread test)
	ln    net.Listener
	conn  net.Conn
}

// startFakeDlv builds the stub, puts it first on PATH and starts serving the
// port it publishes. Everything is torn down with the test.
func startFakeDlv(t *testing.T) *fakeDlv {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; cannot build the fake dlv stub")
	}
	tmp := t.TempDir()
	stubDir := filepath.Join(tmp, "stub")
	binDir := filepath.Join(tmp, "bin")
	for _, d := range []string{stubDir, binDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(stubDir, "main.go"), []byte(fakeDlvStubSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stubDir, "go.mod"), []byte("module fakedlv\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stub := "dlv"
	if runtime.GOOS == "windows" {
		stub = "dlv.exe" // LookPath only honours PATHEXT names there
	}
	build := exec.Command("go", "build", "-o", filepath.Join(binDir, stub), ".")
	build.Dir = stubDir
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake dlv stub: %v\n%s", err, out)
	}

	listenFile := filepath.Join(tmp, "listen")
	t.Setenv("FAKE_DLV_LISTEN", listenFile)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	f := &fakeDlv{
		reqs: make(chan dlvReq, 32),
		fail: make(chan error, 1),
		bps:  map[int]dlvReq{},
	}
	go f.serve(listenFile)
	t.Cleanup(f.close)
	return f
}

func (f *fakeDlv) serve(listenFile string) {
	addr, err := waitForListenAddr(listenFile, 60*time.Second)
	if err != nil {
		f.fail <- err
		return
	}
	// LaunchDebug picked this port and released it again, so binding it here is
	// the same handoff dlv itself would do.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		f.fail <- fmt.Errorf("bind %s: %w", addr, err)
		return
	}
	f.mu.Lock()
	f.ln = ln
	f.mu.Unlock()
	conn, err := ln.Accept()
	if err != nil {
		return // closed by cleanup
	}
	f.mu.Lock()
	f.conn = conn
	f.mu.Unlock()
	dec := json.NewDecoder(conn)
	for {
		var req struct {
			ID     int
			Method string
			Params []json.RawMessage
		}
		if err := dec.Decode(&req); err != nil {
			return
		}
		f.handle(conn, req.ID, strings.TrimPrefix(req.Method, "RPCServer."), req.Params)
	}
}

// handle records the request and answers it in dlv's newline-JSON shape. Only
// the breakpoint methods carry state; anything else is acknowledged so an
// unexpected call cannot wedge the session under test.
func (f *fakeDlv) handle(conn net.Conn, id int, method string, params []json.RawMessage) {
	var first json.RawMessage
	if len(params) > 0 {
		first = params[0]
	}
	rec := dlvReq{Method: method}
	result := "null"

	f.mu.Lock()
	switch method {
	case "CreateBreakpoint":
		var in struct {
			Breakpoint struct {
				File string
				Line int
			}
		}
		_ = json.Unmarshal(first, &in)
		f.next++
		rec.ID, rec.File, rec.Line = f.next, in.Breakpoint.File, in.Breakpoint.Line
		f.bps[rec.ID] = rec
		result = fmt.Sprintf(`{"Breakpoint":{"id":%d,"file":%q,"line":%d,"addr":4096}}`, rec.ID, rec.File, rec.Line)
	case "ListBreakpoints":
		parts := make([]string, 0, len(f.bps))
		for _, bp := range f.bps {
			parts = append(parts, fmt.Sprintf(`{"id":%d,"file":%q,"line":%d,"addr":4096}`, bp.ID, bp.File, bp.Line))
		}
		result = `{"Breakpoints":[` + strings.Join(parts, ",") + `]}`
	case "ClearBreakpoint":
		var in struct{ ID int }
		_ = json.Unmarshal(first, &in)
		// Report the location the id stood for: the claim under test is "the
		// breakpoint at F:L was cleared", not "some id went away".
		rec = f.bps[in.ID]
		rec.Method = method
		if rec.ID == 0 {
			rec.ID = in.ID // unknown id: still say what was asked for
		}
		delete(f.bps, in.ID)
	}
	delay := f.delay
	f.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}
	fmt.Fprintf(conn, "{\"id\":%d,\"result\":%s,\"error\":null}\n", id, result)
	// Publish only once the answer is out, so a test that returns the moment it
	// sees a request doesn't tear the session down under a call still waiting
	// for its reply.
	select {
	case f.reqs <- rec:
	default: // generously sized; a full channel means nobody is reading
	}
}

// waitFor returns the next request with the given method, skipping the ones a
// call legitimately makes on the way — ClearBreakpointByLocation resolves the id
// with a ListBreakpoints first.
func (f *fakeDlv) waitFor(t *testing.T, method string, timeout time.Duration) dlvReq {
	t.Helper()
	deadline := time.After(timeout)
	var seen []string
	for {
		select {
		case req := <-f.reqs:
			if req.Method == method {
				return req
			}
			seen = append(seen, req.Method)
		case err := <-f.fail:
			t.Fatalf("fake dlv harness: %v", err)
		case <-deadline:
			t.Fatalf("no %s reached dlv within %s (RPCs seen: %v)", method, timeout, seen)
		}
	}
}

func (f *fakeDlv) setReplyDelay(d time.Duration) {
	f.mu.Lock()
	f.delay = d
	f.mu.Unlock()
}

func (f *fakeDlv) close() {
	f.mu.Lock()
	ln, conn := f.ln, f.conn
	f.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	if ln != nil {
		_ = ln.Close()
	}
}

func waitForListenAddr(path string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if addr := strings.TrimSpace(string(b)); strings.Contains(addr, ":") {
				return addr, nil
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return "", fmt.Errorf("fake dlv never published its listen address to %s", path)
}

// isolateIDEGlobals hands the test its own editor registry and a session
// pointer that starts out nil — the state silkide sits in before anything has
// been launched — and puts both back when the test ends. Split out of
// liveDebugSession so a test can choose when the session appears: the two
// orders are not interchangeable (see the open-before-launch test).
func isolateIDEGlobals(t *testing.T) {
	t.Helper()
	gui.SetUIWakeup(nil) // headless: Post must not nudge GLFW (see gui/uiqueue.go)
	savedEditors, savedDebug := openEditors, globalDebug
	openEditors = map[string]*gui.CodeEditor{}
	globalDebug = nil
	t.Cleanup(func() { openEditors, globalDebug = savedEditors, savedDebug })
}

// newDebuggeeFile writes the file these tests toggle in, alone in its own
// directory so LaunchDebug has a package to point dlv at.
func newDebuggeeFile(t *testing.T) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "main.go")
	src := "package main\n\nfunc main() {\n\tprintln(1)\n\tprintln(2)\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

// openEditorFor registers a real editor tab through openFileInEditor — the one
// place silkide binds an editor to a path, and so the only place the gutter
// hook gets wired.
func openEditorFor(t *testing.T, path string) *gui.CodeEditor {
	t.Helper()
	if !openFileInEditor(gui.NewTabWidget(), path) {
		t.Fatal("openFileInEditor failed")
	}
	ed := openEditors[path]
	if ed == nil {
		t.Fatal("no editor registered for the opened file")
	}
	return ed
}

// liveDebugSession brings up the production pieces the property needs: a real
// core.DebugSession pointed at the fake dlv, and a real editor tab registered
// through openFileInEditor — the one place silkide binds an editor to a path.
func liveDebugSession(t *testing.T, f *fakeDlv) (*gui.CodeEditor, string) {
	t.Helper()
	dir, path := newDebuggeeFile(t)

	sess, err := core.LaunchDebug(dir, nil)
	if err != nil {
		t.Fatalf("LaunchDebug against the fake dlv: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	isolateIDEGlobals(t)
	// runProjectInDebugger publishes the session with this same assignment, but
	// from inside a gui.Post closure, and package main has no way to drain that
	// queue headless (see ui_post_test.go) — so stand in for the UI thread here.
	globalDebug = sess

	// Opening the file AFTER the launch is the second half of the defect: such a
	// file could not have been in the launch snapshot either.
	return openEditorFor(t, path), path
}

// TestGutterToggleDuringLiveSessionReachesDlv: a breakpoint toggled on while the
// session is live must arrive at dlv as a CreateBreakpoint for exactly that file
// and that line — the editor counts lines from 0, the wire from 1.
func TestGutterToggleDuringLiveSessionReachesDlv(t *testing.T) {
	f := startFakeDlv(t)
	ed, path := liveDebugSession(t, f)

	ed.ToggleBreakpoint(3) // what the gutter click and F9 both call

	req := f.waitFor(t, "CreateBreakpoint", 10*time.Second)
	if req.File != path {
		t.Errorf("CreateBreakpoint file = %q, want %q", req.File, path)
	}
	if req.Line != 4 {
		t.Errorf("CreateBreakpoint line = %d, want 4 (editor line 3, 1-based on the wire)", req.Line)
	}
}

// TestGutterToggleOffClearsTheSameBreakpoint: toggling the same line off again
// must clear the breakpoint that was created there — the id dlv handed back for
// that file:line, not merely "some breakpoint".
func TestGutterToggleOffClearsTheSameBreakpoint(t *testing.T) {
	f := startFakeDlv(t)
	ed, path := liveDebugSession(t, f)

	ed.ToggleBreakpoint(3)
	created := f.waitFor(t, "CreateBreakpoint", 10*time.Second)
	if created.File != path || created.Line != 4 {
		t.Fatalf("CreateBreakpoint hit %s:%d, want %s:4", created.File, created.Line, path)
	}

	ed.ToggleBreakpoint(3) // same line, off again

	cleared := f.waitFor(t, "ClearBreakpoint", 10*time.Second)
	if cleared.ID != created.ID {
		t.Errorf("ClearBreakpoint id = %d, want %d (the breakpoint created at %s:4)", cleared.ID, created.ID, path)
	}
	if cleared.File != path || cleared.Line != 4 {
		t.Errorf("ClearBreakpoint hit %s:%d, want %s:4", cleared.File, cleared.Line, path)
	}
}

// TestGutterToggleDoesNotBlockTheUIThread: ToggleBreakpoint runs on the UI
// thread (gutter click / F9) and the RPC serializes on the session mutex, which
// a running debuggee holds until it next stops — so the call must be issued off
// the UI thread. With the debugger stalling its reply, the toggle still has to
// return at once, and the request still has to arrive.
func TestGutterToggleDoesNotBlockTheUIThread(t *testing.T) {
	f := startFakeDlv(t)
	ed, path := liveDebugSession(t, f)
	f.setReplyDelay(2 * time.Second)

	start := time.Now()
	ed.ToggleBreakpoint(3)
	elapsed := time.Since(start)

	req := f.waitFor(t, "CreateBreakpoint", 10*time.Second)
	if req.File != path || req.Line != 4 {
		t.Fatalf("CreateBreakpoint hit %s:%d, want %s:4", req.File, req.Line, path)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("ToggleBreakpoint took %s while the debugger stalled; the RPC must not run on the UI thread", elapsed)
	}
}

// TestGutterToggleOnAnEditorOlderThanTheSessionReachesDlv: production order is
// open first, launch second — you browse the code, then hit Debug — so every
// editor silkide wires is wired while globalDebug is still nil. The hook must
// therefore read the session when the toggle fires, not close over whatever was
// there at wiring time; capturing it early leaves every file opened before the
// launch permanently deaf to its own gutter.
func TestGutterToggleOnAnEditorOlderThanTheSessionReachesDlv(t *testing.T) {
	f := startFakeDlv(t)
	dir, path := newDebuggeeFile(t)
	isolateIDEGlobals(t)

	ed := openEditorFor(t, path) // wired with no debugger running

	sess, err := core.LaunchDebug(dir, nil)
	if err != nil {
		t.Fatalf("LaunchDebug against the fake dlv: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	globalDebug = sess // the assignment runProjectInDebugger makes, after the fact

	ed.ToggleBreakpoint(3)

	created := f.waitFor(t, "CreateBreakpoint", 10*time.Second)
	if created.File != path {
		t.Errorf("CreateBreakpoint file = %q, want %q", created.File, path)
	}
	if created.Line != 4 {
		t.Errorf("CreateBreakpoint line = %d, want 4 (editor line 3, 1-based on the wire)", created.Line)
	}

	// "Permanently deaf" covers both directions, and only the off-toggle can be
	// wrong on its own: an editor that can set a breakpoint but not clear one
	// leaves the debugger stopping at a line whose gutter dot the user has
	// already turned off, with nothing on screen to explain it.
	ed.ToggleBreakpoint(3) // same line, off again

	cleared := f.waitFor(t, "ClearBreakpoint", 10*time.Second)
	if cleared.ID != created.ID {
		t.Errorf("ClearBreakpoint id = %d, want %d (the breakpoint created at %s:4)", cleared.ID, created.ID, path)
	}
	if cleared.File != path || cleared.Line != 4 {
		t.Errorf("ClearBreakpoint hit %s:%d, want %s:4", cleared.File, cleared.Line, path)
	}
}

// noSessionChildEnv marks the re-executed half of the no-session test below.
const noSessionChildEnv = "SILKIDE_TEST_TOGGLE_WITHOUT_SESSION"

// TestGutterToggleWithoutASessionDoesNotKillTheIDE: setting breakpoints before
// launching anything is the ordinary way to use a debugger, and the hook fires
// on every one of those clicks. With no session the only right move is to drop
// it — runProjectInDebugger will snapshot the gutter at launch — because the RPC
// runs in a bare goroutine, where a nil session is not an error return but a
// dereference that takes the process with it.
//
// That is why this runs the toggle in a child process: an unrecovered panic in
// a goroutine cannot be caught by the goroutine that started it, so the only
// place the crash is observable as a value is the child's exit status. The child
// waits before returning, so the goroutine gets its chance to run rather than
// being outlived by the test.
//
// On its own "nothing happened" would also pass if the hook were never wired;
// what rules that out is the tests above, which fire the same hook on the same
// openFileInEditor path and watch it reach the wire.
func TestGutterToggleWithoutASessionDoesNotKillTheIDE(t *testing.T) {
	if os.Getenv(noSessionChildEnv) == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), noSessionChildEnv+"=1")
		out, err := cmd.CombinedOutput()
		if err == nil {
			return
		}
		if strings.Contains(string(out), "panic:") {
			t.Fatalf("a gutter toggle with no debugger running crashed the process — the IDE goes down with it (%v):\n%s", err, out)
		}
		t.Fatalf("no-session toggle scenario failed (%v):\n%s", err, out)
	}

	// Child: no fake dlv, no session — just the editor a user opens first.
	_, path := newDebuggeeFile(t)
	isolateIDEGlobals(t)
	ed := openEditorFor(t, path)

	ed.ToggleBreakpoint(3) // set one before launching, as everybody does
	ed.ToggleBreakpoint(3) // and change your mind: the clear path derefs too

	// Nothing to wait *for* — a correct hook starts no goroutine at all — so give
	// the runtime room to schedule the one a broken hook would have started.
	time.Sleep(500 * time.Millisecond)
}

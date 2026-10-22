package ged

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/gui"
)

func explorerFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, n := range []string{"pkg/deep/中文.go", "docs/readme.md", "icon/app.png", "vendor/ignore.go", ".hidden", "main.go", "z.go", "a.go"} {
		p := filepath.Join(root, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func scanRequest(root string) explorerScanRequest {
	return explorerScanRequest{root: root, expanded: map[string]bool{root: true}, excluded: copyExplorerSet(skipDirs)}
}
func findEntry(e *fileEntry, path string) *fileEntry {
	if e == nil {
		return nil
	}
	if e.path == path {
		return e
	}
	for _, c := range e.children {
		if got := findEntry(c, path); got != nil {
			return got
		}
	}
	return nil
}
func testExplorer(t *testing.T) (*FileExplorer, func()) {
	t.Helper()
	e := NewFileExplorer()
	e.SetSize(250, 240)
	queue := make(chan func(), 8)
	e.post = func(f func()) { queue <- f }
	t.Cleanup(func() {
		e.CancelScan()
		until := time.Now().Add(4 * time.Second)
		for e.scanWorker != nil {
			e.scanWorker.mu.Lock()
			running := e.scanWorker.running
			e.scanWorker.mu.Unlock()
			if !running {
				return
			}
			if time.Now().After(until) {
				t.Error("scan worker did not stop")
				return
			}
			time.Sleep(time.Millisecond)
		}
	})
	return e, func() {
		t.Helper()
		timeout := time.NewTimer(4 * time.Second)
		defer timeout.Stop()
		for e.Loading() {
			select {
			case f := <-queue:
				f()
			case <-timeout.C:
				t.Fatal("scan UI delivery timed out")
			}
		}
	}
}
func TestExplorerScanLazyFilterAndHidden(t *testing.T) {
	root := explorerFixture(t)
	req := scanRequest(root)
	r := scanExplorer(context.Background(), req)
	if r.err != nil {
		t.Fatal(r.err)
	}
	var names []string
	for _, e := range r.root.children {
		names = append(names, e.name)
	}
	if !reflect.DeepEqual(names, []string{"docs", "icon", "pkg", "a.go", "main.go", "z.go"}) {
		t.Fatal(names)
	}
	pkg := findEntry(r.root, filepath.Join(root, "pkg"))
	if pkg.loaded || pkg.expanded || len(pkg.children) != 0 {
		t.Fatalf("collapsed directory traversed: %+v", pkg)
	}
	req.hidden = true
	req.filter = "中文"
	r = scanExplorer(context.Background(), req)
	if r.err != nil || findEntry(r.root, filepath.Join(root, "pkg", "deep", "中文.go")) == nil {
		t.Fatalf("Unicode filter: %+v", r)
	}
	if len(r.root.children) != 1 || r.root.children[0].name != "pkg" {
		t.Fatal("filter retained unrelated siblings")
	}
	req.filter = ""
	r = scanExplorer(context.Background(), req)
	if findEntry(r.root, filepath.Join(root, ".hidden")) == nil {
		t.Fatal("hidden toggle lost file")
	}
	req.expanded[root] = false
	r = scanExplorer(context.Background(), req)
	if r.root.expanded || r.root.loaded || len(r.root.children) != 0 {
		t.Fatal("root cannot remain collapsed")
	}
}
func TestExplorerScanLimitsErrorsAndCancellation(t *testing.T) {
	root := explorerFixture(t)
	req := scanRequest(root)
	req.limit = 2
	r := scanExplorer(context.Background(), req)
	if diagnostic.CodeOf(r.err) != diagnostic.FileScan || len(r.root.children) > 2 {
		t.Fatalf("limit: %+v", r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r = scanExplorer(ctx, req); !errors.Is(r.err, context.Canceled) {
		t.Fatalf("cancel: %v", r.err)
	}
	req.root = filepath.Join(root, "missing")
	r = scanExplorer(context.Background(), req)
	if diagnostic.CodeOf(r.err) != diagnostic.FileScan || !errors.Is(r.err, os.ErrNotExist) {
		t.Fatal(r.err)
	}
	req = scanRequest(root)
	req.filter = "needle"
	p := root
	for i := 0; i < explorerDepthLimit+3; i++ {
		p = filepath.Join(p, "d")
	}
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
	r = scanExplorer(context.Background(), req)
	if diagnostic.CodeOf(r.err) != diagnostic.FileScan || !strings.Contains(r.err.Error(), "depth") {
		t.Fatalf("depth: %v", r.err)
	}
}
func TestExplorerScanDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Fatal(err)
	}
	req := scanRequest(root)
	req.filter = "outside"
	r := scanExplorer(context.Background(), req)
	if r.err != nil {
		t.Fatal(r.err)
	}
	e := findEntry(r.root, filepath.Join(root, "outside"))
	if e == nil || !e.isSymlink || !e.isDir || len(e.children) != 0 {
		t.Fatalf("followed external link: %+v", e)
	}
	req.filter = "secret"
	r = scanExplorer(context.Background(), req)
	if len(r.root.children) != 0 {
		t.Fatal("traversed symlink target")
	}
}
func TestExplorerStateRevealKeyboardAndStale(t *testing.T) {
	root := explorerFixture(t)
	e, wait := testExplorer(t)
	e.SetRootDir(root)
	wait()
	path := filepath.Join(root, "pkg", "deep", "中文.go")
	if !e.RevealPath(path) {
		t.Fatal("reveal rejected")
	}
	wait()
	if e.SelectedPath() != path {
		t.Fatal(e.SelectedPath())
	}
	e.Refresh()
	wait()
	if e.SelectedPath() != path {
		t.Fatal("refresh lost selection")
	}
	var opened string
	e.SigFileOpen(func(p string) { opened = p })
	e.OnKeyDown(gui.KeyEnter, false)
	if opened != path {
		t.Fatal("Enter did not open selected file")
	}
	e.OnKeyDown(gui.KeyLeft, false)
	if e.SelectedPath() != filepath.Dir(path) {
		t.Fatal("Left failed parent navigation")
	}
	e.OnKeyDown(gui.KeyLeft, false)
	wait()
	if findEntry(e.root, filepath.Dir(path)).expanded {
		t.Fatal("Left failed collapse")
	}
	e.OnKeyDown(gui.KeyRight, false)
	wait()
	if !findEntry(e.root, filepath.Dir(path)).expanded {
		t.Fatal("Right failed expand")
	}
	old := e.scanGeneration
	e.SetRootDir("")
	e.applyScan(explorerScanResult{root: &fileEntry{name: "stale"}, generation: old})
	if e.root != nil || e.Loading() {
		t.Fatal("stale snapshot resurrected closed project")
	}
	for _, key := range []int{gui.KeyHome, gui.KeyEnd, gui.KeyUp, gui.KeyDown, gui.KeyPageUp, gui.KeyPageDown, gui.KeyRight, gui.KeyLeft} {
		e.OnKeyDown(key, false)
		if e.selectedIdx != -1 {
			t.Fatalf("empty selection %d key %d", e.selectedIdx, key)
		}
	}
	e.SetRootDir(root)
	wait()
	if !e.RevealPath(root) {
		t.Fatal("root reveal")
	}
	wait()
	for p := range e.expanded {
		if p != root {
			t.Fatalf("root reveal expanded outside directory: %s", p)
		}
	}
	if e.RevealPath(filepath.Dir(root)) {
		t.Fatal("outside root accepted")
	}
	e.SetFilter("中文")
	wait()
	if e.Filter() != "中文" {
		t.Fatal("filter state")
	}
	e.OnKeyDown(gui.KeyEsc, false)
	wait()
	if e.Filter() != "" {
		t.Fatal("Esc filter clear")
	}
}
func TestExplorerWorkerLatestAndRetirement(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	result := make(chan explorerScanResult, 4)
	var calls atomic.Int32
	w := &explorerScanWorker{scan: func(ctx context.Context, r explorerScanRequest) explorerScanResult {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		return explorerScanResult{generation: r.generation}
	}, deliver: func(r explorerScanResult) { result <- r }}
	w.submit(explorerScanRequest{generation: 1})
	<-started
	for i := uint64(2); i <= 1000; i++ {
		w.submit(explorerScanRequest{generation: i})
	}
	close(release)
	select {
	case r := <-result:
		if r.generation != 1000 {
			t.Fatal(r.generation)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker stalled")
	}
	if calls.Load() != 2 {
		t.Fatalf("not coalesced: %d", calls.Load())
	}
	w.stop()
}
func TestExplorerUIQueueBoundedAndPhaseOrdered(t *testing.T) {
	var queue []func()
	var applied []explorerScanResult
	d := explorerDelivery{post: func(f func()) { queue = append(queue, f) }, apply: func(r explorerScanResult) { applied = append(applied, r) }}
	for i := uint64(1); i <= 1000; i++ {
		d.offer(explorerScanResult{generation: i})
		d.offer(explorerScanResult{generation: i, gitOnly: true})
	}
	d.offer(explorerScanResult{generation: 3})
	if len(queue) != 1 {
		t.Fatalf("UI queue unbounded: %d", len(queue))
	}
	queue[0]()
	if len(applied) != 2 || applied[0].generation != 1000 || applied[0].gitOnly || !applied[1].gitOnly {
		t.Fatalf("bad delivery: %+v", applied)
	}
}
func TestExplorerGitNULPathsAndConflicts(t *testing.T) {
	root := t.TempDir()
	data := []byte("?? spaced -> name.go\x00 M 中文.go\x00R  new\nname.go\x00old.go\x00UU conflict.go\x00AA both.go\x00DD deleted.go\x00?? ../escape\x00")
	got := parseGitPorcelainZ(data, root)
	want := map[string]rune{}
	for n, c := range map[string]rune{"spaced -> name.go": '?', "中文.go": 'M', "new\nname.go": 'A', "conflict.go": '!', "both.go": '!', "deleted.go": '!'} {
		want[filepath.Join(root, n)] = c
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
func TestExplorerGitIntegration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	p := filepath.Join(root, "中文 file.go")
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := scanExplorerGit(context.Background(), root); got[p] != '?' {
		t.Fatal(got)
	}
}
func TestExplorerGitSubtreeAndUntrackedNestedFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	sub := filepath.Join(root, "sub dir")
	nested := filepath.Join(sub, "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	file := filepath.Join(nested, "中文.go")
	outside := filepath.Join(root, "outside.go")
	for _, p := range []string{file, outside} {
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got := scanExplorerGit(context.Background(), sub)
	if got[file] != '?' || len(got) != 1 {
		t.Fatalf("subtree mapping omitted file or leaked sibling: %v", got)
	}
}

func TestExplorerElisionAndViewport(t *testing.T) {
	e := NewFileExplorer()
	e.SetSize(160, 120)
	e.ensureFonts()
	for _, s := range []string{"中文😀很长的名称.go", strings.Repeat("jW", 200)} {
		for _, w := range []float64{0, 3, 25, 80, 200} {
			got := explorerElide(e.normalFont, s, w)
			if !utf8.ValidString(got) || explorerTextWidth(e.normalFont, got) > w+.01 {
				t.Fatalf("elision %q width %g", got, w)
			}
		}
	}
	if explorerDisplayName("x\ny\tz") != "x�y�z" {
		t.Fatal("control characters not escaped")
	}
	for i := 0; i < 50000; i++ {
		e.flatList = append(e.flatList, &fileEntry{name: fmt.Sprint(i)})
	}
	e.scrollY = 1234.5
	first, last := e.visibleRows(120)
	if first < 1 || last-first > 7 || last < first {
		t.Fatalf("viewport %d..%d", first, last)
	}
}

func TestExplorerShortcutOwnership(t *testing.T) {
	e := NewFileExplorer()
	var override gui.IShortcutOverride = e
	if !override.HandleShortcut(gui.KeyF5, 0) || !override.HandleShortcut(gui.KeyF2, 0) {
		t.Fatal("host can steal focused explorer shortcuts")
	}
	for _, k := range []struct {
		key  int
		mods uint8
	}{{gui.KeyF5, gui.ModShift}, {gui.KeyF2, gui.ModAction}, {'S', gui.ModAction}, {'F', gui.ModAction | gui.ModShift}} {
		if override.HandleShortcut(k.key, k.mods) {
			t.Fatalf("unowned host shortcut stolen: %+v", k)
		}
	}
	e.SetEnabled(false)
	if override.HandleShortcut(gui.KeyF5, 0) {
		t.Fatal("disabled explorer consumed shortcut")
	}
}

func TestExplorerDirectoryDoubleClickSurvivesScan(t *testing.T) {
	e, wait := testExplorer(t)
	root := explorerFixture(t)
	e.SetRootDir(root)
	wait()
	e.ensureFonts()
	y := e.headerHeight() + 1.5*e.rowHeight // docs, the first directory after root
	e.OnLeftDown(40, y)
	wait()
	dir := filepath.Join(root, "docs")
	if !findEntry(e.root, dir).expanded {
		t.Fatal("first click did not expand")
	}
	// Keep the double-click time window deterministic even on slow CI disks.
	e.lastClickTime = time.Now()
	e.OnLeftDown(40, y)
	wait()
	if !findEntry(e.root, dir).expanded {
		t.Fatal("asynchronous scan reset double-click identity and collapsed the directory")
	}
}

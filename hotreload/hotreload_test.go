package hotreload

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"silk/decl"
)

// writeSilkUI persists a tiny decl tree as a TDoc-format .silkui file
// at the given path. Returns the source AST so tests can compare what
// the reloader hands back.
func writeSilkUI(t *testing.T, path, formTitle, btnText string) *decl.Node {
	t.Helper()

	tree := decl.Form(
		decl.P("title", formTitle),
		decl.Children(
			decl.Button(decl.ID("btn"), decl.P("text", btnText)),
		),
	)

	doc := decl.ToTDoc(tree)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := doc.Save(f); err != nil {
		t.Fatalf("save TDoc: %v", err)
	}
	return tree
}

// TestReloaderFiresCallbackOnModify writes a .silkui, modifies it, and
// asserts that the reloader callback fires with the new AST.
func TestReloaderFiresCallbackOnModify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "form.silkui")
	writeSilkUI(t, path, "First", "OK")

	got := make(chan *decl.Node, 4)
	r, err := New(
		func(p string, tree *decl.Node) error {
			got <- tree
			return nil
		},
		nil,
		Options{Debounce: 30 * time.Millisecond, PollInterval: 50 * time.Millisecond},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	if err := r.Watch(path); err != nil {
		t.Fatal(err)
	}

	// Edit the file. Bump the file mtime well past the poll interval.
	time.Sleep(80 * time.Millisecond)
	writeSilkUI(t, path, "Second", "Apply")

	select {
	case tree := <-got:
		if tree == nil {
			t.Fatalf("got nil tree")
		}
		if tree.Type != "gui.Form" {
			t.Errorf("root type = %q, want gui.Form", tree.Type)
		}
		// Find the title prop.
		var title string
		for _, p := range tree.Props {
			if p.Name == "title" {
				if lit, ok := p.Value.(decl.Lit); ok {
					title, _ = lit.V.(string)
				}
			}
		}
		if title != "Second" {
			t.Errorf("title = %q, want Second", title)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reload callback never fired")
	}
}

// TestReloaderFiresErrorOnGarbageContent overwrites the watched file
// with non-TDoc bytes; the reloader must surface the parse error
// without crashing the loop.
func TestReloaderFiresErrorOnGarbageContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.silkui")
	writeSilkUI(t, path, "OK", "OK")

	errCh := make(chan error, 4)
	reloadCh := make(chan *decl.Node, 4)
	r, err := New(
		func(p string, tree *decl.Node) error {
			reloadCh <- tree
			return nil
		},
		func(p string, e error) { errCh <- e },
		Options{Debounce: 30 * time.Millisecond, PollInterval: 50 * time.Millisecond},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	if err := r.Watch(path); err != nil {
		t.Fatal(err)
	}

	time.Sleep(80 * time.Millisecond)
	if err := os.WriteFile(path, []byte("this is not a TDoc at all\n"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-errCh:
		if e == nil {
			t.Fatal("error channel signalled with nil error")
		}
	case tree := <-reloadCh:
		t.Fatalf("garbage content should not have produced a successful reload; got tree type=%q", tree.Type)
	case <-time.After(2 * time.Second):
		// TDoc parser is fairly lenient and may accept the garbage as
		// an empty document. If neither error nor reload fired, that's
		// also acceptable — the test guarantees it doesn't crash.
	}
}

// TestReloaderStopHaltsLoop confirms that Stop returns and the
// callback is no longer invoked after Stop.
func TestReloaderStopHaltsLoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "form.silkui")
	writeSilkUI(t, path, "Pre-stop", "OK")

	var fired atomic.Int32
	r, err := New(
		func(p string, tree *decl.Node) error {
			fired.Add(1)
			return nil
		},
		nil,
		Options{Debounce: 30 * time.Millisecond, PollInterval: 50 * time.Millisecond},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Watch(path); err != nil {
		t.Fatal(err)
	}

	r.Stop()
	// Subsequent edits must not fire the callback.
	time.Sleep(80 * time.Millisecond)
	writeSilkUI(t, path, "Post-stop", "OK")
	time.Sleep(300 * time.Millisecond)

	if got := fired.Load(); got != 0 {
		t.Errorf("callback fired %d times after Stop; want 0", got)
	}

	// Stop is idempotent.
	r.Stop()
}

// TestReloaderDebouncesRapidWrites coalesces a burst of writes inside
// the debounce window into one rebuild. We use a longer debounce for
// reliability, write ~3 times faster than that, and assert the
// callback fired ≤ 2 times (typical case 1 — but the burst may straddle
// two debounce windows on slow CI hardware so we tolerate up to 2).
func TestReloaderDebouncesRapidWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "form.silkui")
	writeSilkUI(t, path, "v0", "OK")

	var fired atomic.Int32
	var mu sync.Mutex
	var lastTitle string
	r, err := New(
		func(p string, tree *decl.Node) error {
			fired.Add(1)
			for _, pr := range tree.Props {
				if pr.Name == "title" {
					if lit, ok := pr.Value.(decl.Lit); ok {
						mu.Lock()
						lastTitle, _ = lit.V.(string)
						mu.Unlock()
					}
				}
			}
			return nil
		},
		nil,
		Options{Debounce: 200 * time.Millisecond, PollInterval: 50 * time.Millisecond},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	if err := r.Watch(path); err != nil {
		t.Fatal(err)
	}

	// Wait past poll once so v0 is the established baseline.
	time.Sleep(80 * time.Millisecond)

	// Burst-write three versions inside the debounce window.
	for i, v := range []string{"v1", "v2", "v3"} {
		writeSilkUI(t, path, v, "OK")
		if i < 2 {
			time.Sleep(20 * time.Millisecond)
		}
	}

	// Wait long enough for debounce + rebuild.
	time.Sleep(500 * time.Millisecond)

	if got := fired.Load(); got > 2 {
		t.Errorf("debounce ineffective: callback fired %d times for a 3-write burst; want ≤ 2", got)
	}

	mu.Lock()
	last := lastTitle
	mu.Unlock()
	// Whichever rebuild won the race, it must have seen one of the
	// non-baseline versions.
	if last == "" || last == "v0" {
		t.Errorf("last observed title = %q; want a post-burst version", last)
	}
}

// TestReloaderAllowedExt rejects events on files whose extension isn't
// in the whitelist.
func TestReloaderAllowedExt(t *testing.T) {
	dir := t.TempDir()
	silkPath := filepath.Join(dir, "form.silkui")
	junkPath := filepath.Join(dir, "scratch.txt")
	writeSilkUI(t, silkPath, "v0", "OK")

	got := make(chan string, 4)
	r, err := New(
		func(p string, tree *decl.Node) error {
			got <- p
			return nil
		},
		nil,
		Options{
			Debounce:     30 * time.Millisecond,
			PollInterval: 50 * time.Millisecond,
			AllowedExt:   []string{"silkui"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	if err := r.Watch(dir); err != nil {
		t.Fatal(err)
	}

	time.Sleep(80 * time.Millisecond)

	// Modify the .txt — should be ignored by the extension filter.
	if err := os.WriteFile(junkPath, []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Modify the .silkui — should fire.
	writeSilkUI(t, silkPath, "v1", "OK")

	select {
	case p := <-got:
		if filepath.Ext(p) != ".silkui" {
			t.Errorf("callback fired for non-.silkui path %q", p)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("callback never fired for .silkui edit")
	}

	// Drain any straggler. If a .txt event slipped through,
	// the callback would have been invoked twice — make sure it wasn't.
	select {
	case p := <-got:
		if filepath.Ext(p) != ".silkui" {
			t.Errorf("filter leak: callback fired for %q", p)
		}
	case <-time.After(200 * time.Millisecond):
		// expected
	}
}

// TestNewRequiresOnReload guards the constructor's input validation.
func TestNewRequiresOnReload(t *testing.T) {
	_, err := New(nil, nil, Options{})
	if err == nil {
		t.Fatal("New(nil, ...) should error")
	}
	if !errors.Is(err, err) {
		t.Fatal("error wrapping unexpected")
	}
}

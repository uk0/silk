package ged

import (
	"errors"
	"reflect"
	"testing"

	"github.com/uk0/silk/ide/runconfig"
)

// newNamedStore builds a three-configuration store used by most of the
// tests below: "run app" (the default), "debug app" and "tests".
func newNamedStore(t *testing.T) *runconfig.Store {
	t.Helper()
	s := runconfig.NewStore()
	cfgs := []runconfig.Config{
		{
			Name:    "run app",
			Target:  "./cmd/app",
			Args:    []string{"-v", "--port=8080"},
			WorkDir: "/tmp/app",
			Env:     map[string]string{"FOO": "1", "BAR": "2"},
			Mode:    runconfig.ModeRun,
			KitName: "go-1.25",
			PreRun:  []string{"build"},
			Default: true,
		},
		{Name: "debug app", Target: "./cmd/app", Mode: runconfig.ModeDebug, KitName: "go-1.25"},
		{Name: "tests", Target: "./...", Mode: runconfig.ModeTest},
	}
	for _, c := range cfgs {
		if err := s.Add(c); err != nil {
			t.Fatalf("Add(%q): %v", c.Name, err)
		}
	}
	return s
}

// TestRunConfigPanel_SetStoreSelectsDefault verifies attaching a store
// selects its default configuration and loads that configuration's
// fields into the editor.
func TestRunConfigPanel_SetStoreSelectsDefault(t *testing.T) {
	p := NewRunConfigPanel()
	p.SetStore(newNamedStore(t))

	if got := p.Selected(); got != "run app" {
		t.Fatalf("Selected() = %q, want %q", got, "run app")
	}
	if got := p.Target(); got != "./cmd/app" {
		t.Errorf("Target() = %q, want %q", got, "./cmd/app")
	}
	if got := p.Mode(); got != runconfig.ModeRun {
		t.Errorf("Mode() = %q, want %q", got, runconfig.ModeRun)
	}
	if got := p.Kit(); got != "go-1.25" {
		t.Errorf("Kit() = %q, want %q", got, "go-1.25")
	}
	// The single-config layer mirrors the selected configuration: argv is
	// joined into one line, env comes back sorted by key.
	legacy := p.Config()
	if legacy.Args != "-v --port=8080" {
		t.Errorf("Config().Args = %q, want %q", legacy.Args, "-v --port=8080")
	}
	if legacy.WorkingDir != "/tmp/app" {
		t.Errorf("Config().WorkingDir = %q, want %q", legacy.WorkingDir, "/tmp/app")
	}
	if want := []string{"BAR=2", "FOO=1"}; !reflect.DeepEqual(legacy.Env, want) {
		t.Errorf("Config().Env = %#v, want %#v", legacy.Env, want)
	}
	if got := len(p.Configs()); got != 3 {
		t.Errorf("Configs() = %d entries, want 3", got)
	}
}

// TestRunConfigPanel_SetStoreNilDetaches verifies a nil store leaves the
// panel a plain single-config editor.
func TestRunConfigPanel_SetStoreNilDetaches(t *testing.T) {
	p := NewRunConfigPanel()
	p.SetStore(newNamedStore(t))
	p.SetStore(nil)

	if p.Store() != nil {
		t.Error("Store() != nil after SetStore(nil)")
	}
	if got := p.Selected(); got != "" {
		t.Errorf("Selected() = %q, want empty", got)
	}
	if got := len(p.Configs()); got != 0 {
		t.Errorf("Configs() = %d entries, want 0", got)
	}
}

// TestRunConfigPanel_SelectUpdatesFieldsAndFires verifies selecting a
// different configuration reloads every edited field and fires SigSelect
// (and SigChanged, so single-config hosts keep tracking the panel).
func TestRunConfigPanel_SelectUpdatesFieldsAndFires(t *testing.T) {
	p := NewRunConfigPanel()
	p.SetStore(newNamedStore(t))

	selected := 0
	var seen runconfig.Config
	p.SigSelect(func(c runconfig.Config) {
		selected++
		seen = c
	})
	changed := 0
	p.SigChanged(func(RunConfig) { changed++ })

	if !p.SelectConfig("debug app") {
		t.Fatal("SelectConfig(debug app) = false, want true")
	}
	if selected != 1 {
		t.Fatalf("SigSelect fired %d times, want 1", selected)
	}
	if seen.Name != "debug app" || seen.Mode != runconfig.ModeDebug {
		t.Fatalf("SigSelect carried %#v", seen)
	}
	if got := p.Selected(); got != "debug app" {
		t.Errorf("Selected() = %q, want %q", got, "debug app")
	}
	if got := p.Mode(); got != runconfig.ModeDebug {
		t.Errorf("Mode() = %q, want %q", got, runconfig.ModeDebug)
	}
	// "debug app" has no args / workdir / env, so the editor is cleared and
	// the single-config callback sees the change.
	if changed != 1 {
		t.Fatalf("SigChanged fired %d times, want 1", changed)
	}
	legacy := p.Config()
	if legacy.Args != "" || legacy.WorkingDir != "" || len(legacy.Env) != 0 {
		t.Fatalf("editor not reloaded: %#v", legacy)
	}

	// Re-selecting the same configuration is a no-op.
	if !p.SelectConfig("debug app") {
		t.Fatal("re-SelectConfig = false, want true")
	}
	if selected != 1 || changed != 1 {
		t.Fatalf("re-select fired select=%d changed=%d, want 1/1", selected, changed)
	}

	// Unknown names are refused and change nothing.
	if p.SelectConfig("ghost") {
		t.Error("SelectConfig(ghost) = true, want false")
	}
	if got := p.Selected(); got != "debug app" {
		t.Errorf("Selected() = %q after a failed select, want %q", got, "debug app")
	}
	if selected != 1 {
		t.Errorf("failed select fired SigSelect %d times, want 1", selected)
	}
}

// TestRunConfigPanel_SelectIndex verifies list-order selection, which is
// what a list-row click goes through.
func TestRunConfigPanel_SelectIndex(t *testing.T) {
	p := NewRunConfigPanel()
	p.SetStore(newNamedStore(t))

	fired := 0
	p.SigSelect(func(runconfig.Config) { fired++ })

	if !p.SelectIndex(2) {
		t.Fatal("SelectIndex(2) = false, want true")
	}
	if got := p.Selected(); got != "tests" {
		t.Fatalf("Selected() = %q, want %q", got, "tests")
	}
	if got := p.Target(); got != "./..." {
		t.Errorf("Target() = %q, want %q", got, "./...")
	}
	if fired != 1 {
		t.Errorf("SigSelect fired %d, want 1", fired)
	}
	for _, bad := range []int{-1, 3, 99} {
		if p.SelectIndex(bad) {
			t.Errorf("SelectIndex(%d) = true, want false", bad)
		}
	}
	if fired != 1 {
		t.Errorf("out-of-range SelectIndex fired SigSelect: %d", fired)
	}
}

// TestRunConfigPanel_ApplyWritesStoreAndFires verifies Apply merges the
// edited fields into the stored configuration and fires SigApply.
func TestRunConfigPanel_ApplyWritesStoreAndFires(t *testing.T) {
	store := newNamedStore(t)
	p := NewRunConfigPanel()
	p.SetStore(store)
	if !p.SelectConfig("tests") {
		t.Fatal("SelectConfig(tests) failed")
	}

	applied := 0
	var seen runconfig.Config
	p.SigApply(func(c runconfig.Config) {
		applied++
		seen = c
	})

	p.SetTarget("./gui")
	p.SetMode(runconfig.ModeBench)
	p.SetKit("go-tip")
	p.SetArgs("-bench=. -benchmem")
	p.SetWorkingDir("/tmp/bench")
	p.AddEnv("GOFLAGS=-mod=mod")

	// Nothing reaches the store before Apply.
	if before, _ := store.Get("tests"); before.Target != "./..." {
		t.Fatalf("store mutated before Apply: %#v", before)
	}

	if err := p.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if applied != 1 {
		t.Fatalf("SigApply fired %d times, want 1", applied)
	}

	got, ok := store.Get("tests")
	if !ok {
		t.Fatal("configuration disappeared from the store")
	}
	want := runconfig.Config{
		Name:    "tests",
		Target:  "./gui",
		Args:    []string{"-bench=.", "-benchmem"},
		WorkDir: "/tmp/bench",
		Env:     map[string]string{"GOFLAGS": "-mod=mod"},
		Mode:    runconfig.ModeBench,
		KitName: "go-tip",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stored config = %#v\nwant %#v", got, want)
	}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("SigApply carried %#v\nwant %#v", seen, want)
	}
	// The list snapshot behind Draw / hit-test tracks the store.
	if p.Configs()[2].Target != "./gui" {
		t.Fatalf("list snapshot stale: %#v", p.Configs()[2])
	}
}

// TestRunConfigPanel_ApplyKeepsIdentityFields verifies Apply preserves the
// parts of a configuration the panel does not edit — pre-run tasks and
// the default flag.
func TestRunConfigPanel_ApplyKeepsIdentityFields(t *testing.T) {
	store := newNamedStore(t)
	p := NewRunConfigPanel()
	p.SetStore(store)

	p.SetArgs("-race")
	if err := p.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := store.Get("run app")
	if !reflect.DeepEqual(got.PreRun, []string{"build"}) {
		t.Errorf("PreRun = %#v, want [build]", got.PreRun)
	}
	if !got.Default {
		t.Error("Default flag lost through Apply")
	}
	if d, _ := store.Default(); d.Name != "run app" {
		t.Errorf("Default() = %q, want %q", d.Name, "run app")
	}
}

// TestRunConfigPanel_ApplyValidationRejects verifies a blank target is
// refused, the store stays untouched and SigApply stays silent.
func TestRunConfigPanel_ApplyValidationRejects(t *testing.T) {
	store := newNamedStore(t)
	p := NewRunConfigPanel()
	p.SetStore(store)
	if !p.SelectConfig("tests") {
		t.Fatal("SelectConfig(tests) failed")
	}

	applied := 0
	p.SigApply(func(runconfig.Config) { applied++ })

	p.SetTarget("")
	if err := p.Apply(); !errors.Is(err, runconfig.ErrEmptyTarget) {
		t.Fatalf("Apply with a blank target = %v, want ErrEmptyTarget", err)
	}
	if applied != 0 {
		t.Errorf("SigApply fired %d times on a rejected Apply", applied)
	}
	if got, _ := store.Get("tests"); got.Target != "./..." {
		t.Fatalf("store written by a rejected Apply: %#v", got)
	}
}

// TestRunConfigPanel_ApplyWithoutSelection verifies Apply needs a
// selection to write into.
func TestRunConfigPanel_ApplyWithoutSelection(t *testing.T) {
	p := NewRunConfigPanel()
	p.SetStore(runconfig.NewStore()) // empty store: nothing gets selected
	if got := p.Selected(); got != "" {
		t.Fatalf("Selected() = %q on an empty store, want empty", got)
	}
	if err := p.Apply(); !errors.Is(err, runconfig.ErrNotFound) {
		t.Fatalf("Apply without a selection = %v, want ErrNotFound", err)
	}
}

// TestRunConfigPanel_AddConfig verifies a new configuration lands in the
// store, becomes selected, and that the store's validation surfaces.
func TestRunConfigPanel_AddConfig(t *testing.T) {
	store := newNamedStore(t)
	p := NewRunConfigPanel()
	p.SetStore(store)

	selected := 0
	p.SigSelect(func(runconfig.Config) { selected++ })

	if err := p.AddConfig("  profile  "); err != nil {
		t.Fatalf("AddConfig: %v", err)
	}
	if got := p.Selected(); got != "profile" {
		t.Fatalf("Selected() = %q, want %q (name trimmed)", got, "profile")
	}
	if selected != 1 {
		t.Errorf("SigSelect fired %d, want 1", selected)
	}
	got, ok := store.Get("profile")
	if !ok {
		t.Fatal("new configuration missing from the store")
	}
	if got.Target != "." || got.Mode != runconfig.ModeRun {
		t.Errorf("new configuration = %#v, want target . / mode run", got)
	}
	if got.Default {
		t.Error("new configuration stole the default flag")
	}

	// Validation: blank names and duplicates are refused.
	if err := p.AddConfig("   "); !errors.Is(err, runconfig.ErrEmptyName) {
		t.Fatalf("AddConfig(blank) = %v, want ErrEmptyName", err)
	}
	if err := p.AddConfig("tests"); !errors.Is(err, runconfig.ErrDuplicateName) {
		t.Fatalf("AddConfig(duplicate) = %v, want ErrDuplicateName", err)
	}
	if store.Len() != 4 {
		t.Fatalf("store Len = %d, want 4", store.Len())
	}
}

// TestRunConfigPanel_DuplicateSelected verifies the copy carries the
// source's fields under a new name, is not the default, and is selected.
func TestRunConfigPanel_DuplicateSelected(t *testing.T) {
	store := newNamedStore(t)
	p := NewRunConfigPanel()
	p.SetStore(store) // selects "run app", the default

	if err := p.DuplicateSelected("run app (copy)"); err != nil {
		t.Fatalf("DuplicateSelected: %v", err)
	}
	if got := p.Selected(); got != "run app (copy)" {
		t.Fatalf("Selected() = %q, want the copy", got)
	}
	src, _ := store.Get("run app")
	cp, ok := store.Get("run app (copy)")
	if !ok {
		t.Fatal("copy missing from the store")
	}
	if cp.Target != src.Target || !reflect.DeepEqual(cp.Args, src.Args) ||
		cp.WorkDir != src.WorkDir || !reflect.DeepEqual(cp.Env, src.Env) ||
		cp.Mode != src.Mode || cp.KitName != src.KitName {
		t.Fatalf("copy = %#v\nsrc = %#v", cp, src)
	}
	if cp.Default {
		t.Error("copy stole the default flag")
	}
	if d, _ := store.Default(); d.Name != "run app" {
		t.Errorf("Default() = %q, want %q", d.Name, "run app")
	}
	// Editing the copy leaves the source alone.
	p.SetTarget("./cmd/other")
	if err := p.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if again, _ := store.Get("run app"); again.Target != src.Target {
		t.Errorf("source mutated through the copy: %q", again.Target)
	}

	// A duplicate name is refused.
	if err := p.DuplicateSelected("tests"); !errors.Is(err, runconfig.ErrDuplicateName) {
		t.Fatalf("DuplicateSelected(tests) = %v, want ErrDuplicateName", err)
	}
}

// TestRunConfigPanel_RemoveSelected verifies removal moves the selection
// to the first remaining configuration and clears the editor when the
// list runs empty.
func TestRunConfigPanel_RemoveSelected(t *testing.T) {
	store := newNamedStore(t)
	p := NewRunConfigPanel()
	p.SetStore(store)
	if !p.SelectConfig("debug app") {
		t.Fatal("SelectConfig(debug app) failed")
	}

	if !p.RemoveSelected() {
		t.Fatal("RemoveSelected = false, want true")
	}
	if _, ok := store.Get("debug app"); ok {
		t.Error("configuration still in the store after RemoveSelected")
	}
	if got := p.Selected(); got != "run app" {
		t.Fatalf("Selected() = %q, want the first remaining (run app)", got)
	}
	if store.Len() != 2 {
		t.Fatalf("store Len = %d, want 2", store.Len())
	}

	// Drain the store: the editor clears and the single-config callback
	// sees the reset.
	changed := 0
	p.SigChanged(func(RunConfig) { changed++ })
	if !p.RemoveSelected() {
		t.Fatal("RemoveSelected (run app) = false")
	}
	if !p.RemoveSelected() {
		t.Fatal("RemoveSelected (tests) = false")
	}
	if p.RemoveSelected() {
		t.Fatal("RemoveSelected on an empty store = true, want false")
	}
	if got := p.Selected(); got != "" {
		t.Errorf("Selected() = %q on an empty store, want empty", got)
	}
	if got := p.Config(); got.Args != "" || got.WorkingDir != "" || len(got.Env) != 0 {
		t.Errorf("editor not cleared: %#v", got)
	}
	if got := p.Target(); got != "" {
		t.Errorf("Target() = %q on an empty store, want empty", got)
	}
	if changed == 0 {
		t.Error("draining the store never fired SigChanged")
	}
}

// TestRunConfigPanel_NoStoreOperations verifies the named layer degrades
// cleanly when no store is attached — the single-config editor still
// works, every store operation reports instead of panicking.
func TestRunConfigPanel_NoStoreOperations(t *testing.T) {
	p := NewRunConfigPanel()

	if err := p.AddConfig("a"); !errors.Is(err, errNoRunConfigStore) {
		t.Errorf("AddConfig = %v, want errNoRunConfigStore", err)
	}
	if err := p.DuplicateSelected("a"); !errors.Is(err, errNoRunConfigStore) {
		t.Errorf("DuplicateSelected = %v, want errNoRunConfigStore", err)
	}
	if err := p.Apply(); !errors.Is(err, errNoRunConfigStore) {
		t.Errorf("Apply = %v, want errNoRunConfigStore", err)
	}
	if p.SelectConfig("a") {
		t.Error("SelectConfig = true without a store")
	}
	if p.SelectIndex(0) {
		t.Error("SelectIndex = true without a store")
	}
	if p.RemoveSelected() {
		t.Error("RemoveSelected = true without a store")
	}

	// The original single-config API is unaffected.
	p.SetConfig(RunConfig{Args: "-v", WorkingDir: "/tmp", Env: []string{"K=V"}})
	if got := p.Config().Args; got != "-v" {
		t.Errorf("Config().Args = %q, want -v", got)
	}
}

// TestRunConfigPanel_LegacyEditsStayLocal verifies single-config edits do
// not leak into the store until Apply runs.
func TestRunConfigPanel_LegacyEditsStayLocal(t *testing.T) {
	store := newNamedStore(t)
	p := NewRunConfigPanel()
	p.SetStore(store)

	p.SetArgs("-x")
	p.SetWorkingDir("/elsewhere")
	p.AddEnv("NEW=1")

	got, _ := store.Get("run app")
	if !reflect.DeepEqual(got.Args, []string{"-v", "--port=8080"}) ||
		got.WorkDir != "/tmp/app" || len(got.Env) != 2 {
		t.Fatalf("store mutated by single-config edits: %#v", got)
	}
	// EditedConfig is the merged, not-yet-committed view.
	edited := p.EditedConfig()
	if !reflect.DeepEqual(edited.Args, []string{"-x"}) {
		t.Errorf("EditedConfig().Args = %#v, want [-x]", edited.Args)
	}
	if edited.WorkDir != "/elsewhere" {
		t.Errorf("EditedConfig().WorkDir = %q", edited.WorkDir)
	}
	if edited.Env["NEW"] != "1" || edited.Env["FOO"] != "1" {
		t.Errorf("EditedConfig().Env = %#v", edited.Env)
	}
	if edited.Name != "run app" || !edited.Default {
		t.Errorf("EditedConfig() lost identity: %#v", edited)
	}
}

// TestRunConfigPanel_SetModeValidation verifies the mode field only
// accepts real launch modes and that the click-to-cycle order wraps.
func TestRunConfigPanel_SetModeValidation(t *testing.T) {
	p := NewRunConfigPanel()
	p.SetStore(newNamedStore(t))

	p.SetMode("profile")
	if got := p.Mode(); got != runconfig.ModeRun {
		t.Errorf("invalid SetMode changed the mode to %q", got)
	}
	p.SetMode(runconfig.ModeTest)
	if got := p.Mode(); got != runconfig.ModeTest {
		t.Errorf("Mode() = %q, want %q", got, runconfig.ModeTest)
	}

	// Cycling from every mode lands on the next one, wrapping at the end.
	want := map[string]string{
		runconfig.ModeRun:   runconfig.ModeDebug,
		runconfig.ModeDebug: runconfig.ModeTest,
		runconfig.ModeTest:  runconfig.ModeBench,
		runconfig.ModeBench: runconfig.ModeRun,
	}
	for from, to := range want {
		if got := nextRunMode(from); got != to {
			t.Errorf("nextRunMode(%q) = %q, want %q", from, got, to)
		}
	}
	// An unset mode cycles from the effective one (run).
	if got := nextRunMode(""); got != runconfig.ModeRun {
		t.Errorf("nextRunMode(\"\") = %q, want %q", got, runconfig.ModeRun)
	}
}

// TestRunConfigPanel_TargetKitEdits verifies the two free-text fields and
// their no-op guards.
func TestRunConfigPanel_TargetKitEdits(t *testing.T) {
	p := NewRunConfigPanel()
	p.SetStore(newNamedStore(t))

	p.SetTarget("./cmd/silkide")
	if got := p.Target(); got != "./cmd/silkide" {
		t.Errorf("Target() = %q", got)
	}
	p.SetKit("clang-kit")
	if got := p.Kit(); got != "clang-kit" {
		t.Errorf("Kit() = %q", got)
	}
	// Idempotent writes are silently dropped.
	p.SetTarget("./cmd/silkide")
	p.SetKit("clang-kit")
	if p.Target() != "./cmd/silkide" || p.Kit() != "clang-kit" {
		t.Errorf("no-op edits changed the fields: %q / %q", p.Target(), p.Kit())
	}
}

// TestRunConfigPanel_NamedHitTest checks the hit-tester over the new
// configuration list and its toolbar, without touching Draw.
func TestRunConfigPanel_NamedHitTest(t *testing.T) {
	p := NewRunConfigPanel()
	p.SetSize(320, 600)
	p.SetStore(newNamedStore(t))

	// One code per list row, decoded back to the row index.
	for i := 0; i < 3; i++ {
		y := p.listY() + float64(i)*rcListRowH + rcListRowH/2
		hit := p.hitRow(40, y)
		idx, ok := listRowOf(hit)
		if !ok || idx != i {
			t.Fatalf("list row %d: hitRow = %d -> (%d, %v)", i, hit, idx, ok)
		}
	}
	// Synthetic codes never decode as list rows.
	for _, code := range []int{rcHoverNone, rcHoverArgs, rcHoverWD, rcHoverAdd,
		rcHoverTarget, rcHoverMode, rcHoverKit, rcHoverCfgNew, rcHoverCfgDup,
		rcHoverCfgDel, rcHoverCfgApply, 0, 5} {
		if _, ok := listRowOf(code); ok {
			t.Errorf("listRowOf(%d) reported a list row", code)
		}
	}

	// Toolbar buttons, left to right.
	btnY := p.cfgBtnY() + rcAddBtnH/2
	wantBtn := []int{rcHoverCfgNew, rcHoverCfgDup, rcHoverCfgDel, rcHoverCfgApply}
	for i, want := range wantBtn {
		x := cfgBtnX(i) + rcCfgBtnW/2
		if got := p.hitRow(x, btnY); got != want {
			t.Errorf("button %d: hitRow = %d, want %d", i, got, want)
		}
	}
	// Past the last button the toolbar row is inert.
	if got := p.hitRow(cfgBtnX(rcCfgBtnCount)+20, btnY); got != rcHoverNone {
		t.Errorf("empty toolbar space = %d, want %d", got, rcHoverNone)
	}

	// The three new field rows, then the two original ones.
	rows := []struct {
		y    float64
		want int
	}{
		{p.rowYTarget(), rcHoverTarget},
		{p.rowYMode(), rcHoverMode},
		{p.rowYKit(), rcHoverKit},
		{p.rowYArgs(), rcHoverArgs},
		{p.rowYWD(), rcHoverWD},
	}
	for _, r := range rows {
		if got := p.hitRow(40, r.y+rcRowH/2); got != r.want {
			t.Errorf("row at y=%.1f = %d, want %d", r.y, got, r.want)
		}
	}
}

// TestRunConfigPanel_EmptyListHitTest verifies the placeholder row of an
// empty list is inert and the rows below it still hit-test.
func TestRunConfigPanel_EmptyListHitTest(t *testing.T) {
	p := NewRunConfigPanel()
	p.SetSize(320, 600)
	p.SetStore(runconfig.NewStore())

	if got := p.hitRow(40, p.listY()+rcListRowH/2); got != rcHoverNone {
		t.Errorf("placeholder row = %d, want %d", got, rcHoverNone)
	}
	if got := p.hitRow(40, p.rowYTarget()+rcRowH/2); got != rcHoverTarget {
		t.Errorf("target row = %d, want %d", got, rcHoverTarget)
	}
	// The reserved placeholder row keeps the layout from collapsing.
	if got := p.listRowCount(); got != 1 {
		t.Errorf("listRowCount() = %d on an empty list, want 1", got)
	}
}

// TestRunConfigPanel_ContentHeightGrows verifies the scrollable content
// height tracks the configuration list.
func TestRunConfigPanel_ContentHeightGrows(t *testing.T) {
	p := NewRunConfigPanel()
	empty := p.contentHeight()
	p.SetStore(newNamedStore(t))
	full := p.contentHeight()
	if full <= empty {
		t.Fatalf("contentHeight did not grow: %.1f -> %.1f", empty, full)
	}
	if want := empty + 2*rcListRowH + 2*rcEnvRowH; full != want {
		t.Errorf("contentHeight = %.1f, want %.1f", full, want)
	}
}

// TestSplitRunArgs covers the args line -> argv conversion, including the
// nil case that keeps JSON round-trips clean.
func TestSplitRunArgs(t *testing.T) {
	if got := splitRunArgs(""); got != nil {
		t.Errorf("splitRunArgs(\"\") = %#v, want nil", got)
	}
	if got := splitRunArgs("   \t "); got != nil {
		t.Errorf("splitRunArgs(blank) = %#v, want nil", got)
	}
	got := splitRunArgs("  -v   --port=8080\t-x ")
	want := []string{"-v", "--port=8080", "-x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitRunArgs = %#v, want %#v", got, want)
	}
}

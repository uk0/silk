package runconfig

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// sample returns a fully populated configuration used by several tests.
func sample(name string) Config {
	return Config{
		Name:    name,
		Target:  "./cmd/app",
		Args:    []string{"-v", "--port=8080"},
		WorkDir: "/tmp/work",
		Env:     map[string]string{"FOO": "1", "BAR": "2"},
		Mode:    ModeDebug,
		KitName: "go-1.25",
		PreRun:  []string{"build"},
	}
}

// TestConfig_Validate covers the accept/reject matrix: a name and target
// are mandatory, an empty mode means ModeRun, an unknown mode is refused.
func TestConfig_Validate(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want error
	}{
		{"full", sample("app"), nil},
		{"minimal", Config{Name: "a", Target: "."}, nil},
		{"empty mode ok", Config{Name: "a", Target: ".", Mode: ""}, nil},
		{"empty name", Config{Target: "."}, ErrEmptyName},
		{"blank name", Config{Name: "   \t", Target: "."}, ErrEmptyName},
		{"empty target", Config{Name: "a"}, ErrEmptyTarget},
		{"blank target", Config{Name: "a", Target: "  "}, ErrEmptyTarget},
		{"bad mode", Config{Name: "a", Target: ".", Mode: "profile"}, ErrInvalidMode},
	}
	for _, tc := range cases {
		got := tc.cfg.Validate()
		if !errors.Is(got, tc.want) {
			t.Errorf("%s: Validate() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestValidMode checks every launch mode is accepted and nothing else is.
func TestValidMode(t *testing.T) {
	for _, m := range Modes() {
		if !ValidMode(m) {
			t.Errorf("ValidMode(%q) = false, want true", m)
		}
	}
	for _, m := range []string{"", "RUN", "profile", "debug "} {
		if ValidMode(m) {
			t.Errorf("ValidMode(%q) = true, want false", m)
		}
	}
	if got := len(Modes()); got != 4 {
		t.Errorf("len(Modes()) = %d, want 4", got)
	}
}

// TestConfig_EffectiveMode verifies the unset mode reads as ModeRun.
func TestConfig_EffectiveMode(t *testing.T) {
	if got := (Config{}).EffectiveMode(); got != ModeRun {
		t.Errorf("empty EffectiveMode = %q, want %q", got, ModeRun)
	}
	if got := (Config{Mode: ModeTest}).EffectiveMode(); got != ModeTest {
		t.Errorf("EffectiveMode = %q, want %q", got, ModeTest)
	}
}

// TestConfig_CloneIsDeep verifies mutating a clone leaves the original
// untouched — the store relies on this to hand out safe copies.
func TestConfig_CloneIsDeep(t *testing.T) {
	src := sample("app")
	cp := src.Clone()
	cp.Args[0] = "MUTATED"
	cp.PreRun[0] = "MUTATED"
	cp.Env["FOO"] = "MUTATED"
	cp.Env["NEW"] = "x"

	if src.Args[0] != "-v" {
		t.Errorf("Args aliased: %q", src.Args[0])
	}
	if src.PreRun[0] != "build" {
		t.Errorf("PreRun aliased: %q", src.PreRun[0])
	}
	if src.Env["FOO"] != "1" {
		t.Errorf("Env aliased: %q", src.Env["FOO"])
	}
	if _, ok := src.Env["NEW"]; ok {
		t.Error("Env map shared with clone")
	}
}

// TestConfig_CloneKeepsNil verifies Clone does not turn nil collections
// into empty ones (JSON omitempty round-trips depend on it).
func TestConfig_CloneKeepsNil(t *testing.T) {
	cp := Config{Name: "a", Target: "."}.Clone()
	if cp.Args != nil || cp.PreRun != nil || cp.Env != nil {
		t.Errorf("Clone materialised nil fields: %#v", cp)
	}
}

// TestEnvSliceParseEnv_RoundTrip verifies the map <-> "KEY=value" pair is
// lossless and that EnvSlice is sorted by key.
func TestEnvSliceParseEnv_RoundTrip(t *testing.T) {
	c := Config{Env: map[string]string{"ZED": "9", "ALPHA": "1", "MID": ""}}
	got := c.EnvSlice()
	want := []string{"ALPHA=1", "MID=", "ZED=9"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EnvSlice = %#v, want %#v", got, want)
	}
	if back := ParseEnv(got); !reflect.DeepEqual(back, c.Env) {
		t.Fatalf("ParseEnv(EnvSlice()) = %#v, want %#v", back, c.Env)
	}
}

// TestEnvSliceParseEnv_Edges covers empty input and the odd entries the
// panel's free-text env rows can produce.
func TestEnvSliceParseEnv_Edges(t *testing.T) {
	if got := (Config{}).EnvSlice(); got != nil {
		t.Errorf("EnvSlice of empty Env = %#v, want nil", got)
	}
	if got := ParseEnv(nil); got != nil {
		t.Errorf("ParseEnv(nil) = %#v, want nil", got)
	}
	if got := ParseEnv([]string{"", "   "}); got != nil {
		t.Errorf("ParseEnv(blanks) = %#v, want nil", got)
	}
	got := ParseEnv([]string{" FOO=1 ", "BARE", "K=a=b", "FOO=2"})
	want := map[string]string{"FOO": "2", "BARE": "", "K": "a=b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseEnv = %#v, want %#v", got, want)
	}
}

// TestStore_AddGetList covers the happy path of CRUD reads: order is
// insertion order and Get returns the stored value.
func TestStore_AddGetList(t *testing.T) {
	s := NewStore()
	if s.Len() != 0 {
		t.Fatalf("new store Len = %d, want 0", s.Len())
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("new store List = %#v, want empty", got)
	}
	for _, n := range []string{"run app", "debug app", "tests"} {
		if err := s.Add(Config{Name: n, Target: "."}); err != nil {
			t.Fatalf("Add(%q): %v", n, err)
		}
	}
	if s.Len() != 3 {
		t.Fatalf("Len = %d, want 3", s.Len())
	}
	var names []string
	for _, c := range s.List() {
		names = append(names, c.Name)
	}
	if want := []string{"run app", "debug app", "tests"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("List order = %#v, want %#v", names, want)
	}
	if c, ok := s.Get("tests"); !ok || c.Target != "." {
		t.Fatalf("Get(tests) = %#v, %v", c, ok)
	}
	if _, ok := s.Get("nope"); ok {
		t.Error("Get(nope) = true, want false")
	}
	if _, ok := s.Get(""); ok {
		t.Error("Get(\"\") = true, want false")
	}
}

// TestStore_AddTrimsNameAndRejectsDuplicates verifies unique-name
// enforcement, including the trimmed comparison.
func TestStore_AddTrimsNameAndRejectsDuplicates(t *testing.T) {
	s := NewStore()
	if err := s.Add(Config{Name: "  app  ", Target: "."}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if c, _ := s.Get("app"); c.Name != "app" {
		t.Fatalf("stored name = %q, want %q", c.Name, "app")
	}
	if err := s.Add(Config{Name: "app", Target: "./cmd"}); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("duplicate Add = %v, want ErrDuplicateName", err)
	}
	if err := s.Add(Config{Name: "\tapp\n", Target: "./cmd"}); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("padded duplicate Add = %v, want ErrDuplicateName", err)
	}
	// Case differs — a distinct configuration.
	if err := s.Add(Config{Name: "App", Target: "."}); err != nil {
		t.Fatalf("Add(App): %v", err)
	}
	if s.Len() != 2 {
		t.Fatalf("Len = %d, want 2", s.Len())
	}
}

// TestStore_AddRejectsInvalid verifies validation runs before insertion.
func TestStore_AddRejectsInvalid(t *testing.T) {
	s := NewStore()
	if err := s.Add(Config{Target: "."}); !errors.Is(err, ErrEmptyName) {
		t.Fatalf("Add(no name) = %v, want ErrEmptyName", err)
	}
	if err := s.Add(Config{Name: "app"}); !errors.Is(err, ErrEmptyTarget) {
		t.Fatalf("Add(no target) = %v, want ErrEmptyTarget", err)
	}
	if err := s.Add(Config{Name: "app", Target: ".", Mode: "nope"}); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("Add(bad mode) = %v, want ErrInvalidMode", err)
	}
	if s.Len() != 0 {
		t.Fatalf("invalid configs were stored: %#v", s.List())
	}
}

// TestStore_ListAndGetReturnCopies verifies the store never leaks its
// internal slices or maps.
func TestStore_ListAndGetReturnCopies(t *testing.T) {
	s := NewStore()
	if err := s.Add(sample("app")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got := s.List()
	got[0].Args[0] = "MUTATED"
	got[0].Env["FOO"] = "MUTATED"

	c, _ := s.Get("app")
	if c.Args[0] != "-v" || c.Env["FOO"] != "1" {
		t.Fatalf("store mutated through List(): %#v", c)
	}
	c.Args[0] = "MUTATED"
	again, _ := s.Get("app")
	if again.Args[0] != "-v" {
		t.Fatalf("store mutated through Get(): %#v", again)
	}
}

// TestStore_Update covers in-place edits, renames, the duplicate-rename
// rejection and the unknown-name error.
func TestStore_Update(t *testing.T) {
	s := NewStore()
	for _, n := range []string{"a", "b"} {
		if err := s.Add(Config{Name: n, Target: "."}); err != nil {
			t.Fatalf("Add(%q): %v", n, err)
		}
	}

	// In-place edit keeps the position.
	if err := s.Update("a", Config{Name: "a", Target: "./cmd/a", Mode: ModeTest}); err != nil {
		t.Fatalf("Update(a): %v", err)
	}
	c, _ := s.Get("a")
	if c.Target != "./cmd/a" || c.Mode != ModeTest {
		t.Fatalf("updated config = %#v", c)
	}
	if s.List()[0].Name != "a" {
		t.Fatalf("Update moved the entry: %#v", s.List())
	}

	// Rename to a free name.
	if err := s.Update("a", Config{Name: "a2", Target: "."}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, ok := s.Get("a"); ok {
		t.Error("old name still present after rename")
	}
	if _, ok := s.Get("a2"); !ok {
		t.Error("new name missing after rename")
	}
	if s.List()[0].Name != "a2" {
		t.Fatalf("rename moved the entry: %#v", s.List())
	}

	// Rename onto a taken name is refused and changes nothing.
	if err := s.Update("a2", Config{Name: "b", Target: "./x"}); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("colliding rename = %v, want ErrDuplicateName", err)
	}
	if c, _ := s.Get("a2"); c.Target != "." {
		t.Fatalf("refused Update still wrote: %#v", c)
	}

	// Validation and unknown names.
	if err := s.Update("a2", Config{Name: "a2"}); !errors.Is(err, ErrEmptyTarget) {
		t.Fatalf("invalid Update = %v, want ErrEmptyTarget", err)
	}
	if err := s.Update("ghost", Config{Name: "ghost", Target: "."}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update(ghost) = %v, want ErrNotFound", err)
	}
}

// TestStore_Remove verifies removal by name and the no-op case.
func TestStore_Remove(t *testing.T) {
	s := NewStore()
	for _, n := range []string{"a", "b", "c"} {
		if err := s.Add(Config{Name: n, Target: "."}); err != nil {
			t.Fatalf("Add(%q): %v", n, err)
		}
	}
	if !s.Remove("b") {
		t.Fatal("Remove(b) = false, want true")
	}
	if s.Remove("b") {
		t.Fatal("second Remove(b) = true, want false")
	}
	var names []string
	for _, c := range s.List() {
		names = append(names, c.Name)
	}
	if want := []string{"a", "c"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("after Remove, List = %#v, want %#v", names, want)
	}
	// The freed name can be reused.
	if err := s.Add(Config{Name: "b", Target: "."}); err != nil {
		t.Fatalf("re-Add(b): %v", err)
	}
}

// TestStore_DefaultSelection covers the default-configuration rules: the
// first entry is the implicit default, SetDefault is exclusive, and
// removing the default falls back to the first survivor.
func TestStore_DefaultSelection(t *testing.T) {
	s := NewStore()
	if _, ok := s.Default(); ok {
		t.Fatal("Default() on an empty store = true, want false")
	}
	for _, n := range []string{"a", "b", "c"} {
		if err := s.Add(Config{Name: n, Target: "."}); err != nil {
			t.Fatalf("Add(%q): %v", n, err)
		}
	}

	// No explicit flag: the first entry is the implicit default.
	if d, ok := s.Default(); !ok || d.Name != "a" {
		t.Fatalf("implicit Default = %#v, %v; want a", d, ok)
	}

	if !s.SetDefault("c") {
		t.Fatal("SetDefault(c) = false, want true")
	}
	if d, _ := s.Default(); d.Name != "c" {
		t.Fatalf("Default = %q, want c", d.Name)
	}
	if s.SetDefault("ghost") {
		t.Fatal("SetDefault(ghost) = true, want false")
	}

	// Only one entry may carry the flag.
	if !s.SetDefault("b") {
		t.Fatal("SetDefault(b) = false, want true")
	}
	flagged := 0
	for _, c := range s.List() {
		if c.Default {
			flagged++
			if c.Name != "b" {
				t.Errorf("flagged config = %q, want b", c.Name)
			}
		}
	}
	if flagged != 1 {
		t.Fatalf("%d configs flagged Default, want 1", flagged)
	}

	// Add with the flag pre-set steals the default.
	if err := s.Add(Config{Name: "d", Target: ".", Default: true}); err != nil {
		t.Fatalf("Add(d): %v", err)
	}
	if d, _ := s.Default(); d.Name != "d" {
		t.Fatalf("Default after flagged Add = %q, want d", d.Name)
	}

	// Removing the default falls back to the first remaining entry.
	if !s.Remove("d") {
		t.Fatal("Remove(d) = false")
	}
	if d, _ := s.Default(); d.Name != "a" {
		t.Fatalf("Default after removing the flagged one = %q, want a", d.Name)
	}
}

// TestStore_UpdateKeepsDefaultExclusive verifies a flagged Update clears
// the flag elsewhere.
func TestStore_UpdateKeepsDefaultExclusive(t *testing.T) {
	s := NewStore()
	if err := s.Add(Config{Name: "a", Target: ".", Default: true}); err != nil {
		t.Fatalf("Add(a): %v", err)
	}
	if err := s.Add(Config{Name: "b", Target: "."}); err != nil {
		t.Fatalf("Add(b): %v", err)
	}
	if err := s.Update("b", Config{Name: "b", Target: ".", Default: true}); err != nil {
		t.Fatalf("Update(b): %v", err)
	}
	if c, _ := s.Get("a"); c.Default {
		t.Error("a still flagged Default after b took it")
	}
	if d, _ := s.Default(); d.Name != "b" {
		t.Fatalf("Default = %q, want b", d.Name)
	}
}

// TestStore_SaveLoadRoundTrip verifies the JSON file round-trips every
// field, and that Save creates the project-local directory.
func TestStore_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := StorePath(dir)
	if want := filepath.Join(dir, ".silkide", "runconfig.json"); path != want {
		t.Fatalf("StorePath = %q, want %q", path, want)
	}

	s := NewStore()
	full := sample("run app")
	full.Default = true
	if err := s.Add(full); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.Add(Config{Name: "tests", Target: "./...", Mode: ModeTest}); err != nil {
		t.Fatalf("Add(tests): %v", err)
	}
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got.List(), s.List()) {
		t.Fatalf("round-trip mismatch:\n got %#v\nwant %#v", got.List(), s.List())
	}
	if d, ok := got.Default(); !ok || d.Name != "run app" {
		t.Fatalf("loaded Default = %#v, %v; want run app", d, ok)
	}
	// The loaded store is a live store: names stay unique.
	if err := got.Add(Config{Name: "tests", Target: "."}); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("Add on loaded store = %v, want ErrDuplicateName", err)
	}
}

// TestStore_SaveEmptyRoundTrip verifies an empty store survives the trip.
func TestStore_SaveEmptyRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runconfig.json")
	if err := NewStore().Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Len() != 0 {
		t.Fatalf("loaded Len = %d, want 0", got.Len())
	}
}

// TestLoad_Errors verifies a missing file, malformed JSON and a
// hand-edited file that breaks the invariants are all reported.
func TestLoad_Errors(t *testing.T) {
	dir := t.TempDir()

	if _, err := Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("Load(missing) = nil error, want one")
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Load(bad); err == nil {
		t.Error("Load(malformed) = nil error, want one")
	}

	dup := filepath.Join(dir, "dup.json")
	raw := `{"configs":[{"name":"a","target":"."},{"name":"a","target":"./x"}]}`
	if err := os.WriteFile(dup, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Load(dup); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("Load(duplicate names) = %v, want ErrDuplicateName", err)
	}

	noTarget := filepath.Join(dir, "notarget.json")
	if err := os.WriteFile(noTarget, []byte(`{"configs":[{"name":"a"}]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Load(noTarget); !errors.Is(err, ErrEmptyTarget) {
		t.Errorf("Load(no target) = %v, want ErrEmptyTarget", err)
	}
}

package ged

import (
	"reflect"
	"testing"
)

// TestParseEnvLines_BasicAndBlanks verifies parseEnvLines drops blank
// lines and trims surrounding whitespace, preserving entry order.
func TestParseEnvLines_BasicAndBlanks(t *testing.T) {
	raw := "FOO=1\n\n  BAR=2  \n\t\nBAZ=3\n"
	got := parseEnvLines(raw)
	want := []string{"FOO=1", "BAR=2", "BAZ=3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseEnvLines = %#v\nwant %#v", got, want)
	}
}

// TestParseEnvLines_Empty verifies the empty input returns a nil slice.
func TestParseEnvLines_Empty(t *testing.T) {
	if got := parseEnvLines(""); got != nil {
		t.Fatalf("parseEnvLines(\"\") = %#v, want nil", got)
	}
}

// TestJoinEnvLines_RoundTrip verifies join then parse returns the same
// slice (modulo the nil-vs-empty distinction).
func TestJoinEnvLines_RoundTrip(t *testing.T) {
	in := []string{"A=1", "B=2", "C=3"}
	raw := joinEnvLines(in)
	if raw != "A=1\nB=2\nC=3" {
		t.Fatalf("joinEnvLines = %q, want %q", raw, "A=1\nB=2\nC=3")
	}
	out := parseEnvLines(raw)
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("round-trip: %#v -> %#v", in, out)
	}
}

// TestJoinEnvLines_Empty verifies empty input becomes the empty string.
func TestJoinEnvLines_Empty(t *testing.T) {
	if got := joinEnvLines(nil); got != "" {
		t.Fatalf("joinEnvLines(nil) = %q, want \"\"", got)
	}
	if got := joinEnvLines([]string{}); got != "" {
		t.Fatalf("joinEnvLines([]) = %q, want \"\"", got)
	}
}

// TestParseEnvLines_OrderPreserved verifies the parser is stable across
// inputs with no blank lines.
func TestParseEnvLines_OrderPreserved(t *testing.T) {
	raw := "Z=1\nA=2\nM=3"
	got := parseEnvLines(raw)
	want := []string{"Z=1", "A=2", "M=3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseEnvLines preserved order = %#v\nwant %#v", got, want)
	}
}

// TestRunConfigPanel_NewIsEmpty verifies the freshly constructed panel
// returns a zero config.
func TestRunConfigPanel_NewIsEmpty(t *testing.T) {
	p := NewRunConfigPanel()
	got := p.Config()
	want := RunConfig{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Config() = %#v, want %#v", got, want)
	}
}

// TestRunConfigPanel_SetConfigGetConfig verifies a known struct round
// trips through SetConfig and Config().
func TestRunConfigPanel_SetConfigGetConfig(t *testing.T) {
	p := NewRunConfigPanel()
	cfg := RunConfig{
		Args:       "-v --flag",
		WorkingDir: "/tmp/work",
		Env:        []string{"FOO=1", "BAR=2"},
	}
	p.SetConfig(cfg)
	got := p.Config()
	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("Config() = %#v\nwant %#v", got, cfg)
	}
}

// TestRunConfigPanel_SetConfigNormalisesEnv verifies the panel pushes
// env entries through parse/join, dropping blanks and trimming spaces.
func TestRunConfigPanel_SetConfigNormalisesEnv(t *testing.T) {
	p := NewRunConfigPanel()
	p.SetConfig(RunConfig{
		Env: []string{"FOO=1", "   ", "BAR=2  ", ""},
	})
	got := p.Config().Env
	want := []string{"FOO=1", "BAR=2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalised env = %#v\nwant %#v", got, want)
	}
}

// TestRunConfigPanel_ConfigReturnsCopy verifies mutating the returned
// slice does not affect the panel.
func TestRunConfigPanel_ConfigReturnsCopy(t *testing.T) {
	p := NewRunConfigPanel()
	p.SetConfig(RunConfig{Env: []string{"FOO=1"}})

	got := p.Config()
	got.Env[0] = "MUTATED"

	again := p.Config()
	if again.Env[0] != "FOO=1" {
		t.Fatalf("Config() did not return a copy: panel Env = %#v", again.Env)
	}
}

// TestRunConfigPanel_SigChangedFiresOnDifferentSetConfig verifies the
// changed callback fires when SetConfig pushes a different value.
func TestRunConfigPanel_SigChangedFiresOnDifferentSetConfig(t *testing.T) {
	p := NewRunConfigPanel()
	var got RunConfig
	fired := 0
	p.SigChanged(func(c RunConfig) {
		got = c
		fired++
	})

	want := RunConfig{Args: "-x", WorkingDir: "/a", Env: []string{"K=V"}}
	p.SetConfig(want)
	if fired != 1 {
		t.Fatalf("SigChanged fired %d times, want 1", fired)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SigChanged got %#v\nwant %#v", got, want)
	}
}

// TestRunConfigPanel_SigChangedNoFireOnEqualSetConfig verifies the
// callback is NOT fired when SetConfig pushes the same value.
func TestRunConfigPanel_SigChangedNoFireOnEqualSetConfig(t *testing.T) {
	p := NewRunConfigPanel()
	cfg := RunConfig{Args: "-x", WorkingDir: "/a", Env: []string{"K=V"}}
	p.SetConfig(cfg)

	fired := 0
	p.SigChanged(func(c RunConfig) { fired++ })

	// Re-pushing the identical config must be a no-op.
	p.SetConfig(cfg)
	if fired != 0 {
		t.Fatalf("SigChanged fired %d times on equal SetConfig, want 0", fired)
	}

	// A trivially different config still fires.
	p.SetConfig(RunConfig{Args: "-y", WorkingDir: "/a", Env: []string{"K=V"}})
	if fired != 1 {
		t.Fatalf("SigChanged fired %d times after a real change, want 1", fired)
	}
}

// TestRunConfigPanel_SigChangedFiresOnArgsEdit verifies an in-panel
// edit via SetArgs fires the changed callback with the new config.
func TestRunConfigPanel_SigChangedFiresOnArgsEdit(t *testing.T) {
	p := NewRunConfigPanel()
	fired := 0
	var seen RunConfig
	p.SigChanged(func(c RunConfig) {
		fired++
		seen = c
	})

	p.SetArgs("--verbose")
	if fired != 1 {
		t.Fatalf("SetArgs fired %d, want 1", fired)
	}
	if seen.Args != "--verbose" {
		t.Errorf("seen.Args = %q, want %q", seen.Args, "--verbose")
	}

	// No-op SetArgs must NOT fire.
	p.SetArgs("--verbose")
	if fired != 1 {
		t.Fatalf("no-op SetArgs fired again: %d, want 1", fired)
	}
}

// TestRunConfigPanel_SigChangedFiresOnWorkingDirEdit verifies SetWorkingDir.
func TestRunConfigPanel_SigChangedFiresOnWorkingDirEdit(t *testing.T) {
	p := NewRunConfigPanel()
	fired := 0
	p.SigChanged(func(c RunConfig) { fired++ })

	p.SetWorkingDir("/tmp")
	if fired != 1 {
		t.Fatalf("SetWorkingDir fired %d, want 1", fired)
	}
	p.SetWorkingDir("/tmp")
	if fired != 1 {
		t.Fatalf("no-op SetWorkingDir fired again: %d, want 1", fired)
	}
}

// TestRunConfigPanel_AddRemoveEnvFiresChanged verifies env add and remove
// drive the changed callback and that Config reflects the new state.
func TestRunConfigPanel_AddRemoveEnvFiresChanged(t *testing.T) {
	p := NewRunConfigPanel()
	fired := 0
	p.SigChanged(func(c RunConfig) { fired++ })

	// Empty placeholder does not change the canonical (blank-dropped) cfg,
	// so the callback is correctly silent.
	p.AddEnv("")
	if fired != 0 {
		t.Fatalf("blank AddEnv fired %d, want 0", fired)
	}

	// A real value flushes through and fires.
	p.SetEnvAt(0, "FOO=1")
	if fired != 1 {
		t.Fatalf("SetEnvAt fired %d, want 1", fired)
	}
	if got := p.Config().Env; !reflect.DeepEqual(got, []string{"FOO=1"}) {
		t.Fatalf("Env = %#v, want [FOO=1]", got)
	}

	// Removing the only env row clears cfg.Env and fires once.
	p.RemoveEnv(0)
	if fired != 2 {
		t.Fatalf("RemoveEnv fired %d, want 2", fired)
	}
	if got := p.Config().Env; len(got) != 0 {
		t.Fatalf("after RemoveEnv, Env = %#v, want empty", got)
	}
}

// TestRunConfigPanel_HitRowMapping is a nil-safe smoke test that the
// hit-tester returns the expected synthetic codes for the canonical
// row layout, without exercising Draw.
func TestRunConfigPanel_HitRowMapping(t *testing.T) {
	p := NewRunConfigPanel()
	p.SetSize(300, 400)
	p.SetConfig(RunConfig{Env: []string{"A=1", "B=2"}})

	// Header click maps to none.
	if got := p.hitRow(50, 5); got != rcHoverNone {
		t.Errorf("header hit = %d, want %d", got, rcHoverNone)
	}
	// Args row center.
	argsCenter := p.rowYArgs() + rcRowH/2
	if got := p.hitRow(50, argsCenter); got != rcHoverArgs {
		t.Errorf("args hit = %d, want %d", got, rcHoverArgs)
	}
	// Working dir row center.
	wdCenter := p.rowYWD() + rcRowH/2
	if got := p.hitRow(50, wdCenter); got != rcHoverWD {
		t.Errorf("wd hit = %d, want %d", got, rcHoverWD)
	}
	// First env row.
	env0 := p.envBlockY() + rcEnvRowH/2
	if got := p.hitRow(50, env0); got != 0 {
		t.Errorf("env[0] hit = %d, want 0", got)
	}
	// Second env row.
	env1 := p.envBlockY() + rcEnvRowH + rcEnvRowH/2
	if got := p.hitRow(50, env1); got != 1 {
		t.Errorf("env[1] hit = %d, want 1", got)
	}
	// Add button.
	add := p.addBtnY() + rcAddBtnH/2
	if got := p.hitRow(rcPadLeft+1, add); got != rcHoverAdd {
		t.Errorf("add hit = %d, want %d", got, rcHoverAdd)
	}
}

// TestRunConfigPanel_SizeHints sanity-checks the default size hints.
func TestRunConfigPanel_SizeHints(t *testing.T) {
	p := NewRunConfigPanel()
	sh := p.SizeHints()
	if sh.Width <= 0 || sh.Height <= 0 {
		t.Errorf("SizeHints = %+v, want positive defaults", sh)
	}
}

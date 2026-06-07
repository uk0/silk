package ged

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestMergeEnv_EmptyExtra: extra is empty → base passes through unchanged.
func TestMergeEnv_EmptyExtra(t *testing.T) {
	base := []string{"A=1", "B=2", "C=3"}
	got := mergeEnv(base, nil)
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("nil extra: want %v, got %v", base, got)
	}
	got = mergeEnv(base, []string{})
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("empty extra: want %v, got %v", base, got)
	}
}

// TestMergeEnv_BothEmpty: nothing in, empty (or nil-ish) out.
func TestMergeEnv_BothEmpty(t *testing.T) {
	got := mergeEnv(nil, nil)
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
	got = mergeEnv([]string{}, []string{})
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}

// TestMergeEnv_NewKeyAppended: keys not in base are appended at the end
// in the order they first appeared in extra.
func TestMergeEnv_NewKeyAppended(t *testing.T) {
	base := []string{"A=1", "B=2"}
	extra := []string{"C=3", "D=4"}
	got := mergeEnv(base, extra)
	want := []string{"A=1", "B=2", "C=3", "D=4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

// TestMergeEnv_OverrideInPlace: a key present in both is replaced in
// base's position; extra value wins; no duplicate entry is appended.
func TestMergeEnv_OverrideInPlace(t *testing.T) {
	base := []string{"A=1", "B=2", "C=3"}
	extra := []string{"B=NEW"}
	got := mergeEnv(base, extra)
	want := []string{"A=1", "B=NEW", "C=3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

// TestMergeEnv_OverrideRelativeOrder: overriding several keys preserves
// the original base order for those positions; new keys still go to the
// end.
func TestMergeEnv_OverrideRelativeOrder(t *testing.T) {
	base := []string{"A=1", "B=2", "C=3", "D=4"}
	extra := []string{"C=cc", "A=aa", "E=ee"}
	got := mergeEnv(base, extra)
	want := []string{"A=aa", "B=2", "C=cc", "D=4", "E=ee"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

// TestMergeEnv_LastWinsWithinExtra: multiple entries for the same key in
// extra collapse to the LAST one (Qt/VS Code style explicit-env wins).
func TestMergeEnv_LastWinsWithinExtra(t *testing.T) {
	base := []string{"X=base"}
	extra := []string{"X=first", "X=second", "X=third"}
	got := mergeEnv(base, extra)
	want := []string{"X=third"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}

	// Last-wins applies for new keys too.
	base = []string{"A=1"}
	extra = []string{"B=one", "B=two"}
	got = mergeEnv(base, extra)
	want = []string{"A=1", "B=two"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("new-key last-wins: want %v, got %v", want, got)
	}
}

// TestMergeEnv_MalformedPreserved: an entry with no '=' is preserved as
// a passthrough rather than dropped (matches os/exec behaviour, which
// doesn't validate Env entries).
func TestMergeEnv_MalformedPreserved(t *testing.T) {
	base := []string{"A=1", "GARBAGE_BASE", "B=2"}
	extra := []string{"C=3", "GARBAGE_EXTRA"}
	got := mergeEnv(base, extra)
	// base malformed entry keeps its position; extra malformed entry goes
	// after appended new keys (we treat malformed-from-extra as "neither
	// override nor new key, just preserve").
	want := []string{"A=1", "GARBAGE_BASE", "B=2", "C=3", "GARBAGE_EXTRA"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

// TestMergeEnv_DoesNotMutate: neither input slice is touched.
func TestMergeEnv_DoesNotMutate(t *testing.T) {
	base := []string{"A=1", "B=2"}
	extra := []string{"B=NEW", "C=3"}
	baseCopy := append([]string(nil), base...)
	extraCopy := append([]string(nil), extra...)
	_ = mergeEnv(base, extra)
	if !reflect.DeepEqual(base, baseCopy) {
		t.Fatalf("base mutated: want %v, got %v", baseCopy, base)
	}
	if !reflect.DeepEqual(extra, extraCopy) {
		t.Fatalf("extra mutated: want %v, got %v", extraCopy, extra)
	}
}

// TestMergeEnv_EmptyValue: KEY= (value is empty string) is a valid override.
func TestMergeEnv_EmptyValue(t *testing.T) {
	base := []string{"A=1", "B=2"}
	extra := []string{"A="}
	got := mergeEnv(base, extra)
	want := []string{"A=", "B=2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

// TestTerminalSetExtraEnv_CopiesOnSet: mutating the caller's slice after
// SetExtraEnv must not leak into the panel's stored state.
func TestTerminalSetExtraEnv_CopiesOnSet(t *testing.T) {
	p := &TerminalPanel{}
	in := []string{"A=1", "B=2"}
	p.SetExtraEnv(in)

	// Mutate the caller-owned slice.
	in[0] = "A=HACKED"

	got := p.ExtraEnv()
	want := []string{"A=1", "B=2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SetExtraEnv didn't snapshot input: want %v, got %v", want, got)
	}
}

// TestTerminalExtraEnv_ReturnsCopy: mutating the result of ExtraEnv()
// must not leak back into the panel's stored state.
func TestTerminalExtraEnv_ReturnsCopy(t *testing.T) {
	p := &TerminalPanel{}
	p.SetExtraEnv([]string{"A=1", "B=2"})

	got := p.ExtraEnv()
	got[0] = "A=HACKED"

	again := p.ExtraEnv()
	want := []string{"A=1", "B=2"}
	if !reflect.DeepEqual(again, want) {
		t.Fatalf("ExtraEnv didn't return a copy: want %v, got %v", want, again)
	}
}

// TestTerminalExtraEnv_EmptyAndNil: empty / nil inputs round-trip to nil.
func TestTerminalExtraEnv_EmptyAndNil(t *testing.T) {
	p := &TerminalPanel{}
	if got := p.ExtraEnv(); got != nil {
		t.Fatalf("default ExtraEnv: want nil, got %v", got)
	}
	p.SetExtraEnv([]string{"A=1"})
	p.SetExtraEnv(nil)
	if got := p.ExtraEnv(); got != nil {
		t.Fatalf("after SetExtraEnv(nil): want nil, got %v", got)
	}
	p.SetExtraEnv([]string{"A=1"})
	p.SetExtraEnv([]string{})
	if got := p.ExtraEnv(); got != nil {
		t.Fatalf("after SetExtraEnv([]): want nil, got %v", got)
	}
}

// TestMergeEnv_LargeBaseUntouched: a representative os.Environ()-shaped
// input is preserved entry-for-entry when there's no overlap.
func TestMergeEnv_LargeBaseUntouched(t *testing.T) {
	base := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/Users/me",
		"LANG=en_US.UTF-8",
		"TERM=xterm-256color",
		"SHELL=/bin/zsh",
	}
	extra := []string{"DEBUG=1"}
	got := mergeEnv(base, extra)
	// All base keys must appear, plus DEBUG appended.
	keys := func(env []string) []string {
		ks := make([]string, 0, len(env))
		for _, e := range env {
			if i := strings.IndexByte(e, '='); i >= 0 {
				ks = append(ks, e[:i])
			}
		}
		sort.Strings(ks)
		return ks
	}
	wantKeys := []string{"DEBUG", "HOME", "LANG", "PATH", "SHELL", "TERM"}
	if got := keys(got); !reflect.DeepEqual(got, wantKeys) {
		t.Fatalf("keys: want %v, got %v", wantKeys, got)
	}
}

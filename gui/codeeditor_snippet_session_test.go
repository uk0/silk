package gui

import (
	"testing"

	"github.com/uk0/silk/snippet"
)

// ---------------------------------------------------------------------------
// snippetSession — pure, GL-free tab-stop tracking over a snippet.Expansion.
// These tests never construct a CodeEditor or a window; they exercise the
// session type directly (offset shifting, Next/Prev ordering, mirror grouping,
// Active lifecycle).
// ---------------------------------------------------------------------------

// TestSnippetSessionShiftAndOrder builds a session from the canonical "if"
// snippet inserted at byte offset N and checks that stop ranges are shifted by
// N, Next() walks $1 then $0, and Active() goes false after $0.
func TestSnippetSessionShiftAndOrder(t *testing.T) {
	const N = 10
	exp, err := snippet.Expand("if ${1:cond} {\n\t${0}\n}")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	// Expanded text is "if cond {\n\t\n}": $1 default "cond" occupies bytes
	// [3,7); $0 is a zero-width caret at byte 11.
	if want := "if cond {\n\t\n}"; exp.Text != want {
		t.Fatalf("Expand text = %q, want %q", exp.Text, want)
	}

	sess := newSnippetSession(exp, N)
	if sess == nil {
		t.Fatal("newSnippetSession returned nil for a stopped expansion")
	}
	if !sess.Active() {
		t.Fatal("session should be active from creation")
	}

	// First Next -> $1, default "cond", unshifted [3,7) shifted to [3+N,7+N).
	rs, ok := sess.Next()
	if !ok {
		t.Fatal("Next() #1 (want $1) returned ok=false")
	}
	if len(rs) != 1 {
		t.Fatalf("$1 yielded %d ranges, want 1", len(rs))
	}
	if rs[0].Start != 3+N || rs[0].End != 7+N {
		t.Errorf("$1 range = [%d,%d), want shifted [%d,%d)", rs[0].Start, rs[0].End, 3+N, 7+N)
	}
	if !sess.Active() {
		t.Error("session should still be active while on $1")
	}

	// Second Next -> $0, zero-width, unshifted [11,11] shifted to [11+N,11+N].
	rs, ok = sess.Next()
	if !ok {
		t.Fatal("Next() #2 (want $0) returned ok=false")
	}
	if len(rs) != 1 || rs[0].Start != 11+N || rs[0].End != 11+N {
		t.Errorf("$0 range = %+v, want single zero-width stop at %d", rs, 11+N)
	}

	// $0 is the final caret: the session is done.
	if sess.Active() {
		t.Error("session should be inactive after landing on $0")
	}
	if _, ok := sess.Next(); ok {
		t.Error("Next() past $0 should return ok=false")
	}
}

// TestSnippetSessionPrevWalksBack confirms Prev() steps back from $0 to $1 and
// is a no-op at the first stop.
func TestSnippetSessionPrevWalksBack(t *testing.T) {
	const N = 4
	exp, err := snippet.Expand("if ${1:cond} {\n\t${0}\n}")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	sess := newSnippetSession(exp, N)
	sess.Next() // $1
	sess.Next() // $0

	rs, ok := sess.Prev()
	if !ok {
		t.Fatal("Prev() from $0 returned ok=false")
	}
	if len(rs) != 1 || rs[0].Start != 3+N || rs[0].End != 7+N {
		t.Errorf("Prev range = %+v, want $1 at shifted [%d,%d)", rs, 3+N, 7+N)
	}
	if _, ok := sess.Prev(); ok {
		t.Error("Prev() at the first stop should return ok=false")
	}
}

// TestSnippetSessionMirrorGroupsOneStep checks that mirrored stops (same Index)
// collapse into a single step: one Next() yields both ranges.
func TestSnippetSessionMirrorGroupsOneStep(t *testing.T) {
	const N = 5
	exp, err := snippet.Expand("${1:x} $1")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	// Expanded text is "x ": the default "x" occupies [0,1); the bare mirror $1
	// is a zero-width caret at byte 2.
	if want := "x "; exp.Text != want {
		t.Fatalf("Expand text = %q, want %q", exp.Text, want)
	}

	sess := newSnippetSession(exp, N)
	rs, ok := sess.Next()
	if !ok {
		t.Fatal("Next() (want mirrored $1) returned ok=false")
	}
	if len(rs) != 2 {
		t.Fatalf("mirrored $1 yielded %d ranges, want 2", len(rs))
	}
	if rs[0].Start != 0+N || rs[0].End != 1+N {
		t.Errorf("mirror range[0] = [%d,%d), want shifted [%d,%d)", rs[0].Start, rs[0].End, 0+N, 1+N)
	}
	if rs[1].Start != 2+N || rs[1].End != 2+N {
		t.Errorf("mirror range[1] = [%d,%d), want zero-width at %d", rs[1].Start, rs[1].End, 2+N)
	}
	// Both mirrors are one step and there is no $0, so the session ends here.
	if sess.Active() {
		t.Error("single-step session should be inactive after its only step")
	}
}

// TestSnippetSessionNoStops covers a placeholder-free expansion (nil session)
// and the nil-receiver safety of the session methods.
func TestSnippetSessionNoStops(t *testing.T) {
	exp, err := snippet.Expand("plain text, no stops")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if s := newSnippetSession(exp, 0); s != nil {
		t.Errorf("newSnippetSession = %+v, want nil for a stop-less expansion", s)
	}

	var s *snippetSession // nil
	if s.Active() {
		t.Error("nil session should report Active() == false")
	}
	if _, ok := s.Next(); ok {
		t.Error("nil session Next() should return ok=false")
	}
	if _, ok := s.Prev(); ok {
		t.Error("nil session Prev() should return ok=false")
	}
}

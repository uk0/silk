package ged

import (
	"testing"
)

// --- fixtures ---

// insightsLegend is a gopls-shaped semantic legend: 8 token types (indices
// 0..7) and 9 modifiers (bits 0..8). Anything outside those ranges in a test
// stream is deliberately unresolvable.
func insightsLegend() SemanticLegend {
	return SemanticLegend{
		Types:     []string{"namespace", "type", "function", "variable", "parameter", "property", "keyword", "comment"},
		Modifiers: []string{"declaration", "definition", "readonly", "static", "deprecated", "abstract", "async", "documentation", "defaultLibrary"},
	}
}

// insightsTokenData is a 4-token delta-encoded stream exercising every rule of
// the encoding in one fixture:
//
//	quintuple 0: deltaLine 2  -> line 2, char 5 (fresh line, absolute char)
//	quintuple 1: deltaLine 0  -> SAME line, char 5+5=10 (relative continuation)
//	quintuple 2: deltaLine 1  -> line 3, char 2 (absolute again)
//	quintuple 3: deltaLine 3  -> line 6, unresolvable type index + out-of-legend
//	                             modifier bit
func insightsTokenData() []uint32 {
	return []uint32{
		2, 5, 3, 2, 3, // function, declaration|definition
		0, 5, 4, 4, 0, // parameter, no modifiers
		1, 2, 6, 1, 4, // type, readonly
		3, 0, 2, 99, 512, // type index 99 unknown, modifier bit 9 not in legend
	}
}

// insightsTokenEqual compares two tokens including the Mods slice, which rules
// out plain ==.
func insightsTokenEqual(a, b Token) bool {
	if a.Line != b.Line || a.StartChar != b.StartChar || a.Length != b.Length || a.Type != b.Type {
		return false
	}
	if len(a.Mods) != len(b.Mods) {
		return false
	}
	for i := range a.Mods {
		if a.Mods[i] != b.Mods[i] {
			return false
		}
	}
	return true
}

// insightsCheckTokens asserts a decoded token slice matches want field by
// field, reporting the first mismatch.
func insightsCheckTokens(t *testing.T, what string, got, want []Token) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: decoded %d tokens, want %d: %+v", what, len(got), len(want), got)
	}
	for i := range want {
		if !insightsTokenEqual(got[i], want[i]) {
			t.Errorf("%s: token[%d] = %+v, want %+v", what, i, got[i], want[i])
		}
	}
}

// --- (a) semantic tokens: decode ---

// TestSemanticTokensDecodeStream checks the delta decode turns the quintuple
// stream into absolute positions: a same-line continuation adds to the previous
// character, a line change resets it to the absolute value, and unresolvable
// legend indices degrade to an empty type / no modifiers instead of failing.
func TestSemanticTokensDecodeStream(t *testing.T) {
	st := NewSemanticTokens(insightsLegend())
	if err := st.SetData("v1", insightsTokenData()); err != nil {
		t.Fatalf("SetData: %v", err)
	}

	want := []Token{
		{Line: 2, StartChar: 5, Length: 3, Type: "function", Mods: []string{"declaration", "definition"}},
		{Line: 2, StartChar: 10, Length: 4, Type: "parameter"},
		{Line: 3, StartChar: 2, Length: 6, Type: "type", Mods: []string{"readonly"}},
		{Line: 6, StartChar: 0, Length: 2},
	}
	insightsCheckTokens(t, "SetData", st.Tokens(), want)

	if st.ResultID() != "v1" {
		t.Errorf("ResultID() = %q, want %q", st.ResultID(), "v1")
	}
}

// TestSemanticTokensOnLine checks the per-line index the overlay queries:
// both tokens of line 2 come back in character order, and a bare line yields
// nothing.
func TestSemanticTokensOnLine(t *testing.T) {
	st := NewSemanticTokens(insightsLegend())
	if err := st.SetData("v1", insightsTokenData()); err != nil {
		t.Fatalf("SetData: %v", err)
	}

	on := st.TokensOnLine(2)
	if len(on) != 2 {
		t.Fatalf("TokensOnLine(2) returned %d tokens, want 2: %+v", len(on), on)
	}
	if on[0].StartChar != 5 || on[1].StartChar != 10 {
		t.Errorf("TokensOnLine(2) chars = %d,%d, want 5,10", on[0].StartChar, on[1].StartChar)
	}
	if got := st.TokensOnLine(4); got != nil {
		t.Errorf("TokensOnLine(4) = %+v, want nil for a line with no tokens", got)
	}
	if got := st.TokensOnLine(6); len(got) != 1 {
		t.Errorf("TokensOnLine(6) returned %d tokens, want 1", len(got))
	}
}

// TestSemanticTokensModifierBitmask covers the bitmask -> legend-name
// expansion on its own: bit order is LSB first, a zero mask allocates nothing,
// and bits with no legend entry are ignored rather than inventing names.
func TestSemanticTokensModifierBitmask(t *testing.T) {
	mods := insightsLegend().Modifiers
	cases := []struct {
		mask uint32
		want []string
	}{
		{0, nil},
		{1, []string{"declaration"}},
		{1 << 1, []string{"definition"}},
		{3, []string{"declaration", "definition"}},
		{1<<0 | 1<<2, []string{"declaration", "readonly"}},
		{1 << 8, []string{"defaultLibrary"}},
		{1 << 9, nil},                       // bit past the legend: ignored
		{1<<2 | 1<<9, []string{"readonly"}}, // known bit kept, unknown dropped
		{0xFFFFFFFF, mods},                  // every legend bit set
	}
	for _, c := range cases {
		got := expandTokenModifiers(mods, c.mask)
		if len(got) != len(c.want) {
			t.Errorf("expandTokenModifiers(mask=%d) = %v, want %v", c.mask, got, c.want)
			continue
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("expandTokenModifiers(mask=%d)[%d] = %q, want %q", c.mask, i, got[i], c.want[i])
			}
		}
	}
}

// TestSemanticTokensLegendLateArrival checks a legend that shows up after the
// first token stream still resolves it: SetLegend re-decodes the retained raw
// data instead of waiting for the next server reply.
func TestSemanticTokensLegendLateArrival(t *testing.T) {
	st := NewSemanticTokens(SemanticLegend{})
	if err := st.SetData("v1", insightsTokenData()); err != nil {
		t.Fatalf("SetData: %v", err)
	}
	for i, tok := range st.Tokens() {
		if tok.Type != "" || tok.Mods != nil {
			t.Fatalf("token[%d] resolved %q/%v without a legend", i, tok.Type, tok.Mods)
		}
	}

	st.SetLegend(insightsLegend())
	got := st.Tokens()
	if len(got) != 4 {
		t.Fatalf("SetLegend dropped tokens: %d left, want 4", len(got))
	}
	if got[0].Type != "function" || len(got[0].Mods) != 2 {
		t.Errorf("after SetLegend token[0] = %+v, want function with 2 modifiers", got[0])
	}
}

// --- (a) semantic tokens: delta update ---

// TestSemanticTokensApplyEditsReplace applies a delta that rewrites one
// quintuple in place: the replaced token picks up the new character delta and
// type, and the tokens after it keep their absolute positions because their own
// deltas are untouched.
func TestSemanticTokensApplyEditsReplace(t *testing.T) {
	st := NewSemanticTokens(insightsLegend())
	if err := st.SetData("v1", insightsTokenData()); err != nil {
		t.Fatalf("SetData: %v", err)
	}

	// Replace quintuple 1 (raw offsets 5..9) with "same line, char +7, keyword".
	edits := []SemanticTokenEdit{{Start: 5, DeleteCount: 5, Data: []uint32{0, 7, 2, 6, 0}}}
	if err := st.ApplyEdits("v2", edits); err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}

	want := []Token{
		{Line: 2, StartChar: 5, Length: 3, Type: "function", Mods: []string{"declaration", "definition"}},
		{Line: 2, StartChar: 12, Length: 2, Type: "keyword"},
		{Line: 3, StartChar: 2, Length: 6, Type: "type", Mods: []string{"readonly"}},
		{Line: 6, StartChar: 0, Length: 2},
	}
	insightsCheckTokens(t, "ApplyEdits(replace)", st.Tokens(), want)
	if st.ResultID() != "v2" {
		t.Errorf("ResultID() = %q, want %q", st.ResultID(), "v2")
	}
	if len(st.Data()) != 20 {
		t.Errorf("Data() length = %d, want 20 after an equal-size replace", len(st.Data()))
	}
}

// TestSemanticTokensApplyEditsInsertAndDelete applies two ascending edits in
// one delta — a pure insert at the head and a pure delete at the tail — and
// checks both offsets were taken against the PREVIOUS stream (a simultaneous
// splice), not against the partially rewritten one.
func TestSemanticTokensApplyEditsInsertAndDelete(t *testing.T) {
	st := NewSemanticTokens(insightsLegend())
	if err := st.SetData("v1", insightsTokenData()); err != nil {
		t.Fatalf("SetData: %v", err)
	}

	edits := []SemanticTokenEdit{
		{Start: 0, DeleteCount: 0, Data: []uint32{0, 0, 1, 7, 0}}, // new first token on line 0
		{Start: 15, DeleteCount: 5},                               // drop the last token
	}
	if err := st.ApplyEdits("v2", edits); err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}

	want := []Token{
		{Line: 0, StartChar: 0, Length: 1, Type: "comment"},
		{Line: 2, StartChar: 5, Length: 3, Type: "function", Mods: []string{"declaration", "definition"}},
		{Line: 2, StartChar: 10, Length: 4, Type: "parameter"},
		{Line: 3, StartChar: 2, Length: 6, Type: "type", Mods: []string{"readonly"}},
	}
	insightsCheckTokens(t, "ApplyEdits(insert+delete)", st.Tokens(), want)
	if got := st.TokensOnLine(6); got != nil {
		t.Errorf("TokensOnLine(6) = %+v, want nil after the tail token was deleted", got)
	}
}

// TestSemanticTokensApplyEditsEmpty checks the "nothing changed" delta: no
// edits keeps the tokens and only refreshes the result id, so the next delta
// request quotes the right previousResultId.
func TestSemanticTokensApplyEditsEmpty(t *testing.T) {
	st := NewSemanticTokens(insightsLegend())
	if err := st.SetData("v1", insightsTokenData()); err != nil {
		t.Fatalf("SetData: %v", err)
	}
	if err := st.ApplyEdits("v2", nil); err != nil {
		t.Fatalf("ApplyEdits(nil): %v", err)
	}
	if len(st.Tokens()) != 4 {
		t.Errorf("empty delta changed the token count to %d, want 4", len(st.Tokens()))
	}
	if st.ResultID() != "v2" {
		t.Errorf("ResultID() = %q, want %q", st.ResultID(), "v2")
	}
}

// TestSemanticTokensRejectsMalformed checks every bad payload is refused AND
// leaves the previous highlighting intact — a broken reply must not blank a
// working overlay.
func TestSemanticTokensRejectsMalformed(t *testing.T) {
	st := NewSemanticTokens(insightsLegend())
	if err := st.SetData("v1", insightsTokenData()); err != nil {
		t.Fatalf("SetData: %v", err)
	}

	if err := st.SetData("bad", []uint32{1, 2, 3}); err == nil {
		t.Error("SetData with a length that is not a multiple of 5 should fail")
	}
	badEdits := map[string][]SemanticTokenEdit{
		"out of range":      {{Start: 18, DeleteCount: 5}},
		"descending":        {{Start: 10, DeleteCount: 5}, {Start: 0, DeleteCount: 0, Data: []uint32{0, 0, 1, 7, 0}}},
		"overlapping":       {{Start: 0, DeleteCount: 6}, {Start: 5, DeleteCount: 5}},
		"misaligned result": {{Start: 0, DeleteCount: 1}},
		"negative delete":   {{Start: 0, DeleteCount: -1}},
	}
	for what, edits := range badEdits {
		if err := st.ApplyEdits("bad", edits); err == nil {
			t.Errorf("ApplyEdits(%s) should fail", what)
		}
	}

	// State survived every rejection.
	insightsCheckTokens(t, "after rejections", st.Tokens(), []Token{
		{Line: 2, StartChar: 5, Length: 3, Type: "function", Mods: []string{"declaration", "definition"}},
		{Line: 2, StartChar: 10, Length: 4, Type: "parameter"},
		{Line: 3, StartChar: 2, Length: 6, Type: "type", Mods: []string{"readonly"}},
		{Line: 6, StartChar: 0, Length: 2},
	})
	if st.ResultID() != "v1" {
		t.Errorf("ResultID() = %q, want it unchanged at %q after rejections", st.ResultID(), "v1")
	}
}

// TestSemanticTokensCopySemantics verifies the model owns its data: mutating
// the caller's raw stream after SetData, or the slices handed back by
// Tokens()/Data(), does not reach the decoded state.
func TestSemanticTokensCopySemantics(t *testing.T) {
	st := NewSemanticTokens(insightsLegend())
	in := insightsTokenData()
	if err := st.SetData("v1", in); err != nil {
		t.Fatalf("SetData: %v", err)
	}

	in[0] = 99 // caller keeps using its slice
	if got := st.Tokens()[0]; got.Line != 2 {
		t.Errorf("input mutation leaked: token[0] = %+v, want line 2", got)
	}

	out := st.Tokens()
	out[0].Type = "MUTATED"
	if got := st.Tokens()[0]; got.Type != "function" {
		t.Errorf("Tokens() mutation leaked: token[0].Type = %q", got.Type)
	}

	raw := st.Data()
	raw[0] = 77
	if got := st.Data(); got[0] != 2 {
		t.Errorf("Data() mutation leaked: raw[0] = %d, want 2", got[0])
	}

	st.Clear()
	if len(st.Tokens()) != 0 || len(st.Data()) != 0 || st.ResultID() != "" {
		t.Errorf("Clear left state behind: %d tokens, %d raw, id %q", len(st.Tokens()), len(st.Data()), st.ResultID())
	}
	if len(st.Legend().Types) != 8 {
		t.Errorf("Clear dropped the legend: %d types left, want 8", len(st.Legend().Types))
	}
}

// --- (b) inlay hints ---

// insightsHints is a representative hint set: two hints on line 4 given out of
// column order, plus one on line 9.
func insightsHints() []Hint {
	return []Hint{
		{Line: 4, Col: 20, Label: "int", Kind: HintKindType, PaddingLeft: true},
		{Line: 4, Col: 9, Label: "name:", Kind: HintKindParameter, PaddingRight: true},
		{Line: 9, Col: 2, Label: "err error", Kind: HintKindType},
	}
}

// TestInlayHintsRevisionGating is the core freshness rule: hints answer only
// for the revision they were computed against. Once the document revision
// advances they are dropped instead of being painted at columns that moved.
func TestInlayHintsRevisionGating(t *testing.T) {
	h := NewInlayHints()
	if !h.Set(3, insightsHints()) {
		t.Fatal("Set(3, ...) was rejected on a fresh model")
	}
	if h.Revision() != 3 {
		t.Errorf("Revision() = %d, want 3", h.Revision())
	}
	if h.Stale(3) {
		t.Error("Stale(3) = true right after Set(3, ...)")
	}
	if !h.Stale(4) {
		t.Error("Stale(4) = false, want true: the buffer moved past revision 3")
	}
	if got := h.HintsFor(3); len(got) != 3 {
		t.Errorf("HintsFor(3) returned %d hints, want 3", len(got))
	}
	if got := h.HintsFor(4); got != nil {
		t.Errorf("HintsFor(4) = %+v, want nil for a stale revision", got)
	}

	// Document edited: revision advances, hints go away.
	h.SetRevision(4)
	if h.Revision() != 4 {
		t.Errorf("Revision() = %d after SetRevision(4), want 4", h.Revision())
	}
	if got := h.Hints(); len(got) != 0 {
		t.Errorf("Hints() = %+v, want empty after the revision advanced", got)
	}
	if h.Stale(4) {
		t.Error("Stale(4) = true after SetRevision(4)")
	}

	// A reply computed for the old revision arrives late and is refused.
	if h.Set(3, insightsHints()) {
		t.Error("Set(3, ...) was accepted after the document reached revision 4")
	}
	if len(h.Hints()) != 0 || h.Revision() != 4 {
		t.Errorf("refused Set mutated state: %d hints at revision %d", len(h.Hints()), h.Revision())
	}

	// A reply for the current revision is accepted again.
	if !h.Set(4, insightsHints()) {
		t.Fatal("Set(4, ...) was rejected at revision 4")
	}
	if len(h.HintsFor(4)) != 3 {
		t.Errorf("HintsFor(4) returned %d hints, want 3", len(h.HintsFor(4)))
	}

	// SetRevision never moves backwards, so a stale notification is harmless.
	h.SetRevision(2)
	if h.Revision() != 4 || len(h.Hints()) != 3 {
		t.Errorf("SetRevision(2) went backwards: revision %d, %d hints", h.Revision(), len(h.Hints()))
	}
}

// TestInlayHintsOnLine checks the per-line query sorts by column (servers may
// answer in any order) and honours the same revision gate as HintsFor.
func TestInlayHintsOnLine(t *testing.T) {
	h := NewInlayHints()
	h.Set(1, insightsHints())

	on := h.HintsOnLine(1, 4)
	if len(on) != 2 {
		t.Fatalf("HintsOnLine(1, 4) returned %d hints, want 2: %+v", len(on), on)
	}
	if on[0].Col != 9 || on[1].Col != 20 {
		t.Errorf("HintsOnLine(1, 4) cols = %d,%d, want 9,20 (column order)", on[0].Col, on[1].Col)
	}
	if got := h.HintsOnLine(1, 5); got != nil {
		t.Errorf("HintsOnLine(1, 5) = %+v, want nil for a line with no hints", got)
	}
	if got := h.HintsOnLine(2, 4); got != nil {
		t.Errorf("HintsOnLine(2, 4) = %+v, want nil for a stale revision", got)
	}

	// The model owns its hints.
	in := insightsHints()
	h.Set(1, in)
	in[0].Label = "MUTATED"
	if got := h.HintsOnLine(1, 4); got[1].Label != "int" {
		t.Errorf("input mutation leaked: hint label = %q", got[1].Label)
	}

	h.Clear()
	if len(h.Hints()) != 0 {
		t.Errorf("Clear left %d hints", len(h.Hints()))
	}
}

// TestInlayHintsText checks the padding flags render as the leading/trailing
// space the server asked for — the only presentation those two booleans have.
func TestInlayHintsText(t *testing.T) {
	cases := []struct {
		hint Hint
		want string
	}{
		{Hint{Label: "int"}, "int"},
		{Hint{Label: "int", PaddingLeft: true}, " int"},
		{Hint{Label: "name:", PaddingRight: true}, "name: "},
		{Hint{Label: "x", PaddingLeft: true, PaddingRight: true}, " x "},
	}
	for _, c := range cases {
		if got := c.hint.Text(); got != c.want {
			t.Errorf("Hint%+v.Text() = %q, want %q", c.hint, got, c.want)
		}
	}
}

// --- (c) code lens ---

// insightsLenses is a representative lens set: two lenses sharing line 10 and
// one on line 40.
func insightsLenses() []Lens {
	return []Lens{
		{Line: 10, Title: "3 references", Command: "silk.showReferences", Args: []string{"main.go", "10"}},
		{Line: 10, Title: "run test", Command: "silk.runTest", Args: []string{"TestFoo"}},
		{Line: 40, Title: "implementations", Command: "silk.showImplementations"},
	}
}

// TestCodeLensHitTestAndExecute checks hit-testing by line and the execute
// callback: the index within a line's lens list is what the drawer's x
// hit-test produces, a miss is inert, and the whole Lens (command + args)
// reaches the host.
func TestCodeLensHitTestAndExecute(t *testing.T) {
	c := NewCodeLens()
	c.SetLenses(insightsLenses())

	on := c.LensesOnLine(10)
	if len(on) != 2 {
		t.Fatalf("LensesOnLine(10) returned %d lenses, want 2: %+v", len(on), on)
	}
	if on[0].Title != "3 references" || on[1].Title != "run test" {
		t.Errorf("LensesOnLine(10) order = %q,%q, want insertion order", on[0].Title, on[1].Title)
	}
	if got := c.LensesOnLine(11); got != nil {
		t.Errorf("LensesOnLine(11) = %+v, want nil for an undecorated line", got)
	}

	var fired []Lens
	c.OnExecute(func(l Lens) { fired = append(fired, l) })

	if !c.ExecuteAt(10, 1) {
		t.Fatal("ExecuteAt(10, 1) = false, want true for the second lens on line 10")
	}
	if len(fired) != 1 {
		t.Fatalf("callback fired %d times, want 1", len(fired))
	}
	if fired[0].Command != "silk.runTest" || len(fired[0].Args) != 1 || fired[0].Args[0] != "TestFoo" {
		t.Errorf("callback got %+v, want the silk.runTest lens with its args", fired[0])
	}

	if !c.ExecuteAt(40, 0) {
		t.Error("ExecuteAt(40, 0) = false, want true for the only lens on line 40")
	}
	if len(fired) != 2 || fired[1].Command != "silk.showImplementations" {
		t.Errorf("second activation = %+v, want the silk.showImplementations lens", fired)
	}

	// Misses: index past the line's lenses, negative index, undecorated line.
	for _, c2 := range []struct{ line, idx int }{{10, 2}, {10, -1}, {11, 0}, {41, 0}} {
		if c.ExecuteAt(c2.line, c2.idx) {
			t.Errorf("ExecuteAt(%d, %d) = true, want false", c2.line, c2.idx)
		}
	}
	if len(fired) != 2 {
		t.Errorf("a missed hit-test fired the callback: %d activations, want 2", len(fired))
	}

	c.Clear()
	if len(c.Lenses()) != 0 || c.LensesOnLine(10) != nil {
		t.Error("Clear left lenses behind")
	}
	if c.ExecuteAt(10, 0) {
		t.Error("ExecuteAt after Clear = true, want false")
	}
}

// TestCodeLensNoCallback checks activation without a registered callback is
// inert rather than a nil-func panic, and that the model owns its lenses.
func TestCodeLensNoCallback(t *testing.T) {
	c := NewCodeLens()
	in := insightsLenses()
	c.SetLenses(in)

	if !c.ExecuteAt(40, 0) {
		t.Error("ExecuteAt(40, 0) = false, want true even without a callback")
	}

	in[2].Title = "MUTATED"
	if got := c.LensesOnLine(40); got[0].Title != "implementations" {
		t.Errorf("input mutation leaked: lens title = %q", got[0].Title)
	}
	out := c.Lenses()
	out[0].Title = "MUTATED"
	if got := c.Lenses(); got[0].Title != "3 references" {
		t.Errorf("Lenses() mutation leaked: lens title = %q", got[0].Title)
	}
}

// --- the aggregate layer ---

// insightsLayer builds a layer carrying all three kinds of decoration on line
// 2 at revision 0.
func insightsLayer(t *testing.T) *CodeInsightsLayer {
	t.Helper()
	l := NewCodeInsightsLayer(insightsLegend())
	if err := l.SemanticTokens().SetData("v1", insightsTokenData()); err != nil {
		t.Fatalf("SetData: %v", err)
	}
	if !l.InlayHints().Set(l.Revision(), []Hint{{Line: 2, Col: 8, Label: "int", Kind: HintKindType, PaddingLeft: true}}) {
		t.Fatal("Set hints at the layer revision was rejected")
	}
	l.CodeLens().SetLenses([]Lens{{Line: 2, Title: "2 references", Command: "silk.showReferences"}})
	return l
}

// TestCodeInsightsLayerLine checks the aggregate per-line query the overlay
// consumes, and the freshness split after an edit: hints go, tokens and lenses
// stay until fresh results land.
func TestCodeInsightsLayerLine(t *testing.T) {
	l := insightsLayer(t)

	li := l.Line(2)
	if li.Line != 2 || len(li.Tokens) != 2 || len(li.Hints) != 1 || len(li.Lenses) != 1 {
		t.Fatalf("Line(2) = %+v, want 2 tokens / 1 hint / 1 lens", li)
	}
	if li.Empty() {
		t.Error("Line(2).Empty() = true on a fully decorated line")
	}
	if !l.Line(5).Empty() {
		t.Errorf("Line(5) = %+v, want empty", l.Line(5))
	}

	// Same revision: a redundant notification changes nothing.
	l.SetRevision(0)
	if len(l.Line(2).Hints) != 1 {
		t.Error("SetRevision to the current revision dropped the hints")
	}

	// Buffer edited: the hint's column no longer describes the text.
	l.SetRevision(1)
	if l.Revision() != 1 {
		t.Errorf("Revision() = %d, want 1", l.Revision())
	}
	li = l.Line(2)
	if len(li.Hints) != 0 {
		t.Errorf("Line(2).Hints = %+v, want none after the revision advanced", li.Hints)
	}
	if len(li.Tokens) != 2 || len(li.Lenses) != 1 {
		t.Errorf("Line(2) lost tokens/lenses on an edit: %+v", li)
	}

	// Fresh hints for the new revision come back through.
	if !l.InlayHints().Set(1, []Hint{{Line: 2, Col: 9, Label: "int", Kind: HintKindType}}) {
		t.Fatal("Set hints at revision 1 was rejected")
	}
	if len(l.Line(2).Hints) != 1 {
		t.Error("hints for the current revision did not reach Line(2)")
	}
}

// TestCodeInsightsLayerLines checks the viewport query: it skips undecorated
// lines, clamps a negative start, and returns rows in line order.
func TestCodeInsightsLayerLines(t *testing.T) {
	l := insightsLayer(t)

	rows := l.Lines(0, 7)
	if len(rows) != 3 {
		t.Fatalf("Lines(0, 7) returned %d rows, want 3 (lines 2, 3, 6): %+v", len(rows), rows)
	}
	if rows[0].Line != 2 || rows[1].Line != 3 || rows[2].Line != 6 {
		t.Errorf("Lines(0, 7) lines = %d,%d,%d, want 2,3,6", rows[0].Line, rows[1].Line, rows[2].Line)
	}
	if rows = l.Lines(-5, 2); len(rows) != 1 || rows[0].Line != 2 {
		t.Errorf("Lines(-5, 2) = %+v, want just line 2", rows)
	}
	if rows = l.Lines(7, 20); rows != nil {
		t.Errorf("Lines(7, 20) = %+v, want nil past the last decoration", rows)
	}

	l.Clear()
	if rows = l.Lines(0, 100); rows != nil {
		t.Errorf("Lines after Clear = %+v, want nil", rows)
	}
	if l.Revision() != 0 {
		t.Errorf("Clear moved the revision to %d, want 0", l.Revision())
	}
	if len(l.SemanticTokens().Legend().Types) != 8 {
		t.Error("Clear dropped the semantic legend")
	}
}

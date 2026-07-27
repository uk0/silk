package ged

import (
	"reflect"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// sampleMergeSides is the fixture every test below shares: ours rewrites
// line B (nobody else touched it → auto-merged) and both sides rewrite
// line D differently (→ one conflict). core.Merge3 turns it into
// stable / ours / stable / conflict / stable.
func sampleMergeSides() (base, ours, theirs []string) {
	base = []string{"A", "B", "C", "D", "E"}
	ours = []string{"A", "X", "C", "DD", "E"}
	theirs = []string{"A", "B", "C", "DT", "E"}
	return
}

// sampleMergeModel is a model over the fixture, with nothing resolved yet.
func sampleMergeModel(t *testing.T) *MergeModel {
	t.Helper()
	base, ours, theirs := sampleMergeSides()
	m := NewMergeModel(core.Merge3(base, ours, theirs))
	if got := m.Count(); got != 5 {
		t.Fatalf("chunks = %d, want 5", got)
	}
	if got := m.ConflictCount(); got != 1 {
		t.Fatalf("conflicts = %d, want 1", got)
	}
	return m
}

// conflictIndex finds the single conflict chunk in a model.
func conflictIndex(t *testing.T, m *MergeModel) int {
	t.Helper()
	for i := 0; i < m.Count(); i++ {
		c, _ := m.Chunk(i)
		if c.Kind == core.MergeConflict {
			return i
		}
	}
	t.Fatal("no conflict chunk in the model")
	return -1
}

// TestMergeModelChunkKinds pins the fixture's chunk structure: the
// one-sided edit is auto-merged as an ours chunk, only the overlapping one
// is a conflict.
func TestMergeModelChunkKinds(t *testing.T) {
	m := sampleMergeModel(t)
	want := []core.MergeKind{
		core.MergeStable, core.MergeOurs, core.MergeStable, core.MergeConflict, core.MergeStable,
	}
	for i, wk := range want {
		c, ok := m.Chunk(i)
		if !ok {
			t.Fatalf("Chunk(%d) missing", i)
		}
		if c.Kind != wk {
			t.Errorf("chunk[%d].Kind = %v, want %v", i, c.Kind, wk)
		}
	}
	if _, ok := m.Chunk(99); ok {
		t.Error("Chunk(99) reported ok on an out-of-range index")
	}
}

// TestMergeModelUnresolvedCounterAndCanSave: the counter starts at the
// conflict count, drops to zero once answered, and comes back when the
// answer is cleared. CanSave tracks it.
func TestMergeModelUnresolvedCounterAndCanSave(t *testing.T) {
	m := sampleMergeModel(t)
	ci := conflictIndex(t, m)

	if got := m.UnresolvedCount(); got != 1 {
		t.Fatalf("unresolved = %d, want 1", got)
	}
	if m.CanSave() {
		t.Fatal("CanSave() = true with an unanswered conflict")
	}

	if !m.Resolve(ci, MergeChoiceTheirs) {
		t.Fatal("Resolve(theirs) returned false")
	}
	if got := m.UnresolvedCount(); got != 0 {
		t.Errorf("unresolved after Resolve = %d, want 0", got)
	}
	if !m.CanSave() {
		t.Error("CanSave() = false after every conflict was answered")
	}
	if got := m.Choice(ci); got != MergeChoiceTheirs {
		t.Errorf("Choice = %v, want theirs", got)
	}

	// Clearing the answer puts the conflict back in the unresolved count.
	if !m.Resolve(ci, MergeChoiceNone) {
		t.Fatal("Resolve(none) returned false")
	}
	if got := m.UnresolvedCount(); got != 1 || m.CanSave() {
		t.Errorf("unresolved = %d / CanSave = %v after clearing, want 1 / false", got, m.CanSave())
	}
}

// TestMergeModelEmptyCanSave: nothing to decide → savable.
func TestMergeModelEmptyCanSave(t *testing.T) {
	m := NewMergeModel(nil)
	if !m.CanSave() || m.UnresolvedCount() != 0 || m.Count() != 0 {
		t.Errorf("empty model: CanSave=%v unresolved=%d count=%d, want true/0/0",
			m.CanSave(), m.UnresolvedCount(), m.Count())
	}
	if got := m.Result(); got != "" {
		t.Errorf("Result() = %q, want empty", got)
	}
}

// TestMergeModelResolutionChoices checks each answer produces the expected
// merged text. The auto-merged ours edit (B → X) is present in all of them.
func TestMergeModelResolutionChoices(t *testing.T) {
	cases := []struct {
		name   string
		choice MergeChoice
		manual []string
		want   string
	}{
		{name: "ours", choice: MergeChoiceOurs, want: "A\nX\nC\nDD\nE"},
		{name: "theirs", choice: MergeChoiceTheirs, want: "A\nX\nC\nDT\nE"},
		{name: "both", choice: MergeChoiceBoth, want: "A\nX\nC\nDD\nDT\nE"},
		{name: "manual", choice: MergeChoiceManual, manual: []string{"DM1", "DM2"}, want: "A\nX\nC\nDM1\nDM2\nE"},
		{name: "manual empty", choice: MergeChoiceManual, want: "A\nX\nC\nE"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := sampleMergeModel(t)
			ci := conflictIndex(t, m)
			if c.choice == MergeChoiceManual {
				if !m.SetManual(ci, c.manual) {
					t.Fatal("SetManual returned false")
				}
			} else if !m.Resolve(ci, c.choice) {
				t.Fatalf("Resolve(%v) returned false", c.choice)
			}
			if !m.CanSave() {
				t.Error("CanSave() = false after answering the only conflict")
			}
			if got := m.Result(); got != c.want {
				t.Errorf("Result() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestMergeModelSetManualCopiesLines: the model keeps its own copy of the
// hand-written lines.
func TestMergeModelSetManualCopiesLines(t *testing.T) {
	m := sampleMergeModel(t)
	ci := conflictIndex(t, m)
	lines := []string{"DM"}
	m.SetManual(ci, lines)
	lines[0] = "MUTATED"
	if got := m.Result(); got != "A\nX\nC\nDM\nE" {
		t.Errorf("Result() = %q, want the manual lines unaffected by the caller's mutation", got)
	}
}

// TestMergeModelResolveRejects: only conflict chunks take an answer, and
// the manual answer must come through SetManual (it needs its lines).
func TestMergeModelResolveRejects(t *testing.T) {
	m := sampleMergeModel(t)
	ci := conflictIndex(t, m)

	if m.Resolve(0, MergeChoiceOurs) {
		t.Error("Resolve on a stable chunk returned true")
	}
	if m.Resolve(-1, MergeChoiceOurs) || m.Resolve(99, MergeChoiceOurs) {
		t.Error("Resolve on an out-of-range index returned true")
	}
	if m.SetManual(0, []string{"x"}) {
		t.Error("SetManual on a stable chunk returned true")
	}
	if m.Resolve(ci, MergeChoiceManual) {
		t.Error("Resolve(manual) returned true, want it rejected in favour of SetManual")
	}
	if got := m.Choice(ci); got != MergeChoiceNone {
		t.Errorf("Choice after the rejected calls = %v, want none", got)
	}
	if m.CanSave() {
		t.Error("CanSave() = true after only rejected resolutions")
	}
}

// TestMergeModelResultKeepsMarkersWhileUnresolved: an unanswered conflict
// is written back out with git markers, and that text parses back into one
// conflict — Result() is never a silently dropped hunk.
func TestMergeModelResultKeepsMarkersWhileUnresolved(t *testing.T) {
	m := sampleMergeModel(t)
	text := m.Result()
	want := strings.Join([]string{
		"A", "X", "C",
		"<<<<<<< ours", "DD", "||||||| base", "D", "=======", "DT", ">>>>>>> theirs",
		"E",
	}, "\n")
	if text != want {
		t.Fatalf("Result() = %q,\nwant %q", text, want)
	}

	back, err := core.ParseConflictMarkers(strings.Split(text, "\n"))
	if err != nil {
		t.Fatalf("re-parsing Result() failed: %v", err)
	}
	n := 0
	for _, c := range back {
		if c.Kind == core.MergeConflict {
			n++
		}
	}
	if n != 1 {
		t.Errorf("conflicts after re-parsing = %d, want 1 (%+v)", n, back)
	}
}

// TestMergeModelRows checks the flattened row list: one header per chunk,
// an unresolved conflict listing both candidate sides, and the resolved
// conflict collapsing to the chosen side.
func TestMergeModelRows(t *testing.T) {
	m := sampleMergeModel(t)
	rows := m.Rows()

	// 5 headers + A, X, C, (DD + DT), E = 11 rows.
	if len(rows) != 11 {
		t.Fatalf("rows = %d, want 11: %+v", len(rows), rows)
	}
	if !rows[0].Header || rows[0].Kind != core.MergeStable {
		t.Errorf("rows[0] = %+v, want a stable header", rows[0])
	}
	if rows[1].Header || rows[1].Text != "A" || rows[1].Side != MergeSideMerged {
		t.Errorf("rows[1] = %+v, want the merged content line \"A\"", rows[1])
	}

	// The conflict header, then its ours side, then its theirs side.
	ci := conflictIndex(t, m)
	hdr := -1
	for i, r := range rows {
		if r.Header && r.Chunk == ci {
			hdr = i
			break
		}
	}
	if hdr < 0 {
		t.Fatal("no header row for the conflict chunk")
	}
	if rows[hdr].Choice != MergeChoiceNone || !strings.Contains(rows[hdr].Text, "未解决") {
		t.Errorf("conflict header = %+v, want an unresolved caption", rows[hdr])
	}
	if rows[hdr+1].Text != "DD" || rows[hdr+1].Side != MergeSideOurs {
		t.Errorf("rows[%d] = %+v, want the ours side \"DD\"", hdr+1, rows[hdr+1])
	}
	if rows[hdr+2].Text != "DT" || rows[hdr+2].Side != MergeSideTheirs {
		t.Errorf("rows[%d] = %+v, want the theirs side \"DT\"", hdr+2, rows[hdr+2])
	}

	// Once answered the conflict shows only the chosen side.
	m.Resolve(ci, MergeChoiceTheirs)
	rows = m.Rows()
	if len(rows) != 10 {
		t.Fatalf("rows after resolving = %d, want 10: %+v", len(rows), rows)
	}
	for i, r := range rows {
		if r.Header && r.Chunk == ci {
			if r.Choice != MergeChoiceTheirs || !strings.Contains(r.Text, "theirs") {
				t.Errorf("resolved conflict header = %+v, want the theirs answer", r)
			}
			if rows[i+1].Text != "DT" || rows[i+1].Side != MergeSideMerged {
				t.Errorf("rows[%d] = %+v, want the merged line \"DT\"", i+1, rows[i+1])
			}
			break
		}
	}
}

// TestMergeModelSetChunksResetsAnswers: reloading the merge drops every
// previous resolution.
func TestMergeModelSetChunksResetsAnswers(t *testing.T) {
	m := sampleMergeModel(t)
	ci := conflictIndex(t, m)
	m.Resolve(ci, MergeChoiceOurs)

	base, ours, theirs := sampleMergeSides()
	m.SetChunks(core.Merge3(base, ours, theirs))
	if got := m.Choice(ci); got != MergeChoiceNone {
		t.Errorf("Choice after SetChunks = %v, want none", got)
	}
	if m.CanSave() {
		t.Error("CanSave() = true right after SetChunks re-introduced the conflict")
	}
}

// TestMergeEditorSaveRefusesUntilResolved is the load-bearing gate: the
// SigSave callback must not fire while a conflict is unanswered, and must
// hand over the fully merged text once it is.
func TestMergeEditorSaveRefusesUntilResolved(t *testing.T) {
	e := NewMergeEditor()
	e.SetMerge(sampleMergeSides())

	saved := 0
	var got string
	e.SigSave(func(text string) {
		saved++
		got = text
	})

	if e.UnresolvedCount() != 1 || e.CanSave() {
		t.Fatalf("unresolved = %d / CanSave = %v, want 1 / false", e.UnresolvedCount(), e.CanSave())
	}
	if e.Save() {
		t.Error("Save() reported success with an unanswered conflict")
	}
	if saved != 0 {
		t.Fatalf("SigSave fired %d times while a conflict remained", saved)
	}

	ci := conflictIndex(t, e.Model())
	if !e.Resolve(ci, MergeChoiceOurs) {
		t.Fatal("Resolve returned false")
	}
	if !e.CanSave() {
		t.Fatal("CanSave() = false after the only conflict was answered")
	}
	if !e.Save() {
		t.Fatal("Save() refused after every conflict was answered")
	}
	if saved != 1 {
		t.Errorf("SigSave fired %d times, want 1", saved)
	}
	if want := "A\nX\nC\nDD\nE"; got != want {
		t.Errorf("saved text = %q, want %q", got, want)
	}
}

// TestMergeEditorResolvedCallback: SigResolved reports every answer,
// including the manual one, and rejected calls stay silent.
func TestMergeEditorResolvedCallback(t *testing.T) {
	e := NewMergeEditor()
	e.SetMerge(sampleMergeSides())
	ci := conflictIndex(t, e.Model())

	type ev struct {
		index  int
		choice MergeChoice
	}
	var seen []ev
	e.SigResolved(func(index int, choice MergeChoice) {
		seen = append(seen, ev{index, choice})
	})

	e.Resolve(ci, MergeChoiceBoth)
	e.SetManual(ci, []string{"DM"})
	e.Resolve(0, MergeChoiceOurs) // stable chunk: rejected, no callback

	want := []ev{{ci, MergeChoiceBoth}, {ci, MergeChoiceManual}}
	if !reflect.DeepEqual(seen, want) {
		t.Errorf("SigResolved events = %+v, want %+v", seen, want)
	}
	if got := e.Result(); got != "A\nX\nC\nDM\nE" {
		t.Errorf("Result() = %q, want the manual answer", got)
	}
}

// TestMergeEditorSetConflictText loads a file that already carries git
// markers, resolves it and checks the markers are gone from the result.
func TestMergeEditorSetConflictText(t *testing.T) {
	text := strings.Join([]string{
		"head",
		"<<<<<<< HEAD",
		"mine",
		"=======",
		"yours",
		">>>>>>> feature",
		"tail",
	}, "\n")

	e := NewMergeEditor()
	if err := e.SetConflictText(text); err != nil {
		t.Fatalf("SetConflictText: %v", err)
	}
	if e.ConflictCount() != 1 || e.UnresolvedCount() != 1 {
		t.Fatalf("conflicts = %d / unresolved = %d, want 1 / 1", e.ConflictCount(), e.UnresolvedCount())
	}
	if e.CanSave() {
		t.Error("CanSave() = true on a freshly loaded conflicted file")
	}

	ci := conflictIndex(t, e.Model())
	if !e.Resolve(ci, MergeChoiceBoth) {
		t.Fatal("Resolve returned false")
	}
	want := "head\nmine\nyours\ntail"
	if got := e.Result(); got != want {
		t.Errorf("Result() = %q, want %q", got, want)
	}
}

// TestMergeEditorSetConflictTextEmpty: an empty document is an empty
// chunk list, not one blank line.
func TestMergeEditorSetConflictTextEmpty(t *testing.T) {
	e := NewMergeEditor()
	if err := e.SetConflictText(""); err != nil {
		t.Fatalf("SetConflictText(\"\"): %v", err)
	}
	if e.Model().Count() != 0 || len(e.Rows()) != 0 {
		t.Errorf("chunks = %d / rows = %d, want 0 / 0", e.Model().Count(), len(e.Rows()))
	}
}

// TestMergeEditorSetConflictTextMalformed: a malformed file still loads
// (so the user can fix it) and the parse error is reported.
func TestMergeEditorSetConflictTextMalformed(t *testing.T) {
	e := NewMergeEditor()
	err := e.SetConflictText("<<<<<<< HEAD\nmine\n=======\nyours")
	if err == nil {
		t.Fatal("expected an error for the unterminated conflict")
	}
	if e.ConflictCount() != 1 {
		t.Errorf("conflicts = %d, want the salvaged one", e.ConflictCount())
	}
}

// TestMergeEditorClickResolvesAndSaves drives the widget through its
// hit-test: a click on the "theirs" chip of the conflict header answers it,
// and a click on the Save chip in the status band then fires SigSave.
func TestMergeEditorClickResolvesAndSaves(t *testing.T) {
	e := NewMergeEditor()
	e.SetSize(400, 300)
	e.SetMerge(sampleMergeSides())

	var got string
	fired := false
	e.SigSave(func(text string) {
		fired = true
		got = text
	})

	// Find the conflict's header row and confirm the hit-test agrees before
	// clicking it.
	rows := e.Rows()
	hdr := -1
	for i, r := range rows {
		if r.Header && r.Kind == core.MergeConflict {
			hdr = i
			break
		}
	}
	if hdr < 0 {
		t.Fatal("no conflict header row")
	}
	y := mergeEditorHeaderH + float64(hdr)*e.rowHeight + 3
	if idx := e.rowAt(y); idx != hdr {
		t.Fatalf("rowAt(%v) = %d, want %d", y, idx, hdr)
	}

	// A click left of the chips must not resolve anything.
	e.OnLeftDown(mergeRowTextX, y)
	if e.UnresolvedCount() != 1 {
		t.Fatalf("a click on the caption resolved the conflict")
	}

	// Middle of the "theirs" chip.
	x0, x1 := mergeChoiceRect(400, 1)
	e.OnLeftDown((x0+x1)/2, y)
	if e.UnresolvedCount() != 0 {
		t.Fatalf("unresolved = %d after clicking the theirs chip, want 0", e.UnresolvedCount())
	}
	if c := e.Model().Choice(rows[hdr].Chunk); c != MergeChoiceTheirs {
		t.Fatalf("choice = %v, want theirs", c)
	}

	// Save chip in the status band.
	e.OnLeftDown(400-mergeHeaderPad-mergeSaveBtnW/2, mergeEditorHeaderH/2)
	if !fired {
		t.Fatal("SigSave did not fire on the Save chip")
	}
	if want := "A\nX\nC\nDT\nE"; got != want {
		t.Errorf("saved text = %q, want %q", got, want)
	}
}

// TestMergeEditorClickSaveRefuses: the Save chip is inert while a conflict
// is unanswered.
func TestMergeEditorClickSaveRefuses(t *testing.T) {
	e := NewMergeEditor()
	e.SetSize(400, 300)
	e.SetMerge(sampleMergeSides())
	fired := false
	e.SigSave(func(string) { fired = true })

	e.OnLeftDown(400-mergeHeaderPad-mergeSaveBtnW/2, mergeEditorHeaderH/2)
	if fired {
		t.Error("SigSave fired from the Save chip with an unanswered conflict")
	}
}

// TestMergeRowAtY exercises the pure row hit-test: rows start at
// topOffset, rowH tall; the band / past-the-end / degenerate cases give -1.
func TestMergeRowAtY(t *testing.T) {
	const (
		top = mergeEditorHeaderH
		rh  = 20.0
		n   = 3
	)
	cases := []struct {
		y    float64
		want int
	}{
		{y: 0, want: -1},
		{y: top - 1, want: -1},
		{y: top, want: 0},
		{y: top + rh - 0.5, want: 0},
		{y: top + rh, want: 1},
		{y: top + 2*rh + 5, want: 2},
		{y: top + 3*rh, want: -1},
		{y: top + 100*rh, want: -1},
	}
	for _, c := range cases {
		if got := mergeRowAtY(c.y, top, rh, n); got != c.want {
			t.Errorf("mergeRowAtY(%v) = %d, want %d", c.y, got, c.want)
		}
	}
	if got := mergeRowAtY(top+5, top, 0, n); got != -1 {
		t.Errorf("mergeRowAtY with rowH=0 = %d, want -1", got)
	}
}

// TestMergeChoiceAtX checks the answer chips are right-aligned and
// disjoint, and that the caption area hits none of them.
func TestMergeChoiceAtX(t *testing.T) {
	const w = 400.0
	for i, want := range mergeChoiceOrder {
		x0, x1 := mergeChoiceRect(w, i)
		if x1 <= x0 {
			t.Fatalf("chip %d has an empty span [%v,%v)", i, x0, x1)
		}
		for _, x := range []float64{x0, (x0 + x1) / 2, x1 - 0.5} {
			if got := mergeChoiceAtX(x, w); got != want {
				t.Errorf("mergeChoiceAtX(%v) = %v, want %v", x, got, want)
			}
		}
	}
	// The last chip ends at the right pad; the first starts well right of
	// the caption.
	if _, x1 := mergeChoiceRect(w, len(mergeChoiceOrder)-1); x1 != w-mergeHeaderPad {
		t.Errorf("last chip ends at %v, want %v", x1, w-mergeHeaderPad)
	}
	for _, x := range []float64{0, mergeRowTextX, 100, w - mergeHeaderPad + 1} {
		if got := mergeChoiceAtX(x, w); got != MergeChoiceNone {
			t.Errorf("mergeChoiceAtX(%v) = %v, want none", x, got)
		}
	}
}

// TestMergeSaveHit checks the Save chip's span in the status band.
func TestMergeSaveHit(t *testing.T) {
	const w = 400.0
	if !mergeSaveHit(w-mergeHeaderPad-mergeSaveBtnW/2, mergeEditorHeaderH/2, w) {
		t.Error("the middle of the Save chip missed")
	}
	if mergeSaveHit(mergeRowTextX, mergeEditorHeaderH/2, w) {
		t.Error("the caption area hit the Save chip")
	}
	if mergeSaveHit(w-mergeHeaderPad-mergeSaveBtnW/2, mergeEditorHeaderH+1, w) {
		t.Error("a row below the band hit the Save chip")
	}
	if mergeSaveHit(w-1, mergeEditorHeaderH/2, w) {
		t.Error("the right pad hit the Save chip")
	}
}

// TestMergeStatusLabel pins the three states of the status caption.
func TestMergeStatusLabel(t *testing.T) {
	if got := mergeStatusLabel(0, 0); !strings.Contains(got, "无冲突") {
		t.Errorf("mergeStatusLabel(0,0) = %q, want the no-conflict caption", got)
	}
	if got := mergeStatusLabel(3, 2); !strings.Contains(got, "3") || !strings.Contains(got, "2") {
		t.Errorf("mergeStatusLabel(3,2) = %q, want both counts", got)
	}
	if got := mergeStatusLabel(3, 0); !strings.Contains(got, "已全部解决") {
		t.Errorf("mergeStatusLabel(3,0) = %q, want the all-resolved caption", got)
	}
}

// TestMergeChoiceString pins the answer names used in captions and tests.
func TestMergeChoiceString(t *testing.T) {
	cases := []struct {
		choice MergeChoice
		want   string
	}{
		{MergeChoiceNone, "none"},
		{MergeChoiceOurs, "ours"},
		{MergeChoiceTheirs, "theirs"},
		{MergeChoiceBoth, "both"},
		{MergeChoiceManual, "manual"},
		{MergeChoice(99), "none"},
	}
	for _, c := range cases {
		if got := c.choice.String(); got != c.want {
			t.Errorf("MergeChoice(%d).String() = %q, want %q", int(c.choice), got, c.want)
		}
	}
}

// TestMergeEditorFactoryRegistered checks the panel is reachable through
// the object factory and listed as a tool view, the way silkide installs
// the docked panes.
func TestMergeEditorFactoryRegistered(t *testing.T) {
	obj := core.New("ged.MergeEditor")
	if obj == nil {
		t.Fatal("core.New(\"ged.MergeEditor\") = nil, factory not registered")
	}
	ed, ok := obj.(*MergeEditor)
	if !ok {
		t.Fatalf("factory produced %T, want *MergeEditor", obj)
	}
	// The factory route calls Init(self), so the model must already be
	// there — silkide docks the panel without ever calling NewMergeEditor.
	if ed.Model() == nil {
		t.Fatal("the factory-built editor has no model")
	}
	if ed.UnresolvedCount() != 0 || !ed.CanSave() {
		t.Errorf("fresh editor: unresolved = %d / CanSave = %v, want 0 / true",
			ed.UnresolvedCount(), ed.CanSave())
	}
	def, ok := gui.GetToolViewDef("ged.MergeEditor")
	if !ok {
		t.Fatal("ged.MergeEditor is not registered as a tool view")
	}
	if def.Name == "" {
		t.Error("the tool view has no name")
	}
}

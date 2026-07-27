package ged

import (
	"strconv"
	"strings"

	"github.com/uk0/silk/core"
)

// MergeChoice is how one conflict chunk got answered. Non-conflict chunks
// never carry a choice — they already know their final text.
type MergeChoice int

const (
	MergeChoiceNone   MergeChoice = iota // unresolved: the merge still owes an answer here
	MergeChoiceOurs                      // take our side
	MergeChoiceTheirs                    // take their side
	MergeChoiceBoth                      // keep both, ours first
	MergeChoiceManual                    // caller-supplied lines (see SetManual)
)

// String gives the stable name used in the chunk header caption and in
// tests.
func (c MergeChoice) String() string {
	switch c {
	case MergeChoiceOurs:
		return "ours"
	case MergeChoiceTheirs:
		return "theirs"
	case MergeChoiceBoth:
		return "both"
	case MergeChoiceManual:
		return "manual"
	}
	return "none"
}

// MergeSide tags which of the three sides a content row came from, so the
// renderer can tint it. Rows that are already part of the merged text
// (context, auto-merged edits and resolved conflicts) are MergeSideMerged;
// an unresolved conflict lists its two candidate sides instead.
type MergeSide int

const (
	MergeSideMerged MergeSide = iota
	MergeSideOurs
	MergeSideTheirs
)

// MergeRow is one display line of the conflict editor: either a chunk
// header (Header == true, Text is the caption) or one content line
// belonging to that chunk. Chunk indexes into the model's chunk list, so a
// click on a header row knows which conflict to resolve.
type MergeRow struct {
	Chunk  int
	Kind   core.MergeKind
	Header bool
	Choice MergeChoice // header rows only: the chunk's current answer
	Side   MergeSide
	Text   string
}

// MergeModel is the conflict editor's data layer: the chunk list produced
// by core.Merge3 (or core.ParseConflictMarkers) plus one resolution per
// conflict chunk. It is pure Go — no widget, no GL, no git — so the whole
// resolve/count/Result loop is unit-testable on its own, exactly like the
// panel/host split the sibling panes use.
//
// Resolutions live in slices parallel to the chunk list rather than inside
// core.MergeChunk: the chunks stay the immutable output of the merge
// engine, and re-running the merge simply replaces them (SetChunks resets
// every answer along with them).
type MergeModel struct {
	chunks  []core.MergeChunk
	choices []MergeChoice
	manual  [][]string
}

// NewMergeModel builds a model over chunks (nil is fine — an empty model).
func NewMergeModel(chunks []core.MergeChunk) *MergeModel {
	m := new(MergeModel)
	m.SetChunks(chunks)
	return m
}

// SetChunks replaces the chunk list and clears every resolution. The
// chunk slice is copied so a later host mutation cannot desync it from the
// parallel choice slices; the per-chunk line slices are shared with the
// caller and must be treated as read-only (core.Merge3 already hands out
// freshly allocated ones).
func (m *MergeModel) SetChunks(chunks []core.MergeChunk) {
	m.chunks = make([]core.MergeChunk, len(chunks))
	copy(m.chunks, chunks)
	m.choices = make([]MergeChoice, len(chunks))
	m.manual = make([][]string, len(chunks))
}

// Count returns the number of chunks.
func (m *MergeModel) Count() int {
	return len(m.chunks)
}

// Chunk returns chunk i and whether i was in range.
func (m *MergeModel) Chunk(i int) (core.MergeChunk, bool) {
	if i < 0 || i >= len(m.chunks) {
		return core.MergeChunk{}, false
	}
	return m.chunks[i], true
}

// Choice returns the resolution of chunk i (MergeChoiceNone for a
// non-conflict chunk, an unresolved conflict, or an out-of-range index).
func (m *MergeModel) Choice(i int) MergeChoice {
	if i < 0 || i >= len(m.choices) {
		return MergeChoiceNone
	}
	return m.choices[i]
}

// Resolve answers conflict chunk i with ours / theirs / both, or clears
// the answer again with MergeChoiceNone. It reports whether anything
// changed: an out-of-range index, a non-conflict chunk, and
// MergeChoiceManual (which needs its lines — use SetManual) are all
// rejected.
func (m *MergeModel) Resolve(i int, choice MergeChoice) bool {
	if i < 0 || i >= len(m.chunks) || m.chunks[i].Kind != core.MergeConflict {
		return false
	}
	switch choice {
	case MergeChoiceNone, MergeChoiceOurs, MergeChoiceTheirs, MergeChoiceBoth:
	default:
		return false
	}
	m.choices[i] = choice
	m.manual[i] = nil
	return true
}

// SetManual answers conflict chunk i with hand-written lines — the "edit
// it myself" resolution. A nil / empty slice is a legal answer: it means
// "take neither side". The lines are copied. Returns whether the index
// named a conflict chunk.
func (m *MergeModel) SetManual(i int, lines []string) bool {
	if i < 0 || i >= len(m.chunks) || m.chunks[i].Kind != core.MergeConflict {
		return false
	}
	out := make([]string, len(lines))
	copy(out, lines)
	m.choices[i] = MergeChoiceManual
	m.manual[i] = out
	return true
}

// ConflictCount is how many chunks need an answer at all.
func (m *MergeModel) ConflictCount() int {
	n := 0
	for _, c := range m.chunks {
		if c.Kind == core.MergeConflict {
			n++
		}
	}
	return n
}

// UnresolvedCount is how many conflicts are still unanswered — the number
// the editor's status band shows and the gate CanSave checks.
func (m *MergeModel) UnresolvedCount() int {
	n := 0
	for i, c := range m.chunks {
		if c.Kind == core.MergeConflict && m.choices[i] == MergeChoiceNone {
			n++
		}
	}
	return n
}

// CanSave reports whether the merged text is complete: every conflict has
// an answer. An empty model can save (there is nothing left to decide).
func (m *MergeModel) CanSave() bool {
	return m.UnresolvedCount() == 0
}

// ResultLines is the merged text as lines. Unresolved conflicts are
// written back out with git conflict markers so the result is always the
// full current state of the file, never a silently dropped hunk — saving
// it is what CanSave forbids.
func (m *MergeModel) ResultLines() []string {
	var out []string
	for i := range m.chunks {
		out = append(out, m.chunkLines(i)...)
	}
	return out
}

// Result is ResultLines joined with '\n' and no trailing newline.
func (m *MergeModel) Result() string {
	return strings.Join(m.ResultLines(), "\n")
}

// Rows flattens the chunk list into display rows: one header per chunk,
// then its lines. An unresolved conflict lists both candidate sides
// (tagged MergeSideOurs / MergeSideTheirs) so the user can compare them;
// every other chunk lists the lines it contributes to the merged text.
func (m *MergeModel) Rows() []MergeRow {
	var rows []MergeRow
	for i, c := range m.chunks {
		choice := m.choices[i]
		rows = append(rows, MergeRow{
			Chunk:  i,
			Kind:   c.Kind,
			Header: true,
			Choice: choice,
			Text:   mergeHeaderText(i, c, choice),
		})
		if c.Kind == core.MergeConflict && choice == MergeChoiceNone {
			for _, ln := range c.Ours {
				rows = append(rows, MergeRow{Chunk: i, Kind: c.Kind, Side: MergeSideOurs, Text: ln})
			}
			for _, ln := range c.Theirs {
				rows = append(rows, MergeRow{Chunk: i, Kind: c.Kind, Side: MergeSideTheirs, Text: ln})
			}
			continue
		}
		for _, ln := range m.chunkLines(i) {
			rows = append(rows, MergeRow{Chunk: i, Kind: c.Kind, Side: MergeSideMerged, Text: ln})
		}
	}
	return rows
}

// chunkLines returns the lines chunk i contributes to the merged text —
// the single place the resolution rules live, shared by ResultLines and
// Rows.
func (m *MergeModel) chunkLines(i int) []string {
	c := m.chunks[i]
	if c.Kind != core.MergeConflict {
		return c.Resolved()
	}
	switch m.choices[i] {
	case MergeChoiceOurs:
		return c.Ours
	case MergeChoiceTheirs:
		return c.Theirs
	case MergeChoiceBoth:
		out := make([]string, 0, len(c.Ours)+len(c.Theirs))
		out = append(out, c.Ours...)
		return append(out, c.Theirs...)
	case MergeChoiceManual:
		return m.manual[i]
	}
	return core.RenderConflictMarkers([]core.MergeChunk{c}, mergeMarkerLabels())
}

// mergeMarkerLabels are the labels used when an unresolved conflict has to
// be written back out as text. diff3 style (the base section is kept) so
// nothing the merge engine found is lost on the way out.
func mergeMarkerLabels() core.MergeLabels {
	return core.MergeLabels{Ours: "ours", Base: "base", Theirs: "theirs"}
}

// mergeHeaderText formats a chunk's header caption: a 1-based number, the
// chunk kind, the line counts that matter for that kind, and — for a
// conflict — either the answer or a "未解决" marker.
func mergeHeaderText(index int, c core.MergeChunk, choice MergeChoice) string {
	n := "#" + strconv.Itoa(index+1) + " "
	switch c.Kind {
	case core.MergeConflict:
		s := n + "冲突 / conflict (ours " + strconv.Itoa(len(c.Ours)) +
			" / theirs " + strconv.Itoa(len(c.Theirs)) + ")"
		if choice == MergeChoiceNone {
			return s + " — 未解决"
		}
		return s + " — " + choice.String()
	case core.MergeOurs:
		return n + "ours (" + strconv.Itoa(len(c.Ours)) + " 行)"
	case core.MergeTheirs:
		return n + "theirs (" + strconv.Itoa(len(c.Theirs)) + " 行)"
	}
	return n + "stable (" + strconv.Itoa(len(c.Ours)) + " 行)"
}

package gui

import (
	"cmp"
	"slices"
)

// Each edit contributes inserted bytes at its start, then subtracts one
// byte of removed prefix per step until its old range end. This is piecewise
// linear even when offsets are adjacent, duplicate or inside a removal.
type editorCaretOffsetEvent struct {
	offset, inserted, slopeDelta int
}

func shiftEditorCaretOffsets(offsets []int, edits []editorByteEdit) {
	// Avoid temporary sort/index storage for the usual small caret group.
	if len(offsets) <= 32 || len(edits) <= 24 {
		for i, off := range offsets {
			shifted := off
			for _, edit := range edits {
				if off < edit.offset {
					continue
				}
				shifted += len(edit.after) - min(off-edit.offset, len(edit.before))
			}
			offsets[i] = shifted
		}
		return
	}

	events := make([]editorCaretOffsetEvent, 0, 2*len(edits))
	for _, edit := range edits {
		if len(edit.before) == 0 {
			events = append(events, editorCaretOffsetEvent{offset: edit.offset, inserted: len(edit.after)})
		} else {
			events = append(events, editorCaretOffsetEvent{edit.offset, len(edit.after), -1},
				editorCaretOffsetEvent{offset: edit.offset + len(edit.before), slopeDelta: 1})
		}
	}
	slices.SortFunc(events, func(a, b editorCaretOffsetEvent) int { return cmp.Compare(a.offset, b.offset) })
	order := make([]int, len(offsets))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int { return cmp.Compare(offsets[a], offsets[b]) })

	last, slope, shift, next := 0, 0, 0, 0
	for _, i := range order {
		off := offsets[i]
		for next < len(events) && events[next].offset <= off {
			ev := events[next]
			shift += slope*(ev.offset-last) + ev.inserted
			last = ev.offset
			slope += ev.slopeDelta
			next++
		}
		offsets[i] = off + shift + slope*(off-last)
	}
}

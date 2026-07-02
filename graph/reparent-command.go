package graph

import (
	"fmt"
	"sort"
)

// reparentRecord captures one item moving from `from` to `to`. A nil
// `to` detaches the item (SetParent(nil)); a nil `from` means the item
// was unparented before the command (e.g. a freshly-created container),
// so undo detaches it again.
//
// fromIndex is the item's position among `from`'s children at the moment
// Add() recorded the move (i.e. before the command is applied). Undo uses
// it to put the item back in the exact slot it left, instead of appending
// it — which is what silently reordered siblings before this field existed.
// It is -1 when `from` is nil (nothing to restore a position into).
type reparentRecord struct {
	item      IItem
	from, to  IItem
	fromIndex int
}

// ReparentCommand is an undoable structural edit that moves a set of
// items between parents — the basis for the designer's "Lay Out" (wrap a
// selection in a new container) and "Break Layout" (dissolve a container)
// operations, both of which are pure reparenting once the container item
// exists.
//
// Redo replays the moves in insertion order, tail-appending each item into
// its target (the operations' natural behaviour). Undo restores every item
// to its original parent AT ITS ORIGINAL INDEX (reparentRecord.fromIndex),
// so a Redo followed by an Undo returns the tree to byte-identical sibling
// order.
//
// Undo does NOT simply walk the records in reverse: SetParentAt inserts at
// an absolute index, so items returning to the SAME parent must be
// reinserted in increasing fromIndex order (a lower slot must be filled
// before a higher one). Undo therefore stable-sorts the records by
// fromIndex before applying them. The relative order between DIFFERENT
// parents does not matter — inserting a child into a parent that has not
// been reattached yet is fine, because the child is carried along when that
// parent is itself restored — so a single global sort by fromIndex is
// enough and is robust to whatever order the caller recorded the moves in
// ("Lay Out" records in canvas-position order, not child order).
//
// Like AddCommand, the command is built in the pre-applied state and
// PushCommand's Push() calls Redo() to apply it.
type ReparentCommand struct {
	records []reparentRecord
	isUndo  bool
	label   string
}

// NewReparentCommand returns an empty command. label is shown by Text().
func NewReparentCommand(label string) *ReparentCommand {
	return &ReparentCommand{label: label}
}

// Add records that `item` moves from `from` to `to` when the command is
// applied. Pass to=nil to detach, from=nil if the item had no parent
// before the command.
//
// Add must be called before the command is applied (the pre-applied state,
// as PushCommand expects): it snapshots the item's current index via
// IndexInParent(), which at this point is its position inside `from`, so
// Undo can later restore that exact slot.
func (cmd *ReparentCommand) Add(item, from, to IItem) {
	cmd.records = append(cmd.records, reparentRecord{
		item:      item,
		from:      from,
		to:        to,
		fromIndex: item.IndexInParent(),
	})
}

// Count returns the number of reparent records.
func (cmd *ReparentCommand) Count() int { return len(cmd.records) }

func (cmd *ReparentCommand) Redo() {
	if cmd.isUndo {
		panic("illegal Redo()")
	}
	for i := 0; i < len(cmd.records); i++ {
		cmd.records[i].item.SetParent(cmd.records[i].to)
	}
	cmd.isUndo = true
}

func (cmd *ReparentCommand) Undo() {
	if !cmd.isUndo {
		panic("illegal Undo()")
	}
	// Restore in increasing original index so same-parent siblings land in
	// their exact slots (see the type doc). fromIndex is a plain int, so
	// this is a valid total order; the stable sort keeps ties (and thus the
	// relative order of records for different parents) as recorded.
	order := make([]int, len(cmd.records))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return cmd.records[order[a]].fromIndex < cmd.records[order[b]].fromIndex
	})
	for _, i := range order {
		r := &cmd.records[i]
		r.item.SetParentAt(r.from, r.fromIndex)
	}
	cmd.isUndo = false
}

func (cmd *ReparentCommand) Text() string {
	if cmd.label != "" {
		return cmd.label
	}
	return fmt.Sprintf("Reparent %d item(s)", len(cmd.records))
}

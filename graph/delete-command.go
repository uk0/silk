package graph

import (
	"fmt"
)

// deleteRecord captures one deleted item together with the parent it was
// removed from and the sibling index it occupied. index is the LIVE position
// inside parent at the moment Redo() detached this item — not necessarily the
// item's original absolute index, because an earlier removal in the same batch
// shifts every later sibling down by one. Undo re-inserts via
// SetParentAt(parent, index); paired with the reverse replay order in Undo(),
// each re-insert is the exact inverse of its detach, so the parent's child
// order (z-order) is restored exactly regardless of how many siblings go.
type deleteRecord struct {
	item   IItem
	parent IItem
	index  int
}

// DeleteCommand is the undoable mirror image of AddCommand: where AddCommand
// attaches items on Redo and detaches them on Undo, DeleteCommand detaches on
// Redo and re-attaches on Undo. It exists so the designer's "Delete" drops a
// selection without data loss — Ctrl+Z brings every removed widget back at its
// original parent and z-order slot instead of losing it.
//
// Add() records only the item; the parent and index are snapshotted in Redo()
// (the pre-applied → applied transition PushCommand's Push() drives), exactly
// like ged's zorderCommand snapshots oldIdx there. Capturing at detach time —
// not in Add() — is what makes the reverse-order restore in Undo() correct for
// a multi-item batch: detaching at live indices i0,i1,… and re-inserting at
// those same indices in reverse inverts the whole sequence (the undo of a;b;c
// is c⁻¹;b⁻¹;a⁻¹), so even deleting every child of a parent restores their
// order. Like AddCommand it is built in the pre-applied state and Push() calls
// Redo() to apply it; the isUndo guard panics on out-of-order calls.
type DeleteCommand struct {
	records []deleteRecord
	isUndo  bool
	label   string
}

// NewDeleteCommand returns an empty command. label is shown by Text().
func NewDeleteCommand(label string) *DeleteCommand {
	return &DeleteCommand{label: label}
}

// Add records that item is to be deleted. Call it before the command is
// applied (the pre-applied state PushCommand expects); the item's parent and
// index are captured later, in Redo(), against the live tree.
func (cmd *DeleteCommand) Add(item IItem) {
	cmd.records = append(cmd.records, deleteRecord{item: item})
}

// Count returns the number of items the command deletes.
func (cmd *DeleteCommand) Count() int { return len(cmd.records) }

func (cmd *DeleteCommand) Redo() {
	if cmd.isUndo {
		panic("illegal Redo()")
	}
	// Snapshot parent+index against the LIVE tree immediately before detaching
	// each item, so a later sibling's shifted position is what gets recorded
	// (that is exactly what the reverse-order Undo relies on — see the type
	// doc). Detaching mirrors AddCommand.Undo's SetParent(nil).
	for i := 0; i < len(cmd.records); i++ {
		r := &cmd.records[i]
		r.parent = r.item.Parent()
		r.index = r.item.IndexInParent()
		r.item.SetParent(nil)
	}
	cmd.isUndo = true
}

func (cmd *DeleteCommand) Undo() {
	if !cmd.isUndo {
		panic("illegal Undo()")
	}
	// Reverse order: re-inserting at the captured live indices back-to-front is
	// the exact inverse of the front-to-back detach in Redo, so every sibling —
	// including those from a fully emptied parent — lands back in its original
	// slot. SetParentAt(parent, index) restores position; plain SetParent would
	// tail-append and silently reorder.
	for i := len(cmd.records) - 1; i >= 0; i-- {
		r := &cmd.records[i]
		r.item.SetParentAt(r.parent, r.index)
	}
	cmd.isUndo = false
}

func (cmd *DeleteCommand) Text() string {
	if cmd.label != "" {
		return cmd.label
	}
	return fmt.Sprintf("Delete %d item(s)", len(cmd.records))
}

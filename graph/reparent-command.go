package graph

import "fmt"

// reparentRecord captures one item moving from `from` to `to`. A nil
// `to` detaches the item (SetParent(nil)); a nil `from` means the item
// was unparented before the command (e.g. a freshly-created container),
// so undo detaches it again.
type reparentRecord struct {
	item     IItem
	from, to IItem
}

// ReparentCommand is an undoable structural edit that moves a set of
// items between parents — the basis for the designer's "Lay Out" (wrap a
// selection in a new container) and "Break Layout" (dissolve a container)
// operations, both of which are pure reparenting once the container item
// exists.
//
// Records are applied in insertion order on Redo and reverse order on
// Undo, so a caller can order them to satisfy dependencies: e.g. attach a
// new container to the scene BEFORE reparenting children into it (Redo),
// which then unwinds correctly (children leave, then the container
// detaches) on Undo.
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
func (cmd *ReparentCommand) Add(item, from, to IItem) {
	cmd.records = append(cmd.records, reparentRecord{item: item, from: from, to: to})
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
	for i := len(cmd.records) - 1; i >= 0; i-- {
		cmd.records[i].item.SetParent(cmd.records[i].from)
	}
	cmd.isUndo = false
}

func (cmd *ReparentCommand) Text() string {
	if cmd.label != "" {
		return cmd.label
	}
	return fmt.Sprintf("Reparent %d item(s)", len(cmd.records))
}

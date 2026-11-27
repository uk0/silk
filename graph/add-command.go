package graph

import (
	//	"github.com/uk0/silk/core"
	"fmt"
)

type addRecord struct {
	item, parent IItem
	index        int
}

type AddCommand struct {
	records []addRecord
	isUndo  bool
}

func NewAddCommand() *AddCommand {
	return new(AddCommand)
}

func (cmd *AddCommand) AddItem(item, parent IItem) {
	cmd.AddItemAt(item, parent, -1)
}

// AddItemAt records an unparented item to insert into the given sibling slot
// when the command is applied. Indices are interpreted in application order,
// with -1 preserving the usual append. Redo restores the same slot.
func (cmd *AddCommand) AddItemAt(item, parent IItem, index int) {
	if item.Parent() != nil {
		panic("add: item already has a parent")
	}
	cmd.records = append(cmd.records, addRecord{item, parent, index})
}

func (cmd *AddCommand) Redo() {
	if cmd.isUndo {
		panic("irregal Redo()")
	}
	for i := 0; i < len(cmd.records); i++ {
		r := cmd.records[i]
		if r.index == -1 {
			r.item.SetParent(r.parent)
		} else {
			r.item.SetParentAt(r.parent, r.index)
		}
	}
	cmd.isUndo = true
}

func (cmd *AddCommand) Undo() {
	if !cmd.isUndo {
		panic("irregal Undo()")
	}
	for i := len(cmd.records) - 1; i >= 0; i-- {
		cmd.records[i].item.SetParent(nil)
	}
	cmd.isUndo = false
}

func (cmd *AddCommand) Text() string {
	if len(cmd.records) < 2 {
		return fmt.Sprintf("Add %d item", len(cmd.records))
	}

	return fmt.Sprintf("Add %d items", len(cmd.records))
}

func (cmd *AddCommand) Count() int {
	return len(cmd.records)
}

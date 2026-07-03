package graph

import (
	//	"github.com/uk0/silk/core"
	"fmt"
)

type addRecord struct {
	item, parent IItem
}

type AddCommand struct {
	records []addRecord
	isUndo  bool
}

func NewAddCommand() *AddCommand {
	return new(AddCommand)
}

func (cmd *AddCommand) AddItem(item, parent IItem) {
	if item.Parent() != nil {
		panic("item.Parent() != ni")
	}
	record := addRecord{item, parent}
	cmd.records = append(cmd.records, record)
}

func (cmd *AddCommand) Redo() {
	if cmd.isUndo {
		panic("irregal Redo()")
	}
	for i := 0; i < len(cmd.records); i++ {
		cmd.records[i].item.SetParent(cmd.records[i].parent)
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

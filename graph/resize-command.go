package graph

import (
	"fmt"
	"github.com/uk0/silk/geom"
)

type resizeRecord struct {
	item IItem
	rect geom.Rect
}

type ResizeCommand struct {
	records []resizeRecord
	isUndo  bool
}

func NewResizeCommand() *ResizeCommand {
	return new(ResizeCommand)
}

func (cmd *ResizeCommand) AddItem(item IItem, rect geom.Rect) {
	record := resizeRecord{item, rect}
	cmd.records = append(cmd.records, record)
}

func (cmd *ResizeCommand) Redo() {
	if cmd.isUndo {
		panic("irregal Redo()")
	}
	for i := 0; i < len(cmd.records); i++ {
		oldRect := cmd.records[i].item.Bounds1()
		cmd.records[i].item.SetBounds1(cmd.records[i].rect)
		cmd.records[i].rect = oldRect
	}
	cmd.isUndo = true
}

func (cmd *ResizeCommand) Undo() {
	if !cmd.isUndo {
		panic("irregal Undo()")
	}
	for i := len(cmd.records) - 1; i >= 0; i-- {
		oldRect := cmd.records[i].item.Bounds1()
		cmd.records[i].item.SetBounds1(cmd.records[i].rect)
		cmd.records[i].rect = oldRect
	}
	cmd.isUndo = false
}

func (cmd *ResizeCommand) Text() string {
	if len(cmd.records) < 2 {
		return fmt.Sprintf("Resize %d item", len(cmd.records))
	}

	return fmt.Sprintf("Resize %d items", len(cmd.records))
}

func (cmd *ResizeCommand) Count() int {
	return len(cmd.records)
}

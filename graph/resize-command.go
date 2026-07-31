package graph

import (
	"fmt"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/gui"
	"math"
)

// resizeRectBy grows rect by (dw, dh) anchored at its top-left corner, so X
// and Y never move. Both extents are floored at minSize because a keyboard
// resize repeats for as long as the key is held: without the floor, shrinking
// walks width or height through zero into a negative extent, which
// NormalizeCopy then flips inside out into a mirrored rectangle.
func resizeRectBy(rect geom.Rect, dw, dh, minSize float64) geom.Rect {
	return geom.Rect{
		X:      rect.X,
		Y:      rect.Y,
		Width:  math.Max(rect.Width+dw, minSize),
		Height: math.Max(rect.Height+dh, minSize),
	}
}

type resizeRecord struct {
	item IItem
	rect geom.Rect
}

type ResizeCommand struct {
	records    []resizeRecord
	isUndo     bool
	mergeToken uint64
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

// SetMergeToken tags cmd as belonging to one continuous gesture; see
// MoveCommand.SetMergeToken for the contract.
func (cmd *ResizeCommand) SetMergeToken(token uint64) {
	cmd.mergeToken = token
}

// MergeWidth folds next into cmd so a held Ctrl+arrow leaves one undo step
// instead of one per key repeat. Same contract and same reasoning as
// MoveCommand.MergeWidth — keep the two in step; only the record type differs.
func (cmd *ResizeCommand) MergeWidth(next gui.ICommand) bool {
	other, ok := next.(*ResizeCommand)
	if !ok || cmd.mergeToken == 0 || cmd.mergeToken != other.mergeToken {
		return false
	}
	if len(cmd.records) != len(other.records) {
		return false
	}
	for i := range cmd.records {
		if cmd.records[i].item != other.records[i].item {
			return false
		}
	}
	return true
}

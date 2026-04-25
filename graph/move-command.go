package graph

import (
	"fmt"
	"math"
)

type moveRecord struct {
	item IItem
	x, y float64
}

type MoveCommand struct {
	records []moveRecord
	isUndo  bool
}

func NewMoveCommand() *MoveCommand {
	return new(MoveCommand)
}

func (cmd *MoveCommand) AddItem(item IItem, toX, toY float64) {
	record := moveRecord{item, toX, toY}
	cmd.records = append(cmd.records, record)
}

func (cmd *MoveCommand) Redo() {
	if cmd.isUndo {
		panic("irregal Redo()")
	}
	for i := 0; i < len(cmd.records); i++ {
		oldX, oldY := cmd.records[i].item.Pos()
		// Snap to 1mm grid for precise alignment
		newX := math.Round(cmd.records[i].x)
		newY := math.Round(cmd.records[i].y)
		cmd.records[i].item.SetPos(newX, newY)
		cmd.records[i].x, cmd.records[i].y = oldX, oldY
	}
	cmd.isUndo = true
}

func (cmd *MoveCommand) Undo() {
	if !cmd.isUndo {
		panic("irregal Undo()")
	}
	for i := len(cmd.records) - 1; i >= 0; i-- {
		oldX, oldY := cmd.records[i].item.Pos()
		cmd.records[i].item.SetPos(cmd.records[i].x, cmd.records[i].y)
		cmd.records[i].x, cmd.records[i].y = oldX, oldY
	}
	cmd.isUndo = false
}

func (cmd *MoveCommand) Text() string {
	if len(cmd.records) < 2 {
		return fmt.Sprintf("Move %d item", len(cmd.records))
	}

	return fmt.Sprintf("Move %d items", len(cmd.records))
}

func (cmd *MoveCommand) Count() int {
	return len(cmd.records)
}

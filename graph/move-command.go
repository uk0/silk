package graph

import (
	"fmt"
	"math"

	"github.com/uk0/silk/gui"
)

type moveRecord struct {
	item IItem
	x, y float64
	// carried marks a record that was handed the delta of the record above it
	// — AddMoveSubtree walking into a container — instead of owning a target of
	// its own. Redo rounds a target to the grid, and two items that one gesture
	// moved together but rounded apart end up shifted relative to each other:
	// the widget slides inside the container it lives in, by up to 1mm, and
	// only in the saved design, because Undo restores the unrounded positions.
	//
	// The distinction has to sit on the record rather than on the command or at
	// the recording site, because one command holds both kinds: AlignSelection
	// puts N independently computed targets on a single MoveCommand and each of
	// them carries its own subtree. So each target still snaps on its own, and
	// what rides along rides along — the same rule snapSelectionToGrid applies
	// when it corrects a drag outside the undo stack.
	carried bool
}

type MoveCommand struct {
	records    []moveRecord
	isUndo     bool
	mergeToken uint64
}

func NewMoveCommand() *MoveCommand {
	return new(MoveCommand)
}

func (cmd *MoveCommand) AddItem(item IItem, toX, toY float64) {
	record := moveRecord{item, toX, toY, false}
	cmd.records = append(cmd.records, record)
}

// addCarried records an item that shares the preceding item's delta, so Redo
// gives it that item's snap instead of rounding it on its own. See moveRecord.
func (cmd *MoveCommand) addCarried(item IItem, toX, toY float64) {
	record := moveRecord{item, toX, toY, true}
	cmd.records = append(cmd.records, record)
}

func (cmd *MoveCommand) Redo() {
	if cmd.isUndo {
		panic("irregal Redo()")
	}
	var snapX, snapY float64
	for i := 0; i < len(cmd.records); i++ {
		oldX, oldY := cmd.records[i].item.Pos()
		newX, newY := cmd.records[i].x, cmd.records[i].y
		if cmd.records[i].carried {
			newX, newY = newX+snapX, newY+snapY
		} else {
			// Snap to 1mm grid for precise alignment
			newX, newY = math.Round(newX), math.Round(newY)
			snapX, snapY = newX-cmd.records[i].x, newY-cmd.records[i].y
		}
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

// SetMergeToken tags cmd as belonging to one continuous gesture, such as the
// burst of auto-repeat events a held arrow key produces. Commands sharing a
// non-zero token collapse into a single undo step; the zero default means
// "never merge", so a caller that does not opt in keeps one command per push.
func (cmd *MoveCommand) SetMergeToken(token uint64) {
	cmd.mergeToken = token
}

// MergeWidth folds next into cmd so a held arrow key leaves one undo step
// instead of one per key repeat. gui.UndoStack.Push probes the command below
// the stack top for this exact — misspelled — method name and, when it returns
// true, redoes next without growing the stack.
//
// cmd has already been redone, so its records hold the positions from before
// the gesture started, which is precisely what a later Undo must restore.
// Absorbing next is therefore a no-op on cmd's records. That only holds while
// cmd records every item next touches: an item moved by next but absent from
// cmd would keep no undo record at all, so such a list refuses the merge and
// lets the stack start a fresh step instead.
//
// The test is containment, not equality. A later step in the same gesture may
// legitimately drive fewer items — ResizeCommand's generator drops an item
// once it reaches the size floor — and cmd still holds that item's pre-gesture
// state, so the burst stays one undo step. Both lists are built by walking the
// selection in order, so one pass over cmd's records decides it.
func (cmd *MoveCommand) MergeWidth(next gui.ICommand) bool {
	other, ok := next.(*MoveCommand)
	if !ok || cmd.mergeToken == 0 || cmd.mergeToken != other.mergeToken {
		return false
	}
	i := 0
	for j := range other.records {
		for i < len(cmd.records) && cmd.records[i].item != other.records[j].item {
			i++
		}
		if i == len(cmd.records) {
			return false
		}
		i++
	}
	return true
}

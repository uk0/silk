package gui

import (
	"silk/paint"
	"strings"
	"unicode"
)

// RenameInFile renames all occurrences of oldName to newName in the given text.
// It only renames whole-word matches (not substrings), where word boundaries
// are defined by Go identifier rules.
func RenameInFile(text, oldName, newName string) string {
	if oldName == "" || oldName == newName {
		return text
	}
	oldRunes := []rune(oldName)
	newRuneStr := newName

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = renameInLine(line, oldRunes, newRuneStr)
	}
	return strings.Join(lines, "\n")
}

// renameInLine performs whole-word replacement within a single line.
func renameInLine(line string, oldRunes []rune, newName string) string {
	runes := []rune(line)
	oldLen := len(oldRunes)
	if oldLen == 0 || len(runes) < oldLen {
		return line
	}

	var result []rune
	i := 0
	for i < len(runes) {
		// Check if oldRunes matches starting at position i
		if i+oldLen <= len(runes) && runesEqual(runes[i:i+oldLen], oldRunes) {
			// Check left boundary: char before must not be an identifier char
			leftOK := i == 0 || !isIdentChar(runes[i-1])
			// Check right boundary: char after must not be an identifier char
			rightOK := i+oldLen >= len(runes) || !isIdentChar(runes[i+oldLen])

			if leftOK && rightOK {
				result = append(result, []rune(newName)...)
				i += oldLen
				continue
			}
		}
		result = append(result, runes[i])
		i++
	}
	return string(result)
}

// isIdentChar returns true if the rune is a valid Go identifier character.
func isIdentChar(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// runesEqual compares two rune slices for equality.
func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- Rename mode for CodeEditor ---

// activateRename enters rename mode, pre-filling the word at the cursor.
func (this *CodeEditor) activateRename() {
	word := this.wordAtCursor()
	if word == "" {
		return
	}
	this.renameActive = true
	this.renameWord = word
	this.renameText = word
	this.renameCursorPos = len([]rune(word))
	this.Self().Update()
}

// renameAccept performs the whole-word rename and pushes to undo stack.
func (this *CodeEditor) renameAccept() {
	if !this.renameActive {
		return
	}
	oldWord := this.renameWord
	newWord := this.renameText
	this.renameActive = false

	if oldWord == "" || newWord == "" || oldWord == newWord {
		this.renameText = ""
		this.renameWord = ""
		this.renameCursorPos = 0
		this.Self().Update()
		return
	}

	// Save full text for undo as kind 3 (full text replace)
	oldFullText := strings.Join(this.lines, "\n")
	newFullText := RenameInFile(oldFullText, oldWord, newWord)

	if oldFullText == newFullText {
		this.renameText = ""
		this.renameWord = ""
		this.renameCursorPos = 0
		this.Self().Update()
		return
	}

	this.pushUndo(editAction{
		kind:    3,
		line:    this.cursorLine,
		col:     this.cursorCol,
		text:    newFullText,
		oldText: oldFullText,
	})

	this.lines = strings.Split(newFullText, "\n")
	if len(this.lines) == 0 {
		this.lines = []string{""}
	}
	this.rebuildText()
	this.clampCursor()

	this.renameText = ""
	this.renameWord = ""
	this.renameCursorPos = 0
	this.ensureCursorVisible()
	this.Self().Update()
}

// renameCancel exits rename mode without making changes.
func (this *CodeEditor) renameCancel() {
	this.renameActive = false
	this.renameText = ""
	this.renameWord = ""
	this.renameCursorPos = 0
	this.Self().Update()
}

// drawRenameInput draws the inline rename text box at the cursor position.
func (this *CodeEditor) drawRenameInput(g paint.Painter, topOff, editorRight float64) {
	if !this.renameActive {
		return
	}

	fe := this.font.FontExtents()
	lh := fe.Height + 2

	// Calculate cursor screen position
	this.clampCursor()
	line := this.lines[this.cursorLine]
	lineRunes := []rune(line)
	prefixEnd := this.cursorCol
	if prefixEnd > len(lineRunes) {
		prefixEnd = len(lineRunes)
	}
	cx := this.gutterW + 10 - this.scrollX + this.measureText(string(lineRunes[:prefixEnd]))
	cy := float64(this.cursorLine)*lh - this.scrollY + topOff

	// Box dimensions
	inputW := 200.0
	inputH := lh + 8

	// Position the box near the cursor
	bx := cx - 4
	by := cy - 4
	if bx+inputW > editorRight {
		bx = editorRight - inputW - 4
	}
	if bx < this.gutterW+4 {
		bx = this.gutterW + 4
	}

	// Shadow
	g.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 60})
	g.Rectangle(bx+2, by+2, inputW, inputH)
	g.Fill()

	// Background
	g.SetBrush1(paint.Color{R: 45, G: 45, B: 55, A: 250})
	g.Rectangle(bx, by, inputW, inputH)
	g.Fill()

	// Border (highlight color)
	g.SetPen1(paint.Color{R: 80, G: 140, B: 220, A: 255}, 1.5)
	g.Rectangle(bx, by, inputW, inputH)
	g.Stroke()

	// Label
	g.SetFont(this.font)
	g.SetBrush1(paint.Color{R: 100, G: 140, B: 200, A: 200})
	g.DrawText1(bx+4, by-2, "Rename:")

	// Input text
	g.SetBrush1(paint.Color{R: 220, G: 220, B: 230, A: 255})
	g.DrawText1(bx+4, by+fe.Ascent+4, this.renameText)

	// Text cursor
	curPrefix := string([]rune(this.renameText)[:this.renameCursorPos])
	tcx := bx + 4 + this.measureText(curPrefix)
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 230, A: 255})
	g.Rectangle(tcx, by+3, 1, inputH-6)
	g.Fill()
}

package gui

// Undo and Redo expose the same edit history used by keyboard handling to
// menus and toolbars, without synthesizing keys or depending on held modifiers.
func (this *CodeEditor) Undo() { this.undo() }
func (this *CodeEditor) Redo() { this.redo() }

// MarkSaved establishes the buffer snapshot used for tab dirty indicators.
// Hosts call it only after loading or successfully writing the document.
func (this *CodeEditor) MarkSaved() {
	this.breakTypingUndoGroup()
	this.savedText = this.Text()
	this.modifiedRevision = this.textRevision
	this.modified = false
}

// Modified compares exact content once per text revision, then serves idle
// polling from the cache. An undo, replacement or manual edit restoring the
// saved bytes is clean even if its revision/history position is different.
func (this *CodeEditor) Modified() bool {
	text := this.Text()
	if this.modifiedRevision != this.textRevision {
		this.modified = text != this.savedText
		this.modifiedRevision = this.textRevision
	}
	return this.modified
}

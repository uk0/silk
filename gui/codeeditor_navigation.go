package gui

// SetNavigatePositionCallback installs a precise host navigation hook. Columns
// are zero-based runes, not UTF-16 units. Return false if the target cannot be
// opened. This takes precedence over the legacy line-only callback.
func (this *CodeEditor) SetNavigatePositionCallback(fn func(NavPosition) bool) {
	this.cbNavigatePosition = fn
}

// SetNavigationHistoryActions routes Back/Forward gestures to a host's shared
// document history. Passing nil restores standalone editor history.
func (this *CodeEditor) SetNavigationHistoryActions(back, forward func()) {
	this.cbNavBack, this.cbNavForward = back, forward
}

// NavigationPosition snapshots the live caret and vertical viewport on the UI thread.
func (this *CodeEditor) NavigationPosition() NavPosition {
	return NavPosition{FilePath: this.filePath, Line: this.cursorLine, Column: this.cursorCol, ScrollY: this.scrollY}
}

// RestoreNavigationPosition reveals a caret even inside a collapsed region and
// drops old selections/completion. The caller is responsible for file identity.
func (this *CodeEditor) RestoreNavigationPosition(pos NavPosition) {
	this.NavigateToLineCol(pos.Line, pos.Column)
	this.scrollY = max(0, pos.ScrollY)
	this.ensureCursorVisible()
	this.Self().Update()
}

func (this *CodeEditor) restoreNavPosition(pos NavPosition) bool {
	if pos.FilePath != "" && pos.FilePath != this.filePath {
		if this.cbNavigatePosition != nil {
			return this.cbNavigatePosition(pos)
		}
		if this.cbNavigate != nil {
			this.cbNavigate(pos.FilePath, pos.Line)
			return true
		}
		return false
	}
	this.RestoreNavigationPosition(pos)
	return true
}

func (this *CodeEditor) navigateDefinitionTarget(target *NavigationTarget) {
	from := this.NavigationPosition()
	to := NavPosition{FilePath: target.FilePath, Line: target.Line, Column: target.Column}
	if this.cbNavigatePosition != nil {
		if this.cbNavigatePosition(to) && this.cbNavBack == nil {
			this.navStack.RecordJump(from, to)
		}
		return
	}
	if target.FilePath == this.filePath || this.filePath == "" {
		this.NavigateToLineCol(to.Line, to.Column)
		this.navStack.RecordJump(from, this.NavigationPosition())
	} else if this.restoreNavPosition(to) {
		this.navStack.RecordJump(from, to)
	}
}

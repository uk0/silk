package gui

import "testing"

// TestToolDockViewIsNotTheCurrentDocument is the regression guard for a silent
// failure that reached users: pressing the mouse in the designer's widget
// palette made that palette the frame's active view, CurrentDocView handed it
// back as the current document, and every caller — Run, Preview, Save — type
// asserted it to its own document type, got nil, and returned without a word.
// Dragging a widget onto the canvas and then clicking Run did nothing at all.
//
// The palette is docked with a plain AddView, so it never enters toolViews and
// IsToolView cannot see it. What separates a panel from a document is the dock
// it lives in, which is what SuggestDocDock and SuggestToolDock already encode.
func TestToolDockViewIsNotTheCurrentDocument(t *testing.T) {
	f := NewFrame()
	doc := NewLabel("document")
	panel := NewLabel("palette")

	mainDock := NewDock()
	mainDock.isMainDock = true
	toolDock := NewDock() // NewDock is a tool dock until someone says otherwise

	if !f.isDocumentView(doc, mainDock) {
		t.Error("a view in the main dock is not being treated as a document")
	}
	if f.isDocumentView(panel, toolDock) {
		t.Error("a view in a tool dock is being treated as the current document; Run/Preview/Save will type assert it and silently give up")
	}
	if f.isDocumentView(nil, mainDock) {
		t.Error("an empty dock reported a current document")
	}
}

// TestRegisteredToolViewStillExcluded keeps the original rule intact: a panel
// that was registered as a tool view must stay excluded wherever it is docked.
func TestRegisteredToolViewStillExcluded(t *testing.T) {
	f := NewFrame()
	panel := NewLabel("registered panel")
	f.toolViews["panel"] = &_ToolViewInfo{Widget: panel}

	mainDock := NewDock()
	mainDock.isMainDock = true
	if f.isDocumentView(panel, mainDock) {
		t.Error("a registered tool view counted as a document because it happened to sit in the main dock")
	}
}

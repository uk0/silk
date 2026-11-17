package gui

// DismissInlinePopups closes drop-downs owned by an inline widget subtree.
// It snapshots owners before invoking host code, then revalidates ownership
// before every callback. Descendants that left the subtree and newly created
// owner widgets are not admitted by an older snapshot. Independent forms/popups are owner
// boundaries even before their native window has been created; WtChild is inline.
// UI-thread-only. Ordinary fields and small composite controls allocate nothing.
// Reentrant requests share a pass: each owner is invoked at most once, including
// owners visited by a nested request before the outer snapshot reaches them.
func DismissInlinePopups(root IWidget) {
	if root == nil {
		return
	}
	assertUIThread("DismissInlinePopups")
	if inlinePopupPassDepth == 0 {
		inlinePopupPassID++
		if inlinePopupPassID == 0 {
			inlinePopupPassID++
		}
	}
	inlinePopupPassDepth++
	defer func() { inlinePopupPassDepth-- }()
	pass := inlinePopupPassID
	var owners inlinePopupOwners
	collectInlinePopupOwners(root, &owners)
	for i := 0; i < owners.count; i++ {
		owner := owners.at(i)
		w := owner.NakedWidget()
		if w.popupDismissPass != pass && inlinePopupDescendant(root, owner) {
			w.popupDismissPass = pass
			owner.(interface{ HideSubPopup() }).HideSubPopup()
		}
	}
}

// No widget pointers are retained globally. Widget metadata makes reentrant
// admission O(1) without a per-operation map; all mutation is on the UI thread.
var inlinePopupPassDepth int
var inlinePopupPassID uint64

// Keep the common snapshot on the stack without exposing a slice into it to
// append's overflow path. Large custom composites spill once it is exhausted.
type inlinePopupOwners struct {
	small [16]IWidget
	spill []IWidget
	count int
}

func (s *inlinePopupOwners) add(w IWidget) {
	if s.count < len(s.small) {
		s.small[s.count] = w
	} else {
		s.spill = append(s.spill, w)
	}
	s.count++
}
func (s *inlinePopupOwners) at(i int) IWidget {
	if i < len(s.small) {
		return s.small[i]
	}
	return s.spill[i-len(s.small)]
}

func collectInlinePopupOwners(root IWidget, owners *inlinePopupOwners) {
	if _, ok := root.(interface{ HideSubPopup() }); ok {
		owners.add(root)
	}
	// Read-only traversal: no host callbacks run until the snapshot is complete.
	root.NakedWidget().eachChild(func(_ int, child IWidget) bool {
		w := child.NakedWidget()
		if w.wt != WtForm && w.wt != WtPopup {
			collectInlinePopupOwners(child, owners)
		}
		return true
	})
}

func inlinePopupDescendant(root, child IWidget) bool {
	for child != nil {
		if child == root {
			return true
		}
		w := child.NakedWidget()
		if w.wt == WtForm || w.wt == WtPopup {
			return false
		}
		child = w.parent
	}
	return false
}

package gui

import "testing"

type testContentOwner interface {
	IWidget
	SetContent(IWidget)
	Content() IWidget
	Layout()
}

func TestContentSlotFollowsReparenting(t *testing.T) {
	for _, tc := range []struct {
		name string
		new  func() testContentOwner
	}{
		{"card", func() testContentOwner { return NewCard("Drives") }},
		{"group", func() testContentOwner { return NewGroupBox("Drives") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := tc.new()
			old.SetSize(400, 240)
			newOwner := NewGroupBox("Transfer target")
			newOwner.SetSize(240, 160)
			original := NewLabel("Motor A")
			old.SetContent(original)
			newOwner.SetContent(original)
			newOwner.Layout()
			if original.Parent() != newOwner || newOwner.Content() != original {
				t.Fatal("content never reached the new owner")
			}
			if old.Content() != nil {
				t.Error("old owner's content slot still points across the widget tree")
			}
			target := original.Bounds1()
			old.SetSize(320, 200)
			old.Layout()
			if original.Bounds1() != target {
				t.Errorf("resizing the old owner mutated the new owner's child: got %v want %v", original.Bounds1(), target)
			}
			replacement := NewLabel("Motor B")
			old.SetContent(replacement)
			if original.Parent() != newOwner || newOwner.Content() != original || replacement.Parent() != old || old.Content() != replacement {
				t.Fatal("replacing the old slot detached a child from another container")
			}
			original.Detach()
			if newOwner.Content() != nil || original.Parent() != nil {
				t.Fatal("detaching the content left a stale slot")
			}
		})
	}
}

func TestContentSlotRejectsCyclesWithoutReplacingCurrentChild(t *testing.T) {
	for _, owner := range []testContentOwner{NewCard("Card"), NewGroupBox("Group")} {
		form := NewForm()
		owner.SetParent(form)
		child := NewLabel("Keep me")
		owner.SetContent(child)
		for _, invalid := range []IWidget{form, owner} {
			owner.SetContent(invalid)
			if child.Parent() != owner || owner.Content() != child || owner.Parent() != form || form.Parent() != nil {
				t.Fatalf("%T changed a valid tree when asked to create a cycle", owner)
			}
		}
	}
}

func TestContentSlotSameChildDoesNotChangeSiblingOrder(t *testing.T) {
	for _, owner := range []testContentOwner{NewCard("Card"), NewGroupBox("Group")} {
		child := NewLabel("Content")
		overlay := NewLabel("Overlay")
		owner.SetContent(child)
		overlay.SetParent(owner)
		owner.SetContent(child)
		kids := owner.Children()
		if len(kids) != 2 || kids[0] != child || kids[1] != overlay || owner.Content() != child {
			t.Fatalf("%T changed stacking order when setting the existing child", owner)
		}
	}
}

func TestContentSlotBoundsTrackContainerChrome(t *testing.T) {
	card := NewCard("")
	card.SetSize(360, 220)
	cardChild := NewLabel("Pump")
	card.SetContent(cardChild)
	before := cardChild.Bounds1()
	card.SetTitle("Controls")
	if got := cardChild.Bounds1(); got != card.ContentBounds() || got.Y <= before.Y || got.Height >= before.Height {
		t.Fatalf("adding a Card title did not reserve space for it: before=%v after=%v", before, got)
	}
	card.SetPadding(20)
	if got := cardChild.Bounds1(); got != card.ContentBounds() || got.X != 20 || got.Width != 320 {
		t.Fatalf("Card padding did not immediately reach the content: %v", got)
	}

	group := NewGroupBox("")
	group.SetSize(300, 180)
	value := NewLabel("Valve")
	group.SetContent(value)
	group.SetTitle("Controls")
	group.SetCheckable(true)
	if got := value.Bounds1(); got != group.ContentBounds() || got.Y <= 0 || got.Width >= group.Width() {
		t.Fatalf("GroupBox content does not start inside its title and border: %v", got)
	}
	card.SetSize(2, 2)
	group.SetSize(2, 2)
	if card.ContentBounds().Width != 0 || card.ContentBounds().Height != 0 ||
		group.ContentBounds().Width != 0 || group.ContentBounds().Height != 0 {
		t.Fatal("tiny containers produced negative client dimensions")
	}
}

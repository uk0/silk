package gui

import (
	"path/filepath"
	"testing"
)

func TestFormLayoutRootSDKSaveLoadAndDetach(t *testing.T) {
	form := NewForm()
	form.SetSize(420, 330)
	root, other := NewVBox(), NewVBox()
	root.SetParent(form)
	other.SetParent(form)
	other.SetBounds(100, 90, 55, 23)
	root.AddWidget(NewLabel("dashboard"))
	if !form.SetLayoutRoot(root) {
		t.Fatal("attached layout refused")
	}
	want := other.Bounds1()
	form.SetSize(600, 390)
	if other.Bounds1() != want || root.Width() != 600 || root.Height() != 390 {
		t.Fatal("client fill modified an unrelated sibling or missed resize")
	}
	filename := filepath.Join(t.TempDir(), "responsive.silkui")
	if err := SaveForm(form, filename); err != nil {
		t.Fatal(err)
	}
	reopened, err := LoadForm(filename)
	if err != nil {
		t.Fatal(err)
	}
	reopened.SetSize(500, 290)
	newRoot := reopened.LayoutRoot()
	if newRoot == nil || newRoot != reopened.Children()[0] || newRoot.Width() != 500 || newRoot.Height() != 290 {
		t.Fatal("SDK save/reload lost the layout policy")
	}
	foreign := NewForm()
	root.SetParent(foreign)
	if form.layoutRoot != nil || form.LayoutRoot() != nil || form.SetLayoutRoot(root) {
		t.Fatal("old owner retained or could claim a layout from another form")
	}
	if !foreign.SetLayoutRoot(root) {
		t.Fatal("valid new owner refused")
	}
	if !foreign.SetLayoutRoot(nil) || foreign.LayoutRoot() != nil || root.Parent() != foreign {
		t.Fatal("clearing the policy must not unparent the layout")
	}
}

package gui

import (
	"strconv"
	"testing"

	"github.com/uk0/silk/core"
)

// TestRecipePanelSetRecipesStoresAndCopies verifies SetRecipes keeps the names
// in order and defensively copies on both boundaries: mutating the caller's
// slice after SetRecipes, or the slice returned by Recipes(), must not disturb
// the panel's stored state.
func TestRecipePanelSetRecipesStoresAndCopies(t *testing.T) {
	in := []string{"Boot", "Purge", "Fill"}
	p := NewRecipePanel()
	p.SetRecipes(in)

	// Mutating the caller's slice must not reach the panel (input copied).
	in[0] = "MUTATED"
	got := p.Recipes()
	if len(got) != 3 {
		t.Fatalf("Recipes() len = %d, want 3", len(got))
	}
	if got[0] != "Boot" || got[1] != "Purge" || got[2] != "Fill" {
		t.Fatalf("order/copy wrong after input mutation: %v", got)
	}

	// Mutating the returned slice must not reach the panel (output copied).
	got[1] = "MUTATED2"
	again := p.Recipes()
	if again[1] != "Purge" {
		t.Fatalf("Recipes() did not return a copy: %v", again)
	}

	// A fresh SetRecipes clears any prior selection.
	p.selected = 2
	p.SetRecipes([]string{"X"})
	if p.Selected() != "" {
		t.Fatalf("SetRecipes did not reset selection: %q", p.Selected())
	}
}

// TestRecipePanelRowAtY checks the pure header/scroll-aware hit-test: the header
// band maps to -1, the first pixel below it to row 0, and a scroll offset shifts
// the mapping by whole rows.
func TestRecipePanelRowAtY(t *testing.T) {
	p := NewRecipePanel()
	rh := p.rowHeight

	if got := p.rowAtY(recipeHeaderH - 1); got != -1 {
		t.Errorf("rowAtY(header) = %d, want -1", got)
	}
	if got := p.rowAtY(recipeHeaderH + 1); got != 0 {
		t.Errorf("rowAtY(first row) = %d, want 0", got)
	}
	if got := p.rowAtY(recipeHeaderH + rh + 1); got != 1 {
		t.Errorf("rowAtY(second row) = %d, want 1", got)
	}

	p.scrollY = 2 * rh
	if got := p.rowAtY(recipeHeaderH + 1); got != 2 {
		t.Errorf("rowAtY(first row, scrolled 2) = %d, want 2", got)
	}
}

// TestRecipePanelButtonAtX pins the pure footer button hit-test: the row splits
// into four equal cells tiling [0,w), and x outside the panel (or a zero width)
// maps to -1.
func TestRecipePanelButtonAtX(t *testing.T) {
	const w = 400.0 // 4 cells of 100px each
	cases := []struct {
		x    float64
		want int
	}{
		{-1, -1},
		{0, recipeBtnApply},
		{50, recipeBtnApply},
		{150, recipeBtnCapture},
		{250, recipeBtnSave},
		{350, recipeBtnLoad},
		{399.9, recipeBtnLoad},
		{400, -1}, // x == w is outside
	}
	for _, c := range cases {
		if got := recipeButtonAtX(c.x, w); got != c.want {
			t.Errorf("recipeButtonAtX(%v, %v) = %d, want %d", c.x, w, got, c.want)
		}
	}
	if got := recipeButtonAtX(10, 0); got != -1 {
		t.Errorf("recipeButtonAtX with zero width = %d, want -1", got)
	}
}

// TestRecipePanelRowClickSelects confirms a click in the list body selects that
// row, while a click on the header or past the last row leaves the selection
// unchanged.
func TestRecipePanelRowClickSelects(t *testing.T) {
	p := NewRecipePanel()
	p.SetRecipes([]string{"A", "B", "C"})
	p.SetBounds(0, 0, 300, 200)
	rh := p.rowHeight

	if p.Selected() != "" {
		t.Fatalf("fresh panel Selected() = %q, want empty", p.Selected())
	}

	// Header click selects nothing.
	p.OnLeftDown(10, recipeHeaderH-2)
	if p.Selected() != "" {
		t.Fatalf("header click selected %q, want empty", p.Selected())
	}

	// Row 0, then row 2.
	p.OnLeftDown(10, recipeHeaderH+1)
	if p.Selected() != "A" {
		t.Fatalf("row-0 click Selected() = %q, want A", p.Selected())
	}
	p.OnLeftDown(10, recipeHeaderH+2*rh+1)
	if p.Selected() != "C" {
		t.Fatalf("row-2 click Selected() = %q, want C", p.Selected())
	}

	// A click in the list body past the last row is ignored (selection frozen).
	p.OnLeftDown(10, recipeHeaderH+6*rh)
	if p.Selected() != "C" {
		t.Fatalf("past-last-row click changed selection to %q, want frozen C", p.Selected())
	}
}

// TestRecipePanelApplyCaptureFireSig checks that clicking 应用(Apply) / 抓取(Capture)
// fires the matching Sig with the selected recipe name, and that neither fires
// when nothing is selected.
func TestRecipePanelApplyCaptureFireSig(t *testing.T) {
	p := NewRecipePanel()
	p.SetRecipes([]string{"A", "B", "C"})
	p.SetBounds(0, 0, 300, 200) // 4 footer cells of 75px; footer y >= 172

	const footerY = 190.0

	// No selection yet: Apply must not fire.
	applyFired := false
	p.SigApply(func(string) { applyFired = true })
	p.OnLeftDown(30, footerY) // Apply cell [0,75)
	if applyFired {
		t.Fatal("Apply fired with no selection")
	}

	// Select row 1 ("B"), then Apply carries that name.
	p.OnLeftDown(10, recipeHeaderH+p.rowHeight*1.5)
	if p.Selected() != "B" {
		t.Fatalf("setup: Selected() = %q, want B", p.Selected())
	}

	var applyName string
	applyFired = false
	p.SigApply(func(n string) { applyFired = true; applyName = n })
	p.OnLeftDown(30, footerY)
	if !applyFired || applyName != "B" {
		t.Fatalf("Apply fired=%v name=%q, want true B", applyFired, applyName)
	}

	// Capture carries the selected name too (cell 1 = [75,150)).
	var capName string
	capFired := false
	p.SigCapture(func(n string) { capFired = true; capName = n })
	p.OnLeftDown(100, footerY)
	if !capFired || capName != "B" {
		t.Fatalf("Capture fired=%v name=%q, want true B", capFired, capName)
	}
}

// TestRecipePanelSaveLoadFireSig checks that clicking 保存(Save) / 加载(Load) fires
// the matching parameterless Sig, with no selection required.
func TestRecipePanelSaveLoadFireSig(t *testing.T) {
	p := NewRecipePanel()
	p.SetRecipes([]string{"A", "B"})
	p.SetBounds(0, 0, 300, 200) // 4 footer cells of 75px

	const footerY = 190.0

	saveFired := false
	p.SigSave(func() { saveFired = true })
	p.OnLeftDown(180, footerY) // Save cell [150,225)
	if !saveFired {
		t.Fatal("Save click did not fire SigSave")
	}

	loadFired := false
	p.SigLoad(func() { loadFired = true })
	p.OnLeftDown(260, footerY) // Load cell [225,300)
	if !loadFired {
		t.Fatal("Load click did not fire SigLoad")
	}
}

// TestRecipePanelScrollClamp verifies the wheel scroll is pinned to
// [0, maxScroll], where maxScroll accounts for both the header and footer bands,
// and that shrinking the list re-clamps a stale offset.
func TestRecipePanelScrollClamp(t *testing.T) {
	p := NewRecipePanel()
	names := make([]string, 20)
	for i := range names {
		names[i] = "R" + strconv.Itoa(i)
	}
	p.SetRecipes(names)
	p.SetBounds(0, 0, 300, 120)
	rh := p.rowHeight

	wantMax := float64(len(names))*rh - (120 - recipeHeaderH - recipeButtonH)

	p.OnMouseWheel(0, 0, -100) // scroll far down
	if p.scrollY != wantMax {
		t.Fatalf("scrollY after down-wheel = %v, want clamp %v", p.scrollY, wantMax)
	}

	p.OnMouseWheel(0, 0, 100) // scroll far up
	if p.scrollY != 0 {
		t.Fatalf("scrollY after up-wheel = %v, want 0", p.scrollY)
	}

	// Scroll back down, then shrink the list: the offset must re-clamp to 0
	// because the short content fits without scrolling.
	p.OnMouseWheel(0, 0, -100)
	p.SetRecipes([]string{"only"})
	if p.scrollY != 0 {
		t.Fatalf("scrollY after shrink = %v, want 0", p.scrollY)
	}
}

// TestRecipePanelFactoryRegistered checks the factory id resolves to a
// constructible *RecipePanel so the designer can place it.
func TestRecipePanelFactoryRegistered(t *testing.T) {
	obj := core.New("gui.RecipePanel")
	if _, ok := obj.(*RecipePanel); !ok {
		t.Fatalf("factory gui.RecipePanel built %T, want *RecipePanel", obj)
	}
}

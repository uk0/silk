package gui

import (
	"reflect"
	"testing"
)

// TestPaginationItemsSmallShowsAll: when every page fits, no ellipsis
// appears — the token list is just 1..total.
func TestPaginationItemsSmallShowsAll(t *testing.T) {
	got := paginationItems(5, 3, 1, 1)
	want := []int{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paginationItems(5,3,1,1) = %v, want %v", got, want)
	}
}

// TestPaginationItemsLargeMiddle: a current page deep in a large set
// produces ellipses on both sides: 1 … 9 10 11 … 20.
func TestPaginationItemsLargeMiddle(t *testing.T) {
	got := paginationItems(20, 10, 1, 1)
	want := []int{1, pageEllipsis, 9, 10, 11, pageEllipsis, 20}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paginationItems(20,10,1,1) = %v, want %v", got, want)
	}
}

// TestPaginationItemsNearStart: current near the front only needs a
// trailing ellipsis — 1 2 3 4 5 … 20.
func TestPaginationItemsNearStart(t *testing.T) {
	got := paginationItems(20, 3, 1, 1)
	want := []int{1, 2, 3, 4, pageEllipsis, 20}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paginationItems(20,3,1,1) = %v, want %v", got, want)
	}
}

// TestPaginationItemsNearEnd: mirror of NearStart — leading ellipsis
// only. current 19 with sibling 1 clusters 18 19 20, so the row is
// 1 … 18 19 20 (page 17 is not in the show set).
func TestPaginationItemsNearEnd(t *testing.T) {
	got := paginationItems(20, 19, 1, 1)
	want := []int{1, pageEllipsis, 18, 19, 20}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paginationItems(20,19,1,1) = %v, want %v", got, want)
	}
}

// TestPaginationItemsFillsLoneGap: a gap of exactly one page must be
// filled with that page rather than rendered as a single-page
// ellipsis (otherwise "1 … 3" appears where "1 2 3" is shorter and
// clearer). With total 7, current 4, boundary 1, sibling 1 the show
// set is {1, 3,4,5, 7}; the 1→3 and 5→7 gaps are lone gaps and must
// fill to 2 and 6.
func TestPaginationItemsFillsLoneGap(t *testing.T) {
	got := paginationItems(7, 4, 1, 1)
	want := []int{1, 2, 3, 4, 5, 6, 7}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paginationItems(7,4,1,1) = %v, want %v (lone gaps should fill, not ellipsis)", got, want)
	}
}

// TestPaginationItemsZeroSibling: sibling 0 shows only the current
// page in the middle cluster: 1 … 10 … 20.
func TestPaginationItemsZeroSibling(t *testing.T) {
	got := paginationItems(20, 10, 0, 1)
	want := []int{1, pageEllipsis, 10, pageEllipsis, 20}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paginationItems(20,10,0,1) = %v, want %v", got, want)
	}
}

// TestPaginationItemsWiderBoundary: boundary 2 pins two pages at each
// end: 1 2 … 9 10 11 … 19 20.
func TestPaginationItemsWiderBoundary(t *testing.T) {
	got := paginationItems(20, 10, 1, 2)
	want := []int{1, 2, pageEllipsis, 9, 10, 11, pageEllipsis, 19, 20}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paginationItems(20,10,1,2) = %v, want %v", got, want)
	}
}

// TestPaginationItemsEmpty: zero/negative total yields no tokens.
func TestPaginationItemsEmpty(t *testing.T) {
	if got := paginationItems(0, 1, 1, 1); got != nil {
		t.Errorf("paginationItems(0,...) = %v, want nil", got)
	}
	if got := paginationItems(-3, 1, 1, 1); got != nil {
		t.Errorf("paginationItems(-3,...) = %v, want nil", got)
	}
}

// TestPaginationItemsClampsCurrent: an out-of-range current page is
// clamped before the range is built, so the window sits at the end
// rather than off it.
func TestPaginationItemsClampsCurrent(t *testing.T) {
	got := paginationItems(10, 99, 1, 1)
	want := []int{1, pageEllipsis, 9, 10}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paginationItems(10,99,1,1) = %v, want %v", got, want)
	}
}

// TestSetCurrentPageClampsAndFires: SetCurrentPage clamps to
// [1,total] and fires SigChange exactly once per real change.
func TestSetCurrentPageClampsAndFires(t *testing.T) {
	p := NewPagination()
	p.SetTotalPages(10)

	var fired []int
	p.SigChange(func(page int) { fired = append(fired, page) })

	p.SetCurrentPage(5)
	p.SetCurrentPage(5)  // no-op, must not refire
	p.SetCurrentPage(99) // clamp to 10
	p.SetCurrentPage(-3) // clamp to 1
	p.SetCurrentPage(1)  // already 1 after the -3 clamp → no-op

	want := []int{5, 10, 1}
	if !reflect.DeepEqual(fired, want) {
		t.Errorf("SigChange fired %v, want %v", fired, want)
	}
	if p.CurrentPage() != 1 {
		t.Errorf("final CurrentPage = %d, want 1", p.CurrentPage())
	}
}

// TestSetTotalPagesSnapsCurrent: shrinking the total below the active
// page pulls the active page back to the new last page.
func TestSetTotalPagesSnapsCurrent(t *testing.T) {
	p := NewPagination()
	p.SetTotalPages(20)
	p.SetCurrentPage(18)
	p.SetTotalPages(5)
	if p.CurrentPage() != 5 {
		t.Errorf("after shrink, CurrentPage = %d, want 5", p.CurrentPage())
	}
}

// TestPaginationCellsBracketArrows: the cell list always brackets the
// page tokens with a prev arrow first and a next arrow last, and the
// arrows target the adjacent pages.
func TestPaginationCellsBracketArrows(t *testing.T) {
	p := NewPagination()
	p.SetTotalPages(10)
	p.SetCurrentPage(5)
	cells := p.cells()
	if len(cells) < 3 {
		t.Fatalf("expected at least prev+page+next, got %d cells", len(cells))
	}
	if cells[0].kind != cellPrev || cells[0].page != 4 {
		t.Errorf("first cell = %+v, want prev→4", cells[0])
	}
	last := cells[len(cells)-1]
	if last.kind != cellNext || last.page != 6 {
		t.Errorf("last cell = %+v, want next→6", last)
	}
}

// TestPaginationHitTestMapsToCell: hitTest converts an x coordinate
// to the right cell index. Cell 0 (prev arrow) spans [0, cellW); cell
// 1 the next cellW, etc. Out-of-range x returns -1.
func TestPaginationHitTestMapsToCell(t *testing.T) {
	p := NewPagination()
	p.SetTotalPages(10)
	p.SetCurrentPage(5)
	n := len(p.cells())

	if idx := p.hitTest(paginationCellW * 0.5); idx != 0 {
		t.Errorf("hitTest(mid of cell 0) = %d, want 0", idx)
	}
	if idx := p.hitTest(paginationCellW * 1.5); idx != 1 {
		t.Errorf("hitTest(mid of cell 1) = %d, want 1", idx)
	}
	if idx := p.hitTest(-5); idx != -1 {
		t.Errorf("hitTest(negative) = %d, want -1", idx)
	}
	if idx := p.hitTest(float64(n)*paginationCellW + 5); idx != -1 {
		t.Errorf("hitTest(past end) = %d, want -1", idx)
	}
}

// TestPaginationClickPrevNext drives OnLeftDown on the arrow cells and
// confirms the page steps by one in each direction, firing SigChange.
func TestPaginationClickPrevNext(t *testing.T) {
	p := NewPagination()
	p.SetTotalPages(10)
	p.SetCurrentPage(5)

	var fired []int
	p.SigChange(func(page int) { fired = append(fired, page) })

	// Click the prev arrow (cell index 0).
	p.OnLeftDown(paginationCellW*0.5, 10)
	if p.CurrentPage() != 4 {
		t.Errorf("after prev click, page = %d, want 4", p.CurrentPage())
	}

	// Click the next arrow (last cell).
	cells := p.cells()
	nextX := float64(len(cells)-1)*paginationCellW + paginationCellW*0.5
	p.OnLeftDown(nextX, 10)
	if p.CurrentPage() != 5 {
		t.Errorf("after next click, page = %d, want 5", p.CurrentPage())
	}

	want := []int{4, 5}
	if !reflect.DeepEqual(fired, want) {
		t.Errorf("SigChange fired %v, want %v", fired, want)
	}
}

// TestPaginationClickEllipsisInert: clicking a "…" gap cell must not
// change the page — gaps are decorative, not navigable.
func TestPaginationClickEllipsisInert(t *testing.T) {
	p := NewPagination()
	p.SetTotalPages(20)
	p.SetCurrentPage(10) // produces 1 … 9 10 11 … 20

	cells := p.cells()
	gapIdx := -1
	for i, c := range cells {
		if c.kind == cellGap {
			gapIdx = i
			break
		}
	}
	if gapIdx < 0 {
		t.Fatal("expected an ellipsis cell in the 20-page/current-10 layout")
	}

	before := p.CurrentPage()
	fired := false
	p.SigChange(func(int) { fired = true })
	p.OnLeftDown(float64(gapIdx)*paginationCellW+paginationCellW*0.5, 10)
	if p.CurrentPage() != before {
		t.Errorf("ellipsis click changed page to %d, want unchanged %d", p.CurrentPage(), before)
	}
	if fired {
		t.Error("ellipsis click fired SigChange; gaps must be inert")
	}
}

// TestPaginationEmptyTotalNoClick: a control with 0 total pages
// ignores clicks and never fires SigChange.
func TestPaginationEmptyTotalNoClick(t *testing.T) {
	p := NewPagination()
	p.SetTotalPages(0)
	fired := false
	p.SigChange(func(int) { fired = true })
	p.OnLeftDown(paginationCellW*0.5, 10)
	if fired {
		t.Error("click on empty pager fired SigChange")
	}
}

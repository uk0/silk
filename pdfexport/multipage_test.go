package pdfexport

import (
	"strings"
	"testing"

	"silk/paint"
)

// TestNewPageProducesCorrectKidsAndCount verifies the Pages tree
// /Kids array references both pages and /Count reads 2.
func TestNewPageProducesCorrectKidsAndCount(t *testing.T) {
	p := New(200, 300)
	p.SetBrush1(paint.Color{R: 255, A: 255})
	p.Rectangle(0, 0, 100, 100)
	p.Fill()

	p.NewPage()
	p.SetBrush1(paint.Color{R: 0, G: 0, B: 255, A: 255})
	p.Rectangle(0, 0, 100, 100)
	p.Fill()

	out := p.String()
	// /Kids should list page 1 (object 3) and page 2 (object 5).
	if !strings.Contains(out, "/Kids [3 0 R 5 0 R ]") {
		t.Errorf("Pages tree should reference both pages; got:\n%s", out)
	}
	if !strings.Contains(out, "/Count 2") {
		t.Errorf("Pages tree /Count should be 2; got:\n%s", out)
	}
}

// TestPageCountReportsLivePageCount: PageCount is "finished + 1
// (current open)" so a freshly-constructed painter reports 1, after
// one NewPage reports 2, etc.
func TestPageCountReportsLivePageCount(t *testing.T) {
	p := New(100, 100)
	if got := p.PageCount(); got != 1 {
		t.Errorf("initial PageCount = %d, want 1", got)
	}
	p.NewPage()
	if got := p.PageCount(); got != 2 {
		t.Errorf("after 1 NewPage, PageCount = %d, want 2", got)
	}
	p.NewPage()
	p.NewPage()
	if got := p.PageCount(); got != 4 {
		t.Errorf("after 3 NewPage, PageCount = %d, want 4", got)
	}
}

// TestNewPage1AcceptsCustomSize: per-page size overrides — useful for
// title page + landscape body docs.
func TestNewPage1AcceptsCustomSize(t *testing.T) {
	p := New(595, 842) // A4 portrait
	p.SetBrush1(paint.Color{R: 0, A: 255})
	p.Rectangle(0, 0, 100, 100)
	p.Fill()

	p.NewPage1(842, 595) // A4 landscape
	p.SetBrush1(paint.Color{R: 255, A: 255})
	p.Rectangle(0, 0, 100, 100)
	p.Fill()

	out := p.String()
	if !strings.Contains(out, "/MediaBox [0 0 595 842]") {
		t.Errorf("page 1 MediaBox missing 595x842\n%s", out)
	}
	if !strings.Contains(out, "/MediaBox [0 0 842 595]") {
		t.Errorf("page 2 MediaBox missing 842x595\n%s", out)
	}
}

// TestEachPageHasOwnContentsObject: page 1 references contents object
// 4, page 2 references contents object 6 — the per-page content
// streams must be distinct.
func TestEachPageHasOwnContentsObject(t *testing.T) {
	p := New(100, 100)
	p.NewPage()

	out := p.String()
	if !strings.Contains(out, "/Contents 4 0 R") {
		t.Errorf("page 1 should reference Contents 4 0 R\n%s", out)
	}
	if !strings.Contains(out, "/Contents 6 0 R") {
		t.Errorf("page 2 should reference Contents 6 0 R\n%s", out)
	}
	// Both content streams must exist.
	if strings.Count(out, "endstream") < 2 {
		t.Errorf("expected ≥2 content streams; got %d\n%s",
			strings.Count(out, "endstream"), out)
	}
}

// TestNewPageResetsCTM: each page starts with identity CTM. Drawing
// at (0, 0) on page 2 lands at PDF (0, H) regardless of page-1
// translates.
func TestNewPageResetsCTM(t *testing.T) {
	p := New(100, 100)
	p.Translate(50, 50)
	p.NewPage()

	// CTM must be identity again. Drawing rect at user (0, 0, 50, 50)
	// → PDF coords with no translation: re "0 50 50 50 re" (Y-flipped
	// y_pdf = H - y - h = 100 - 0 - 50 = 50).
	p.SetBrush1(paint.Color{R: 0, A: 255})
	p.Rectangle(0, 0, 50, 50)
	p.Fill()

	out := p.String()
	if !strings.Contains(out, "0 50 50 50 re") {
		t.Errorf("page 2 should start with identity CTM; got:\n%s", out)
	}
}

// TestImagesAcrossPagesEmitDistinctObjects: the image XObject pool is
// document-scoped but DrawPixmap does NOT yet deduplicate by pixmap
// identity, so the same pixmap drawn on both pages produces two image
// objects (Im1, Im2). Each page references the matching XObject in
// its own /Resources dict. Dedup is a documented follow-up.
func TestImagesAcrossPagesEmitDistinctObjects(t *testing.T) {
	pix := makeTestPixmap(t)
	p := New(200, 200)
	p.DrawPixmap1(0, 0, pix)
	p.NewPage()
	p.DrawPixmap1(0, 0, pix)

	out := p.String()
	// Two image XObjects total.
	if got := strings.Count(out, "/Type /XObject /Subtype /Image"); got != 2 {
		t.Errorf("expected 2 image XObjects (no dedupe yet), got %d\n%s",
			got, out)
	}
	// Each page references one image: page 1 → Im1, page 2 → Im2.
	if !strings.Contains(out, "/Im1 Do") || !strings.Contains(out, "/Im2 Do") {
		t.Errorf("expected /Im1 Do (page 1) and /Im2 Do (page 2):\n%s", out)
	}
	// Both pages must list both image XObjects in /Resources /XObject
	// (because pages share the document-wide images dict). Verify by
	// counting /Im1 mentions in the page resource dicts.
	if got := strings.Count(out, "/Im1 "); got < 2 {
		t.Errorf("/Im1 should appear in resources for both pages; got %d\n%s",
			got, out)
	}
}

// TestXrefAccurateAfterNewPage: adding a second page changes the
// total object count (5 → 7); xref must list 7+1=8 entries with
// correct byte offsets.
func TestXrefAccurateAfterNewPage(t *testing.T) {
	p := New(100, 100)
	p.NewPage()
	out := p.String()

	// "0 8" header — 7 objects + slot 0.
	if !strings.Contains(out, "\n0 8\n") {
		t.Errorf("xref count line should be '0 8'\n%s", out)
	}
	if !strings.Contains(out, "/Size 8 /Root 1 0 R") {
		t.Errorf("trailer /Size should be 8\n%s", out)
	}

	// Verify every offset points at the right "N 0 obj" header.
	xrefIdx := strings.Index(out, "xref\n")
	cur := xrefIdx + len("xref\n")
	nl := strings.IndexByte(out[cur:], '\n')
	cur += nl + 1
	cur += 20 // free-list

	for i := 1; i <= 7; i++ {
		entry := out[cur : cur+20]
		cur += 20
		off := 0
		for j := 0; j < 10; j++ {
			off = off*10 + int(entry[j]-'0')
		}
		want := []byte{'0' + byte(i), ' ', '0', ' ', 'o', 'b', 'j'}
		if off+len(want) > len(out) || string(out[off:off+len(want)]) != string(want) {
			t.Errorf("xref entry %d offset %d does not point at %q (saw %q)",
				i, off, string(want), out[off:min(off+len(want), len(out))])
		}
	}
}

// TestSinglePageStillWorks: a painter that never calls NewPage should
// produce the same single-page document as before — Catalog at 1,
// Pages at 2, Page at 3, Contents at 4, Font at 5. Locks in
// backwards compat for callers that haven't migrated.
func TestSinglePageStillWorks(t *testing.T) {
	p := New(100, 100)
	p.SetBrush1(paint.Color{R: 0, A: 255})
	p.Rectangle(0, 0, 50, 50)
	p.Fill()

	out := p.String()
	for _, want := range []string{
		"/Kids [3 0 R ]",
		"/Count 1",
		"3 0 obj\n",       // Page 1
		"4 0 obj\n",       // Contents 1
		"/Contents 4 0 R", // Page references Contents 4
		"5 0 obj\n",       // Font at id 5 (preserves old layout)
		"/BaseFont /Helvetica",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n----\n%s", want, out)
		}
	}
}

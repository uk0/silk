package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"silk/ged"
)

// TestSampleMainGoLooksLikeReference locks in the sample seed code so
// reviewers can compare it against the mockup image side-by-side. If
// the mock changes, this test will guide what to update in
// sampleMainGo and the screenshot.
func TestSampleMainGoLooksLikeReference(t *testing.T) {
	src := sampleMainGo()
	for _, want := range []string{
		"package main",
		`"fmt"`,
		`"net/http"`,
		"func main()",
		"http.HandleFunc",
		`"Server starting on :8080"`,
		"func handler",
		`"Hello, gogpu!"`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("sampleMainGo missing %q\n----\n%s", want, src)
		}
	}
}

// TestSampleServerGoLooksLikeReference: server.go tab content matches
// the second tab shown in the mockup.
func TestSampleServerGoLooksLikeReference(t *testing.T) {
	src := sampleServerGo()
	for _, want := range []string{
		"package server",
		"type Server struct",
		"func New() *Server",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("sampleServerGo missing %q\n----\n%s", want, src)
		}
	}
}

// TestSampleGoModLooksLikeReference: go.mod tab content shape.
func TestSampleGoModLooksLikeReference(t *testing.T) {
	src := sampleGoMod()
	for _, want := range []string{
		"module github.com/user/myproject",
		"go 1.25",
		"require",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("sampleGoMod missing %q\n----\n%s", want, src)
		}
	}
}

// TestIDTitleFormat: title should be "<basename> — silkide" so the
// window chrome matches the mockup's "<project> — main.go" pattern.
func TestIDTitleFormat(t *testing.T) {
	got := idTitle()
	if !strings.Contains(got, " — silkide") {
		t.Errorf("idTitle() = %q; should contain ' — silkide' separator", got)
	}
}

// TestExportDesignCanvasSVG drives the export wiring without the
// SaveFileDialog: build a fresh GedView, hand exportDesignCanvas a
// .svg path inside t.TempDir, and confirm the file ends up with the
// SVG XML preamble. Locks in the toolbar's "preview" action contract
// against accidental dispatch regressions.
func TestExportDesignCanvasSVG(t *testing.T) {
	view := ged.NewGedView()
	tmp := filepath.Join(t.TempDir(), "scene.svg")
	if err := exportDesignCanvas(tmp, view); err != nil {
		t.Fatalf("exportDesignCanvas: %v", err)
	}
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "<?xml") {
		t.Fatalf("svg output missing XML preamble: %q", string(data[:min(80, len(data))]))
	}
	if !strings.Contains(string(data), "<svg") {
		t.Errorf("svg output missing <svg> root")
	}
}

// TestExportDesignCanvasPDF: same shape, .pdf path → PDFPainter →
// %PDF header.
func TestExportDesignCanvasPDF(t *testing.T) {
	view := ged.NewGedView()
	tmp := filepath.Join(t.TempDir(), "scene.pdf")
	if err := exportDesignCanvas(tmp, view); err != nil {
		t.Fatalf("exportDesignCanvas: %v", err)
	}
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Fatalf("pdf output missing %%PDF- header: %q", string(data[:min(80, len(data))]))
	}
}

// TestExportDesignCanvasUnknownExtension: paths without a recognised
// extension default to SVG and the output filename gets ".svg"
// appended so the saved file is recognisable.
func TestExportDesignCanvasUnknownExtension(t *testing.T) {
	view := ged.NewGedView()
	base := filepath.Join(t.TempDir(), "scene")
	if err := exportDesignCanvas(base, view); err != nil {
		t.Fatalf("exportDesignCanvas: %v", err)
	}
	want := base + ".svg"
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected %q to exist: %v", want, err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

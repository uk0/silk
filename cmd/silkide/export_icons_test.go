package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

func TestExportDesignCanvasPreservesButtonIcon(t *testing.T) {
	v := ged.NewGedView()
	s := v.GedScene()
	s.SetSize(260, 140)
	row, err := ged.NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}
	row.SetWidgetName("saveButton")
	row.SetBounds(12, 16, 110, 35)
	b, ok := row.Widget().(*gui.Button)
	if !ok {
		t.Fatalf("button factory produced %T", row.Widget())
	}
	b.SetText("Save")
	b.SetIconName("save")
	b.SetTextVisible(true)
	b.SetEnabled(false)
	c := graph.NewAddCommand()
	c.AddItem(row, s)
	s.PushCommand(c)

	for _, tc := range []struct{ suffix, resource string }{
		{".svg", "href=\"data:image/png;base64,"},
		{".pdf", "/Subtype /Image"},
	} {
		t.Run(tc.suffix, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "scene"+tc.suffix)
			if err := exportDesignCanvas(path, v); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(data, []byte(tc.resource)) {
				t.Fatalf("%s export omitted the disabled Save icon", tc.suffix)
			}
			x, y, width, height := row.Bounds()
			_, _, pageW, pageH := s.Bounds()
			want := []float64{x, y, width, height}
			const number = `([-+0-9.eE]+)`
			pagePattern, pageWant := `<svg [^>]*width="`+number+`mm" height="`+number+`mm"`, []float64{pageW, pageH}
			if tc.suffix == ".pdf" {
				pagePattern = `/MediaBox \[0 0 ` + number + ` ` + number + `\]`
				pageWant = []float64{pageW * 72 / 25.4, pageH * 72 / 25.4}
			}
			physical := regexp.MustCompile(pagePattern).FindSubmatch(data)
			if len(physical) != 3 {
				t.Fatalf("%s export lost its physical page size", tc.suffix)
			}
			for i, v := range physical[1:] {
				n, err := strconv.ParseFloat(string(v), 64)
				if err != nil || math.Abs(n-pageWant[i]) > 1e-8 {
					t.Fatalf("%s physical page coordinate %d = %s, want %g", tc.suffix, i, v, pageWant[i])
				}
			}
			pattern := `<image x="` + number + `" y="` + number + `" width="` + number + `" height="` + number + `"`
			if tc.suffix == ".pdf" {
				pattern = `(?m)^` + number + ` 0 0 ` + number + ` ` + number + ` ` + number + ` cm
/Im1 Do`
				want = []float64{width, height, x, pageH - y - height}
			}
			found := regexp.MustCompile(pattern).FindSubmatch(data)
			if len(found) != len(want)+1 {
				t.Fatalf("%s has no axis-aligned image placement", tc.suffix)
			}
			for i, v := range found[1:] {
				n, err := strconv.ParseFloat(string(v), 64)
				if err != nil || math.Abs(n-want[i]) > 1e-9 {
					t.Fatalf("%s image placement coordinate %d = %s, want %g", tc.suffix, i, v, want[i])
				}
			}
		})
	}
}

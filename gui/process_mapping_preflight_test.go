package gui

import (
	"net/http"
	"strings"
	"testing"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/point"
)

func TestProcessMappingEditorRejectsInvalidPointerWithoutChangingProject(t *testing.T) {
	e := NewProcessEditor()
	p := hmi.Project{Version: hmi.Version, Width: 600, Height: 400,
		Points: []hmi.Point{{Definition: point.Definition{Name: "AI11", Kind: point.Float}}},
		Sources: []hmi.Source{{ID: "PLC", URL: "http://localhost/acquisition", Method: http.MethodGet,
			IntervalMS: 1000, TimeoutMS: 500, Mappings: []point.Mapping{{Name: "AI11", Kind: point.Float, Pointer: "/data/raw"}}}},
	}
	if err := e.Diagram.SetProject(p); err != nil {
		t.Fatal(err)
	}
	e.Diagram.ClearHistory()
	changed := 0
	e.Diagram.SigChanged(func() { changed++ })
	if err := e.ApplyMappingRows([][]string{{"PLC", "AI11", "/data/raw~7"}}); err == nil || !strings.Contains(err.Error(), "PLC mapping AI11") {
		t.Fatalf("editor diagnostic: %v", err)
	}
	if got := e.Diagram.Project().Sources[0].Mappings[0].Pointer; got != "/data/raw" || changed != 0 || len(e.Diagram.undo) != 0 {
		t.Fatalf("invalid edit mutated document: path=%q changed=%d undo=%d", got, changed, len(e.Diagram.undo))
	}
	if err := e.ApplyMappingRows([][]string{{"PLC", "AI11", "/data/raw~1value"}}); err != nil {
		t.Fatal(err)
	}
	if got := e.Diagram.Project().Sources[0].Mappings[0].Pointer; got != "/data/raw~1value" || changed != 1 || len(e.Diagram.undo) != 1 {
		t.Fatalf("valid edit missing: path=%q changed=%d undo=%d", got, changed, len(e.Diagram.undo))
	}
	e.Diagram.Undo()
	if got := e.Diagram.Project().Sources[0].Mappings[0].Pointer; got != "/data/raw" {
		t.Fatalf("undo: %q", got)
	}
}

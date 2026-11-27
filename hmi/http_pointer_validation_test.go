package hmi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/point"
)

func TestProjectRejectsMalformedHTTPResponsePointerBeforeRuntime(t *testing.T) {
	p := hmi.Project{
		Version: hmi.Version, Width: 600, Height: 400,
		Points: []hmi.Point{{Definition: point.Definition{Name: "AI11", Kind: point.Float}}},
		Sources: []hmi.Source{{ID: "PLC", URL: "http://localhost/acquisition", Method: http.MethodPost,
			IntervalMS: 1000, TimeoutMS: 1500, Enabled: true, RepeatWrites: true,
			Mappings: []point.Mapping{{Name: "AI11", Kind: point.Float, Pointer: "/測定~1stage/~0json/0"}}}},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("valid escaped mapping: %v", err)
	}
	for _, ptr := range []string{"/測定~8stage/0", "/broken~", "/" + strings.Repeat("x", 4096)} {
		p.Sources[0].Mappings[0].Pointer = ptr
		if err := p.Validate(); err == nil {
			t.Errorf("malformed response mapping accepted by design validation: %q", ptr[:min(len(ptr), 40)])
		} else if !strings.Contains(err.Error(), "JSON pointer") {
			t.Errorf("mapping diagnostic does not name JSON pointer: %v", err)
		}
	}
}

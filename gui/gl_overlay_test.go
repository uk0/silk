package gui

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

// TestBuildChartOverlayTransformsToFramebufferPx drives the pure coordinate
// math of the GPU chart fast-path: local series points and the plot rect must
// map through the painter matrix (scale 2, translate (20,40)) into framebuffer
// pixels, the line width must scale with the matrix, and the color must convert
// to 0..1 rgba. No GL context is involved.
func TestBuildChartOverlayTransformsToFramebufferPx(t *testing.T) {
	m := geom.Mat3x2{Xx: 2, Yy: 2, X0: 20, Y0: 40} // scale 2, translate (20,40)
	plot := geom.Rect{X: 5, Y: 5, Width: 100, Height: 50}
	line := chartSeriesLine{
		color: paint.Color{R: 255, G: 128, B: 0, A: 255},
		pts:   []geom.Vec2{{X: 5, Y: 5}, {X: 105, Y: 55}},
	}

	ov := buildChartOverlay(m, plot, []chartSeriesLine{line}, 2)

	// clip: Transform(5,5)=(30,50), Transform(105,55)=(230,150) -> {30,50,200,100}
	want := geom.Rect{X: 30, Y: 50, Width: 200, Height: 100}
	if ov.clip != want {
		t.Fatalf("clip = %+v, want %+v", ov.clip, want)
	}
	if len(ov.lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(ov.lines))
	}
	pl := ov.lines[0]
	if pl.width != 4 { // 2 (logical) * 2 (matrix scale)
		t.Errorf("width = %v, want 4", pl.width)
	}
	wantPts := []float32{30, 50, 230, 150}
	if len(pl.pts) != len(wantPts) {
		t.Fatalf("pts len = %d, want %d", len(pl.pts), len(wantPts))
	}
	for i, v := range wantPts {
		if pl.pts[i] != v {
			t.Errorf("pts[%d] = %v, want %v", i, pl.pts[i], v)
		}
	}
	if pl.rgba[0] != 1 || pl.rgba[2] != 0 || pl.rgba[3] != 1 {
		t.Errorf("rgba = %v, want R=1 B=0 A=1", pl.rgba)
	}
	if g := pl.rgba[1]; g < 0.49 || g > 0.51 {
		t.Errorf("rgba G = %v, want ~0.502", g)
	}
}

// TestBuildChartOverlayDropsShortLines verifies a series with fewer than two
// points produces no drawable polyline.
func TestBuildChartOverlayDropsShortLines(t *testing.T) {
	m := geom.Mat3x2{Xx: 1, Yy: 1}
	plot := geom.Rect{X: 0, Y: 0, Width: 10, Height: 10}
	lines := []chartSeriesLine{
		{color: paint.Color{A: 255}, pts: []geom.Vec2{{X: 1, Y: 1}}}, // 1 pt -> dropped
	}
	ov := buildChartOverlay(m, plot, lines, 2)
	if len(ov.lines) != 0 {
		t.Fatalf("lines = %d, want 0 (short line dropped)", len(ov.lines))
	}
}

// TestGLOverlaySeamMatchesAcrossBackends catches a hook added to one backend
// file and forgotten in the other. gl_overlay_windows.go compiles only under
// GOOS=windows, so nothing in a mac or Linux build — not the compiler, not the
// rest of this suite — notices when gl_overlay_glfw.go grows a method the
// Windows stub lacks; the break surfaces only in a Windows cross-build, which
// is easy to skip. Comparing the two files as text fails here instead.
func TestGLOverlaySeamMatchesAcrossBackends(t *testing.T) {
	glfwSigs := windowMethodSigs(t, "gl_overlay_glfw.go")
	winSigs := windowMethodSigs(t, "gl_overlay_windows.go")
	if len(glfwSigs) == 0 {
		t.Fatal("no *Window methods found in gl_overlay_glfw.go; this test can no longer see the seam")
	}
	for name, sig := range glfwSigs {
		got, ok := winSigs[name]
		if !ok {
			t.Errorf("gl_overlay_windows.go declares no %s; the Windows build will not compile", name)
			continue
		}
		if got != sig {
			t.Errorf("%s signature differs: glfw %q, windows %q", name, sig, got)
		}
	}
	for name := range winSigs {
		if _, ok := glfwSigs[name]; !ok {
			t.Errorf("gl_overlay_windows.go declares %s with no GLFW counterpart", name)
		}
	}
}

// windowMethodSigs maps each `func (this *Window) Name(...)` in file to its
// normalized parameter and result text. Parameter names count as part of the
// signature: the two overlay files are meant to read as mirrors, so a rename
// on one side alone is worth flagging.
func windowMethodSigs(t *testing.T, file string) map[string]string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("cannot read %s: %v", file, err)
	}
	re := regexp.MustCompile(`(?m)^func \(this \*Window\) (\w+)\(([^)]*)\)([^{]*)\{`)
	sigs := make(map[string]string)
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		sigs[m[1]] = strings.Join(strings.Fields(m[2]+" "+m[3]), " ")
	}
	return sigs
}

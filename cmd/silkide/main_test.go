package main

import (
	"strings"
	"testing"
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

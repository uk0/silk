package ged

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAppendReportsAnImportTheBodyNeeds closes the half its neighbour left
// open. The append path may not rewrite the developer's import block, so a stub
// that needs one is reported instead — but only the PARAMETER list was
// inspected, so a design-time body calling fmt.Println produced
// `undefined: fmt` with AddedStubs naming the method and MissingImports empty.
// Silence is exactly what makes that a puzzle rather than a one-line fix.
func TestAppendReportsAnImportTheBodyNeeds(t *testing.T) {
	dir := t.TempDir()
	scene := NewGedScene()
	a := sceneWidget(t, scene, "gui.Button", "btnA", 10, 10, 30, 8)
	a.SetEventHandler("OnClick", "onA")
	a.SetCode("func onA() {\n\tprintln(1)\n}")

	path := filepath.Join(dir, "app.go")
	opts := CodeGenOptions{PackageName: "main", TypeName: "AppUI"}
	if _, err := scene.GenerateCodeFile(path, opts); err != nil {
		t.Fatalf("first generation: %v", err)
	}

	// Second widget whose BODY — not its parameters — reaches for a package.
	b := sceneWidget(t, scene, "gui.Button", "btnB", 10, 30, 30, 8)
	b.SetEventHandler("OnClick", "onB")
	b.SetCode("func onB() {\n\tfmt.Println(\"x\")\n}")

	res, err := scene.GenerateCodeFile(path, opts)
	if err != nil {
		t.Fatalf("second generation: %v", err)
	}
	if len(res.AddedStubs) != 1 || res.AddedStubs[0] != "onB" {
		t.Fatalf("AddedStubs = %v, want [onB]", res.AddedStubs)
	}
	if !hasString(res.MissingImports, "fmt") {
		user, _ := os.ReadFile(res.UserFile)
		t.Errorf("the append said nothing about the import its stub needs; MissingImports = %v\nuser file:\n%s",
			res.MissingImports, user)
	}
}

// TestAppendDoesNotReportAnImportAlreadyThere keeps the report honest: naming an
// import the file already has would send the developer looking for a problem
// that is not there.
func TestAppendDoesNotReportAnImportAlreadyThere(t *testing.T) {
	dir := t.TempDir()
	scene := NewGedScene()
	a := sceneWidget(t, scene, "gui.Button", "btnA", 10, 10, 30, 8)
	a.SetEventHandler("OnClick", "onA")
	a.SetCode("func onA() {\n\tfmt.Println(\"a\")\n}")

	path := filepath.Join(dir, "app.go")
	opts := CodeGenOptions{PackageName: "main", TypeName: "AppUI"}
	res, err := scene.GenerateCodeFile(path, opts)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	user, err := os.ReadFile(res.UserFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(user), `"fmt"`) {
		t.Errorf("creation did not import what the body uses:\n%s", user)
	}

	b := sceneWidget(t, scene, "gui.Button", "btnB", 10, 30, 30, 8)
	b.SetEventHandler("OnClick", "onB")
	b.SetCode("func onB() {\n\tfmt.Println(\"b\")\n}")
	res, err = scene.GenerateCodeFile(path, opts)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if hasString(res.MissingImports, "fmt") {
		t.Errorf("reported an import the file already has: %v", res.MissingImports)
	}
}

// TestQualifiedPackagesIgnoresTheReceiver — ui.BtnGo is a field access, not a
// package; reporting "ui" as a missing import would be nonsense advice.
func TestQualifiedPackagesIgnoresTheReceiver(t *testing.T) {
	got := qualifiedPackages("func (ui *AppUI) onGo() {\n\tui.BtnGo.SetText(fmt.Sprint(1))\n}")
	if hasString(got, "ui") {
		t.Errorf("the receiver was reported as a package: %v", got)
	}
	if !hasString(got, "fmt") {
		t.Errorf("a real package use was missed: %v", got)
	}
}

func hasString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

package ged

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMethodizeKeepsTheAuthorsReceiverName guards a rewrite that silently broke
// the body it was rewriting. methodize re-hangs a handler off the UI struct,
// which is the generator's call — but it replaced the WHOLE receiver with a
// fixed "ui", so a developer who wrote `func (a *T) onGo() { a.Btn... }` got a
// declaration reading `func (ui *T)` above a body still saying `a`.
//
// Every test in the split unit wrote the receiver as `ui`, so none of them
// could see it.
func TestMethodizeKeepsTheAuthorsReceiverName(t *testing.T) {
	dir := t.TempDir()
	scene := NewGedScene()
	btn := sceneWidget(t, scene, "gui.Button", "btnGo", 10, 10, 30, 8)
	btn.SetEventHandler("OnClick", "onGo")
	btn.SetCode("func (a *ProbeUI) onGo() {\n\ta.BtnGo.SetText(\"hi\")\n}")

	res, err := scene.GenerateCodeFile(filepath.Join(dir, "app.go"),
		CodeGenOptions{PackageName: "main", TypeName: "ProbeUI"})
	if err != nil {
		t.Fatalf("GenerateCodeFile: %v", err)
	}

	user, err := os.ReadFile(res.UserFile)
	if err != nil {
		t.Fatal(err)
	}
	src := string(user)
	if strings.Contains(src, "func (ui *ProbeUI) onGo()") && strings.Contains(src, "a.BtnGo") {
		t.Errorf("the receiver was renamed out from under the body:\n%s", src)
	}
	if !strings.Contains(src, "func (a *ProbeUI) onGo()") {
		t.Errorf("the author's receiver name did not survive:\n%s", src)
	}

	machine, err := os.ReadFile(res.MachineFile)
	if err != nil {
		t.Fatal(err)
	}
	// The pair only type-checks together, so vet both or prove nothing.
	vetGeneratedFiles(t, map[string]string{
		filepath.Base(res.MachineFile): string(machine),
		filepath.Base(res.UserFile):    src,
	})
}

// TestMethodizeStillRetypesAForeignReceiver keeps the fix from over-reaching:
// the receiver TYPE is the generator's to decide (handlers belong to this
// design's UI struct), only the identifier is the author's.
func TestMethodizeStillRetypesAForeignReceiver(t *testing.T) {
	got := methodize("func (a *SomethingElse) onGo() {\n\ta.X()\n}", handlerReceiver("ProbeUI"))
	if !strings.Contains(got, "*ProbeUI") {
		t.Errorf("the receiver type was not re-hung off the design's UI struct: %q", got)
	}
	if !strings.Contains(got, "(a *") {
		t.Errorf("the author's receiver identifier was lost: %q", got)
	}
}

// TestMethodizeHandlesAnAnonymousReceiver — `func (*T) f()` has no identifier
// for the body to use, so there is nothing to preserve and the default stands.
func TestMethodizeHandlesAnAnonymousReceiver(t *testing.T) {
	got := methodize("func (*Old) onGo() {\n\tprintln(1)\n}", handlerReceiver("ProbeUI"))
	if !strings.Contains(got, "func (ui *ProbeUI) onGo()") {
		t.Errorf("anonymous receiver did not fall back to the default: %q", got)
	}
}

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func typedCompanionFixture(t *testing.T) (*ged.GedScene, *ged.FakeWidget, ged.SplitResult) {
	t.Helper()
	scene := ged.NewGedScene()
	scene.SetSize(120, 90)
	scene.SetFormTitle("Before")
	scene.SetFilename(filepath.Join(t.TempDir(), "view.silkui"))
	add := func(factory, name string, y float64) *ged.FakeWidget {
		t.Helper()
		fake, err := ged.NewFakeWidgetFromFactory(factory)
		if err != nil {
			t.Fatal(err)
		}
		fake.SetWidgetName(name)
		fake.SetBounds(5, y, 50, 10)
		fake.SetParent(scene)
		return fake
	}
	button := add("gui.Button", "action", 5)
	button.SetEventHandler("OnClick", "onAction")
	button.SetCode(`func onAction() { ui.Status.SetText("embedded initial") }`)
	add("gui.Label", "status", 20).Widget().(*gui.Label).SetText("ready")
	if !scene.Save() {
		t.Fatal("save design")
	}
	pair, err := scene.GenerateCodeFile(strings.TrimSuffix(scene.Filename(), ".silkui")+".go", ged.CodeGenOptions{PackageName: "ui", TypeName: "StableUI"})
	if err != nil {
		t.Fatal(err)
	}
	return scene, button, pair
}

// Exercise the complete typed route: persisted design -> IDE save generation ->
// actual Go compiler -> constructor -> button action -> user handler -> label.
func TestCompanionPreservesTypedExportAndRunsUserHandler(t *testing.T) {
	scene, button, pair := typedCompanionFixture(t)
	user, err := os.ReadFile(pair.UserFile)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.ReplaceAll(string(user), "embedded initial", "user edit survived")
	if err := os.WriteFile(pair.UserFile, []byte(edited), 0600); err != nil {
		t.Fatal(err)
	}
	// Exercise ownership detection after a Windows-style CRLF checkout too.
	originalMachine, err := os.ReadFile(pair.MachineFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pair.MachineFile, []byte(strings.ReplaceAll(string(originalMachine), "\n", "\r\n")), 0600); err != nil {
		t.Fatal(err)
	}
	button.Widget().(*gui.Button).SetText("Updated button")
	button.SetCode(`func onAction() { ui.Status.SetText("must not overwrite external handler") }`)
	scene.SetFormTitle("Renamed form")
	if !scene.Save() {
		t.Fatal("save revised design")
	}
	for i := 0; i < 2; i++ {
		if err := regenerateCompanion(scene.Filename()); err != nil {
			t.Fatal(err)
		}
	}
	after, err := os.ReadFile(pair.UserFile)
	if err != nil || string(after) != edited {
		t.Fatalf("user source overwritten: %v\n%s", err, after)
	}
	machine, err := os.ReadFile(pair.MachineFile)
	if err != nil || !strings.Contains(string(machine), "func NewStableUI()") || strings.Contains(string(machine), "decl.Node") {
		t.Fatalf("typed constructor replaced: %v\n%s", err, machine)
	}
	if testing.Short() {
		t.Skip("external native SDK compilation")
	}
	check := filepath.Join(filepath.Dir(pair.UserFile), "runtime_test.go")
	if err := os.WriteFile(check, []byte(`package ui
import "testing"
func TestRegeneratedEvent(t *testing.T) {
 ui := NewStableUI()
 if ui.Form.Title() != "Renamed form" || ui.Action.Text() != "Updated button" { t.Fatal("design changes not regenerated") }
 if ui.Status.Text() != "ready" { t.Fatal("initial label changed") }
 ui.Action.Action().Trigger(ui.Action)
 if ui.Status.Text() != "user edit survived" { t.Fatalf("handler not connected: %q", ui.Status.Text()) }
}
`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", pair.MachineFile, pair.UserFile, check)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("regenerated user event failed: %v\n%s", err, output)
	}
}

func TestCompanionRejectsUnsafeExistingOutputs(t *testing.T) {
	for _, failure := range []string{"unowned", "broken-user", "missing-user", "standalone", "ambiguous"} {
		t.Run(failure, func(t *testing.T) {
			scene, _, pair := typedCompanionFixture(t)
			write := func(path, data string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch failure {
			case "unowned":
				write(pair.MachineFile, "package ui\n// developer code\n")
			case "broken-user":
				write(pair.UserFile, "package ui\nfunc broken(")
			case "missing-user":
				if err := os.Remove(pair.UserFile); err != nil {
					t.Fatal(err)
				}
			case "standalone":
				write(pair.MachineFile, scene.GenerateCode(ged.CodeGenOptions{PackageName: "ui", TypeName: "StableUI"}))
			case "ambiguous":
				machine, _ := os.ReadFile(pair.MachineFile)
				write(pair.MachineFile, string(machine)+"\ntype OtherUI struct { Form *gui.Form }\nfunc NewOtherUI() *OtherUI { return nil }\n")
			}
			machine, _ := os.ReadFile(pair.MachineFile)
			user, _ := os.ReadFile(pair.UserFile)
			scene.SetFormTitle("Changed")
			if !scene.Save() {
				t.Fatal("save design")
			}
			if err := regenerateCompanion(scene.Filename()); err == nil {
				t.Fatal("unsafe overwrite accepted")
			}
			afterMachine, _ := os.ReadFile(pair.MachineFile)
			afterUser, _ := os.ReadFile(pair.UserFile)
			if string(machine) != string(afterMachine) || string(user) != string(afterUser) {
				t.Fatal("failure changed output")
			}
			if failure == "missing-user" {
				if _, err := os.Stat(pair.UserFile); !os.IsNotExist(err) {
					t.Fatal("missing user code recreated")
				}
			}
		})
	}
}

func TestCompanionReportsRequiredUserImports(t *testing.T) {
	scene, _, pair := typedCompanionFixture(t)
	picker, err := ged.NewFakeWidgetFromFactory("gui.ColorPicker")
	if err != nil {
		t.Fatal(err)
	}
	picker.SetWidgetName("accent")
	picker.SetBounds(5, 40, 50, 10)
	picker.SetEventHandler("OnColorChanged", "onAccent")
	picker.SetParent(scene)
	if !scene.Save() {
		t.Fatal("save revised design")
	}
	err = regenerateCompanion(scene.Filename())
	if err == nil || !strings.Contains(err.Error(), "generated files saved") || !strings.Contains(err.Error(), "github.com/uk0/silk/paint") {
		t.Fatalf("missing import not reported: %v", err)
	}
	user, readErr := os.ReadFile(pair.UserFile)
	if readErr != nil || !strings.Contains(string(user), "onAccent(c paint.Color)") {
		t.Fatalf("new stub not appended: %v\n%s", readErr, user)
	}
	if strings.Contains(string(user), `"github.com/uk0/silk/paint"`) {
		t.Fatal("rewrote user imports without consent")
	}
}

package ged

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateCodeFileInvalidUserKeepsMachine(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Before")
	addNamedWidget(t, scene, "gui.Button", "action", 5)
	filename := filepath.Join(t.TempDir(), "view.go")
	opts := CodeGenOptions{PackageName: "ui", TypeName: "ViewUI"}
	result, err := scene.GenerateCodeFile(filename, opts)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(result.MachineFile)
	if err != nil {
		t.Fatal(err)
	}
	invalid := []byte("package ui\nfunc broken(")
	if err := os.WriteFile(result.UserFile, invalid, 0600); err != nil {
		t.Fatal(err)
	}
	scene.SetFormTitle("After")
	if _, err := scene.GenerateCodeFile(filename, opts); err == nil {
		t.Fatal("invalid user code accepted")
	}
	after, _ := os.ReadFile(result.MachineFile)
	user, _ := os.ReadFile(result.UserFile)
	if string(after) != string(before) || string(user) != string(invalid) {
		t.Fatal("failed export changed existing files")
	}
}

func TestGenerateCodeFilePreflightPreservesPair(t *testing.T) {
	for _, failure := range []string{"package", "type", "blank", "user-package", "unmapped", "unowned", "directory"} {
		t.Run(failure, func(t *testing.T) {
			scene := NewGedScene()
			scene.SetFormTitle("Before")
			path := filepath.Join(t.TempDir(), "view.go")
			opts := CodeGenOptions{PackageName: "ui", TypeName: "ViewUI"}
			res, err := scene.GenerateCodeFile(path, opts)
			if err != nil {
				t.Fatal(err)
			}
			write := func(path, data string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch failure {
			case "package":
				opts.PackageName = "not-a-package"
			case "type":
				opts.TypeName = "struct"
			case "blank":
				opts.TypeName = "_"
			case "user-package":
				write(res.UserFile, "package another\n// keep me\n")
			case "unmapped":
				addNamedWidget(t, scene, "gui.Menu", "menu", 5)
			case "unowned":
				write(res.MachineFile, "package ui\n// handwritten, not generated\n")
			case "directory":
				if err := os.Remove(res.UserFile); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(res.UserFile, 0700); err != nil {
					t.Fatal(err)
				}
			}
			machine, _ := os.ReadFile(res.MachineFile)
			user, _ := os.ReadFile(res.UserFile)
			scene.SetFormTitle("After")
			failed, err := scene.GenerateCodeFile(path, opts)
			if err == nil {
				t.Fatal("invalid export accepted")
			}
			if failed.UserCreated || len(failed.AddedStubs) != 0 {
				t.Fatal("failed export reported successful writes")
			}
			afterMachine, _ := os.ReadFile(res.MachineFile)
			afterUser, _ := os.ReadFile(res.UserFile)
			if string(machine) != string(afterMachine) || string(user) != string(afterUser) {
				t.Fatal("preflight failure changed files")
			}
		})
	}
}

func TestGenerateCodeFileNoopKeepsMtimeAndPermissions(t *testing.T) {
	scene := NewGedScene()
	path := filepath.Join(t.TempDir(), "view.go")
	opts := CodeGenOptions{PackageName: "ui", TypeName: "ViewUI"}
	res, err := scene.GenerateCodeFile(path, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{res.UserFile, res.MachineFile} {
		if err := os.Chmod(file, 0600); err != nil {
			t.Fatal(err)
		}
	}
	beforeMachine, _ := os.Stat(res.MachineFile)
	beforeUser, _ := os.Stat(res.UserFile)
	if _, err := scene.GenerateCodeFile(path, opts); err != nil {
		t.Fatal(err)
	}
	afterMachine, _ := os.Stat(res.MachineFile)
	afterUser, _ := os.Stat(res.UserFile)
	if !os.SameFile(beforeMachine, afterMachine) || beforeMachine.ModTime() != afterMachine.ModTime() || !os.SameFile(beforeUser, afterUser) || beforeUser.ModTime() != afterUser.ModTime() {
		t.Fatal("identical generation replaced a file")
	}
	scene.SetFormTitle("Changed")
	if _, err := scene.GenerateCodeFile(path, opts); err != nil {
		t.Fatal(err)
	}
	afterMachine, _ = os.Stat(res.MachineFile)
	if afterMachine.Mode().Perm() != beforeMachine.Mode().Perm() {
		t.Fatal("replacement changed permissions")
	}
}

func TestGenerateCodeFilePreservesIdentityAndCRLFUserCode(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Before")
	addSplitButton(t, scene, "action", "onAction", `func onAction() { println("keep") }`)
	path := filepath.Join(t.TempDir(), "view.go")
	opts := CodeGenOptions{PackageName: "ui"}
	pair, err := scene.GenerateCodeFile(path, opts)
	if err != nil {
		t.Fatal(err)
	}
	machine, err := os.ReadFile(pair.MachineFile)
	if err != nil {
		t.Fatal(err)
	}
	originalType, err := SplitCodeType(string(machine))
	if err != nil {
		t.Fatal(err)
	}
	user, err := os.ReadFile(pair.UserFile)
	if err != nil {
		t.Fatal(err)
	}
	userCRLF := strings.ReplaceAll(string(user), "\n", "\r\n")
	for file, data := range map[string]string{pair.MachineFile: strings.ReplaceAll(string(machine), "\n", "\r\n"), pair.UserFile: userCRLF} {
		if err := os.WriteFile(file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	scene.SetFormTitle("After")
	if _, err := scene.GenerateCodeFile(path, opts); err != nil {
		t.Fatal(err)
	}
	machine, err = os.ReadFile(pair.MachineFile)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := SplitCodeType(string(machine)); err != nil || got != originalType {
		t.Fatalf("renaming form changed type to %q: %v", got, err)
	}
	after, _ := os.ReadFile(pair.UserFile)
	if string(after) != userCRLF {
		t.Fatal("user body/CRLF rewritten")
	}
	opts.TypeName = "DifferentUI"
	if _, err := scene.GenerateCodeFile(path, opts); err == nil {
		t.Fatal("incompatible type replacement accepted")
	}
	after, _ = os.ReadFile(pair.MachineFile)
	if string(after) != string(machine) {
		t.Fatal("type conflict changed machine file")
	}
}

func TestGenerateCodeFileRejectsStandaloneExport(t *testing.T) {
	scene := NewGedScene()
	addSplitButton(t, scene, "action", "onAction", `func onAction() { println("preserve embedded code") }`)
	machine := filepath.Join(t.TempDir(), "view.silk.go")
	opts := CodeGenOptions{PackageName: "ui", TypeName: "ViewUI"}
	before := scene.GenerateCode(opts)
	if err := os.WriteFile(machine, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := scene.GenerateCodeFile(machine, opts); err == nil {
		t.Fatal("standalone code overwritten")
	}
	after, _ := os.ReadFile(machine)
	if string(after) != before {
		t.Fatal("standalone source changed")
	}
	if _, err := os.Stat(strings.TrimSuffix(machine, ".silk.go") + ".go"); !os.IsNotExist(err) {
		t.Fatal("unexpected user file", err)
	}
}

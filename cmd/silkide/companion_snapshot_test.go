package main

import (
	"os"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/internal/silkgen"
)

func runCapturedCompanion(path string, doc *core.TDoc, code *ged.CodeSnapshot) error {
	done := make(chan error, 1)
	go func() { done <- regenerateCompanionSnapshot(path, doc, code) }()
	return <-done
}

func TestCapturedCompanionUsesCapturedPrimaryInsteadOfRereadingDisk(t *testing.T) {
	for _, typed := range []bool{false, true} {
		name := "declarative"
		if typed {
			name = "typed"
		}
		t.Run(name, func(t *testing.T) {
			scene, button, pair := typedCompanionFixture(t)
			if !typed {
				if err := os.Remove(pair.MachineFile); err != nil {
					t.Fatal(err)
				}
			}
			button.Widget().(*gui.Button).SetText("captured value")
			scene.SetFormTitle("Captured title")
			p, err := scene.PrepareSave(scene.Filename())
			if err != nil {
				t.Fatal(err)
			}
			doc, code := p.Writer().DocumentCopy(), scene.CaptureCodeSnapshot()
			if !scene.PreparedSaveCurrent(p) {
				t.Fatal("capture changed scene")
			}
			var expected []byte
			if !typed {
				tree, err := decl.FromTDoc(doc.Clone())
				if err != nil {
					t.Fatal(err)
				}
				expected, err = silkgen.GenerateSource(scene.Filename(), "ui", "", tree)
				if err != nil {
					t.Fatal(err)
				}
			}
			// The worker must neither load this deliberately corrupt primary nor
			// consult the now different UI widgets to produce the companion.
			if err := os.WriteFile(scene.Filename(), []byte("not a design\x00"), 0600); err != nil {
				t.Fatal(err)
			}
			button.Widget().(*gui.Button).SetText("later UI value")
			scene.SetFormTitle("Later title")
			user, err := os.ReadFile(pair.UserFile)
			if err != nil {
				t.Fatal(err)
			}
			if err := runCapturedCompanion(scene.Filename(), doc, code); err != nil {
				t.Fatal(err)
			}
			machine, err := os.ReadFile(pair.MachineFile)
			if err != nil {
				t.Fatal(err)
			}
			if typed {
				if !strings.Contains(string(machine), "captured value") || !strings.Contains(string(machine), "Captured title") || strings.Contains(string(machine), "Later title") {
					t.Fatal("typed worker did not use UI capture")
				}
			} else if string(machine) != string(expected) {
				t.Fatal("declarative worker did not use captured primary")
			}
			after, err := os.ReadFile(pair.UserFile)
			if err != nil || string(after) != string(user) {
				t.Fatal("worker modified existing developer file", err)
			}
		})
	}
}

func TestCapturedCompanionRejectsMissingInputsAndUnownedMachine(t *testing.T) {
	s, _, pair := typedCompanionFixture(t)
	doc, code := s.SaveDesign(), s.CaptureCodeSnapshot()
	if err := runCapturedCompanion(s.Filename(), nil, code); err == nil {
		t.Fatal("missing primary capture accepted")
	}
	if err := runCapturedCompanion(s.Filename(), doc, nil); err == nil {
		t.Fatal("typed output accepted without code capture")
	}
	owned := "package ui\n// developer-owned\n"
	if err := os.WriteFile(pair.MachineFile, []byte(owned), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runCapturedCompanion(s.Filename(), doc, code); err == nil {
		t.Fatal("replaced developer-owned output")
	}
	after, err := os.ReadFile(pair.MachineFile)
	if err != nil || string(after) != owned {
		t.Fatal("rejection changed developer output", err)
	}
}

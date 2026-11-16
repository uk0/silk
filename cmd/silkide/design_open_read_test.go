package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func TestDesignPreparationRejectsWrongRootAndBoundedExpansion(t *testing.T) {
	path := designOpenPath(t, "good")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	z := gzip.NewWriter(&compressed)
	if _, err = z.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	doc, err := decodeManualDesign(context.Background(), compressed.Bytes())
	if err != nil || doc == nil {
		t.Fatal("valid compressed document rejected", err)
	}
	for _, data := range [][]byte{nil, []byte("not-a-form"), []byte("42"), []byte("children=broken")} {
		if _, err := decodeManualDesign(context.Background(), data); err == nil {
			t.Fatal("wrong root accepted", string(data))
		}
	}
	compressed.Reset()
	z = gzip.NewWriter(&compressed)
	if _, err = io.CopyN(z, strings.NewReader(strings.Repeat("x", restoreFileLimit+1)), restoreFileLimit+1); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeManualDesign(context.Background(), compressed.Bytes()); err == nil || !strings.Contains(err.Error(), "32 MiB") {
		t.Fatal("expansion bound not enforced", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readManualDesign(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled read admitted", err)
	}
}

func TestManualDesignFailuresPreserveOldSceneAndUndo(t *testing.T) {
	for _, kind := range []string{"missing", "directory", "invalid", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			w, prefs := recentOpenWorkbench(t)
			scene := w.canvas.GedScene()
			scene.PushCommand(gui.NewCompoundCommand())
			w.designOpen.settings.confirmDiscard = func(*ged.GedView) bool { return true }
			before, name, count := scene.SaveDesign().String(), scene.Filename(), scene.UndoStack().Count()
			path := filepath.Join(t.TempDir(), kind+".silkui")
			switch kind {
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "invalid":
				if err := os.WriteFile(path, []byte("not-a-form"), 0600); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				err = f.Truncate(33 << 20)
				_ = f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			if !requestDesignOpen(path, w.tabs, w.canvas, true) {
				t.Fatal("request not admitted")
			}
			pumpTestDesignOpens(t, w)
			if scene.SaveDesign().String() != before || scene.Filename() != name || scene.UndoStack().Count() != count || scene.IsClean() || len(prefs.RecentFiles()) != 0 {
				t.Fatal("failed preparation damaged dirty current scene or recorded MRU")
			}
		})
	}
}

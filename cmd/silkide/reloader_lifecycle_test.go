package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestIDEReloaderRetiresQueuedUIAuthorityBeforeJoining(t *testing.T) {
	w := testWorkbench(t)
	old, retired := globalReloader, retiredIDEReloaders
	globalReloader = nil
	retiredIDEReloaders = nil
	t.Cleanup(func() {
		closeIDEReloader()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if e := waitIDEReloaders(ctx); e != nil {
			t.Error(e)
		}
		globalReloader, retiredIDEReloaders = old, retired
	})
	p := filepath.Join(t.TempDir(), "f.silkui")
	writeRecoveryDesign(t, p, "Initial", "initial")
	if e := openDesignDocument(w.canvas.GedScene(), p, ""); e != nil {
		t.Fatal(e)
	}
	startReloader(w.canvas)
	owned := globalReloader
	writeRecoveryDesign(t, p, "Current owner", "second")
	if applied, e := reloadOwnedCleanDesign(owned, w.canvas, p); e != nil || !applied {
		t.Fatal("active owner could not apply", e)
	}
	closeIDEReloader()
	startReloader(w.canvas)
	if globalReloader == owned {
		t.Fatal("retired owner reused")
	}
	writeRecoveryDesign(t, p, "Do not apply stale UI post", "third")
	if applied, e := reloadOwnedCleanDesign(owned, w.canvas, p); e != nil || applied {
		t.Fatal("retired queued UI post kept authority", e)
	}
	if w.canvas.GedScene().FormTitle() != "Current owner" {
		t.Fatal("stale post replaced the design")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e := waitIDEReloaders(ctx); e != nil {
		t.Fatal(e)
	}
}

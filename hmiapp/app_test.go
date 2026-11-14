package hmiapp

import (
	"context"
	"github.com/uk0/silk/hmi"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGeneratedProjectCompilesAndDocumentRoundTrips(t *testing.T) {
	p := hmi.Template()
	b, e := Generate(p)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "plant.silkhmi")
	if e = hmi.Save(path, p); e != nil {
		t.Fatal(e)
	}
	if _, e = hmi.Load(path); e != nil {
		t.Fatal(e)
	}
	if testing.Short() {
		return
	}
	dir := t.TempDir()
	root, e := filepath.Abs("..")
	if e != nil {
		t.Fatal(e)
	}
	module := "module generatedhmi\n\ngo 1.26.10\n\nrequire github.com/uk0/silk v0.0.0\nreplace github.com/uk0/silk => " + filepath.ToSlash(root) + "\n"
	if e = os.WriteFile(filepath.Join(dir, "go.mod"), []byte(module), 0600); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(dir, "main.go"), b, 0600)
	cmd := exec.CommandContext(context.Background(), "go", "build", "-mod=mod", "-p", "1", ".")
	cmd.Dir = dir
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("generated Go does not build: %v\n%s", e, out)
	}
}

func TestWorkbenchCloseJoinsStartupAndRemainsIdempotent(t *testing.T) {
	for i := 0; i < 5; i++ {
		f, err := New(Config{DatabasePath: filepath.Join(t.TempDir(), "points.db")})
		if err != nil {
			t.Fatal(err)
		}
		f.StartRuntime()
		session := f.session
		if session == nil {
			t.Fatal("run not started")
		}
		f.Close()
		f.Close()
		select {
		case <-session.done:
		case <-time.After(10 * time.Second):
			t.Fatal("startup worker survived Close")
		}
		f.Join()
		f.StartRuntime()
		if f.session != session {
			t.Fatal("closed workbench restarted")
		}
	}
}

package core

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIniReadFailureNeverReplacesOriginal(t *testing.T) {
	for _, content := range [][]byte{[]byte("=\ntype = project\nname = \xd6\xd0\xce\xc4\n"), []byte("=\nname = a\x00b\n"), {0x1f, 0x8b, 8}} {
		path := filepath.Join(t.TempDir(), "project.cml")
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
		ini := LoadTDocIni(path, false)
		var v string
		if err := ini.ReadAttr("type", &v); err == nil {
			t.Fatal("decode failure hidden")
		}
		if err := ini.WriteAttr("opened", 1); err == nil {
			t.Fatal("edit accepted after load failed")
		}
		if err := ini.SyncError(); err == nil {
			t.Fatal("sync accepted after load failed")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, content) {
			t.Fatalf("original lost: %q %v", after, err)
		}
		if !ini.SyncTime().IsZero() {
			t.Fatal("failed sync acknowledged")
		}
	}
}

func TestIniReloadFailureProtectsExternalEdit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.cml")
	ini := LoadTDocIni(path, false)
	if err := ini.WriteAttr("type", "project"); err != nil {
		t.Fatal(err)
	}
	if err := ini.SyncError(); err != nil {
		t.Fatal(err)
	}
	saved := ini.SyncTime()
	if err := ini.WriteAttr("opened", 1); err != nil {
		t.Fatal(err)
	}
	bad := []byte("=\nname = \xff\n")
	if err := os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if err := ini.SyncError(); err == nil {
		t.Fatal("bad external file overwritten")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, bad) || !ini.SyncTime().Equal(saved) {
		t.Fatal("failed reload changed disk/time")
	}
	if err := os.WriteFile(path, []byte("=\nother = recovered\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ini.SyncError(); err != nil {
		t.Fatal(err)
	}
}

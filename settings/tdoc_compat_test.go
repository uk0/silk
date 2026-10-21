package settings

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/core"
)

func TestSettingsEscapedKeysSurviveLoadAndAsyncSave(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		doc := core.NewTDoc()
		keys := []string{"editor profiles/Go macros/中文 😀", "with\ttab", "with\nnewline", "with\rcarriage", "equals=and\\slash"}
		for _, key := range keys {
			if err := doc.WriteAttr(key, "before"); err != nil {
				t.Fatal(err)
			}
		}
		var data bytes.Buffer
		if err := doc.Save1(&data, compressed); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "preferences.tdoc")
		if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		s := New(path)
		for _, key := range keys {
			if got := s.String(key, "missing"); got != "before" {
				t.Fatalf("compressed=%v key=%q loaded=%q status=%v", compressed, key, got, s.Status())
			}
			if err := s.SetString(key, "after"); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.RequestSync(); err != nil {
			t.Fatal(err)
		}
		if err := s.Flush(context.Background()); err != nil {
			t.Fatal(err)
		}
		reloaded := New(path)
		for _, key := range keys {
			if got := reloaded.String(key, "missing"); got != "after" {
				t.Fatalf("compressed=%v key=%q saved=%q status=%v", compressed, key, got, reloaded.Status())
			}
		}
	}
}

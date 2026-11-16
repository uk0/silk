package fileops

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicFingerprintConflictsAndPhysicalResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc")
	write := func(expected FileFingerprint, text string) (FileFingerprint, error) {
		return WriteAtomicAgainst(path, 0644, expected, func(w io.Writer) error { _, err := io.WriteString(w, text); return err })
	}
	if _, err := write(FileFingerprint{}, "must not write"); !errors.Is(err, os.ErrInvalid) {
		t.Fatal(err)
	}
	first, err := write(MissingFingerprint(), "saved A")
	if err != nil || first != FingerprintBytes([]byte("saved A")) {
		t.Fatal(first, err)
	}
	if _, err := write(MissingFingerprint(), "must not overwrite"); !errors.Is(err, ErrChanged) {
		t.Fatal("unknown existing file overwritten", err)
	}
	if err := os.WriteFile(path, []byte("external B"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := write(first, "must preserve external"); !errors.Is(err, ErrChanged) {
		t.Fatal("stale content overwritten", err)
	}
	actual, err := InspectFingerprint(path)
	if err != nil || actual != FingerprintBytes([]byte("external B")) {
		t.Fatal(actual, err)
	}
	next, err := write(actual, "accepted explicit baseline")
	if err != nil || !next.Known() {
		t.Fatal(next, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := write(next, "must not recreate deleted file"); !errors.Is(err, ErrChanged) {
		t.Fatal("deleted baseline silently recreated", err)
	}
	missing, err := InspectFingerprint(path)
	if err != nil || missing != MissingFingerprint() {
		t.Fatal(missing, err)
	}
}

func TestAtomicFingerprintRetainsDuringEncodingProtectionAndReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc")
	if err := os.WriteFile(path, []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}
	baseline := FingerprintBytes([]byte("before"))
	result, err := WriteAtomicAgainst(path, 0644, baseline, func(w io.Writer) error {
		if err := os.WriteFile(path, []byte("external during encode"), 0644); err != nil {
			return err
		}
		_, err := io.WriteString(w, "our bytes")
		return err
	})
	if !errors.Is(err, ErrChanged) || result.Known() {
		t.Fatal("encoding-time conflict lost", result, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "external during encode" {
		t.Fatal(string(data), err)
	}
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0644) })
	_, err = WriteAtomicAgainst(path, 0644, FingerprintBytes(data), func(w io.Writer) error { _, err := io.WriteString(w, "must refuse"); return err })
	if !errors.Is(err, os.ErrPermission) {
		t.Fatal("read-only file replaced", err)
	}
	stages, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".silk-write-*"))
	if err != nil || len(stages) != 0 {
		t.Fatal("staging files leaked", stages, err)
	}
}

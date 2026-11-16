package core

import (
	"io"

	"github.com/uk0/silk/internal/fileops"
)

// FileFingerprint is a content/existence baseline, not a lock or UI identity.
type FileFingerprint = fileops.FileFingerprint

func FingerprintBytes(data []byte) FileFingerprint { return fileops.FingerprintBytes(data) }
func MissingFingerprint() FileFingerprint          { return fileops.MissingFingerprint() }

// SaveFileAtomicAgainst writes a plain document only if its observed baseline
// still matches. This is synchronous; call on a bounded worker with an owned
// independent TDoc, not a scene or a live editor.
func (doc *TDoc) SaveFileAtomicAgainst(path string, expected FileFingerprint) (FileFingerprint, error) {
	return fileops.WriteAtomicAgainst(path, 0644, expected, func(w io.Writer) error { return doc.Save1(w, false) })
}

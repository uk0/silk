package fileops

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

// FileFingerprint is an immutable content/existence baseline, not a file lock
// or a pathname identity. The zero value is unknown, distinct from known absent.
// Hosts must bind it to the document/path whose bytes they actually observed.
type FileFingerprint struct {
	known, exists bool
	sum           [sha256.Size]byte
}

func FingerprintBytes(data []byte) FileFingerprint {
	return FileFingerprint{known: true, exists: true, sum: sha256.Sum256(data)}
}
func MissingFingerprint() FileFingerprint { return FileFingerprint{known: true} }
func (v FileFingerprint) Known() bool     { return v.known }

// InspectFingerprint performs synchronous filesystem I/O. Run it on a bounded
// worker; never use an observation made after queuing as implicit overwrite
// permission for a document whose prior baseline is unknown.
func InspectFingerprint(path string) (FileFingerprint, error) {
	s, err := inspectAtomicTarget(path)
	if err != nil {
		return FileFingerprint{}, err
	}
	return FileFingerprint{known: true, exists: s.info != nil, sum: s.sum}, nil
}

// WriteAtomicAgainst checks a previously observed content baseline inside the
// atomic writer before staging. It also retains the existing identity/mode/hash
// check across encoding. Return the actual encoded fingerprint only on success.
// This detects ordinary conflicts, not adversarial races or external locks.
func WriteAtomicAgainst(path string, mode os.FileMode, expected FileFingerprint, encode func(io.Writer) error) (FileFingerprint, error) {
	return writeAtomicRecorded(path, mode, &expected, encode)
}

// WriteAtomicRecorded retains legacy unconditional-write semantics while
// returning the actual encoded baseline, without a second encode/read pass.
func WriteAtomicRecorded(path string, mode os.FileMode, encode func(io.Writer) error) (FileFingerprint, error) {
	return writeAtomicRecorded(path, mode, nil, encode)
}

func writeAtomicRecorded(path string, mode os.FileMode, expected *FileFingerprint, encode func(io.Writer) error) (FileFingerprint, error) {
	if expected != nil && !expected.known || encode == nil {
		return FileFingerprint{}, &os.PathError{Op: "atomic-write", Path: path, Err: os.ErrInvalid}
	}
	var guard func(atomicTarget) error
	if expected != nil {
		guard = func(actual atomicTarget) error {
			if expected.exists != (actual.info != nil) || expected.exists && expected.sum != actual.sum {
				return fmt.Errorf("%w: saved baseline differs at %s", ErrChanged, path)
			}
			return nil
		}
	}
	hash := sha256.New()
	err := writeAtomicGuarded(path, mode, func(w io.Writer) error {
		return encode(io.MultiWriter(w, hash))
	}, func(dir, pattern string) (atomicOutput, error) {
		return os.CreateTemp(dir, pattern)
	}, ReplaceSnapshotFile, guard)
	if err != nil {
		return FileFingerprint{}, err
	}
	result := FileFingerprint{known: true, exists: true}
	copy(result.sum[:], hash.Sum(nil))
	return result, nil
}

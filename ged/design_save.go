package ged

import (
	"errors"
	"path/filepath"
)

// SaveTo commits UI drafts and atomically writes the design to filename. Only
// after a successful write does it adopt the filename/title and clean marker.
// Failed Save As leaves the original document identity and dirty history intact.
// An extensionless path gains .silkui; legacy design extensions are preserved.
//
// Like Save, this is a synchronous UI-thread API, NOT background admission.
// Do not call it on a worker: capturing the design traverses live widgets.
// Background writers can instead use SaveDesignWithPendingEdits on UI and
// TDoc.SaveFileAtomic on the resulting independent document, with their own
// owned UI completion/clean-marker validation.
func (s *GedScene) SaveTo(filename string) error {
	if filename == "" {
		return errors.New("design save: empty destination")
	}
	if s.saving || s.committingEdits {
		return errors.New("design save: reentrant save during pending edit commit")
	}
	s.saving = true
	defer func() { s.saving = false }()
	previous := s.filename
	if filepath.Ext(filename) == "" {
		filename += DefaultDesignExt
	}
	doc, err := s.SaveDesignWithPendingEdits()
	if err != nil {
		return err
	}
	// Draft commit observers may retarget the scene. Never write the old
	// request under a new identity or restore its path over the observer.
	if s.filename != previous {
		return errors.New("design save: document identity changed during draft commit")
	}
	if err = doc.SaveFileAtomic(filename); err != nil {
		return err
	}
	s.SetFilename(filename)
	s.SetTitle(filepath.Base(filename))
	s.UndoStack().SetClean()
	return nil
}

package ged

import (
	"errors"
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
	p, err := s.prepareSave(filename)
	if err != nil {
		return err
	}
	if err = p.Writer().Write(); err != nil {
		return err
	}
	if !s.completeSave(p) {
		return ErrSaveSuperseded
	}
	return nil
}

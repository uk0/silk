package core

import "github.com/uk0/silk/internal/fileops"

// ErrSaveConflict identifies a destination changed during atomic preparation.
// Use errors.Is on SaveFileAtomic/SaveFileAtomic1 errors. This is optimistic
// conflict detection, not cross-process exclusion or a complete editor baseline.
var ErrSaveConflict = fileops.ErrChanged

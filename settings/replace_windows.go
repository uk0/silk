package settings

import "github.com/uk0/silk/internal/fileops"

func replaceSettingsFile(from, to string) error { return fileops.ReplaceSnapshotFile(from, to) }

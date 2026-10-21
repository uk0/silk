//go:build !windows

package settings

import "os"

func replaceSettingsFile(from, to string) error { return os.Rename(from, to) }

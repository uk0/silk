package settings

import (
	"os"
	"path/filepath"
	"runtime"
)

// DefaultPath returns the OS-conventional path for an app's settings
// file. The output structure follows each platform's HIG-recommended
// layout:
//
//	macOS:   ~/Library/Application Support/<org>/<app>/<app>.silkui
//	Linux:   $XDG_CONFIG_HOME/<org>/<app>/<app>.conf
//	         (falls back to ~/.config/<org>/<app>/<app>.conf when XDG
//	         is unset)
//	Windows: %APPDATA%\<org>\<app>\<app>.ini
//
// Why per-app subdir under per-org subdir: matches what every native
// installer does. A user shipping multiple apps from one org gets all
// of them under one umbrella; a single app stays in its own folder so
// uninstall is just an rm -rf <app>.
//
// Empty org or app substitutes "Silk" / "App" so callers without
// metadata still get a stable path. This is a safety net — production
// apps should always supply both.
func DefaultPath(org, app string) string {
	if org == "" {
		org = "Silk"
	}
	if app == "" {
		app = "App"
	}
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", org, app, app+".silkui")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			home, _ := os.UserHomeDir()
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, org, app, app+".ini")
	default:
		// Linux / *BSD / etc. — XDG with a ~/.config fallback.
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, org, app, app+".conf")
	}
}

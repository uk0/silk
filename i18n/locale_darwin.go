//go:build darwin

package i18n

import (
	"os/exec"
	"strings"
)

// readMacAppleLocale invokes `defaults read -g AppleLocale` to pick up
// the macOS user preference. macOS apps launched from Finder don't
// inherit POSIX locale env vars; the AppleLocale default is the only
// reliable source there.
//
// Returns ("", false) on any error so the caller falls back to the
// "en" default cleanly.
func readMacAppleLocale() (string, bool) {
	cmd := exec.Command("defaults", "read", "-g", "AppleLocale")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", false
	}
	return v, true
}

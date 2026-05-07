//go:build !darwin

package i18n

// readMacAppleLocale is a no-op on non-Darwin platforms; locale
// detection there falls back to the env-var ladder in DetectLocale.
func readMacAppleLocale() (string, bool) { return "", false }

//go:build !windows && !linux

package gui

func desktopFilePortal(_ bool, _ string) (string, bool) { return "", false }
func closeLinuxFilePortalDisplay()                      {}

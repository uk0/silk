package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func ExeFile() string {
	exe, err := os.Executable()
	if err != nil {
		return os.Args[0]
	}
	exe, _ = filepath.EvalSymlinks(exe)
	return strings.ReplaceAll(exe, `\`, `/`)
}

func ExeFileDir() string {
	return filepath.Dir(ExeFile())
}

func ExeFileBaseName(withExtension bool) string {
	file := ExeFile()
	base := filepath.Base(file)
	if withExtension {
		return base
	}
	ext := filepath.Ext(base)
	if ext != "" {
		return base[:len(base)-len(ext)]
	}
	return base
}

func updateSystemIconCache() {
	// stub on macOS
}

func SetDirIcon(dir, icoFile, info string) error {
	return nil
}

func DesktopDir() string {
	home, _ := os.UserHomeDir()
	return home + "/Desktop"
}

func DocumentsDir() string {
	home, _ := os.UserHomeDir()
	return home + "/Documents"
}

func ShellOpen(x string) error {
	return exec.Command("open", x).Start()
}

func Sleep(milliseconds int) {
	time.Sleep(time.Duration(milliseconds) * time.Millisecond)
}

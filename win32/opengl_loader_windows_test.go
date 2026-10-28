//go:build windows

package win32

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWGLRejectsEmbeddedNUL(t *testing.T) {
	if p := WglGetProcAddress("glGetString\x00unexpected"); p != 0 {
		t.Fatal("invalid name resolved", p)
	}
}

func TestWGLLoadsSystemDLLNotProjectFile(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "opengl32.dll"), []byte("not a DLL"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)
	if err := modopengl32.Load(); err != nil {
		t.Fatal("system DLL load affected by project file:", err)
	}
	var buf [32768]uint16
	n, err := windows.GetModuleFileName(windows.Handle(modopengl32.Handle()), &buf[0], uint32(len(buf)))
	if err != nil {
		t.Fatal(err)
	}
	actual := windows.UTF16ToString(buf[:n])
	dir, err := windows.GetWindowsDirectory()
	if err != nil {
		t.Fatal(err)
	}
	valid := false
	// The redirected system directory is also valid for a 32-bit process.
	for _, sub := range []string{"System32", "SysWOW64", "SysArm32"} {
		if strings.EqualFold(filepath.Clean(actual), filepath.Join(dir, sub, "opengl32.dll")) {
			valid = true
		}
	}
	if !valid {
		t.Fatal("OpenGL did not come from a Windows system directory:", actual)
	}
	t.Log("loaded", actual)
}

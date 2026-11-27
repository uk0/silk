//go:build !windows && (!darwin || !cgo)

package gui

func platformOpenFileDialog() string {
	if path, handled := desktopFilePortal(false, ""); handled {
		return path
	}
	return linuxOpenFileDialog()
}
func platformSaveFileDialog() string {
	if path, handled := desktopFilePortal(true, ""); handled {
		return path
	}
	return linuxSaveFileDialog()
}

func platformOpenExtensionDialog(ext string) string {
	if path, handled := desktopFilePortal(false, ext); handled {
		return path
	}
	return runDialogCmd("zenity", "--file-selection", "--title=Open File",
		"--file-filter="+ext+" files | *."+ext, "--file-filter=All Files | *")
}
func platformSaveExtensionDialog(ext string) string {
	if path, handled := desktopFilePortal(true, ext); handled {
		return path
	}
	return runDialogCmd("zenity", "--file-selection", "--save", "--title=Save File",
		"--filename=document."+ext, "--file-filter="+ext+" files | *."+ext, "--file-filter=All Files | *")
}

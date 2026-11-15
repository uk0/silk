//go:build !windows && (!darwin || !cgo)

package gui

func platformOpenFileDialog() string { return linuxOpenFileDialog() }
func platformSaveFileDialog() string { return linuxSaveFileDialog() }

func platformOpenExtensionDialog(ext string) string {
	return runDialogCmd("zenity", "--file-selection", "--title=Open File",
		"--file-filter="+ext+" files | *."+ext, "--file-filter=All Files | *")
}
func platformSaveExtensionDialog(ext string) string {
	return runDialogCmd("zenity", "--file-selection", "--save", "--title=Save File",
		"--filename=document."+ext, "--file-filter="+ext+" files | *."+ext, "--file-filter=All Files | *")
}

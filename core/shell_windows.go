package core

import (
	"silk/win32"
	"io"
	"os"
	"strings"
)

var exeFile string

func ExeFile() string {
	if exeFile == "" {
		exeFile = win32.GetModuleFileName(0)
		exeFile = strings.Replace(exeFile, "\\", "/", -1)
	}
	return exeFile
}

func ExeFileDir() string {
	file := ExeFile()
	pos := strings.LastIndex(file, `/`)
	return file[:pos]
}

func ExeFileBaseName(withExtension bool) string {
	file := ExeFile()
	pos := strings.LastIndex(file, `/`)
	s := file[pos+1:]
	if withExtension {
		return s
	}
	pos = strings.LastIndex(s, `.`)
	if pos != -1 {
		return s[:pos]
	}
	return s
}

func updateSystemIconCache() {
	const SHCNE_ASSOCCHANGED = 0x08000000
	const SHCNF_IDLIST = 0x0000
	const SHCNF_FLUSH = 0x1000
	win32.SHChangeNotify(SHCNE_ASSOCCHANGED, SHCNF_IDLIST|SHCNF_FLUSH, 0, 0)
}

func SetDirIcon(dir, icoFile, info string) error {
	localIcoFile := "_dir_.ico"
	err := CopyFile(dir+"/"+localIcoFile, icoFile)
	if err != nil {
		return err
	}

	desktopIni := dir + "/desktop.ini"
	file, err := os.Create(desktopIni)
	if err != nil {
		return err
	}
	/*
		io.WriteString(file, "[.ShellClassInfo]\r\n")
		io.WriteString(file, "ConfirmFileOp=0\r\n")
		io.WriteString(file, "IconResource="+localIcoFile+",0\r\n")
		io.WriteString(file, "InfoTip="+info+"\r\n")
		io.WriteString(file, "[ViewState]\r\n")
		io.WriteString(file, "Mode=\r\n")
		io.WriteString(file, "Vid=\r\n")
		io.WriteString(file, "FolderType=Generic\r\n")
	*/

	// 以下在WindowsXP下测试通过
	io.WriteString(file, "[.ShellClassInfo]\r\n")
	io.WriteString(file, "IconFile="+localIcoFile+"\r\n")
	io.WriteString(file, "IconIndex=0\r\n")
	io.WriteString(file, "InfoTip="+info+"\r\n")

	file.Close()

	const FILE_ATTRIBUTE_HIDDEN = 0x2
	const FILE_ATTRIBUTE_READONLY = 0x1
	const FILE_ATTRIBUTE_SYSTEM = 0x4

	win32.SetFileAttributes(dir+"/"+localIcoFile,
		FILE_ATTRIBUTE_HIDDEN|FILE_ATTRIBUTE_READONLY|FILE_ATTRIBUTE_SYSTEM)
	win32.SetFileAttributes(desktopIni,
		FILE_ATTRIBUTE_HIDDEN|FILE_ATTRIBUTE_READONLY|FILE_ATTRIBUTE_SYSTEM)
	win32.SetFileAttributes(dir, FILE_ATTRIBUTE_SYSTEM)

	updateSystemIconCache()
	return nil
}

func DesktopDir() string {
	return win32.SHGetFolderPath(win32.CSIDL_DESKTOPDIRECTORY)
}

func DocumentsDir() string {
	return win32.SHGetFolderPath(win32.CSIDL_MYDOCUMENTS)
}

func ShellOpen(x string) error {
	/*
	   		{
	   		hx_trace("[e_ui] Window::openUrl() : " + _url);
	   #ifdef HX_OS_WINDOWS
	   		return (HINSTANCE)32 < ShellExecute(imp->hWnd, L"open", _url.c_str(),L"",L"", SW_SHOW);
	   #endif

	   #ifdef HX_OS_LINUX
	   		String cmd = "xdg-open \'" + String(_url) + "\'"; // note: xdg-open may require perl-uri
	   		return 0 == system(cmd.toUtf8());
	   #endif
	   	}
	*/
	return win32.ShellExecute(0, "open", x, "", "", win32.SW_SHOW)
}

//// 休眠当前线程一段时间
//// 此函数有较大误差, 不可用作精确定时
func Sleep(milliseconds int) {
	win32.Sleep(uint32(milliseconds))
}

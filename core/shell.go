package core

import (
	"fmt"
	"io"
	"os"
	"strings"
)

var resourceDir string
var workspaceDir string
var localDataDir string
var appRootDir string
var dbgToolDir string
var startUpDir string
var tmpDir string

func CopyFile(dst, src string) (err error) {
	sfi, err := os.Stat(src)
	if err != nil {
		return
	}
	if !sfi.Mode().IsRegular() {
		return fmt.Errorf("CopyFile: non-regular source file %s (%q)", sfi.Name(), sfi.Mode().String())
	}
	dfi, err := os.Stat(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		}
	} else {
		if !(dfi.Mode().IsRegular()) {
			return fmt.Errorf("CopyFile: non-regular destination file %s (%q)", dfi.Name(), dfi.Mode().String())
		}
		if os.SameFile(sfi, dfi) {
			return
		}
	}
	if err = os.Link(src, dst); err == nil {
		return
	}
	err = copyFileContents(dst, src)
	return
}

func copyFileContents(dst, src string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return
	}
	err = out.Sync()
	return
}

// 静态数据目录
// 用来存放图标, 语言包等和程序一起发布的静态数据
// 在目前的版本里, 此目录等于程序所在目录
// 但在编写代码时, 请不要用程序所在目录代替此目录, 因为以后会改变
func ResourceDir() string {
	if resourceDir == "" {
		resourceDir = AppInstallDir()
	}
	// exe目录下没有资源时, 按优先级查找
	_, err := os.Stat(resourceDir + "/icon")
	if err != nil {
		// 1. 尝试当前工作目录 (go run 开发模式)
		cwd, cwdErr := os.Getwd()
		if cwdErr == nil {
			if _, e := os.Stat(cwd + "/icon"); e == nil {
				resourceDir = cwd
				Debug(`use resource dir (cwd) = "` + resourceDir + `"`)
				return resourceDir
			}
		}
		// 2. 尝试 GOPATH/bin
		s := os.Getenv("GOPATH")
		if s != "" {
			_, err := os.Stat(s + "/bin/icon")
			if err == nil {
				resourceDir = s + "/bin"
				Debug(`use resource dir = "` + resourceDir + `"`)
			}
		}
	}
	return resourceDir
}

// 本机用户设置目录
// 用来存放和工区无关用户设置, 例如用户习惯, 窗口位置等
// 在目前的版本里, 此目录等于ExeFileDir()+"/local"
// 但在编写代码时, 请不要用(ExeFileDir()+"/local")代替此目录, 因为以后会改变
func LocalDataDir() string {
	if localDataDir == "" {
		localDataDir = ExeFileDir() + "/local"
		os.Mkdir(localDataDir, os.ModeDir)
	}
	return localDataDir
}

// 程序安装目录
// 在目前的版本里, 此目录等于程序所在目录
// 但在编写代码时, 请不要用程序所在目录代替此目录, 因为以后会改变
func AppInstallDir() string {
	if appRootDir == "" {
		appRootDir = ExeFileDir()
	}
	return appRootDir
}

// 本机工作区目录
// 在目前的版本里, 此目录等于ExeFileDir()+"/workspace"
// 但在编写代码时, 请不要用(ExeFileDir()+"/workspace")代替此目录, 因为以后会改变
func WorkspaceDir() string {
	if workspaceDir == "" {
		workspaceDir = ExeFileDir() + "/workspace"
		os.Mkdir(workspaceDir, os.ModeDir)
	}
	return workspaceDir
}

// 调试工具所在目录
// 在目前的版本里, 此目录等于ExeFileDir()+"/dbgtool"
func DbgToolDir() string {
	if dbgToolDir == "" {
		dbgToolDir = ExeFileDir() + "/dbgtool"
	}
	return dbgToolDir
}

// 软件使用的临时文件目录
// 在目前的版本里, 此目录等于 os.TempDir()+"/" + AppShortName()
func TempDir() string {
	if tmpDir == "" {
		tmpDir = strings.Replace(os.TempDir()+"/"+AppShortName(), `\`, `/`, -1)
		os.Mkdir(tmpDir, os.ModeDir)
	}
	return tmpDir
}

//go:build ignore

package main

import (
	"silk/core"
	//	"silk/graph"
	"silk/gui"
	_ "silk/prop"
)

func createFrame() *gui.Frame {
	f := gui.NewFrameWindow()
	f.SetUuid(core.GenUuid())
	f.SetTitle("新框架窗口")

	menu := f.MainMenu()

	btnNewFrame := menu.AddButton1("新框架窗口", nil)
	btnNewFrame.Action().BindFunc0(func() { createFrame() })

	//_, btnViews := menu.AddSubMenu("工具视图", nil, nil)
	//btnViews.SetSubPopupCallback(updateViewsMenu)

	gui.AddToolViewSubMenu(menu)

	btnSaveSession := menu.AddButton1("保存布局", nil)
	btnSaveSession.Action().BindFunc0(func() {
		gui.SaveSessionFile(core.LocalDataDir() + "/session.cml")
	})

	btnLoadSession := menu.AddButton1("加载布局", nil)
	btnLoadSession.Action().BindFunc0(func() {
		gui.LoadSessionFile(core.LocalDataDir() + "/session.cml")
	})

	f.SetVisible(true)

	return f
}

func main() {
	defer core.Close()
	core.SetLogLevel("trace")

	mainFrame := createFrame()
	mainFrame.SetUuidStr("d5ffc927-fcd3-4fc2-b7bc-f6e081b88d1c")
	mainFrame.SetTitle("分析平台")
	mainFrame.SetClosedCallback(func(*gui.Frame) {
		core.Quit()
	})
	mainFrame.Show()

	//	dock := mainFrame.MainDock()

	//for i := 0; i < 10; i++ {
	//	p := gui.NewButton()
	//	dock.AddView(p)
	//	//dock.CloseView(p)
	//}

	gui.LoadSessionFile(core.LocalDataDir() + "/session.cml")
	core.EventLoop()
}

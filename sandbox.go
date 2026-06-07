//go:build ignore

package main

import (
	"fmt"

	"silk/core"
	"silk/ged"
	"silk/graph"
	"silk/gui"
	//	"silk/prop"
	_ "silk/prop"
)

const mainFrameUuid = "d5ffc927-fcd3-4fc2-b7bc-f6e081b88d1c"

func recursiveAddMenu(m *gui.Menu, depth int) {
	for i := 0; i < 10; i++ {
		if depth > 1 && i%3 == depth%3 {
			sm, _ := m.AddSubMenu(fmt.Sprintf("Depth %d, Item %d", depth, i),
				gui.LoadIcon("pencil"), nil)
			recursiveAddMenu(sm, depth-1)
		} else {
			m.AddButton1(fmt.Sprintf("Depth %d, Item %d", depth, i), nil)
		}
	}
}

func onOpen() {
	s := gui.OpenFileDialog()
	core.Trace(s)
}

func recursAddRectItems(parent graph.IItem, depth int) {
	x, y, w, h := parent.Bounds()

	c0 := graph.NewRectItem()
	c0.SetBounds(x+5, y+5, w/2-10, h/2-10)
	c0.SetParent(parent)

	c1 := graph.NewRectItem()
	c1.SetBounds(x+w/2+5, y+5, w/2-10, h/2-10)
	c1.SetParent(parent)

	c2 := graph.NewRectItem()
	c2.SetBounds(x+w/2+5, y+h/2+5, w/2-10, h/2-10)
	c2.SetParent(parent)

	//	c3 := graph.NewRectItem()
	//	c3.SetBounds(x+5, y+h/2+5, w/2-10, h/2-10)
	//	c3.SetParent(parent)

	if depth > 0 {
		recursAddRectItems(c0, depth-1)
		recursAddRectItems(c1, depth-1)
		recursAddRectItems(c2, depth-1)
		//		recursAddRectItems(c3, depth-1)
	}

}

func main() {
	//.SetAppName("测试用沙盒", "sandbox")
	//core.SetStackTraceLevel("trace")
	//	core.SetDebug(true)

	//core.SaveSetting("/aa/bb", "test222222222222")
	//	var tmp string
	//	core.LoadSetting("/aa/bb", &tmp)
	//	core.Trace("tmp=", tmp)

	//doc := core.NewTdoc()
	//doc.WriteAttr("测试1", "测试1的值")
	//doc.WriteAttr("测试2", "测试2的值")
	//doc.WriteAttr("测试3", "测试3的值")
	//doc.WriteAttr("测试4", "测试4的值")

	//core.SaveSetting("/ee/ff", doc)
	//core.ReadIni("/ee/ff", doc)

	//core.Trace(doc)

	mainFrame := gui.NewFrameWindow()
	menu := mainFrame.MainMenu()

	sub1, _ := menu.AddSubMenu("File", gui.LoadIcon("globe"), nil)
	sub1.AddButton1("New", gui.LoadIcon("document"))
	sub1.AddButton1("Open", gui.LoadIcon("folder"))
	sub1.AddWidget(gui.NewSeparator())
	sub1.AddButton1("Exit", gui.LoadIcon("exit"))

	sub2, _ := menu.AddSubMenu("Help", nil, nil)
	sub2.AddButton1("About", nil)
	//p.Action().SetCallback(func(int, float64) {})
	//p.Action().SetCallback(&menu)
	recursiveAddMenu(sub2, 3)

	menu.AddWidget(gui.NewSeparator())
	btn1 := gui.NewButton1("folder", gui.LoadIcon("folder"))
	//btn1.SetTextVisible(true)
	menu.AddWidget(btn1)
	btn1.Action().BindFunc0(onOpen)

	btnNewFrame := menu.AddButton1("新框架窗口", nil)
	btnNewFrame.Action().BindFunc0(func() {
		f := gui.NewFrameWindow()
		f.SetUuid(core.NewUuid())
		f.SetTitle("新框架窗口")
		f.SetVisible(true)
	})

	//btn2 := NewButton("globe", LoadIcon("globe"))
	//btn2.SetTextVisible(true)
	//menu.AddWidget(btn2)

	//btn3 := NewButton("clipboard", LoadIcon("clipboard"))
	//btn3.SetTextVisible(true)
	//menu.AddWidget(btn3)

	btnSaveSession := menu.AddButton1("保存布局", nil)
	btnSaveSession.Action().BindFunc0(func() {
		gui.SaveSessionFile(core.LocalDataDir() + "/session.cml")
	})

	btnLoadSession := menu.AddButton1("加载布局", nil)
	btnLoadSession.Action().BindFunc0(func() {
		gui.LoadSessionFile(core.LocalDataDir() + "/session.cml")
	})

	space := gui.NewSpace(false, true)
	space.SetMinSize(32)
	menu.AddWidget(space)

	edit := gui.NewEdit()
	edit.SetText("ABCD中文汉字什么")
	edit.SetBounds(100, 100, 200, 24)
	menu.AddWidget(edit)

	edit1 := gui.NewEdit()
	var s string
	for i := 0; i < 10; i++ {
		s += "所有投标产品须提供主流电商销售地址，市场上无完全一致型号的，需提供一款同系列同配置（或略低于投报产品配置）的产品销售网址。投标产品投标价格不得高于电商销售价格。ABCD中文汉字什么\nMarkdown is a plain text formatting syntax[5] designed so that it can optionally be converted to HTML using a tool by the same name. \nMarkdown is popularly used as format for readme files, or for writing messages in online discussion forums, or in text editors for the quick creation of rich text documents.\n"
		s += fmt.Sprint(i) + "\n"
	}

	edit1.SetWrap(true)
	edit1.SetAlwaysShowSelection(true)
	edit1.SetText(s)

	mainFrame.SuggestDocDock().AddView(edit1)
	//edit1.SetRect(100, 100, 200, 24)
	//	mainFrame.SetMainWidget(edit1)

	//propSheet := prop.NewPropertySheet()

	//mainFrame.MainDock().AddView(propSheet)
	//mainFrame.SetVisible(false)
	//mainFrame.AttachWindow(WtForm)
	mainFrame.SetTitle("沙盒")
	//mainFrame.Window().SetIcon(LoadIcon("clipboard"))

	//frame1 := NewFrame()
	//frame1.AttachWindow(WtForm)
	//frame1.Window().SetIcon(LoadIcon("propsheet"))
	//frame1.SetSize(280, 500)

	//mainFrame.SuggestToolDock().AddView(propSheet)

	//frame1.Show()

	wmView := ged.NewGedView()
	wmView.AddStandardTools()
	//wmScene := wmap.NewScene()
	//wmView.SetScene(wmScene)
	//	wmView.SetPropertyView(propSheet)
	gui.AddToolViewSubMenu(menu)

	rect := graph.NewRectItem()
	rect.SetBounds(25, 25, 79, 67)
	rect.SetParent(wmView.Scene())
	rect.SetLocalCoord(true)

	rect1 := graph.NewRectItem()
	rect1.SetBounds(15, 15, 45, 36)
	rect1.SetParent(rect)
	//core.Trace(wmScene)

	//graphView.AttachWindow(WtForm)

	recursAddRectItems(wmView.Scene(), 3)

	//	wmView.SigBindPropSheet().Bind(propSheet.SetObjects)

	dbgGraphTree := graph.NewDbgTreeView()
	dbgGraphTree.SetRootItems(wmView.Scene().Children())
	dbgGraphTree.SetRootIndent(true)
	mainFrame.SuggestToolDock().AddView(dbgGraphTree)

	form := gui.NewForm()
	list := gui.NewListWidget()
	list.SetParent(form)
	list.SetBounds(20, 20, 200, 300)
	list.SetIconVisible(true)
	list.SetCheckBoxVisible(true)

	list.Append(gui.ListItem{"aaaa", gui.LoadIcon("goble"), false, nil})
	list.Append(gui.ListItem{"测试", gui.LoadIcon("clipboard"), true, nil})
	list.Append(gui.ListItem{"中文", gui.LoadIcon("folder"), false, nil})
	list.Append(gui.ListItem{"Zoo", gui.LoadIcon("form"), true, nil})
	list.Append(gui.ListItem{"Woody", nil, false, nil})
	list.Append(gui.ListItem{"Crazy", gui.LoadIcon("exit"), false, nil})

	list.SetItem(0, gui.ListItem{"dfafafaf", gui.LoadIcon("apple"), true, "dffff"})

	cb := gui.NewComboBox()
	cb.SetParent(form)
	cb.SetBounds(250, 20, 200, 30)

	cb.Append(gui.ListItem{"aaaa", gui.LoadIcon("goble"), false, nil})
	cb.Append(gui.ListItem{"测试", gui.LoadIcon("clipboard"), true, nil})
	cb.Append(gui.ListItem{"中文", gui.LoadIcon("folder"), false, nil})
	cb.Append(gui.ListItem{"Zoo", gui.LoadIcon("form"), true, nil})
	cb.Append(gui.ListItem{"Woody", nil, false, nil})
	cb.Append(gui.ListItem{"Crazy", gui.LoadIcon("exit"), false, nil})

	mainFrame.SuggestDocDock().AddView(wmView)
	mainFrame.SuggestDocDock().AddView(form)

	mainFrame.SetUuidStr(mainFrameUuid)

	gui.SetDefaultFrame(mainFrame)

	//mainFrame.SetClosingCallback(func(*Frame) {
	//	SaveSessionFile(core.LocalDataDir() + "/session.cml")
	//})

	mainFrame.SetClosedCallback(func(*gui.Frame) {
		core.Quit()
	})

	//LoadSessionFile(core.LocalDataDir() + "/session.cml")

	mainFrame.SetVisible(true)

	core.EventLoop()
}

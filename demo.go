//go:build ignore

package main

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func main() {
	f := gui.NewFrameWindow()
	f.SetUuidStr("demo-001")
	f.SetTitle("Silk UI Framework - Widget Gallery")
	gui.SetDefaultFrame(f)

	menu := f.MainMenu()
	fileMenu, _ := menu.AddSubMenu("文件", nil, nil)
	btnExit := fileMenu.AddButton1("退出", nil)
	btnExit.Action().BindFunc0(func() { core.Quit() })

	viewMenu, _ := menu.AddSubMenu("视图", nil, nil)
	viewMenu.AddButton1("切换暗色主题", nil).Action().BindFunc0(func() {
		if gui.CurrentThemeMode() == gui.ThemeDark {
			gui.SetThemeMode(gui.ThemeLight)
		} else {
			gui.SetThemeMode(gui.ThemeDark)
		}
		for _, w := range gui.AllWindows() {
			w.Update()
		}
	})

	helpMenu, _ := menu.AddSubMenu("帮助", nil, nil)
	helpMenu.AddButton1("关于", nil).Action().BindFunc0(func() {
		gui.ShowMessageDialog(f, "关于", "Silk UI Framework\n62 Widgets\n跨平台 Go UI 框架")
	})

	// Create form
	form := gui.NewForm()
	form.SetTitle("Widget Gallery")

	th := gui.Theme()
	bold := paint.NewFont(th.Font.Family(), 16, true, false)
	section := func(parent gui.IWidget, text string, x, y float64) {
		l := gui.NewLabel(text)
		l.SetFont(bold)
		l.SetTextColor(paint.Color{50, 80, 150, 255})
		l.SetParent(parent)
		l.SetBounds(x, y, 500, 22)
	}

	// ─── Left Column (x=20, width=480) ───
	lx := 20.0
	colW := 460.0
	y := 10.0

	// Title
	title := gui.NewLabel("Silk UI Framework — 62 Widget 组件展示")
	title.SetFont(paint.NewFont(th.Font.Family(), 20, true, false))
	title.SetAlign(gui.AlignCenter)
	title.SetParent(form)
	title.SetBounds(lx, y, colW, 30)
	y += 40

	// ── Buttons ──
	section(form, "按钮 (Button)", lx, y)
	y += 26
	btn1 := gui.NewButton1("普通按钮", nil)
	btn1.SetParent(form)
	btn1.SetBounds(lx, y, 100, 28)
	btn2 := gui.NewButton1("图标按钮", gui.LoadIcon("document"))
	btn2.SetParent(form)
	btn2.SetBounds(lx+110, y, 110, 28)
	btn2.Action().BindFunc0(func() {
		gui.ShowMessageDialog(f, "提示", "按钮被点击了！")
	})
	btn3 := gui.NewButton1("退出", gui.LoadIcon("exit"))
	btn3.SetParent(form)
	btn3.SetBounds(lx+230, y, 80, 28)
	btn3.Action().BindFunc0(func() { core.Quit() })
	y += 36

	// ── CheckBox ──
	section(form, "复选框 (CheckBox)", lx, y)
	y += 26
	cb1 := gui.NewCheckBox()
	cb1.SetText("选项 A")
	cb1.SetParent(form)
	cb1.SetBounds(lx, y, 110, 22)
	cb2 := gui.NewCheckBox()
	cb2.SetText("选项 B (已选)")
	cb2.SetChecked(true)
	cb2.SetParent(form)
	cb2.SetBounds(lx+120, y, 140, 22)
	cb3 := gui.NewCheckBox()
	cb3.SetText("禁用")
	cb3.SetEnabled(false)
	cb3.SetParent(form)
	cb3.SetBounds(lx+270, y, 80, 22)
	y += 30

	// ── RadioButton ──
	section(form, "单选按钮 (RadioButton)", lx, y)
	y += 26
	group := gui.NewRadioGroup()
	rb1 := gui.NewRadioButton("选择 1", group)
	rb1.SetParent(form)
	rb1.SetBounds(lx, y, 90, 22)
	rb1.SetChecked(true)
	rb2 := gui.NewRadioButton("选择 2", group)
	rb2.SetParent(form)
	rb2.SetBounds(lx+100, y, 90, 22)
	rb3 := gui.NewRadioButton("选择 3", group)
	rb3.SetParent(form)
	rb3.SetBounds(lx+200, y, 90, 22)
	y += 30

	// ── Edit + Label ──
	section(form, "文本输入 & 标签", lx, y)
	y += 26
	lbl := gui.NewLabel("用户名:")
	lbl.SetParent(form)
	lbl.SetBounds(lx, y, 60, 24)
	edit := gui.NewEdit()
	edit.SetText("请输入文本")
	edit.SetParent(form)
	edit.SetBounds(lx+65, y, 220, 24)
	y += 32

	// ── ProgressBar + Slider ──
	section(form, "进度条 & 滑块", lx, y)
	y += 26
	pb := gui.NewProgressBar()
	pb.SetValue(0.65)
	pb.SetShowText(true)
	pb.SetParent(form)
	pb.SetBounds(lx, y, 300, 22)
	sliderLabel := gui.NewLabel("值: 65")
	sliderLabel.SetParent(form)
	sliderLabel.SetBounds(lx+310, y, 80, 22)
	y += 28
	slider := gui.NewSlider(0, 100)
	slider.SetValue(65)
	slider.SetParent(form)
	slider.SetBounds(lx, y, 300, 22)
	slider.SetValueChangedCallback(func(_ interface{}, v float64) {
		pb.SetValue(v / 100)
		sliderLabel.SetText(fmt.Sprintf("值: %.0f", v))
	})
	y += 30

	// ── SpinBox + ComboBox ──
	section(form, "数字输入 & 下拉选择", lx, y)
	y += 26
	spin := gui.NewSpinBox()
	spin.SetRange(0, 100)
	spin.SetValue(42)
	spin.SetParent(form)
	spin.SetBounds(lx, y, 100, 26)
	combo := gui.NewComboBox()
	combo.SetParent(form)
	combo.SetBounds(lx+120, y, 160, 28)
	combo.Append(gui.ListItem{Text: "北京"})
	combo.Append(gui.ListItem{Text: "上海"})
	combo.Append(gui.ListItem{Text: "深圳"})
	combo.Append(gui.ListItem{Text: "中文测试"})
	y += 36

	// ── GroupBox ──
	section(form, "分组框 (GroupBox)", lx, y)
	y += 26
	gb := gui.NewGroupBox("用户信息")
	gb.SetParent(form)
	gb.SetBounds(lx, y, 300, 70)
	gbLabel := gui.NewLabel("姓名: 张三    年龄: 28    城市: 北京")
	gbLabel.SetParent(gb)
	gbLabel.SetBounds(10, 20, 280, 40)
	y += 80

	// ── New Widgets ──
	section(form, "新增控件 (New)", lx, y)
	y += 26

	// ToggleSwitch
	ts := gui.NewToggleSwitch()
	ts.SetText("开关")
	ts.SetParent(form)
	ts.SetBounds(lx, y, 100, 24)
	ts2 := gui.NewToggleSwitch()
	ts2.SetText("已开启")
	ts2.SetChecked(true)
	ts2.SetParent(form)
	ts2.SetBounds(lx+110, y, 110, 24)
	y += 30

	// SearchBox
	sb2 := gui.NewSearchBox()
	sb2.SetPlaceholder("搜索组件...")
	sb2.SetParent(form)
	sb2.SetBounds(lx, y, 220, 30)
	y += 36

	// Tag
	tag1 := gui.NewTag("Go")
	tag1.SetParent(form)
	tag1.SetBounds(lx, y, 50, 24)
	tag2 := gui.NewTag("GUI")
	tag2.SetColor(paint.Color{52, 168, 83, 255})
	tag2.SetParent(form)
	tag2.SetBounds(lx+58, y, 50, 24)
	tag3 := gui.NewTag("跨平台")
	tag3.SetColor(paint.Color{234, 67, 53, 255})
	tag3.SetCloseable(true)
	tag3.SetParent(form)
	tag3.SetBounds(lx+116, y, 80, 24)
	y += 32

	// ─── Right Column (x=500, width=260) ───
	rx := 500.0
	rw := 260.0
	ry := 50.0

	section(form, "框架信息", rx, ry)
	ry += 26
	info := gui.NewLabel(fmt.Sprintf(
		"平台: macOS (GLFW+OpenGL)\n"+
			"组件: 62 Widgets\n"+
			"渲染: Cairo 2D\n"+
			"DPI: %.0f\n"+
			"主题: %s",
		gui.ScreenDpi(),
		func() string {
			if gui.CurrentThemeMode() == gui.ThemeDark {
				return "暗色"
			}
			return "亮色"
		}()))
	info.SetParent(form)
	info.SetBounds(rx, ry, rw, 100)
	ry += 110

	// Card in right column
	card := gui.NewCard("快速统计")
	card.SetParent(form)
	card.SetBounds(rx, ry, rw, 100)
	cardLabel := gui.NewLabel("控件: 62\n图表: 5 种\n布局: 9 种\n动画: 12 种缓动")
	cardLabel.SetParent(card)
	cardLabel.SetBounds(12, 32, rw-24, 60)
	ry += 110

	// Avatar
	av := gui.NewAvatar()
	av.SetText("Silk")
	av.SetParent(form)
	av.SetBounds(rx, ry, 40, 40)
	avLabel := gui.NewLabel("Silk Framework\nv2.3.0")
	avLabel.SetParent(form)
	avLabel.SetBounds(rx+50, ry+4, 200, 36)
	ry += 50

	// Badge
	bdg := gui.NewBadge()
	bdg.SetCount(7)
	bdg.SetParent(form)
	bdg.SetBounds(rx, ry, 50, 30)
	bdgLabel := gui.NewLabel("未读通知")
	bdgLabel.SetParent(form)
	bdgLabel.SetBounds(rx+60, ry+4, 100, 24)
	ry += 44

	// Spinner (busy indicator)
	spinner := gui.NewSpinner()
	spinner.SetParent(form)
	spinner.SetBounds(rx, ry, 28, 28)
	spinLabel := gui.NewLabel("加载中...")
	spinLabel.SetParent(form)
	spinLabel.SetBounds(rx+36, ry+4, 120, 24)
	ry += 38

	// Pagination
	pager := gui.NewPagination()
	pager.SetTotalPages(20)
	pager.SetCurrentPage(6)
	pager.SetParent(form)
	pager.SetBounds(rx, ry, rw, 32)

	// Status bar
	status := gui.NewStatusBar()
	status.ShowMessage("Silk UI Framework Demo — 62 组件展示 | 布局引擎 v2")
	f.SetStatusBar(status)

	f.SuggestDocDock().AddView(form)
	f.SetClosedCallback(func(*gui.Frame) { core.Quit() })
	if w := f.Window(); w != nil {
		w.SetSize(800, 680)
		w.MoveToCenter()
	}
	f.Show()
	core.EventLoop()
}

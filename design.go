package main

import (
	"silk/core"
	"silk/ged"
	"silk/graph"
	"silk/gui"
	"silk/prop"
	"fmt"
	"math"
	"os"
	"sort"
	"os/exec"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Recent files tracking
// ---------------------------------------------------------------------------

const maxRecentFiles = 10

var recentFiles []string

func recentFilesPath() string {
	return core.LocalDataDir() + "/recent_files.txt"
}

func loadRecentFiles() {
	data, err := os.ReadFile(recentFilesPath())
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			recentFiles = append(recentFiles, line)
		}
	}
	if len(recentFiles) > maxRecentFiles {
		recentFiles = recentFiles[:maxRecentFiles]
	}
}

func saveRecentFiles() {
	_ = os.WriteFile(recentFilesPath(), []byte(strings.Join(recentFiles, "\n")), 0644)
}

func addRecentFile(filename string) {
	abs, err := filepath.Abs(filename)
	if err == nil {
		filename = abs
	}
	// Remove duplicates
	filtered := make([]string, 0, len(recentFiles))
	for _, f := range recentFiles {
		if f != filename {
			filtered = append(filtered, f)
		}
	}
	recentFiles = append([]string{filename}, filtered...)
	if len(recentFiles) > maxRecentFiles {
		recentFiles = recentFiles[:maxRecentFiles]
	}
	saveRecentFiles()
}

// ---------------------------------------------------------------------------
// Current view helper
// ---------------------------------------------------------------------------

func currentGedView() *ged.GedView {
	f := gui.DefaultFrame()
	view, _ := f.CurrentDocView()
	gv, _ := view.(*ged.GedView)
	return gv
}

// ---------------------------------------------------------------------------
// File operations
// ---------------------------------------------------------------------------

func onFileNew() {
	tmpl := ged.ShowNewProjectDialog(gui.DefaultFrame())

	p := ged.NewGedView()
	f := gui.DefaultFrame()
	f.SuggestDocDock().AddView(p)

	if tmpl != nil {
		ged.ApplyTemplate(p.GedScene(), tmpl)
	}

	// Bind the code panel to the new view
	if codePanel != nil {
		codePanel.BindGedView(p)
	}

	updateWindowTitle("")
}

// onFileNewBlank creates a new blank GedView without showing the template
// dialog. Used at startup to initialize the workspace.
func onFileNewBlank() {
	p := ged.NewGedView()
	f := gui.DefaultFrame()
	f.SuggestDocDock().AddView(p)
	if codePanel != nil {
		codePanel.BindGedView(p)
	}
}

func onFileOpen() {
	filename := gui.OpenFileDialog()
	if filename == "" {
		return
	}
	openDesignFile(filename)
}

func openDesignFile(filename string) {
	originalFilename := filename

	// Check for autosave recovery
	if recovery := ged.CheckRecovery(filename); recovery != "" {
		// Offer to recover from autosave
		if gui.ShowConfirmDialog(gui.DefaultFrame(), "恢复", "发现自动保存的文件，是否恢复？") {
			filename = recovery
		} else {
			ged.CleanupAutosave(filename)
		}
	}

	p := ged.NewGedView()
	if err := p.OpenFile(filename); err != nil {
		core.Error(err)
		return
	}

	// If we loaded from autosave, restore the original filename so saves
	// go to the correct file instead of the .autosave file
	if filename != originalFilename && p.GedScene() != nil {
		p.GedScene().SetFilename(originalFilename)
	}

	f := gui.DefaultFrame()
	f.SuggestDocDock().AddView(p)
	addRecentFile(originalFilename)
	updateWindowTitle(originalFilename)
	// Bind the code panel to the new view
	if codePanel != nil {
		codePanel.BindGedView(p)
	}
	// Start auto-saver for this scene
	ged.InitAutoSaver()
	if ged.GlobalAutoSaver != nil && p.GedScene() != nil {
		ged.GlobalAutoSaver.Start(p.GedScene())
	}
}

func onFileClose() {
	f := gui.DefaultFrame()
	view, dock := f.CurrentDocView()
	if view != nil {
		dock.PromptSaveCloseView(view)
	}
}

func onFileSave() {
	f := gui.DefaultFrame()
	view, _ := f.CurrentDocView()
	if ia, ok := view.(interface {
		Save() bool
	}); ok {
		if ia.Save() {
			// Update title after successful save
			if gv := currentGedView(); gv != nil && gv.GedScene() != nil {
				scene := gv.GedScene()
				_ = scene // title already updated inside Save()
			}
		}
	}
}

func onFileSaveAs() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	filename := gui.SaveFileDialog()
	if filename == "" {
		return
	}
	scene := gv.GedScene()
	if scene == nil {
		return
	}
	doc := scene.SaveDesign()
	if err := doc.SaveFile(filename); err != nil {
		core.Error(err)
		return
	}
	addRecentFile(filename)
	updateWindowTitle(filename)
}

func onSaveAsTemplate() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	scene := gv.GedScene()
	if scene == nil {
		return
	}
	name, ok := gui.ShowInputBox(gui.DefaultFrame(), nil, "保存为模板", "模板名称:", scene.FormTitle())
	if !ok || name == "" {
		return
	}
	desc, ok := gui.ShowInputBox(gui.DefaultFrame(), nil, "保存为模板", "模板描述:", "")
	if !ok {
		return
	}
	if err := ged.SaveAsTemplate(scene, name, desc); err != nil {
		gui.ShowMessageDialog(gui.DefaultFrame(), "保存模板失败", err.Error())
		return
	}
	if sb := gui.DefaultFrame().StatusBar(); sb != nil {
		sb.ShowMessage(fmt.Sprintf("模板 \"%s\" 已保存", name))
	}
}

func updateWindowTitle(filename string) {
	f := gui.DefaultFrame()
	if filename != "" {
		base := filepath.Base(filename)
		f.SetTitle(fmt.Sprintf("Silk Designer - %s", base))
	} else {
		f.SetTitle("Silk Designer - 界面设计器")
	}
	if win := f.Window(); win != nil {
		win.SetTitle(f.Title())
	}
}

// ---------------------------------------------------------------------------
// Edit operations
// ---------------------------------------------------------------------------

func onUndo() {
	if gv := currentGedView(); gv != nil {
		gv.Scene().UndoStack().Undo()
	}
}

func onRedo() {
	if gv := currentGedView(); gv != nil {
		gv.Scene().UndoStack().Redo()
	}
}

func onDelete() {
	if gv := currentGedView(); gv != nil {
		sel := gv.Selection()
		for _, item := range sel.ItemList() {
			item.Detach()
		}
		sel.Clear()
	}
}

func onSelectAll() {
	if gv := currentGedView(); gv != nil {
		for _, item := range gv.Scene().Children() {
			gv.Selection().Add(item)
		}
	}
}

func onCut() {
	if gv := currentGedView(); gv != nil {
		gv.CopySelected()
		gv.DeleteSelectedItems()
	}
}

func onCopy() {
	if gv := currentGedView(); gv != nil {
		gv.CopySelected()
	}
}

func onPaste() {
	if gv := currentGedView(); gv != nil {
		gv.PasteItems()
	}
}

// ---------------------------------------------------------------------------
// Alignment operations (Feature 1)
// ---------------------------------------------------------------------------

func alignLeft() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	items := gv.Selection().ItemList()
	if len(items) < 2 {
		return
	}
	minX := items[0].X()
	for _, it := range items[1:] {
		if it.X() < minX {
			minX = it.X()
		}
	}
	cmd := graph.NewMoveCommand()
	for _, it := range items {
		cmd.AddItem(it, minX, it.Y())
	}
	gv.Scene().PushCommand(cmd)
	gv.Self().Update()
}

func alignRight() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	items := gv.Selection().ItemList()
	if len(items) < 2 {
		return
	}
	maxRight := items[0].X() + items[0].Width()
	for _, it := range items[1:] {
		r := it.X() + it.Width()
		if r > maxRight {
			maxRight = r
		}
	}
	cmd := graph.NewMoveCommand()
	for _, it := range items {
		cmd.AddItem(it, maxRight-it.Width(), it.Y())
	}
	gv.Scene().PushCommand(cmd)
	gv.Self().Update()
}

func alignTop() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	items := gv.Selection().ItemList()
	if len(items) < 2 {
		return
	}
	minY := items[0].Y()
	for _, it := range items[1:] {
		if it.Y() < minY {
			minY = it.Y()
		}
	}
	cmd := graph.NewMoveCommand()
	for _, it := range items {
		cmd.AddItem(it, it.X(), minY)
	}
	gv.Scene().PushCommand(cmd)
	gv.Self().Update()
}

func alignBottom() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	items := gv.Selection().ItemList()
	if len(items) < 2 {
		return
	}
	maxBottom := items[0].Y() + items[0].Height()
	for _, it := range items[1:] {
		b := it.Y() + it.Height()
		if b > maxBottom {
			maxBottom = b
		}
	}
	cmd := graph.NewMoveCommand()
	for _, it := range items {
		cmd.AddItem(it, it.X(), maxBottom-it.Height())
	}
	gv.Scene().PushCommand(cmd)
	gv.Self().Update()
}

func alignCenterH() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	items := gv.Selection().ItemList()
	if len(items) < 2 {
		return
	}
	// Find the average center X
	var sumCx float64
	for _, it := range items {
		sumCx += it.X() + it.Width()/2
	}
	cx := sumCx / float64(len(items))
	cmd := graph.NewMoveCommand()
	for _, it := range items {
		cmd.AddItem(it, math.Round(cx-it.Width()/2), it.Y())
	}
	gv.Scene().PushCommand(cmd)
	gv.Self().Update()
}

func alignCenterV() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	items := gv.Selection().ItemList()
	if len(items) < 2 {
		return
	}
	// Find the average center Y
	var sumCy float64
	for _, it := range items {
		sumCy += it.Y() + it.Height()/2
	}
	cy := sumCy / float64(len(items))
	cmd := graph.NewMoveCommand()
	for _, it := range items {
		cmd.AddItem(it, it.X(), math.Round(cy-it.Height()/2))
	}
	gv.Scene().PushCommand(cmd)
	gv.Self().Update()
}

func distributeH() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	items := gv.Selection().ItemList()
	if len(items) < 3 {
		return
	}
	// Sort by X position
	sorted := make([]graph.IItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].X() < sorted[j].X()
	})
	first := sorted[0].X()
	last := sorted[len(sorted)-1].X()
	step := (last - first) / float64(len(sorted)-1)
	cmd := graph.NewMoveCommand()
	for i, it := range sorted {
		cmd.AddItem(it, math.Round(first+float64(i)*step), it.Y())
	}
	gv.Scene().PushCommand(cmd)
	gv.Self().Update()
}

func distributeV() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	items := gv.Selection().ItemList()
	if len(items) < 3 {
		return
	}
	// Sort by Y position
	sorted := make([]graph.IItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Y() < sorted[j].Y()
	})
	first := sorted[0].Y()
	last := sorted[len(sorted)-1].Y()
	step := (last - first) / float64(len(sorted)-1)
	cmd := graph.NewMoveCommand()
	for i, it := range sorted {
		cmd.AddItem(it, it.X(), math.Round(first+float64(i)*step))
	}
	gv.Scene().PushCommand(cmd)
	gv.Self().Update()
}

// ---------------------------------------------------------------------------
// Layout operations (Feature 3: Qt Creator-style layout tools)
// ---------------------------------------------------------------------------

// applyHBoxLayout wraps selected widgets into a horizontal box container.
func applyHBoxLayout() {
	applyContainerLayout("gui.HBox", "hbox")
}

// applyVBoxLayout wraps selected widgets into a vertical box container.
func applyVBoxLayout() {
	applyContainerLayout("gui.VBox", "vbox")
}

// applyGridLayout wraps selected widgets into a grid layout container.
func applyGridLayout() {
	applyContainerLayout("gui.GridLayout", "gridLayout")
}

// applyContainerLayout wraps the selected widgets into the given container type.
func applyContainerLayout(factoryName, defaultName string) {
	gv := currentGedView()
	if gv == nil {
		return
	}
	items := gv.Selection().ItemList()
	if len(items) < 2 {
		if sb := gui.DefaultFrame().StatusBar(); sb != nil {
			sb.ShowMessage("请至少选择2个控件来应用布局")
		}
		return
	}

	// Calculate bounding box of selected items
	minX, minY := items[0].X(), items[0].Y()
	maxX, maxY := items[0].X()+items[0].Width(), items[0].Y()+items[0].Height()
	for _, it := range items[1:] {
		if it.X() < minX {
			minX = it.X()
		}
		if it.Y() < minY {
			minY = it.Y()
		}
		if it.X()+it.Width() > maxX {
			maxX = it.X() + it.Width()
		}
		if it.Y()+it.Height() > maxY {
			maxY = it.Y() + it.Height()
		}
	}

	// Create the container at bounding box position with some padding
	container, err := ged.NewFakeWidgetFromFactory(factoryName)
	if err != nil {
		if sb := gui.DefaultFrame().StatusBar(); sb != nil {
			sb.ShowMessage("无法创建布局容器: " + err.Error())
		}
		return
	}
	padding := 2.0
	container.SetBounds(minX-padding, minY-padding, (maxX-minX)+padding*2, (maxY-minY)+padding*2)
	container.SetWidgetName(defaultName)
	container.Layout()

	// Remove the original items from the scene
	for _, it := range items {
		it.Detach()
	}

	// Add the container to the scene
	cmd := graph.NewAddCommand()
	cmd.AddItem(container, gv.Scene())
	gv.Scene().PushCommand(cmd)

	gv.Selection().Clear()
	gv.Selection().Add(container)
	gv.Self().Update()

	if sb := gui.DefaultFrame().StatusBar(); sb != nil {
		sb.ShowMessage(fmt.Sprintf("已将 %d 个控件包装到 %s 布局中", len(items), defaultName))
	}
}

// breakLayout removes a layout container and places its child widgets back
// on the canvas at their current positions. If the selected item is a layout
// container, it is removed and replaced by a status message.
func breakLayout() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	items := gv.Selection().ItemList()
	if len(items) != 1 {
		if sb := gui.DefaultFrame().StatusBar(); sb != nil {
			sb.ShowMessage("请选择一个布局容器来打破布局")
		}
		return
	}

	item := items[0]
	fw, ok := item.(*ged.FakeWidget)
	if !ok {
		return
	}

	// Check if this is a layout/container widget
	factory := fw.WidgetFactoryName()
	isLayout := false
	for _, name := range []string{"gui.HBox", "gui.VBox", "gui.GridLayout", "gui.FormLayout"} {
		if factory == name {
			isLayout = true
			break
		}
	}
	if !isLayout {
		if sb := gui.DefaultFrame().StatusBar(); sb != nil {
			sb.ShowMessage("选中的控件不是布局容器")
		}
		return
	}

	// Remove the layout container
	item.Detach()
	gv.Selection().Clear()
	gv.Self().Update()

	if sb := gui.DefaultFrame().StatusBar(); sb != nil {
		sb.ShowMessage("布局已打破")
	}
}

// ---------------------------------------------------------------------------
// Preview & export
// ---------------------------------------------------------------------------

func onPreview() {
	f := gui.DefaultFrame()
	view, _ := f.CurrentDocView()
	gedView, _ := view.(*ged.GedView)
	if gedView == nil {
		return
	}
	scene := gedView.GedScene()
	if scene == nil {
		return
	}
	design := scene.Generate()
	if design == nil {
		return
	}
	form := design.Form()
	if form == nil {
		return
	}

	// Ensure the form has a reasonable default size
	w, h := form.Size()
	if w < 320 {
		w = 320
	}
	if h < 240 {
		h = 240
	}
	form.SetSize(w, h)

	form.AttachWindow(gui.WtForm)
	form.Show()
	if win := form.Window(); win != nil {
		win.MoveToCenter()
	}
}

func onExportGuiGv() {
	gui.DbgExportGuiGv(true)
}

func onExportCode() {
	f := gui.DefaultFrame()
	view, _ := f.CurrentDocView()
	gedView, _ := view.(*ged.GedView)
	if gedView == nil {
		return
	}
	filename := gui.SaveFileDialog()
	if filename == "" {
		return
	}
	opts := ged.CodeGenOptions{
		PackageName: "main",
		TypeName:    gedView.GedScene().FormTitle() + "UI",
	}
	err := gedView.GedScene().GenerateCodeFile(filename, opts)
	if err != nil {
		core.Error(err)
	}
}

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

func onRun() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	scene := gv.GedScene()
	if scene == nil {
		return
	}

	// Export to temp file
	tmpDir := os.TempDir() + "/silk_run"
	os.MkdirAll(tmpDir, 0755)
	goFile := tmpDir + "/main.go"

	opts := ged.CodeGenOptions{PackageName: "main", TypeName: scene.FormTitle() + "UI"}
	err := scene.GenerateCodeFile(goFile, opts)
	if err != nil {
		gui.ShowMessageDialog(gui.DefaultFrame(), "导出错误", err.Error())
		return
	}

	// Compile
	cmd := exec.Command("go", "build", "-o", tmpDir+"/app", goFile)
	cmd.Env = append(os.Environ(), "CGO_CFLAGS=-I/opt/homebrew/Cellar/cairo/1.18.4/include")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Show output in BuildOutput panel if available
		if buildOutput != nil {
			buildOutput.SetOutput(string(output))
			// Switch to the output tab in the right dock
			if rightDockRef != nil {
				idx := rightDockRef.IndexOfView(buildOutput)
				if idx >= 0 {
					rightDockRef.SetActiveIndex(idx)
				}
			}
			// Pass compile errors to the active CodeEditor for inline markers
			if editorTabs != nil {
				if editor := editorTabs.ActiveEditor(); editor != nil {
					editor.SetErrors(buildOutput.ErrorMap())
				}
			}
		} else {
			gui.ShowMessageDialog(gui.DefaultFrame(), "编译错误", string(output))
		}
		return
	}

	// Build succeeded -- show success message and clear error markers
	if buildOutput != nil {
		buildOutput.SetOutput("Build successful\n")
	}
	if editorTabs != nil {
		if editor := editorTabs.ActiveEditor(); editor != nil {
			editor.ClearErrors()
		}
	}

	// Run
	runCmd := exec.Command(tmpDir + "/app")
	runCmd.Start()

	// Update status bar
	if sb := gui.DefaultFrame().StatusBar(); sb != nil {
		sb.ShowMessage("程序已启动: " + tmpDir + "/app")
	}
}

// ---------------------------------------------------------------------------
// About dialog
// ---------------------------------------------------------------------------

func onAbout() {
	ged.ShowAboutDialog(gui.DefaultFrame())
}

// ---------------------------------------------------------------------------
// Menu bar construction
// ---------------------------------------------------------------------------

func createMenuBar(mainFrame *gui.Frame) {
	mainMenu := mainFrame.MainMenu()

	// ---- 文件 ----
	fileMenu, _ := mainMenu.AddSubMenu("文件", nil, nil)

	btnNew := fileMenu.AddButton1("新建    Ctrl+N", gui.LoadIcon("document"))
	btnNew.Action().BindFunc0(onFileNew)

	btnOpen := fileMenu.AddButton1("打开", gui.LoadIcon("folder"))
	btnOpen.Action().BindFunc0(onFileOpen)

	// ---- 最近文件 ----
	recentMenu, recentBtn := fileMenu.AddSubMenu("最近文件", nil, nil)
	recentBtn.SetSubPopupCallback(func(btn gui.IButton) {
		sub := btn.SubPopup().(*gui.Menu)
		sub.Clear()
		if len(recentFiles) == 0 {
			empty := sub.AddButton1("(无)", nil)
			_ = empty
			return
		}
		for _, rf := range recentFiles {
			path := rf
			base := filepath.Base(path)
			b := sub.AddButton1(base, nil)
			b.Action().BindFunc0(func() {
				openDesignFile(path)
			})
		}
	})
	_ = recentMenu

	fileMenu.AddSeparator()

	btnSave := fileMenu.AddButton1("保存    Ctrl+S", gui.LoadIcon("save"))
	btnSave.Action().BindFunc0(onFileSave)

	btnSaveAs := fileMenu.AddButton1("另存为...", nil)
	btnSaveAs.Action().BindFunc0(onFileSaveAs)

	btnSaveTemplate := fileMenu.AddButton1("保存为模板...", nil)
	btnSaveTemplate.Action().BindFunc0(onSaveAsTemplate)

	fileMenu.AddSeparator()

	btnClose := fileMenu.AddButton1("关闭", gui.LoadIcon("close"))
	btnClose.Action().BindFunc0(onFileClose)

	// ---- 编辑 ----
	editMenu, _ := mainMenu.AddSubMenu("编辑", nil, nil)

	btnUndo := editMenu.AddButton1("撤销    Ctrl+Z", nil)
	btnUndo.Action().BindFunc0(onUndo)

	btnRedo := editMenu.AddButton1("重做    Ctrl+Y", nil)
	btnRedo.Action().BindFunc0(onRedo)

	editMenu.AddSeparator()

	btnDelete := editMenu.AddButton1("删除    Delete", nil)
	btnDelete.Action().BindFunc0(onDelete)

	btnSelectAll := editMenu.AddButton1("全选    Ctrl+A", nil)
	btnSelectAll.Action().BindFunc0(onSelectAll)

	editMenu.AddWidget(gui.NewSeparator())
	btnCut := editMenu.AddButton1("剪切    Ctrl+X", nil)
	btnCut.Action().BindFunc0(onCut)
	btnCopy := editMenu.AddButton1("复制    Ctrl+C", nil)
	btnCopy.Action().BindFunc0(onCopy)
	btnPaste := editMenu.AddButton1("粘贴    Ctrl+V", nil)
	btnPaste.Action().BindFunc0(onPaste)

	// ---- 视图 ----
	viewMenu, viewBtn := mainMenu.AddSubMenu("视图", nil, nil)
	viewBtn.SetSubPopupCallback(func(btn gui.IButton) {
		sub := btn.SubPopup().(*gui.Menu)
		sub.Clear()
		for _, p := range mainFrame.ToolViewActions() {
			sub.AddActionButton(p)
		}

		sub.AddSeparator()

		label := "切换暗色主题"
		if gui.CurrentThemeMode() == gui.ThemeDark {
			label = "切换亮色主题"
		}
		btnTheme := sub.AddButton1(label, nil)
		btnTheme.Action().BindFunc0(func() {
			if gui.CurrentThemeMode() == gui.ThemeDark {
				gui.SetThemeMode(gui.ThemeLight)
			} else {
				gui.SetThemeMode(gui.ThemeDark)
			}
			for _, win := range gui.AllWindows() {
				win.Update()
			}
		})
	})
	_ = viewMenu

	// ---- 工具 ----
	toolMenu, _ := mainMenu.AddSubMenu("工具", nil, nil)

	btnPreview := toolMenu.AddButton1("预览    Ctrl+R", gui.LoadIcon("preview"))
	btnPreview.Action().BindFunc0(onPreview)

	btnExportCode := toolMenu.AddButton1("导出Go代码", nil)
	btnExportCode.Action().BindFunc0(onExportCode)

	toolMenu.AddSeparator()

	btnExportGuiGv := toolMenu.AddButton1("导出界面关系图", nil)
	btnExportGuiGv.Action().BindFunc0(onExportGuiGv)

	toolMenu.AddWidget(gui.NewSeparator())
	btnRun := toolMenu.AddButton1("▶ 运行    F5", gui.LoadIcon("preview"))
	btnRun.Action().BindFunc0(onRun)

	// ---- 帮助 ----
	helpMenu, _ := mainMenu.AddSubMenu("帮助", nil, nil)

	btnAbout := helpMenu.AddButton1("关于", nil)
	btnAbout.Action().BindFunc0(onAbout)

	btnShortcuts := helpMenu.AddButton1("快捷键参考", nil)
	btnShortcuts.Action().BindFunc0(func() {
		mainFrame.ShowToolView("ged.ShortcutsPanel")
	})

	// ---- 布局 (Layout) ----
	layoutMenu, _ := mainMenu.AddSubMenu("布局", nil, nil)
	layoutMenu.AddButton1("应用水平布局 (HBox)", nil).Action().BindFunc0(applyHBoxLayout)
	layoutMenu.AddButton1("应用垂直布局 (VBox)", nil).Action().BindFunc0(applyVBoxLayout)
	layoutMenu.AddButton1("应用网格布局 (Grid)", nil).Action().BindFunc0(applyGridLayout)
	layoutMenu.AddWidget(gui.NewSeparator())
	layoutMenu.AddButton1("打破布局", nil).Action().BindFunc0(breakLayout)

	// ---- 排列 (Arrange) ----
	arrangeMenu, _ := mainMenu.AddSubMenu("排列", nil, nil)

	arrangeMenu.AddButton1("左对齐    Alt+L", nil).Action().BindFunc0(alignLeft)
	arrangeMenu.AddButton1("右对齐    Alt+R", nil).Action().BindFunc0(alignRight)
	arrangeMenu.AddButton1("顶对齐    Alt+T", nil).Action().BindFunc0(alignTop)
	arrangeMenu.AddButton1("底对齐    Alt+B", nil).Action().BindFunc0(alignBottom)
	arrangeMenu.AddWidget(gui.NewSeparator())
	arrangeMenu.AddButton1("水平居中    Alt+C", nil).Action().BindFunc0(alignCenterH)
	arrangeMenu.AddButton1("垂直居中    Alt+M", nil).Action().BindFunc0(alignCenterV)
	arrangeMenu.AddWidget(gui.NewSeparator())
	arrangeMenu.AddButton1("水平分布    Alt+H", nil).Action().BindFunc0(distributeH)
	arrangeMenu.AddButton1("垂直分布    Alt+V", nil).Action().BindFunc0(distributeV)

	// ---- Mode buttons: 设计/代码/分屏 ----
	mainMenu.AddWidget(gui.NewSeparator())
	btnDesign := mainMenu.AddButton1("设计", nil)
	btnCode := mainMenu.AddButton1("代码", nil)
	btnSplit := mainMenu.AddButton1("分屏", nil)

	btnDesign.Action().BindFunc0(func() {
		switchViewMode(ViewModeDesign)
	})
	btnCode.Action().BindFunc0(func() {
		switchViewMode(ViewModeCode)
	})
	btnSplit.Action().BindFunc0(func() {
		switchViewMode(ViewModeSplit)
	})
}

// ---------------------------------------------------------------------------
// Panels construction
// ---------------------------------------------------------------------------

// welcomeScreen is the start page shown on launch, removed after first action.
var welcomeScreen *ged.WelcomeScreen

// codePanel is the shared code editing panel, accessible from menu actions.
var codePanel *ged.CodePanel

// buildOutput is the shared build output panel for showing compile results.
var buildOutput *ged.BuildOutput

// fileExplorer is the project file browser panel.
var fileExplorer *ged.FileExplorer

// editorTabs is the multi-tab code editor.
var editorTabs *ged.EditorTabs

// globalSearch is the search-across-files panel.
var globalSearch *ged.GlobalSearchPanel

// codeOutline is the code outline/symbol tree panel.
var codeOutline *ged.CodeOutlinePanel

// terminalPanel is the integrated shell terminal.
var terminalPanel *ged.TerminalPanel

// widgetHelp is the context-sensitive widget documentation panel.
var widgetHelp *ged.WidgetHelp

// centerDock holds the design canvas and code panel tabs.
var centerDock *gui.Dock

// rightDockRef holds a reference to the right dock for switching tabs.
var rightDockRef *gui.Dock

// leftDockRef holds a reference to the left dock for mode switching.
var leftDockRef *gui.Dock

// modeSelector is the mode switching sidebar widget.
var modeSelector *ged.ModeSelector

// ViewMode represents the current editor display mode.
type ViewMode int

const (
	ViewModeDesign ViewMode = iota
	ViewModeCode
	ViewModeSplit
)

var currentViewMode = ViewModeDesign

// switchViewMode switches between design-only, code-only, and split view modes.
func switchViewMode(mode ViewMode) {
	if centerDock == nil || codePanel == nil {
		return
	}
	currentViewMode = mode

	switch mode {
	case ViewModeDesign:
		// Activate the GedView tab (index 0)
		centerDock.SetActiveIndex(0)
	case ViewModeCode:
		// Activate the CodePanel tab (index 1)
		idx := centerDock.IndexOfView(codePanel)
		if idx >= 0 {
			centerDock.SetActiveIndex(idx)
		}
	case ViewModeSplit:
		// For split mode: activate design tab (both tabs are still accessible via tab bar)
		// Show a status message to indicate both views are available
		centerDock.SetActiveIndex(0)
		if sb := gui.DefaultFrame().StatusBar(); sb != nil {
			sb.ShowMessage("分屏模式: 使用标签页切换设计/代码视图")
		}
	}
	updateStatusBarInfo()
}

// bindWidgetHelpTo wires the WidgetHelp panel to receive selection updates
// from the given GedView. Called whenever the active canvas view changes.
func bindWidgetHelpTo(gv *ged.GedView) {
	if gv == nil || widgetHelp == nil {
		return
	}
	gv.AddSelectionCallback(func(items []graph.IItem) {
		if len(items) == 1 {
			if fw, ok := items[0].(*ged.FakeWidget); ok {
				widgetHelp.SetWidget(fw)
				return
			}
		}
		widgetHelp.SetWidget(nil)
	})
}

// dismissWelcomeScreen removes the welcome screen from the center dock.
func dismissWelcomeScreen() {
	if welcomeScreen != nil && centerDock != nil {
		idx := centerDock.IndexOfView(welcomeScreen)
		if idx >= 0 {
			centerDock.RemoveView(welcomeScreen)
		}
		welcomeScreen = nil
	}
}

func createPanels(mainFrame *gui.Frame) {
	onFileNewBlank() // startup: create an initial blank view without the dialog

	if dock, ok := mainFrame.SuggestDocDock().(*gui.Dock); ok {
		centerDock = dock

		// ─── Welcome screen as first tab in center dock ───
		welcomeScreen = ged.NewWelcomeScreen()
		welcomeScreen.SetRecentFiles(recentFiles)
		welcomeScreen.SetNewProjectCallback(func() {
			dismissWelcomeScreen()
			// The initial blank GedView already exists from onFileNewBlank().
			// Just activate it and show the template dialog for customization.
			if gv := currentGedView(); gv != nil {
				// Activate the existing GedView tab
				if centerDock != nil {
					idx := centerDock.IndexOfView(gv)
					if idx >= 0 {
						centerDock.SetActiveIndex(idx)
					}
				}
				// Show template dialog so user can pick a template
				tmpl := ged.ShowNewProjectDialog(gui.DefaultFrame())
				if tmpl != nil {
					ged.ApplyTemplate(gv.GedScene(), tmpl)
				}
			} else {
				onFileNew()
			}
			updateStatusBarInfo()
		})
		welcomeScreen.SetOpenFileCallback(func() {
			dismissWelcomeScreen()
			onFileOpen()
		})
		welcomeScreen.SetOpenRecentCallback(func(path string) {
			dismissWelcomeScreen()
			openDesignFile(path)
		})
		dock.AddView(welcomeScreen)
		// Show welcome tab by default
		if widx := dock.IndexOfView(welcomeScreen); widx >= 0 {
			dock.SetActiveIndex(widx)
		}

		// ─── Left dock: Widget Palette (Design mode) ───
		leftDockI := dock.SplitNewDock(true, false)
		widgetList := ged.NewWidgetList()
		leftDockI.AddView(widgetList)
		if ld, ok := leftDockI.(*gui.Dock); ok {
			leftDockRef = ld
		}

		// ─── File explorer as a separate panel in the same left dock ───
		// In Design mode we show WidgetList tab; in Code mode we show FileExplorer tab
		fileExplorer = ged.NewFileExplorer()
		fileExplorer.SetRootDir(".")
		fileExplorer.SigFileOpen(func(path string) {
			// Switch to code mode and open file
			if editorTabs != nil {
				editorTabs.OpenFile(path)
			}
			// Auto-switch to code mode when opening a file
			if modeSelector != nil && modeSelector.CurrentMode() != ged.ModeEdit {
				modeSelector.SetMode(ged.ModeEdit)
			}
			// Show editor tabs in center dock
			if centerDock != nil {
				idx := centerDock.IndexOfView(editorTabs)
				if idx >= 0 {
					centerDock.SetActiveIndex(idx)
				}
			}
		})
		leftDockI.AddView(fileExplorer)

		// ─── Global search panel in left dock ───
		globalSearch = ged.NewGlobalSearchPanel()
		globalSearch.SetRootDir(".")
		globalSearch.SigOpen(func(path string, line int) {
			if editorTabs != nil {
				editorTabs.OpenFileAtLine(path, line-1) // convert 1-based to 0-based
			}
			if modeSelector != nil && modeSelector.CurrentMode() != ged.ModeEdit {
				modeSelector.SetMode(ged.ModeEdit)
			}
			if centerDock != nil {
				idx := centerDock.IndexOfView(editorTabs)
				if idx >= 0 {
					centerDock.SetActiveIndex(idx)
				}
			}
		})
		leftDockI.AddView(globalSearch)

		// ─── Right dock: Properties + Code + Tree + Build Output ───
		rightDockI := dock.SplitNewDock(false, false)
		propSheet := prop.NewPropertySheet()
		_ = mainFrame.ToolViewActions()
		rightDockI.AddView(propSheet)

		codePanel = ged.NewCodePanel()
		rightDockI.AddView(codePanel)

		dbgTree := graph.NewDbgTreeView()
		rightDockI.AddView(dbgTree)

		buildOutput = ged.NewBuildOutput()
		rightDockI.AddView(buildOutput)

		// ─── Code outline panel in right dock ───
		codeOutline = ged.NewCodeOutlinePanel()
		codeOutline.SetNavigateCallback(func(line int) {
			if editorTabs != nil {
				if editor := editorTabs.ActiveEditor(); editor != nil {
					editor.ScrollToLine(line)
				}
			}
		})
		rightDockI.AddView(codeOutline)

		// ─── Widget help panel in right dock ───
		widgetHelp = ged.NewWidgetHelp()
		rightDockI.AddView(widgetHelp)

		// ─── Integrated terminal panel in right dock ───
		terminalPanel = ged.NewTerminalPanel()
		rightDockI.AddView(terminalPanel)

		if rightDock, ok := rightDockI.(*gui.Dock); ok {
			rightDockRef = rightDock
			rightDock.SetActiveIndex(0)
		}

		// ─── Multi-tab code editor in center dock ───
		editorTabs = ged.NewEditorTabs()
		centerDock.AddView(editorTabs)

		// ─── Bind code panel to canvas for design↔code sync ───
		if gv := currentGedView(); gv != nil {
			codePanel.BindGedView(gv)
			bindWidgetHelpTo(gv)
		}
		refreshTreeForCurrentView(dbgTree)

		// ─── Tab change listener: rebind dependent panels ───
		centerDock.SetTabChangedCallback(func(idx int) {
			view := centerDock.ActiveView()
			if gv, ok := view.(*ged.GedView); ok {
				// Rebind code panel
				if codePanel != nil {
					codePanel.BindGedView(gv)
				}
				// Rebind widget-help selection listener
				bindWidgetHelpTo(gv)
				// Update object inspector tree
				scene := gv.GedScene()
				if scene != nil {
					dbgTree.SetRootItems(scene.Children())
				}
				// Update window title from scene title (set to filename base on load)
				title := ""
				if scene != nil {
					title = scene.Title()
				}
				updateWindowTitle(title)
				// Update status bar
				if sb := mainFrame.StatusBar(); sb != nil {
					count := gv.Selection().Count()
					if count == 0 {
						sb.ShowMessage("Ready")
					} else {
						sb.ShowMessage(fmt.Sprintf("Selected: %d items", count))
					}
				}
			}
			// Update code outline when switching to editor tabs
			if codeOutline != nil && editorTabs != nil {
				if editor := editorTabs.ActiveEditor(); editor != nil {
					codeOutline.SetEditor(editor)
				}
			}
		})

		// ─── Mode switching config ───
		// Design mode: show widget palette tab, hide file explorer tab
		// Code mode: show file explorer tab, hide widget palette tab
		ged.GlobalModeConfig = ged.ModeConfig{
			WidgetListDock:   leftDockRef,
			DesignDock:       centerDock,
			PropertyDock:     rightDockRef,
		}

		// Wire mode switching to control left dock tabs + center dock tabs
		ged.OnDesignMode = func() {
			// Left dock: show widget palette tab
			if leftDockRef != nil {
				idx := leftDockRef.IndexOfView(widgetList)
				if idx >= 0 {
					leftDockRef.SetActiveIndex(idx)
				}
			}
			// Center dock: show design canvas
			if centerDock != nil {
				// Find the first GedView (design canvas)
				for i := 0; i < centerDock.ViewCount(); i++ {
					if _, ok := centerDock.ViewAtIndex(i).(*ged.GedView); ok {
						centerDock.SetActiveIndex(i)
						break
					}
				}
			}
			// Right dock: show property sheet
			if rightDockRef != nil {
				rightDockRef.SetActiveIndex(0) // property sheet
			}
		}
		ged.OnEditMode = func() {
			// Left dock: show file explorer tab
			if leftDockRef != nil {
				idx := leftDockRef.IndexOfView(fileExplorer)
				if idx >= 0 {
					leftDockRef.SetActiveIndex(idx)
				}
			}
			// Center dock: show editor tabs
			if centerDock != nil {
				idx := centerDock.IndexOfView(editorTabs)
				if idx >= 0 {
					centerDock.SetActiveIndex(idx)
				}
			}
			// Right dock: show build output
			if rightDockRef != nil {
				idx := rightDockRef.IndexOfView(buildOutput)
				if idx >= 0 {
					rightDockRef.SetActiveIndex(idx)
				}
			}
		}

		// Start in design mode — show widget palette
		if leftDockRef != nil {
			idx := leftDockRef.IndexOfView(widgetList)
			if idx >= 0 {
				leftDockRef.SetActiveIndex(idx)
			}
		}
	}

	// Create mode selector sidebar on the far left
	modeSelector = ged.NewModeSelector()
	modeSelector.SigModeChanged(func(mode ged.DesignerMode) {
		switch mode {
		case ged.ModeDesign:
			ged.SwitchToDesignMode()
			if sb := mainFrame.StatusBar(); sb != nil {
				sb.ShowMessage("已切换到设计模式")
			}
		case ged.ModeEdit:
			ged.SwitchToEditMode()
			if sb := mainFrame.StatusBar(); sb != nil {
				sb.ShowMessage("已切换到代码模式")
			}
		}
	})
	mainFrame.SetLeftSidebar(modeSelector)
}

// refreshTreeForCurrentView sets up the DbgTreeView to show items from the
// current GedView, and re-syncs whenever the scene changes.
func refreshTreeForCurrentView(dbgTree *graph.DbgTreeView) {
	syncTree := func() {
		gv := currentGedView()
		if gv == nil {
			dbgTree.SetRootItems(nil)
			return
		}
		scene := gv.GedScene()
		if scene == nil {
			dbgTree.SetRootItems(nil)
			return
		}
		dbgTree.SetRootItems(scene.Children())
	}

	// Initial sync
	syncTree()

	// Hook into the GedView lifecycle: refresh tree when selection changes
	// (which implies items may have been added/removed).
	// We also refresh on item attach/detach via a periodic idle check
	// by wrapping into the existing SigSelectionChanged.
	if gv := currentGedView(); gv != nil {
		origCb := gv.Selection() // ensure selection is initialized
		_ = origCb
		gv.SigItemAttached(func(s interface{}, item, parent graph.IItem) {
			syncTree()
		})
		gv.SigItemDetached(func(s interface{}, item, parent graph.IItem) {
			syncTree()
		})
	}
}

// ---------------------------------------------------------------------------
// Toolbar construction (Qt Creator-style quick access toolbar)
// ---------------------------------------------------------------------------

func createToolBar(mainFrame *gui.Frame) {
	tb := gui.NewToolBar()

	// File operations (icon-only, like Qt Creator)
	tb.AddAction("", gui.LoadIcon("document"), onFileNew)
	tb.AddAction("", gui.LoadIcon("folder"), onFileOpen)
	tb.AddAction("", gui.LoadIcon("save"), onFileSave)

	tb.AddSeparator()

	// Edit operations
	tb.AddAction("", gui.LoadIcon("edit-undo"), onUndo)
	tb.AddAction("", gui.LoadIcon("edit-redo"), onRedo)

	tb.AddSeparator()

	// Preview & Run
	tb.AddAction("", gui.LoadIcon("preview"), onPreview)
	tb.AddAction("", gui.LoadIcon("run"), onRun)

	tb.AddSeparator()

	// Alignment tools
	tb.AddAction("", gui.LoadIcon("align-left"), alignLeft)
	tb.AddAction("", gui.LoadIcon("align-center"), alignCenterH)
	tb.AddAction("", gui.LoadIcon("align-right"), alignRight)

	mainFrame.SetToolBar(tb)
}

// ---------------------------------------------------------------------------
// Status bar construction
// ---------------------------------------------------------------------------

// statusZoomLabel shows the current zoom level (e.g. "100%").
var statusZoomLabel *gui.Label

// statusWidgetCountLabel shows the number of widgets on the canvas.
var statusWidgetCountLabel *gui.Label

// statusModeLabel shows the current mode (design/code).
var statusModeLabel *gui.Label

// statusInfoLabel shows selected widget type and position.
var statusInfoLabel *gui.Label

func createStatusBar(mainFrame *gui.Frame) {
	statusBar := gui.NewStatusBar()
	statusBar.ShowMessage("Ready")
	statusBar.SetParent(mainFrame)
	mainFrame.SetStatusBar(statusBar)

	// Create permanent indicator labels on the right side
	statusModeLabel = gui.NewLabel("设计")
	statusModeLabel.SetSize(50, 20)
	statusBar.AddPermanentWidget(statusModeLabel)

	statusWidgetCountLabel = gui.NewLabel("0 widgets")
	statusWidgetCountLabel.SetSize(70, 20)
	statusBar.AddPermanentWidget(statusWidgetCountLabel)

	statusZoomLabel = gui.NewLabel("100%")
	statusZoomLabel.SetSize(50, 20)
	statusBar.AddPermanentWidget(statusZoomLabel)

	statusInfoLabel = gui.NewLabel("")
	statusInfoLabel.SetSize(120, 20)
	statusBar.AddPermanentWidget(statusInfoLabel)
}

// updateStatusBarInfo refreshes the permanent status bar indicators.
func updateStatusBarInfo() {
	gv := currentGedView()

	// Zoom level
	if statusZoomLabel != nil {
		zoom := 100.0
		if gv != nil {
			zoom = gv.ZoomFactor() * 100
		}
		statusZoomLabel.SetText(fmt.Sprintf("%.0f%%", zoom))
	}

	// Widget count
	if statusWidgetCountLabel != nil {
		count := 0
		if gv != nil && gv.Scene() != nil {
			count = len(gv.Scene().Children())
		}
		statusWidgetCountLabel.SetText(fmt.Sprintf("%d widgets", count))
	}

	// Current mode
	if statusModeLabel != nil {
		switch currentViewMode {
		case ViewModeDesign:
			statusModeLabel.SetText("设计")
		case ViewModeCode:
			statusModeLabel.SetText("代码")
		case ViewModeSplit:
			statusModeLabel.SetText("分屏")
		}
	}

	// Selected widget info
	if statusInfoLabel != nil && gv != nil {
		sel := gv.Selection()
		if sel != nil && sel.Count() == 1 {
			item := sel.ItemList()[0]
			if fw, ok := item.(*ged.FakeWidget); ok {
				name := fw.WidgetFactoryName()
				if idx := strings.LastIndex(name, "."); idx >= 0 {
					name = name[idx+1:]
				}
				statusInfoLabel.SetText(fmt.Sprintf("%s (%.0f,%.0f)", name, fw.X(), fw.Y()))
			}
		} else if sel != nil && sel.Count() > 1 {
			statusInfoLabel.SetText(fmt.Sprintf("%d selected", sel.Count()))
		} else {
			statusInfoLabel.SetText("")
		}
	}
}

// ---------------------------------------------------------------------------
// Main entry point (Feature 3: organized structure)
// ---------------------------------------------------------------------------

func main() {
	loadRecentFiles()

	// Register F5 run callback and Ctrl+R preview callback
	ged.RunCallback = onRun
	ged.PreviewCallback = onPreview

	// Double-click on canvas widget → switch to code panel tab
	ged.ShowCodePanelCallback = func() {
		if rightDockRef == nil || codePanel == nil {
			return
		}
		idx := rightDockRef.IndexOfView(codePanel)
		if idx >= 0 {
			rightDockRef.SetActiveIndex(idx)
		}
		codePanel.Focus()
	}

	// Register alignment keyboard shortcuts (Alt+key)
	ged.AlignLeftCallback = alignLeft
	ged.AlignRightCallback = alignRight
	ged.AlignTopCallback = alignTop
	ged.AlignBottomCallback = alignBottom
	ged.AlignCenterHCallback = alignCenterH
	ged.AlignCenterVCallback = alignCenterV
	ged.DistributeHCallback = distributeH
	ged.DistributeVCallback = distributeV

	mainFrame := gui.NewFrameWindow()
	mainFrame.SetUuidStr("8a075fb7-6924-4615-9e8c-5aff13e560e9")
	mainFrame.SetTitle("Silk Designer - 界面设计器")

	gui.SetDefaultFrame(mainFrame)

	createMenuBar(mainFrame)
	createToolBar(mainFrame)
	createPanels(mainFrame)
	createStatusBar(mainFrame)

	// Wire status bar updates into selection changes of the initial view
	if gv := currentGedView(); gv != nil {
		gv.AddSelectionCallback(func(items []graph.IItem) {
			updateStatusBarInfo()
		})
	}
	updateStatusBarInfo()

	// Register Ctrl+1 / Ctrl+2 mode switching shortcuts
	ged.SwitchToDesignCallback = func() { modeSelector.SetMode(ged.ModeDesign) }
	ged.SwitchToEditCallback = func() { modeSelector.SetMode(ged.ModeEdit) }

	// Register Ctrl+P quick file open
	qo := ged.GetQuickOpen()
	qo.SetRootDir(".")
	qo.SetOpenCallback(func(path string) {
		if editorTabs != nil {
			editorTabs.OpenFile(path)
		}
		if modeSelector != nil && modeSelector.CurrentMode() != ged.ModeEdit {
			modeSelector.SetMode(ged.ModeEdit)
		}
		if centerDock != nil {
			idx := centerDock.IndexOfView(editorTabs)
			if idx >= 0 {
				centerDock.SetActiveIndex(idx)
			}
		}
	})
	ged.QuickOpenCallback = func() {
		qo.Show()
	}

	mainFrame.SetClosedCallback(func(*gui.Frame) { core.Quit() })
	mainFrame.SetClosingCallback(func(*gui.Frame) {
		// Persist the frame/dock layout first (used by the existing
		// LoadSessionFile call). Then capture our own higher-level session
		// state (open editor tabs, active file, mode, window size).
		gui.SaveSessionFile(core.LocalDataDir() + "/design_session.cml")
		saveDesignerSession(mainFrame)
	})
	gui.LoadSessionFile(core.LocalDataDir() + "/design_session.cml")

	// Load the previously-persisted designer session (open files, mode,
	// window size). If none exists yet, LoadSession returns a zero value
	// and applySession does nothing, so first-run behavior is unchanged.
	prev := ged.LoadSession()

	// Restore window size first — createPanels is already done.
	if win := mainFrame.Window(); win != nil {
		w, h := 1400.0, 900.0
		if prev.WindowWidth > 400 && prev.WindowHeight > 300 {
			w = float64(prev.WindowWidth)
			h = float64(prev.WindowHeight)
		}
		win.SetSize(w, h)
		win.MoveToCenter()
	}

	// Restore open editor tabs + active file + mode.
	applyDesignerSession(prev)

	mainFrame.Show()

	core.EventLoop()
}

// saveDesignerSession captures the current designer state and writes it
// to disk. Called from the window-closing callback.
func saveDesignerSession(mainFrame *gui.Frame) {
	state := ged.SessionState{
		LastMode: 0,
	}
	if modeSelector != nil && modeSelector.CurrentMode() == ged.ModeEdit {
		state.LastMode = 1
	}
	if editorTabs != nil {
		state.OpenFiles = editorTabs.OpenFilePaths()
		state.ActiveFile = editorTabs.ActiveFilePath()
	}
	if len(recentFiles) > 0 {
		state.LastProject = recentFiles[0]
	}
	if win := mainFrame.Window(); win != nil {
		_, _, w, h := win.Bounds()
		state.WindowWidth = int(w)
		state.WindowHeight = int(h)
	}
	if err := ged.SaveSession(state); err != nil {
		core.Warn("SaveSession: ", err)
	}
}

// applyDesignerSession restores the previously-persisted working state.
// Safe to call with a zero SessionState — nothing will be restored.
func applyDesignerSession(state ged.SessionState) {
	// Re-open files in the editor tabs in saved order.
	if editorTabs != nil {
		for _, path := range state.OpenFiles {
			if path == "" {
				continue
			}
			if _, err := os.Stat(path); err != nil {
				continue
			}
			editorTabs.OpenFile(path)
		}
		if state.ActiveFile != "" {
			editorTabs.ActivateFile(state.ActiveFile)
		}
	}

	// Switch to last-used mode. Defer to the mode selector so its sidebar
	// icon state stays in sync with the dock visibility.
	if modeSelector != nil {
		if state.LastMode == 1 {
			modeSelector.SetMode(ged.ModeEdit)
		} else {
			modeSelector.SetMode(ged.ModeDesign)
		}
	}
}

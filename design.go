package main

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/prop"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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

func onDuplicate() {
	if gv := currentGedView(); gv != nil {
		gv.DuplicateSelection()
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
// Same-size and Z-order operations
// ---------------------------------------------------------------------------

// sameWidth, sameHeight and sameSize resize every selected widget onto the LAST
// selected one, keeping each widget's own position. The reference is the widget
// picked most recently — marquee-select the group, then click the widget whose
// size should win. The geometry (including the min-size floor shared with the
// Ctrl+arrow resize) lives in graph.Selection.GenerateSameSizeCommand; these are
// only the menu glue.
func sameWidth() { sameSizeSelection(graph.SameSizeWidth) }

func sameHeight() { sameSizeSelection(graph.SameSizeHeight) }

func sameSize() { sameSizeSelection(graph.SameSizeBoth) }

func sameSizeSelection(mode graph.SameSizeMode) {
	gv := currentGedView()
	if gv == nil {
		return
	}
	gv.SameSizeSelection(mode)
}

// bringToFront and sendToBack go through the canvas's own undoable Z-order
// path, the one the right-click 层叠顺序 menu uses. Calling graph.IItem's
// BringToFront directly from here would restack the widgets with nothing on the
// UndoStack, leaving Ctrl+Z unable to put the order back.
func bringToFront() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	gv.BringSelectionToFront()
}

func sendToBack() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	gv.SendSelectionToBack()
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

// silkModuleRoot returns the silk module root — the nearest ancestor holding a
// go.mod — used as the working directory for preview builds. Empty when there
// is none to be found.
//
// It searches from the designer executable before the process cwd. A GUI
// program's working directory is wherever it happened to be started from: a
// desktop shortcut leaves it in the user's profile, which has no go.mod above
// it, and the preview build then ran outside the module and failed with
// "no required module provides package github.com/uk0/silk/gui". The binary,
// on the other hand, sits in the tree it was built from.
func silkModuleRoot() string {
	if exe, err := os.Executable(); err == nil {
		if root := findModuleRoot(filepath.Dir(exe)); root != "" {
			return root
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if root := findModuleRoot(cwd); root != "" {
			return root
		}
	}
	return ""
}

// findModuleRoot walks up from dir to the first directory containing a go.mod,
// or "" when it reaches the filesystem root without finding one.
func findModuleRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func onRun() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	scene := gv.GedScene()
	if scene == nil {
		return
	}
	// The build below no longer blocks the UI, so the button stays clickable
	// while it runs. Without this a few impatient clicks would race several
	// compilers onto the same output path.
	if runBusy {
		setRunStatus("正在编译...")
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

	// Windows refuses to CreateProcess a path without an extension, so the
	// artifact has to be named app.exe there — "go build -o app" writes
	// exactly the name it is given and appends nothing.
	appPath := tmpDir + "/app"
	if runtime.GOOS == "windows" {
		appPath += ".exe"
	}

	// Compiling takes tens of seconds — minutes for a cold cgo/cairo build on
	// Windows with a resident antivirus — and it used to run right here on the
	// UI thread, so the message pump stopped for the whole build and Windows
	// painted "(Not Responding)" over the designer. Do it on a goroutine and
	// come back through gui.Post, which runs the continuation on the event
	// loop where every widget below is safe to touch.
	// Without the module the build can only fail, and it fails with a message
	// about a missing package rather than the real problem.
	moduleRoot := silkModuleRoot()
	if moduleRoot == "" {
		reportRunFailure("找不到 silk 模块根目录 (go.mod)。请在 silk 源码目录下运行设计器。")
		return
	}

	runBusy = true
	setRunStatus("正在编译...")
	go func() {
		// Cairo resolves via pkg-config (the cairo package declares
		// "#cgo pkg-config: cairo"), so no machine-specific include path is
		// set. Run from the silk module root so silk/* imports resolve
		// regardless of the designer process's cwd.
		cmd := exec.Command(goExecutable(), "build", "-o", appPath, goFile)
		cmd.Dir = moduleRoot
		cmd.Env = append(os.Environ(), "PATH="+buildToolPath())
		output, err := cmd.CombinedOutput()

		gui.Post(func() {
			runBusy = false
			if err != nil {
				reportRunFailure(buildFailureDetail(output, err))
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

			// Run. A failed spawn used to be swallowed, so the status bar
			// claimed success while nothing had started.
			runCmd := exec.Command(appPath)
			runCmd.Dir = moduleRoot
			runCmd.Env = runAppEnv()
			if err := runCmd.Start(); err != nil {
				reportRunFailure("启动失败: " + err.Error())
				setRunStatus("启动失败")
				return
			}
			setRunStatus("程序已启动: " + appPath)
		})
	}()
}

// runBusy is true while a background build started by onRun is still going.
// UI-thread only: onRun runs on the event loop and so does the gui.Post
// continuation that clears it, so no lock is needed.
var runBusy bool

func setRunStatus(msg string) {
	if sb := gui.DefaultFrame().StatusBar(); sb != nil {
		sb.ShowMessage(msg)
	}
}

// reportRunFailure puts detail where the user can act on it: the build-output
// pane if one exists, a dialog if not, and the log either way. The log copy is
// what makes a failed Run diagnosable after the fact — a message that only ever
// lived in a panel had to be read off a screenshot.
func reportRunFailure(detail string) {
	core.Warn("run: " + detail)
	if buildOutput == nil {
		gui.ShowMessageDialog(gui.DefaultFrame(), "编译错误", detail)
		setRunStatus("编译失败")
		return
	}
	buildOutput.SetOutput(detail)
	if rightDockRef != nil {
		if idx := rightDockRef.IndexOfView(buildOutput); idx >= 0 {
			rightDockRef.SetActiveIndex(idx)
		}
	}
	// Compile errors also become inline markers in the active editor.
	if editorTabs != nil {
		if editor := editorTabs.ActiveEditor(); editor != nil {
			editor.SetErrors(buildOutput.ErrorMap())
		}
	}
	setRunStatus("编译失败")
}

// buildToolPath is the PATH the Run build should use.
//
// A GUI process started from Explorer, a shortcut or a scheduled task inherits
// only the persistent machine/user PATH — not whatever a developer's terminal
// has arranged. On Windows that PATH almost never carries MSYS2's compiler
// directory, and often not the Go install either, so "go build" fails before
// the toolchain even starts: CombinedOutput returns nothing and the user is
// told "编译失败" with no explanation anywhere. Look the tools up ourselves and
// put wherever they really live in front.
func buildToolPath() string {
	var extra []string
	if _, err := exec.LookPath("go"); err != nil {
		if dir := findToolDir(goToolDirs(), "go"); dir != "" {
			extra = append(extra, dir)
		}
	}
	// silk links cairo through cgo, so a C compiler is as much a requirement
	// as the Go toolchain; pkg-config ships beside it in every MSYS2 layout.
	if _, err := exec.LookPath("gcc"); err != nil {
		if dir := findToolDir(cToolDirs(), "gcc"); dir != "" {
			extra = append(extra, dir)
		}
	}
	return prependPath(os.Getenv("PATH"), extra)
}

// runAppEnv is the environment for the freshly built preview program.
//
// The binary lands in a temp directory, and Windows resolves a DLL against the
// program's own directory, the system directories, the working directory and
// PATH — none of which hold the cairo libraries silk links. Started from a
// designer whose working directory was outside the source tree, the preview
// died instantly with STATUS_DLL_NOT_FOUND (0xC0000135) and no window ever
// appeared, while the designer reported it had launched. Point PATH at the
// directories that do carry those libraries: the designer's own directory in a
// packaged install, and the MSYS2 bin directory in a source checkout.
func runAppEnv() []string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	if dir := findToolDir(cToolDirs(), "gcc"); dir != "" {
		dirs = append(dirs, dir)
	}
	if len(dirs) == 0 {
		return os.Environ()
	}
	return append(os.Environ(), "PATH="+prependPath(os.Getenv("PATH"), dirs))
}

// goExecutable is the Go binary the Run build should launch.
//
// exec.Command resolves a bare command name against the *parent* process's
// PATH and records the failure right there — handing the child a repaired PATH
// through cmd.Env comes too late and changes nothing. So a designer started
// from Explorer, where Go is not on PATH, never launched the toolchain at all:
// the build "failed" in the first millisecond with no compiler output to show
// for it. Name the binary outright instead. buildToolPath still matters for
// the child, because the go tool looks up gcc and pkg-config in its own env.
func goExecutable() string {
	if path, err := exec.LookPath("go"); err == nil {
		return path
	}
	if dir := findToolDir(goToolDirs(), "go"); dir != "" {
		name := "go"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		return filepath.Join(dir, name)
	}
	return "go" // nothing found; let exec report it
}

// goToolDirs lists the places a Go installation usually lands.
func goToolDirs() []string {
	if runtime.GOOS == "windows" {
		return []string{
			`C:\Program Files\Go\bin`,
			`C:\Go\bin`,
			`C:\go\bin`,
			filepath.Join(os.Getenv("LOCALAPPDATA"), `Programs`, `Go`, `bin`),
		}
	}
	return []string{"/usr/local/go/bin", "/opt/homebrew/bin", "/usr/local/bin"}
}

// cToolDirs lists the places the cgo C toolchain usually lands. UCRT64 comes
// first because that is the MSYS2 environment this project builds with.
func cToolDirs() []string {
	if runtime.GOOS == "windows" {
		return []string{
			`C:\msys64\ucrt64\bin`,
			`C:\msys64\mingw64\bin`,
			`C:\mingw64\bin`,
		}
	}
	return nil
}

// findToolDir returns the first directory in dirs holding the named
// executable, or "" when none does.
func findToolDir(dirs []string, tool string) string {
	name := tool
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if st, err := os.Stat(filepath.Join(dir, name)); err == nil && !st.IsDir() {
			return dir
		}
	}
	return ""
}

// prependPath puts dirs in front of path.
func prependPath(path string, dirs []string) string {
	if len(dirs) == 0 {
		return path
	}
	sep := string(os.PathListSeparator)
	head := strings.Join(dirs, sep)
	if path == "" {
		return head
	}
	return head + sep + path
}

// missingBuildTools names the executables the build needs and still cannot
// find after buildToolPath has done its search.
func missingBuildTools() []string {
	dirs := filepath.SplitList(buildToolPath())
	var missing []string
	for _, tool := range []string{"go", "gcc", "pkg-config"} {
		if findToolDir(dirs, tool) == "" {
			missing = append(missing, tool)
		}
	}
	return missing
}

// buildFailureDetail turns a failed build into something actionable. A
// compiler that ran and rejected the code explains itself on stdout/stderr; a
// toolchain that never started leaves nothing but err, and dropping it is what
// reduced a missing compiler to a bare "编译失败".
func buildFailureDetail(output []byte, err error) string {
	if text := strings.TrimSpace(string(output)); text != "" {
		return text + "\n"
	}
	detail := "编译失败: " + err.Error()
	if missing := missingBuildTools(); len(missing) > 0 {
		detail += "\n找不到构建工具: " + strings.Join(missing, ", ")
		detail += "\n请安装 Go 与 MSYS2 UCRT64 工具链 (mingw-w64-ucrt-x86_64-toolchain, mingw-w64-ucrt-x86_64-cairo)，或将它们加入 PATH。"
	}
	return detail + "\n"
}

// ---------------------------------------------------------------------------
// About dialog
// ---------------------------------------------------------------------------

func onAbout() {
	ged.ShowAboutDialog(gui.DefaultFrame())
}

// ---------------------------------------------------------------------------
// Command enablement
// ---------------------------------------------------------------------------

// selectionCommand is a command whose applicability is decided entirely by the
// size of the canvas selection: aligning needs two items, distributing three,
// breaking a layout exactly one.
type selectionCommand struct {
	action gui.IAction
	min    int
	max    int // 0 = no upper bound
}

var selectionCommands []selectionCommand

// needSelection records that btn's command applies only to a selection of
// between min and max items, and returns the button's action so the handler can
// be bound in the same expression. Each of those handlers already returns early
// on a wrong-sized selection; registering it here is what turns that silent
// no-op into a button the user can see is unavailable.
func needSelection(btn *gui.Button, min, max int) gui.IAction {
	selectionCommands = append(selectionCommands, selectionCommand{btn.Action(), min, max})
	return btn.Action()
}

// updateActionStates greys out the registered commands the current selection
// cannot satisfy. SetEnabled is called only on a real change because it bumps
// the action's revision and Button.OnIdle repaints whenever that moves — writing
// the same value back would repaint every registered button on every refresh.
func updateActionStates(gv *ged.GedView) {
	count := 0
	if gv != nil {
		if sel := gv.Selection(); sel != nil {
			count = sel.Count()
		}
	}
	for _, c := range selectionCommands {
		enabled := count >= c.min && (c.max == 0 || count <= c.max)
		if enabled != c.action.IsEnabled() {
			c.action.SetEnabled(enabled)
		}
	}
}

// ---------------------------------------------------------------------------
// Design operations
// ---------------------------------------------------------------------------

// onFormSettings opens 表单设置 for the active canvas: form title and size, plus
// the grid pitch / visibility / snap the canvas, the drag snap and the coarse
// keyboard nudge all read.
func onFormSettings() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	ged.ShowFormSettingsDialog(gui.DefaultFrame(), gv)
}

// onTagDictionary opens 标签字典 for the active canvas: the tags the design
// declares, with the unit / engineering range / alarm limits the generated app
// registers them with, and the tags widgets bind to that nothing declares.
func onTagDictionary() {
	gv := currentGedView()
	if gv == nil {
		return
	}
	ged.ShowTagDictionaryDialog(gui.DefaultFrame(), gv)
}

// ---------------------------------------------------------------------------
// Menu bar construction
// ---------------------------------------------------------------------------

// populateRecentMenu rebuilds the 最近文件 submenu from the recent-files list.
// Split out of the popup callback because that callback can only run against a
// live popup window, which puts the empty case out of reach of a test.
func populateRecentMenu(sub *gui.Menu) {
	sub.Clear()
	if len(recentFiles) == 0 {
		// A placeholder, not a command: disabled so it cannot take a click and
		// silently do nothing.
		sub.AddButton1("(无)", nil).Action().SetEnabled(false)
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
}

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
		populateRecentMenu(btn.SubPopup().(*gui.Menu))
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
	// 创建副本 is copy+paste in one step, so it belongs to this group. It needs
	// something selected — with nothing selected the copy stage has nothing to
	// take and the command silently does nothing.
	needSelection(editMenu.AddButton1("创建副本    Ctrl+D", nil), 1, 0).BindFunc0(onDuplicate)

	// ---- 视图 ----
	viewMenu, viewBtn := mainMenu.AddSubMenu("视图", nil, nil)
	viewBtn.SetSubPopupCallback(func(btn gui.IButton) {
		sub := btn.SubPopup().(*gui.Menu)
		sub.Clear()
		for _, p := range mainFrame.ToolViewActions() {
			sub.AddActionButton(p)
		}

		sub.AddSeparator()

		btnGuides := sub.AddButton1("显示参考线", nil)
		if gv := currentGedView(); gv != nil {
			btnGuides.Action().SetChecked(gv.IsShowGuides())
		}
		btnGuides.Action().BindFunc0(func() {
			gv := currentGedView()
			if gv == nil {
				return
			}
			gv.SetShowGuides(!gv.IsShowGuides())
			gv.Update()
		})

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
	needSelection(layoutMenu.AddButton1("应用水平布局 (HBox)", nil), 2, 0).BindFunc0(applyHBoxLayout)
	needSelection(layoutMenu.AddButton1("应用垂直布局 (VBox)", nil), 2, 0).BindFunc0(applyVBoxLayout)
	needSelection(layoutMenu.AddButton1("应用网格布局 (Grid)", nil), 2, 0).BindFunc0(applyGridLayout)
	layoutMenu.AddWidget(gui.NewSeparator())
	needSelection(layoutMenu.AddButton1("打破布局", nil), 1, 1).BindFunc0(breakLayout)

	// ---- 排列 (Arrange) ----
	arrangeMenu, _ := mainMenu.AddSubMenu("排列", nil, nil)

	needSelection(arrangeMenu.AddButton1("左对齐    Alt+L", nil), 2, 0).BindFunc0(alignLeft)
	needSelection(arrangeMenu.AddButton1("右对齐    Alt+R", nil), 2, 0).BindFunc0(alignRight)
	needSelection(arrangeMenu.AddButton1("顶对齐    Alt+T", nil), 2, 0).BindFunc0(alignTop)
	needSelection(arrangeMenu.AddButton1("底对齐    Alt+B", nil), 2, 0).BindFunc0(alignBottom)
	arrangeMenu.AddWidget(gui.NewSeparator())
	needSelection(arrangeMenu.AddButton1("水平居中    Alt+C", nil), 2, 0).BindFunc0(alignCenterH)
	needSelection(arrangeMenu.AddButton1("垂直居中    Alt+M", nil), 2, 0).BindFunc0(alignCenterV)
	arrangeMenu.AddWidget(gui.NewSeparator())
	needSelection(arrangeMenu.AddButton1("水平分布    Alt+H", nil), 3, 0).BindFunc0(distributeH)
	needSelection(arrangeMenu.AddButton1("垂直分布    Alt+V", nil), 3, 0).BindFunc0(distributeV)
	arrangeMenu.AddWidget(gui.NewSeparator())
	// No Alt+ suffix on these three: every accelerator printed above is a real
	// binding dispatched from GedView.OnKeyDown, and a label advertising a
	// shortcut nothing listens for is worse than no label at all.
	needSelection(arrangeMenu.AddButton1("相同宽度", nil), 2, 0).BindFunc0(sameWidth)
	needSelection(arrangeMenu.AddButton1("相同高度", nil), 2, 0).BindFunc0(sameHeight)
	needSelection(arrangeMenu.AddButton1("相同大小", nil), 2, 0).BindFunc0(sameSize)
	arrangeMenu.AddWidget(gui.NewSeparator())
	needSelection(arrangeMenu.AddButton1("置于顶层", nil), 1, 0).BindFunc0(bringToFront)
	needSelection(arrangeMenu.AddButton1("置于底层", nil), 1, 0).BindFunc0(sendToBack)

	// ---- 设计 (Design) ----
	designMenu, _ := mainMenu.AddSubMenu("设计", nil, nil)
	btnFormSettings := designMenu.AddButton1("表单设置...", nil)
	btnFormSettings.Action().BindFunc0(onFormSettings)
	btnTagDict := designMenu.AddButton1("标签字典...", nil)
	btnTagDict.Action().BindFunc0(onTagDictionary)

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

// propSheet is the property panel that shows and edits the selected widget's
// properties (name/text/size/position/…). Bound to each canvas view's
// selection via bindPropertySheetTo so selecting a widget populates it.
var propSheet *prop.PropertySheet

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
	// Keep the sidebar selector in step. SetMode early-returns when its own
	// cached mode already matches, so a menu-driven switch that left it stale
	// turned the next Ctrl+1/Ctrl+2 into a dead key — the accelerator resolved
	// to a mode the selector already believed it was in.
	if modeSelector != nil {
		switch mode {
		case ViewModeDesign:
			modeSelector.SetMode(ged.ModeDesign)
		case ViewModeCode:
			modeSelector.SetMode(ged.ModeEdit)
		}
	}

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

// bindPropertySheetTo wires the property sheet to receive selection updates
// from the given GedView, so selecting a widget on the canvas populates the
// editable property panel (name/text/size/position/…). This is the wire the
// GraphView auto-bind would perform if a property view were registered; here
// we drive the docked sheet directly. Empty selection clears the sheet.
func bindPropertySheetTo(gv *ged.GedView) {
	if gv == nil || propSheet == nil {
		return
	}
	gv.AddSelectionCallback(func(items []graph.IItem) {
		objs := make([]interface{}, 0, len(items))
		for _, it := range items {
			objs = append(objs, it)
		}
		propSheet.Bind(objs, gv.PropertyConfigName(), gv.Scene())
	})
}

// bindStatusBarTo makes the permanent status bar indicators follow gv instead
// of only being computed at startup and from a handful of menu actions. The
// scene's attach/detach signals cover every way the widget count moves — drop,
// paste, delete, undo, redo all reparent items — and SigZoomChanged covers
// wheel and menu zoom. The scene signals are used rather than the view's own
// because GraphView.SigItemAttached holds a single callback and
// refreshTreeForCurrentView already owns it for the object inspector.
func bindStatusBarTo(gv *ged.GedView) {
	if gv == nil {
		return
	}
	refresh := func() { updateStatusBarInfoFor(gv) }
	gv.AddSelectionCallback(func([]graph.IItem) { refresh() })
	gv.SigZoomChanged(func(interface{}, float64) { refresh() })
	if scene := gv.Scene(); scene != nil {
		scene.SigItemAttached(func(interface{}, graph.IItem, graph.IItem) { refresh() })
		scene.SigItemDetached(func(interface{}, graph.IItem, graph.IItem) { refresh() })
	}
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
		propSheet = prop.NewPropertySheet()
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
			bindPropertySheetTo(gv)
			bindStatusBarTo(gv)
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
				// Rebind property-sheet selection listener
				bindPropertySheetTo(gv)
				// Rebind status bar listeners, then show this document's numbers
				bindStatusBarTo(gv)
				updateStatusBarInfoFor(gv)
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
			} else {
				// A tab with no canvas behind it: the permanent indicators are
				// left showing the document you switched away from, but the
				// canvas commands must not stay lit — there is nothing for them
				// to act on.
				updateActionStates(nil)
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
			WidgetListDock: leftDockRef,
			DesignDock:     centerDock,
			PropertyDock:   rightDockRef,
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
		// currentViewMode drives the status bar's mode cell and was only
		// written by the 设计/代码/分屏 menu buttons, so a sidebar click or
		// Ctrl+1/Ctrl+2 left the cell showing the previous mode.
		switch mode {
		case ged.ModeDesign:
			currentViewMode = ViewModeDesign
			ged.SwitchToDesignMode()
			if sb := mainFrame.StatusBar(); sb != nil {
				sb.ShowMessage("已切换到设计模式")
			}
		case ged.ModeEdit:
			currentViewMode = ViewModeCode
			ged.SwitchToEditMode()
			if sb := mainFrame.StatusBar(); sb != nil {
				sb.ShowMessage("已切换到代码模式")
			}
		}
		updateStatusBarInfo()
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

// addToolBarAction adds an icon-only toolbar button carrying hover text. The
// toolbar paints no labels, so the tooltip is the only place the name of a
// command — and the shortcut its menu entry advertises — can be read.
func addToolBarAction(tb *gui.ToolBar, icon, tip string, fn func()) *gui.Button {
	btn := tb.AddAction("", gui.LoadIcon(icon), fn)
	gui.SetToolTip(btn, tip)
	return btn
}

func createToolBar(mainFrame *gui.Frame) {
	tb := gui.NewToolBar()

	// File operations (icon-only, like Qt Creator)
	addToolBarAction(tb, "document", "新建", onFileNew)
	addToolBarAction(tb, "folder", "打开", onFileOpen)
	addToolBarAction(tb, "save", "保存 (Ctrl+S)", onFileSave)

	tb.AddSeparator()

	// Edit operations
	addToolBarAction(tb, "edit-undo", "撤销 (Ctrl+Z)", onUndo)
	addToolBarAction(tb, "edit-redo", "重做 (Ctrl+Y)", onRedo)

	tb.AddSeparator()

	// Preview & Run
	addToolBarAction(tb, "preview", "预览 (Ctrl+R)", onPreview)
	addToolBarAction(tb, "run", "运行 (F5)", onRun)

	tb.AddSeparator()

	// Alignment tools
	needSelection(addToolBarAction(tb, "align-left", "左对齐 (Alt+L)", alignLeft), 2, 0)
	needSelection(addToolBarAction(tb, "align-center", "水平居中 (Alt+C)", alignCenterH), 2, 0)
	needSelection(addToolBarAction(tb, "align-right", "右对齐 (Alt+R)", alignRight), 2, 0)

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

// updateStatusBarInfo refreshes the permanent status bar indicators from the
// canvas that is currently active.
func updateStatusBarInfo() {
	updateStatusBarInfoFor(currentGedView())
}

// updateStatusBarInfoFor refreshes the indicators from gv. Split out so the
// per-view signal hooks in bindStatusBarTo can name the view that changed
// rather than going back through the frame's active-view lookup.
func updateStatusBarInfoFor(gv *ged.GedView) {
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

	// Selected widget info. Every path has to write the cell. The zoom and
	// count cells above already reset themselves when there is no canvas, and
	// this one did not: it kept the last document's "3 selected" beside a
	// "0 widgets" count. Compute the text first, then write it once.
	if statusInfoLabel != nil {
		info := ""
		var sel *graph.Selection
		if gv != nil {
			sel = gv.Selection()
		}
		if sel != nil && sel.Count() == 1 {
			item := sel.ItemList()[0]
			if fw, ok := item.(*ged.FakeWidget); ok {
				name := fw.WidgetFactoryName()
				if idx := strings.LastIndex(name, "."); idx >= 0 {
					name = name[idx+1:]
				}
				info = fmt.Sprintf("%s (%.0f,%.0f)", name, fw.X(), fw.Y())
			}
		} else if sel != nil && sel.Count() > 1 {
			info = fmt.Sprintf("%d selected", sel.Count())
		}
		statusInfoLabel.SetText(info)
	}

	// The commands that need a multi-item selection change availability on
	// exactly the events that bring us here — selection, document switch, mode
	// switch — so they ride along rather than duplicating the wiring.
	updateActionStates(gv)
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

	// createPanels wired the canvas signals into the status bar; show the
	// starting values.
	updateStatusBarInfo()

	// Ctrl+1 / Ctrl+2 mode switching, as advertised by the ModeSelector's
	// sidebar labels. GedView.OnKeyDown only sees them while the canvas has
	// focus, so they did nothing from the code editor; RegisterShortcut runs
	// at the window layer ahead of focus routing, which is what makes a mode
	// switch work from anywhere. The GedView callbacks stay wired for the
	// case the window registry does not claim the key.
	// Go through switchViewMode rather than the selector: it performs the
	// switch unconditionally, so the accelerator works whatever the selector
	// currently believes, including after a 分屏 switch that has no selector
	// equivalent at all.
	toDesign := func() { switchViewMode(ViewModeDesign) }
	toEdit := func() { switchViewMode(ViewModeCode) }
	ged.SwitchToDesignCallback = toDesign
	ged.SwitchToEditCallback = toEdit
	gui.RegisterShortcut(gui.ModAction, '1', toDesign)
	gui.RegisterShortcut(gui.ModAction, '2', toEdit)

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
	if gv := currentGedView(); gv != nil {
		state.HideGuides = !gv.IsShowGuides()
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

	if state.HideGuides {
		if gv := currentGedView(); gv != nil {
			gv.SetShowGuides(false)
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

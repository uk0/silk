package ged

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"silk/core"
	"silk/graph"
	"silk/gui"
	"silk/paint"
)

// ---------------------------------------------------------------------------
// Project Template Types
// ---------------------------------------------------------------------------

// ProjectTemplate defines a complete project template with form dimensions
// and a set of pre-configured widgets.
type ProjectTemplate struct {
	Name        string
	Description string
	Icon        string
	Width       float64 // form width in mm
	Height      float64 // form height in mm
	Widgets     []TemplateWidget
}

// TemplateWidget defines a single widget inside a project template.
type TemplateWidget struct {
	Factory string
	Name    string
	X, Y    float64 // mm
	W, H    float64 // mm
	Text    string  // default text
	Code    string  // event handler code
}

// ---------------------------------------------------------------------------
// Built-in Templates
// ---------------------------------------------------------------------------

// BuiltinTemplates returns all available project templates.
func BuiltinTemplates() []*ProjectTemplate {
	return []*ProjectTemplate{
		templateBlank(),
		templateLoginForm(),
		templateDataBrowser(),
		templateSettingsPanel(),
		templateDashboard(),
		templateChatUI(),
	}
}

// templateBlank returns an empty project template.
func templateBlank() *ProjectTemplate {
	return &ProjectTemplate{
		Name:        "空白应用",
		Description: "空白表单，从零开始设计",
		Icon:        "document",
		Width:       120,
		Height:      80,
		Widgets:     nil,
	}
}

// templateLoginForm returns a login form template.
func templateLoginForm() *ProjectTemplate {
	return &ProjectTemplate{
		Name:        "登录表单",
		Description: "用户登录界面，包含用户名、密码输入和登录按钮",
		Icon:        "form",
		Width:       90,
		Height:      75,
		Widgets: []TemplateWidget{
			{Factory: "gui.Label", Name: "LabelTitle", X: 10, Y: 5, W: 70, H: 8, Text: "用户登录"},
			{Factory: "gui.Label", Name: "LabelUser", X: 10, Y: 20, W: 20, H: 5, Text: "用户名:"},
			{Factory: "gui.Edit", Name: "EditUser", X: 32, Y: 19, W: 48, H: 6, Text: ""},
			{Factory: "gui.Label", Name: "LabelPass", X: 10, Y: 32, W: 20, H: 5, Text: "密码:"},
			{Factory: "gui.Edit", Name: "EditPass", X: 32, Y: 31, W: 48, H: 6, Text: ""},
			{Factory: "gui.Button", Name: "BtnLogin", X: 20, Y: 46, W: 25, H: 7, Text: "登录",
				Code: "func onLoginClick() {\n\tuser := ui.EditUser.Text()\n\tpass := ui.EditPass.Text()\n\tif user != \"\" && pass != \"\" {\n\t\tui.LabelStatus.SetText(\"登录中...\")\n\t}\n}\n"},
			{Factory: "gui.Button", Name: "BtnCancel", X: 50, Y: 46, W: 25, H: 7, Text: "取消",
				Code: "func onCancelClick() {\n\tui.EditUser.SetText(\"\")\n\tui.EditPass.SetText(\"\")\n\tui.LabelStatus.SetText(\"\")\n}\n"},
			{Factory: "gui.Label", Name: "LabelStatus", X: 10, Y: 58, W: 70, H: 5, Text: ""},
		},
	}
}

// templateDataBrowser returns a data browser template.
func templateDataBrowser() *ProjectTemplate {
	return &ProjectTemplate{
		Name:        "数据浏览器",
		Description: "数据查询与浏览界面，包含搜索框和数据表格",
		Icon:        "document",
		Width:       140,
		Height:      100,
		Widgets: []TemplateWidget{
			{Factory: "gui.Label", Name: "LabelTitle", X: 5, Y: 3, W: 50, H: 7, Text: "数据浏览器"},
			{Factory: "gui.Edit", Name: "EditSearch", X: 5, Y: 14, W: 95, H: 6, Text: ""},
			{Factory: "gui.Button", Name: "BtnSearch", X: 104, Y: 14, W: 30, H: 6, Text: "搜索",
				Code: "func onSearchClick() {\n\tkeyword := ui.EditSearch.Text()\n\t_ = keyword\n\t// TODO: implement search logic\n}\n"},
			{Factory: "gui.Table", Name: "TableData", X: 5, Y: 25, W: 129, H: 60},
			{Factory: "gui.StatusBar", Name: "StatusInfo", X: 5, Y: 90, W: 129, H: 6},
		},
	}
}

// templateSettingsPanel returns a settings panel template.
func templateSettingsPanel() *ProjectTemplate {
	return &ProjectTemplate{
		Name:        "设置面板",
		Description: "应用设置界面，包含基本设置和高级设置分组",
		Icon:        "propsheet",
		Width:       120,
		Height:      110,
		Widgets: []TemplateWidget{
			// Basic Settings Group
			{Factory: "gui.GroupBox", Name: "GroupBasic", X: 5, Y: 5, W: 110, H: 35, Text: "基本设置"},
			{Factory: "gui.Label", Name: "LabelName", X: 12, Y: 16, W: 18, H: 5, Text: "名称:"},
			{Factory: "gui.Edit", Name: "EditName", X: 32, Y: 15, W: 75, H: 6, Text: ""},
			{Factory: "gui.Label", Name: "LabelLang", X: 12, Y: 27, W: 18, H: 5, Text: "语言:"},
			{Factory: "gui.ComboBox", Name: "ComboLang", X: 32, Y: 26, W: 75, H: 6},
			{Factory: "gui.CheckBox", Name: "ChkAutoSave", X: 12, Y: 35, W: 30, H: 6, Text: "自动保存"},
			// Advanced Settings Group
			{Factory: "gui.GroupBox", Name: "GroupAdvanced", X: 5, Y: 46, W: 110, H: 38, Text: "高级设置"},
			{Factory: "gui.Label", Name: "LabelTimeout", X: 12, Y: 57, W: 18, H: 5, Text: "超时:"},
			{Factory: "gui.SpinBox", Name: "SpinTimeout", X: 32, Y: 56, W: 40, H: 6},
			{Factory: "gui.Label", Name: "LabelQuality", X: 12, Y: 68, W: 18, H: 5, Text: "质量:"},
			{Factory: "gui.Slider", Name: "SliderQuality", X: 32, Y: 68, W: 60, H: 5},
			{Factory: "gui.Label", Name: "LabelQualityVal", X: 95, Y: 68, W: 15, H: 5, Text: "50%"},
			// Buttons
			{Factory: "gui.Button", Name: "BtnSave", X: 35, Y: 92, W: 25, H: 7, Text: "保存",
				Code: "func onSaveClick() {\n\t// TODO: save settings\n}\n"},
			{Factory: "gui.Button", Name: "BtnReset", X: 65, Y: 92, W: 25, H: 7, Text: "重置",
				Code: "func onResetClick() {\n\t// TODO: reset to defaults\n}\n"},
		},
	}
}

// templateDashboard returns a dashboard template.
func templateDashboard() *ProjectTemplate {
	return &ProjectTemplate{
		Name:        "仪表盘",
		Description: "系统监控仪表盘，显示CPU、内存、磁盘使用率",
		Icon:        "document",
		Width:       120,
		Height:      85,
		Widgets: []TemplateWidget{
			{Factory: "gui.Label", Name: "LabelTitle", X: 10, Y: 3, W: 100, H: 8, Text: "仪表盘"},
			// CPU
			{Factory: "gui.Label", Name: "LabelCPU", X: 10, Y: 18, W: 30, H: 5, Text: "CPU使用率"},
			{Factory: "gui.ProgressBar", Name: "ProgressCPU", X: 42, Y: 18, W: 68, H: 5},
			// Memory
			{Factory: "gui.Label", Name: "LabelMem", X: 10, Y: 30, W: 30, H: 5, Text: "内存使用率"},
			{Factory: "gui.ProgressBar", Name: "ProgressMem", X: 42, Y: 30, W: 68, H: 5},
			// Disk
			{Factory: "gui.Label", Name: "LabelDisk", X: 10, Y: 42, W: 30, H: 5, Text: "磁盘使用率"},
			{Factory: "gui.ProgressBar", Name: "ProgressDisk", X: 42, Y: 42, W: 68, H: 5},
			// Status
			{Factory: "gui.Label", Name: "LabelStatus", X: 10, Y: 60, W: 100, H: 5, Text: "系统运行正常"},
		},
	}
}

// templateChatUI returns a chat interface template.
func templateChatUI() *ProjectTemplate {
	return &ProjectTemplate{
		Name:        "聊天界面",
		Description: "即时通讯聊天界面，包含消息列表和发送按钮",
		Icon:        "document",
		Width:       110,
		Height:      100,
		Widgets: []TemplateWidget{
			{Factory: "gui.Label", Name: "LabelTitle", X: 5, Y: 3, W: 100, H: 7, Text: "Silk Chat"},
			{Factory: "gui.ListWidget", Name: "ListMessages", X: 5, Y: 13, W: 100, H: 68},
			{Factory: "gui.Edit", Name: "EditMessage", X: 5, Y: 84, W: 73, H: 6, Text: ""},
			{Factory: "gui.Button", Name: "BtnSend", X: 80, Y: 84, W: 25, H: 6, Text: "发送",
				Code: "func onSendClick() {\n\tmsg := ui.EditMessage.Text()\n\tif msg != \"\" {\n\t\tui.ListMessages.Append(gui.ListItem{Text: msg})\n\t\tui.EditMessage.SetText(\"\")\n\t}\n}\n"},
		},
	}
}

// ---------------------------------------------------------------------------
// Save Current Design as Template
// ---------------------------------------------------------------------------

// templateDir returns the directory used for custom templates.
func templateDir() string {
	return "templates"
}

// SaveAsTemplate persists the current scene design as a reusable template.
// It creates the templates/ directory if needed, saves the scene's TDoc to
// "templates/{name}.silkui", and writes a metadata file with name+description.
func SaveAsTemplate(scene *GedScene, name, description string) error {
	if scene == nil {
		return fmt.Errorf("no scene to save")
	}
	dir := templateDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create template directory: %w", err)
	}

	// Sanitize name for use as a filename
	safeName := strings.ReplaceAll(name, "/", "_")
	safeName = strings.ReplaceAll(safeName, "\\", "_")
	safeName = strings.TrimSpace(safeName)
	if safeName == "" {
		return fmt.Errorf("template name is empty")
	}

	// Save the scene's TDoc
	doc := scene.SaveDesign()
	designPath := filepath.Join(dir, safeName+".silkui")
	if err := doc.SaveFile(designPath); err != nil {
		return fmt.Errorf("failed to save template: %w", err)
	}

	// Write metadata
	metaPath := filepath.Join(dir, safeName+".meta")
	metaContent := "name=" + name + "\ndescription=" + description + "\n"
	if err := os.WriteFile(metaPath, []byte(metaContent), 0644); err != nil {
		return fmt.Errorf("failed to save template metadata: %w", err)
	}

	return nil
}

// LoadCustomTemplates scans the templates/ directory for .silkui files and
// returns custom templates alongside built-in ones.
func LoadCustomTemplates() []*ProjectTemplate {
	dir := templateDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var templates []*ProjectTemplate
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".silkui") {
			continue
		}

		baseName := strings.TrimSuffix(entry.Name(), ".silkui")
		designPath := filepath.Join(dir, entry.Name())
		metaPath := filepath.Join(dir, baseName+".meta")

		// Read metadata
		tmplName := baseName
		tmplDesc := "自定义模板"
		if metaData, err := os.ReadFile(metaPath); err == nil {
			for _, line := range strings.Split(string(metaData), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name=") {
					tmplName = strings.TrimPrefix(line, "name=")
				}
				if strings.HasPrefix(line, "description=") {
					tmplDesc = strings.TrimPrefix(line, "description=")
				}
			}
		}

		// Load the TDoc to extract form dimensions
		doc, err := core.LoadTDocFile(designPath)
		if err != nil {
			continue
		}

		tmpl := &ProjectTemplate{
			Name:        tmplName,
			Description: tmplDesc,
			Icon:        "document",
			Width:       120,
			Height:      80,
		}

		// Try to read bounds from the TDoc
		var bStr string
		doc.ReadAttr("bounds", &bStr)
		_ = bStr // dimensions already set to defaults above

		templates = append(templates, tmpl)
	}

	return templates
}

// AllTemplates returns both built-in and custom templates.
func AllTemplates() []*ProjectTemplate {
	all := BuiltinTemplates()
	custom := LoadCustomTemplates()
	if len(custom) > 0 {
		all = append(all, custom...)
	}
	return all
}

// ---------------------------------------------------------------------------
// Template Selection Dialog
// ---------------------------------------------------------------------------

// ShowNewProjectDialog displays a dialog for selecting a project template.
// Returns the selected template, or nil if the user cancelled.
func ShowNewProjectDialog(parent gui.IWidget) *ProjectTemplate {
	templates := AllTemplates()

	dlg := gui.NewDialog("新建项目", parent)

	// Create a list widget with icons and taller rows for template cards
	list := gui.NewListWidget()
	list.SetSelectionVisible(true)
	list.SetIconVisible(true)
	list.SetIconSize(24)
	list.SetRowHeight(56)

	for _, tmpl := range templates {
		list.Append(gui.ListItem{
			Text: tmpl.Name + "  --  " + tmpl.Description,
			Icon: templateIconOrDefault(tmpl.Icon),
			Data: tmpl,
		})
	}
	dlg.SetContent(list)
	dlg.AddButton("确定", gui.DialogOK)
	dlg.AddButton("取消", gui.DialogCancel)

	// Larger dialog for comfortable template browsing
	dlg.SetSize(560, 420)

	result := dlg.ShowModal()
	if result != gui.DialogOK {
		return nil
	}

	idx := list.ActiveIndex()
	if idx < 0 || idx >= len(templates) {
		return nil
	}
	return templates[idx]
}

// ---------------------------------------------------------------------------
// Apply Template to GedScene
// ---------------------------------------------------------------------------

// ApplyTemplate populates a GedScene with widgets defined in the given
// ProjectTemplate. The scene is resized and a title is assigned. Each widget
// is created via the factory system and added through the undo stack so the
// user can revert the operation.
func ApplyTemplate(scene *GedScene, tmpl *ProjectTemplate) {
	if scene == nil || tmpl == nil {
		return
	}

	scene.SetSize(tmpl.Width, tmpl.Height)
	scene.SetFormTitle(tmpl.Name)

	for _, tw := range tmpl.Widgets {
		item, err := NewFakeWidgetFromFactory(tw.Factory)
		if err != nil {
			continue
		}
		item.SetWidgetName(tw.Name)
		item.SetBounds(tw.X, tw.Y, tw.W, tw.H)

		// Apply default text content
		if tw.Text != "" {
			applyWidgetText(item, tw.Text)
		}

		// Apply event handler code
		if tw.Code != "" {
			item.SetCode(tw.Code)
		}

		// Synchronize the embedded widget pixel size
		item.Layout()

		cmd := graph.NewAddCommand()
		cmd.AddItem(item, scene)
		scene.PushCommand(cmd)
	}
}

// applyWidgetText sets the text of the widget inside a FakeWidget using the
// appropriate setter for its concrete type.
func applyWidgetText(fake *FakeWidget, text string) {
	w := fake.Widget()
	if w == nil {
		return
	}
	switch v := w.(type) {
	case *gui.Label:
		v.SetText(text)
	case *gui.Button:
		v.SetText(text)
	case *gui.Edit:
		v.SetText(text)
	case *gui.CheckBox:
		v.SetText(text)
	case *gui.RadioButton:
		v.SetText(text)
	case *gui.GroupBox:
		v.SetTitle(text)
	default:
		// Try generic SetText interface
		if setter, ok := w.(interface{ SetText(string) }); ok {
			setter.SetText(text)
		}
	}
}

// applyProgressValue sets a ProgressBar value if the widget is a ProgressBar.
func applyProgressValue(fake *FakeWidget, value float64) {
	w := fake.Widget()
	if w == nil {
		return
	}
	if pb, ok := w.(*gui.ProgressBar); ok {
		pb.SetValue(value)
		pb.SetShowText(true)
	}
}

// ---------------------------------------------------------------------------
// Quick Template Shortcut
// ---------------------------------------------------------------------------

// NewProjectFromTemplate creates a fully populated GedView from a template.
// This is a convenience function for programmatic use.
func NewProjectFromTemplate(tmpl *ProjectTemplate) *GedView {
	gv := NewGedView()
	if tmpl != nil {
		ApplyTemplate(gv.GedScene(), tmpl)
	}
	return gv
}

// NewProjectFromTemplateName creates a GedView from a template identified by
// its Chinese name string. Returns nil if no matching template is found.
func NewProjectFromTemplateName(name string) *GedView {
	for _, tmpl := range BuiltinTemplates() {
		if tmpl.Name == name {
			return NewProjectFromTemplate(tmpl)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Icon helper for fallback
// ---------------------------------------------------------------------------

// templateIconOrDefault loads a named icon, falling back to "document" if the
// named icon does not exist.
func templateIconOrDefault(name string) paint.Icon {
	ico := gui.LoadIcon(name)
	if ico == nil {
		ico = gui.LoadIcon("document")
	}
	return ico
}

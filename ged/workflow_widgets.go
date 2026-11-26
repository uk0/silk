package ged

// Register the new SDK controls through the same all-or-nothing mapping used
// by external widgets: palette, concrete constructors and bindable events must
// agree. Their constructors do no I/O and never send a print job.
func init() {
	for _, def := range []WidgetDef{
		{
			Factory: "gui.GanttChart", Category: "图表 (Charts)",
			GoType: "*gui.GanttChart", ImportPath: "github.com/uk0/silk/gui", Constructor: "gui.NewGanttChart()",
			Events:       []WidgetEvent{{Name: "OnTasksChanged", Code: "\tui.%s.SigTasksChanged(%s)\n"}},
			DefaultEvent: "OnTasksChanged",
		},
		{
			Factory: "gui.PrintButton", Category: "输入控件 (Input)",
			GoType: "*gui.PrintButton", ImportPath: "github.com/uk0/silk/gui", Constructor: "gui.NewPrintButton()",
			Events:       []WidgetEvent{{Name: "OnPrintFinished", Code: "\tui.%s.SigPrintFinished(%s)\n", Params: "jobID string, errText string"}},
			DefaultEvent: "OnPrintFinished",
		},
	} {
		if err := RegisterWidget(def); err != nil {
			panic(err)
		}
	}
	industrialDesignProps["gui.GanttChart"] = []designProp{
		{"TasksJSON", "SetTasksJSON", dpString},
		{"DayWidth", "SetDayWidth", dpFloat},
		{"RowHeight", "SetRowHeight", dpFloat},
		{"TableWidth", "SetTableWidth", dpFloat},
		{"IsReadOnly", "SetReadOnly", dpBool},
		{"CascadeDependencies", "SetCascadeDependencies", dpBool},
	}
	industrialDesignProps["gui.PrintButton"] = []designProp{
		{"TextVisibleWithIcon", "SetTextVisible", dpBool},
		{"DocumentPath", "SetDocumentPath", dpString},
		{"PrinterName", "SetPrinterName", dpString},
		{"Copies", "SetCopies", dpInt},
		{"JobTitle", "SetJobTitle", dpString},
	}
}

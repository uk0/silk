package ged

func init() {
	if err := RegisterWidget(WidgetDef{
		Factory: "gui.JSONView", Category: "数据视图 (Data)", GoType: "*gui.JSONView", ImportPath: "github.com/uk0/silk/gui", Constructor: "gui.NewJSONView()",
		Events: []WidgetEvent{
			{Name: "OnSelected", Code: "\tui.%s.SigSelected(%s)\n", Params: "pointer string"},
			{Name: "OnError", Code: "\tui.%s.SigError(%s)\n", Params: "message string"},
		}, DefaultEvent: "OnSelected",
	}); err != nil {
		panic(err)
	}
	industrialDesignProps["gui.JSONView"] = []designProp{{"JSONText", "SetJSONText", dpString}, {"PageSize", "SetPageSize", dpInt}, {"RootPointer", "SetRootPointer", dpString}}
}

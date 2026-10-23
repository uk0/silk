package ged

func init() {
	if err := RegisterWidget(WidgetDef{
		Factory: "gui.HTTPButton", Category: "网络与工具 (Network & Tools)",
		GoType: "*gui.HTTPButton", ImportPath: "github.com/uk0/silk/gui", Constructor: "gui.NewHTTPButton()",
		Events: []WidgetEvent{
			{Name: "OnResult", Code: "\tui.%s.SigResult(%s)\n", Params: "value string"},
			{Name: "OnIntResult", Code: "\tui.%s.SigIntResult(%s)\n", Params: "value int64"},
			{Name: "OnFloatResult", Code: "\tui.%s.SigFloatResult(%s)\n", Params: "value float64"},
			{Name: "OnBoolResult", Code: "\tui.%s.SigBoolResult(%s)\n", Params: "value bool"},
			{Name: "OnError", Code: "\tui.%s.SigError(%s)\n", Params: "message string"},
			{Name: "OnBusyChanged", Code: "\tui.%s.SigBusyChanged(%s)\n", Params: "busy bool"},
		}, DefaultEvent: "OnResult",
	}); err != nil {
		panic(err)
	}
	industrialDesignProps["gui.HTTPButton"] = []designProp{
		{"URL", "SetURL", dpString}, {"Method", "SetMethod", dpString},
		{"HeadersJSON", "SetHeadersJSON", dpString}, {"Body", "SetBody", dpString},
		{"ResultType", "SetResultType", dpString}, {"JSONPointer", "SetJSONPointer", dpString},
		{"TimeoutMS", "SetTimeoutMS", dpInt}, {"MaxResponseKB", "SetMaxResponseKB", dpInt},
	}
}

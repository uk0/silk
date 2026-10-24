package ged

func init() {
	if err := RegisterWidget(WidgetDef{
		Factory: "gui.TransferProgress", Category: "网络与工具 (Network & Tools)",
		GoType: "*gui.TransferProgress", ImportPath: "github.com/uk0/silk/gui", Constructor: "gui.NewTransferProgress()",
		Events:       []WidgetEvent{{Name: "OnStateChanged", Code: "\tui.%s.SigStateChanged(%s)\n", Params: "state string"}},
		DefaultEvent: "OnStateChanged",
	}); err != nil {
		panic(err)
	}
	industrialDesignProps["gui.TransferProgress"] = []designProp{{"Label", "SetLabel", dpString}, {"ShowBytes", "SetShowBytes", dpBool}}
}

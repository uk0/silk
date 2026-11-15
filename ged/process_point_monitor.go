package ged

func init() {
	if err := RegisterWidget(WidgetDef{
		Factory: "gui.ProcessPointMonitor", Category: "组态 (SCADA)", GoType: "*gui.ProcessPointMonitor",
		ImportPath: "github.com/uk0/silk/gui", Constructor: "gui.NewProcessPointMonitor()",
	}); err != nil {
		panic(err)
	}
}

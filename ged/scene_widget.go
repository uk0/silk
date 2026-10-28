package ged

func init() {
	if err := RegisterWidget(WidgetDef{Factory: "gui.SceneView", Category: "组态 (SCADA)", GoType: "*gui.SceneView", ImportPath: "github.com/uk0/silk/gui", Constructor: "gui.NewSceneView()", Events: []WidgetEvent{
		{Name: "OnSelected", Code: "\tui.%s.SigSelected(%s)\n", Params: "id string"},
		{Name: "OnModelLoaded", Code: "\tui.%s.SigModelLoaded(%s)\n"},
		{Name: "OnError", Code: "\tui.%s.SigError(%s)\n", Params: "message string"},
	}, DefaultEvent: "OnSelected"}); err != nil {
		panic(err)
	}
	industrialDesignProps["gui.SceneView"] = []designProp{{"ModelSource", "SetModelSource", dpString}, {"Yaw", "SetYaw", dpFloat}, {"Pitch", "SetPitch", dpFloat}, {"Distance", "SetDistance", dpFloat}}
}

package ged

func init() {
	for _, name := range []string{"SpriteView", "ProcessValue", "ProcessEquipment", "ProcessDiagram", "ProcessEditor"} {
		if err := RegisterWidget(WidgetDef{Factory: "gui." + name, Category: "组态 (SCADA)", GoType: "*gui." + name, ImportPath: "github.com/uk0/silk/gui", Constructor: "gui.New" + name + "()"}); err != nil {
			panic(err)
		}
	}
	industrialDesignProps["gui.ProcessEditor"] = []designProp{{"ProjectJSON", "SetProjectJSON", dpString}}
	industrialDesignProps["gui.ProcessDiagram"] = []designProp{{"ProjectJSON", "SetProjectJSON", dpString}, {"IsEditing", "SetEditing", dpBool}, {"Zoom", "SetZoom", dpFloat}}
	industrialDesignProps["gui.ProcessEquipment"] = []designProp{{"Kind", "SetKind", dpString}, {"Label", "SetLabel", dpString}}
	industrialDesignProps["gui.SpriteView"] = []designProp{{"IconName", "SetIconName", dpString}, {"ActiveIconName", "SetActiveIconName", dpString}, {"Active", "SetActive", dpBool}}
	industrialDesignProps["gui.ProcessValue"] = []designProp{{"TransformJSON", "SetTransformJSON", dpString}, {"ColorRulesJSON", "SetColorRulesJSON", dpString}, {"Title", "SetTitle", dpString}, {"Unit", "SetUnit", dpString}, {"PreviewValue", "SetPreviewValue", dpFloat}, {"Min", "SetMin", dpFloat}, {"Max", "SetMax", dpFloat}, {"AlarmAbove", "SetAlarmAbove", dpFloat}, {"Precision", "SetPrecision", dpInt}}
}

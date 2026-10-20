package ged

func init() {
	for _, name := range []string{"SpriteView", "ProcessValue"} {
		if err := RegisterWidget(WidgetDef{Factory: "gui." + name, Category: "组态 (SCADA)", GoType: "*gui." + name, ImportPath: "github.com/uk0/silk/gui", Constructor: "gui.New" + name + "()"}); err != nil {
			panic(err)
		}
	}
	industrialDesignProps["gui.SpriteView"] = []designProp{{"IconName", "SetIconName", dpString}, {"ActiveIconName", "SetActiveIconName", dpString}, {"Active", "SetActive", dpBool}}
	industrialDesignProps["gui.ProcessValue"] = []designProp{{"Title", "SetTitle", dpString}, {"Unit", "SetUnit", dpString}, {"Value", "SetValue", dpFloat}, {"Min", "SetMin", dpFloat}, {"Max", "SetMax", dpFloat}, {"AlarmAbove", "SetAlarmAbove", dpFloat}, {"Precision", "SetPrecision", dpInt}}
}

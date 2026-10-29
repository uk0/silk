package gui

import "github.com/uk0/silk/i18n"

func init() {
	i18n.AddMany("zh", map[string]string{
		"Color affects this node's mesh, not child materials": "颜色只作用于当前节点网格，不改变子节点材质",
		"3D Scene Editor": "3D 场景编辑器", "Edit 3D Scene...": "编辑 3D 场景...",
		"Scene nodes": "场景节点", "Edit node...": "编辑节点...", "Set binding...": "设置绑定...",
		"Remove binding": "删除绑定", "Fit scene": "适合场景", "Use changes": "应用更改",
		"Edit scene node": "编辑场景节点", "Parent ID (empty = root)": "父节点 ID（空为根）",
		"Local position X": "局部位置 X", "Local position Y": "局部位置 Y", "Local position Z": "局部位置 Z",
		"Color (#RRGGBB)": "颜色 (#RRGGBB)", "Hide this subtree": "隐藏此节点及其子节点",
		"Authored rotation / scale preserved": "保留原始旋转和缩放", "Apply node": "应用节点",
		"Set scene binding": "设置场景绑定", "Node ID": "节点 ID", "Tag name": "点位名称",
		"Property": "属性", "Gain": "倍率", "Offset": "偏移", "False color": "假值颜色", "True color": "真值颜色",
		"Mapping": "映射", "Set binding": "设置绑定", "Enter a finite number": "请输入有限数值",
		"Use an opaque color in #RRGGBB format":      "请使用 #RRGGBB 格式的不透明颜色",
		"value × gain + offset; rotation in radians": "值 × 倍率 + 偏移；旋转单位为弧度",
		"Boolean tag; no numeric conversion":         "布尔点位；不进行数值转换",
		"Scene changed; reopen the editor":           "场景已更改，请重新打开编辑器",
		"Document changed; reopen the scene editor":  "文档已更改，请重新打开场景编辑器",
		"%d nodes · %d bindings":                     "%d 个节点 · %d 个绑定",
	})
}

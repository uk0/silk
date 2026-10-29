package gui

import "github.com/uk0/silk/i18n"

func init() {
	i18n.AddMany("zh", map[string]string{
		"Tag prefix": "点位前缀", "Nodes": "节点", "Import...": "导入...",
		"Ground plane": "地平面", "Surface: upright": "表面：保持竖直", "Surface: align normal": "表面：法线对齐",
		"Prepare surfaces": "准备表面", "Preparing surfaces...": "正在准备表面...", "Retry surfaces": "重试表面", "Surfaces ready": "表面已就绪",
		"Surface placement has no ground fallback; Shift grid snapping applies only to ground mode.":                       "表面模式未命中不会退回地面；Shift 网格吸附只适用于地面模式。",
		"Surface modes place the visible template's bottom-center. Normal alignment follows mesh winding, not the camera.": "以可见模板的底部中心贴合表面；法线对齐遵循网格绕序，不随相机翻转。",

		"Device library": "设备库", "Import model...": "导入模型...", "Search devices": "搜索设备", "Tag prefix (empty = device ID)": "点位前缀（空为设备 ID）", "Insert device": "插入设备", "No matching devices": "没有匹配设备", "Loading model...": "正在加载模型...", "Drop device": "放置设备", "Pump": "泵", "Tank": "储罐", "Conveyor": "输送机", "Drag onto the ground plane; Shift snaps to 1 unit. No live points are started.": "拖到地平面放置；Shift 按 1 单位吸附；不会启动实时点位。",

		"Move node": "移动节点", "Rotate node": "旋转节点", "Scale node": "缩放节点",
		"Rotate around X/Y/Z; Shift snaps to 15 degrees; Escape cancels drag":             "绕 X/Y/Z 轴旋转；Shift 按 15 度吸附；Escape 取消拖动",
		"Scale on X/Y/Z or center; Shift snaps in 10 percent ratios; Escape cancels drag": "沿 X/Y/Z 轴或中心缩放；Shift 按 10% 倍率吸附；Escape 取消拖动",
		"Move on X/Y/Z; Shift snaps to 1 unit; Escape cancels drag":                       "沿 X/Y/Z 轴拖动；Shift 按 1 单位吸附；Escape 取消拖动",
		"Color affects this node's mesh, not child materials":                             "颜色只作用于当前节点网格，不改变子节点材质",
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

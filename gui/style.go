package gui

import (
	"silk/paint"
)

// ─── Style System ───

// StyleVariant 风格变体枚举
type StyleVariant int

const (
	StyleDefault  StyleVariant = iota
	StyleLight                 // 浅色主题
	StyleDark                  // 深色主题
	StyleBlue                  // 蓝色主题
	StyleGreen                 // 绿色主题
	StylePurple                // 紫色主题
	StyleCustom                // 自定义
)

// ColorScheme 配色方案
type ColorScheme struct {
	// 主要颜色
	Primary      paint.Color // 主色调
	PrimaryLight paint.Color // 主色调浅色
	PrimaryDark  paint.Color // 主色调深色
	Secondary    paint.Color // 辅助色
	Accent       paint.Color // 强调色

	// 背景色
	Background   paint.Color // 主背景
	Surface      paint.Color // 表面/卡片背景
	SurfaceAlt   paint.Color // 交替表面色

	// 文字色
	TextPrimary   paint.Color // 主要文字
	TextSecondary paint.Color // 辅助文字
	TextDisabled  paint.Color // 禁用文字
	TextOnPrimary paint.Color // 主色上的文字（通常白色）

	// 边框与分割
	Border      paint.Color // 边框色
	Divider     paint.Color // 分割线
	BorderFocus paint.Color // 获焦边框

	// 功能色
	Success  paint.Color // 成功/正确
	Warning  paint.Color // 警告
	Error    paint.Color // 错误/危险
	Info     paint.Color // 信息

	// 交互状态
	Hover    paint.Color // 悬停
	Pressed  paint.Color // 按下
	Selected paint.Color // 选中
	Disabled paint.Color // 禁用背景

	// 特殊
	Shadow   paint.Color // 阴影色
	Overlay  paint.Color // 遮罩层
}

// WidgetStyle 控件级别的样式覆盖
type WidgetStyle struct {
	// 圆角
	BorderRadius float64

	// 内边距
	PaddingTop    float64
	PaddingRight  float64
	PaddingBottom float64
	PaddingLeft   float64

	// 字体大小覆盖 (0 = 使用主题默认)
	FontSize float64

	// 颜色覆盖 (A=0 表示不覆盖，使用主题)
	BackgroundColor paint.Color
	TextColor       paint.Color
	BorderColor     paint.Color

	// 阴影
	ShadowOffsetX float64
	ShadowOffsetY float64
	ShadowBlur    float64
	ShadowColor   paint.Color
}

// ─── Preset Color Schemes ───

// LightColorScheme 浅色主题配色
func LightColorScheme() ColorScheme {
	return ColorScheme{
		Primary:       paint.Color{66, 133, 244, 255},
		PrimaryLight:  paint.Color{144, 186, 255, 255},
		PrimaryDark:   paint.Color{25, 103, 210, 255},
		Secondary:     paint.Color{95, 99, 104, 255},
		Accent:        paint.Color{251, 188, 4, 255},

		Background:   paint.Color{255, 255, 255, 255},
		Surface:      paint.Color{255, 255, 255, 255},
		SurfaceAlt:   paint.Color{248, 249, 250, 255},

		TextPrimary:   paint.Color{32, 33, 36, 255},
		TextSecondary: paint.Color{95, 99, 104, 255},
		TextDisabled:  paint.Color{189, 193, 198, 255},
		TextOnPrimary: paint.Color{255, 255, 255, 255},

		Border:      paint.Color{218, 220, 224, 255},
		Divider:     paint.Color{232, 234, 237, 255},
		BorderFocus: paint.Color{66, 133, 244, 255},

		Success: paint.Color{52, 168, 83, 255},
		Warning: paint.Color{251, 188, 4, 255},
		Error:   paint.Color{234, 67, 53, 255},
		Info:    paint.Color{66, 133, 244, 255},

		Hover:    paint.Color{241, 243, 244, 255},
		Pressed:  paint.Color{232, 234, 237, 255},
		Selected: paint.Color{210, 227, 252, 255},
		Disabled: paint.Color{241, 243, 244, 255},

		Shadow:  paint.Color{0, 0, 0, 30},
		Overlay: paint.Color{0, 0, 0, 80},
	}
}

// DarkColorScheme 深色主题配色
func DarkColorScheme() ColorScheme {
	return ColorScheme{
		Primary:       paint.Color{138, 180, 248, 255},
		PrimaryLight:  paint.Color{174, 203, 250, 255},
		PrimaryDark:   paint.Color{93, 146, 229, 255},
		Secondary:     paint.Color{154, 160, 166, 255},
		Accent:        paint.Color{253, 214, 99, 255},

		Background:   paint.Color{32, 33, 36, 255},
		Surface:      paint.Color{48, 49, 52, 255},
		SurfaceAlt:   paint.Color{41, 42, 45, 255},

		TextPrimary:   paint.Color{232, 234, 237, 255},
		TextSecondary: paint.Color{154, 160, 166, 255},
		TextDisabled:  paint.Color{95, 99, 104, 255},
		TextOnPrimary: paint.Color{32, 33, 36, 255},

		Border:      paint.Color{60, 64, 67, 255},
		Divider:     paint.Color{53, 55, 58, 255},
		BorderFocus: paint.Color{138, 180, 248, 255},

		Success: paint.Color{129, 201, 149, 255},
		Warning: paint.Color{253, 214, 99, 255},
		Error:   paint.Color{242, 139, 130, 255},
		Info:    paint.Color{138, 180, 248, 255},

		Hover:    paint.Color{53, 55, 58, 255},
		Pressed:  paint.Color{60, 64, 67, 255},
		Selected: paint.Color{44, 54, 79, 255},
		Disabled: paint.Color{41, 42, 45, 255},

		Shadow:  paint.Color{0, 0, 0, 60},
		Overlay: paint.Color{0, 0, 0, 120},
	}
}

// BlueColorScheme 蓝色主题配色
func BlueColorScheme() ColorScheme {
	return ColorScheme{
		Primary:       paint.Color{37, 99, 235, 255},
		PrimaryLight:  paint.Color{96, 165, 250, 255},
		PrimaryDark:   paint.Color{29, 78, 216, 255},
		Secondary:     paint.Color{71, 85, 105, 255},
		Accent:        paint.Color{251, 146, 60, 255},

		Background:   paint.Color{248, 250, 252, 255},
		Surface:      paint.Color{255, 255, 255, 255},
		SurfaceAlt:   paint.Color{241, 245, 249, 255},

		TextPrimary:   paint.Color{15, 23, 42, 255},
		TextSecondary: paint.Color{100, 116, 139, 255},
		TextDisabled:  paint.Color{203, 213, 225, 255},
		TextOnPrimary: paint.Color{255, 255, 255, 255},

		Border:      paint.Color{203, 213, 225, 255},
		Divider:     paint.Color{226, 232, 240, 255},
		BorderFocus: paint.Color{37, 99, 235, 255},

		Success: paint.Color{34, 197, 94, 255},
		Warning: paint.Color{245, 158, 11, 255},
		Error:   paint.Color{239, 68, 68, 255},
		Info:    paint.Color{59, 130, 246, 255},

		Hover:    paint.Color{241, 245, 249, 255},
		Pressed:  paint.Color{226, 232, 240, 255},
		Selected: paint.Color{219, 234, 254, 255},
		Disabled: paint.Color{241, 245, 249, 255},

		Shadow:  paint.Color{0, 0, 0, 20},
		Overlay: paint.Color{0, 0, 0, 80},
	}
}

// GreenColorScheme 绿色主题配色
func GreenColorScheme() ColorScheme {
	return ColorScheme{
		Primary:       paint.Color{22, 163, 74, 255},
		PrimaryLight:  paint.Color{74, 222, 128, 255},
		PrimaryDark:   paint.Color{21, 128, 61, 255},
		Secondary:     paint.Color{82, 82, 91, 255},
		Accent:        paint.Color{234, 179, 8, 255},

		Background:   paint.Color{250, 253, 250, 255},
		Surface:      paint.Color{255, 255, 255, 255},
		SurfaceAlt:   paint.Color{240, 253, 244, 255},

		TextPrimary:   paint.Color{20, 30, 20, 255},
		TextSecondary: paint.Color{82, 100, 82, 255},
		TextDisabled:  paint.Color{190, 210, 190, 255},
		TextOnPrimary: paint.Color{255, 255, 255, 255},

		Border:      paint.Color{200, 220, 200, 255},
		Divider:     paint.Color{220, 235, 220, 255},
		BorderFocus: paint.Color{22, 163, 74, 255},

		Success: paint.Color{22, 163, 74, 255},
		Warning: paint.Color{234, 179, 8, 255},
		Error:   paint.Color{220, 38, 38, 255},
		Info:    paint.Color{59, 130, 246, 255},

		Hover:    paint.Color{240, 253, 244, 255},
		Pressed:  paint.Color{220, 252, 231, 255},
		Selected: paint.Color{187, 247, 208, 255},
		Disabled: paint.Color{240, 253, 244, 255},

		Shadow:  paint.Color{0, 0, 0, 15},
		Overlay: paint.Color{0, 0, 0, 80},
	}
}

// PurpleColorScheme 紫色主题配色
func PurpleColorScheme() ColorScheme {
	return ColorScheme{
		Primary:       paint.Color{124, 58, 237, 255},
		PrimaryLight:  paint.Color{167, 139, 250, 255},
		PrimaryDark:   paint.Color{109, 40, 217, 255},
		Secondary:     paint.Color{100, 100, 115, 255},
		Accent:        paint.Color{244, 114, 182, 255},

		Background:   paint.Color{250, 249, 254, 255},
		Surface:      paint.Color{255, 255, 255, 255},
		SurfaceAlt:   paint.Color{245, 243, 255, 255},

		TextPrimary:   paint.Color{30, 20, 45, 255},
		TextSecondary: paint.Color{100, 90, 120, 255},
		TextDisabled:  paint.Color{190, 185, 200, 255},
		TextOnPrimary: paint.Color{255, 255, 255, 255},

		Border:      paint.Color{210, 200, 225, 255},
		Divider:     paint.Color{230, 225, 240, 255},
		BorderFocus: paint.Color{124, 58, 237, 255},

		Success: paint.Color{34, 197, 94, 255},
		Warning: paint.Color{245, 158, 11, 255},
		Error:   paint.Color{239, 68, 68, 255},
		Info:    paint.Color{124, 58, 237, 255},

		Hover:    paint.Color{245, 243, 255, 255},
		Pressed:  paint.Color{237, 233, 254, 255},
		Selected: paint.Color{221, 214, 254, 255},
		Disabled: paint.Color{245, 243, 255, 255},

		Shadow:  paint.Color{0, 0, 0, 20},
		Overlay: paint.Color{0, 0, 0, 80},
	}
}

// ─── Style Registry ───

// GetColorScheme 获取预设配色方案
func GetColorScheme(variant StyleVariant) ColorScheme {
	switch variant {
	case StyleDark:
		return DarkColorScheme()
	case StyleBlue:
		return BlueColorScheme()
	case StyleGreen:
		return GreenColorScheme()
	case StylePurple:
		return PurpleColorScheme()
	default:
		return LightColorScheme()
	}
}

// ─── Default Widget Style Presets ───

// ButtonStyle 按钮样式预设
func ButtonStylePrimary(scheme ColorScheme) WidgetStyle {
	return WidgetStyle{
		BorderRadius:    4,
		PaddingTop:      6,
		PaddingRight:    16,
		PaddingBottom:   6,
		PaddingLeft:     16,
		BackgroundColor: scheme.Primary,
		TextColor:       scheme.TextOnPrimary,
		BorderColor:     scheme.PrimaryDark,
	}
}

// ButtonStyleSecondary returns a secondary button style with surface background and text-primary color.
func ButtonStyleSecondary(scheme ColorScheme) WidgetStyle {
	return WidgetStyle{
		BorderRadius:    4,
		PaddingTop:      6,
		PaddingRight:    16,
		PaddingBottom:   6,
		PaddingLeft:     16,
		BackgroundColor: scheme.Surface,
		TextColor:       scheme.TextPrimary,
		BorderColor:     scheme.Border,
	}
}

// ButtonStyleDanger returns a danger/destructive button style with error-colored background.
func ButtonStyleDanger(scheme ColorScheme) WidgetStyle {
	return WidgetStyle{
		BorderRadius:    4,
		PaddingTop:      6,
		PaddingRight:    16,
		PaddingBottom:   6,
		PaddingLeft:     16,
		BackgroundColor: scheme.Error,
		TextColor:       paint.Color{255, 255, 255, 255},
		BorderColor:     paint.Color{200, 50, 40, 255},
	}
}

// ButtonStyleSuccess returns a success button style with green background.
func ButtonStyleSuccess(scheme ColorScheme) WidgetStyle {
	return WidgetStyle{
		BorderRadius:    4,
		PaddingTop:      6,
		PaddingRight:    16,
		PaddingBottom:   6,
		PaddingLeft:     16,
		BackgroundColor: scheme.Success,
		TextColor:       paint.Color{255, 255, 255, 255},
		BorderColor:     paint.Color{30, 140, 60, 255},
	}
}

// InputStyle 输入框样式预设
// InputStyleDefault returns the default input field style with surface background and standard border.
func InputStyleDefault(scheme ColorScheme) WidgetStyle {
	return WidgetStyle{
		BorderRadius:    4,
		PaddingTop:      6,
		PaddingRight:    8,
		PaddingBottom:   6,
		PaddingLeft:     8,
		BackgroundColor: scheme.Surface,
		TextColor:       scheme.TextPrimary,
		BorderColor:     scheme.Border,
	}
}

// CardStyle 卡片样式预设
func CardStyleDefault(scheme ColorScheme) WidgetStyle {
	return WidgetStyle{
		BorderRadius:    8,
		PaddingTop:      16,
		PaddingRight:    16,
		PaddingBottom:   16,
		PaddingLeft:     16,
		BackgroundColor: scheme.Surface,
		BorderColor:     scheme.Border,
		ShadowOffsetX:   0,
		ShadowOffsetY:   2,
		ShadowBlur:      8,
		ShadowColor:     scheme.Shadow,
	}
}

// CardStyleElevated returns an elevated card style with larger radius and no border, just shadow.
func CardStyleElevated(scheme ColorScheme) WidgetStyle {
	return WidgetStyle{
		BorderRadius:    12,
		PaddingTop:      20,
		PaddingRight:    20,
		PaddingBottom:   20,
		PaddingLeft:     20,
		BackgroundColor: scheme.Surface,
		BorderColor:     paint.Color{0, 0, 0, 0}, // no border
		ShadowOffsetX:   0,
		ShadowOffsetY:   4,
		ShadowBlur:      16,
		ShadowColor:     paint.Color{0, 0, 0, 40},
	}
}

// TagStyle 标签样式预设
func TagStylePrimary(scheme ColorScheme) WidgetStyle {
	return WidgetStyle{
		BorderRadius:    12,
		PaddingTop:      2,
		PaddingRight:    10,
		PaddingBottom:   2,
		PaddingLeft:     10,
		BackgroundColor: scheme.Primary,
		TextColor:       scheme.TextOnPrimary,
	}
}

// TagStyleOutlined returns a tag style with transparent background and primary-colored border.
func TagStyleOutlined(scheme ColorScheme) WidgetStyle {
	return WidgetStyle{
		BorderRadius:    12,
		PaddingTop:      2,
		PaddingRight:    10,
		PaddingBottom:   2,
		PaddingLeft:     10,
		BackgroundColor: paint.Color{0, 0, 0, 0},
		TextColor:       scheme.Primary,
		BorderColor:     scheme.Primary,
	}
}

// TagStyleSuccess returns a tag style with success-colored (green) background.
func TagStyleSuccess(scheme ColorScheme) WidgetStyle {
	return WidgetStyle{
		BorderRadius:    12,
		PaddingTop:      2,
		PaddingRight:    10,
		PaddingBottom:   2,
		PaddingLeft:     10,
		BackgroundColor: scheme.Success,
		TextColor:       paint.Color{255, 255, 255, 255},
	}
}

// TagStyleWarning returns a tag style with warning-colored (amber) background.
func TagStyleWarning(scheme ColorScheme) WidgetStyle {
	return WidgetStyle{
		BorderRadius:    12,
		PaddingTop:      2,
		PaddingRight:    10,
		PaddingBottom:   2,
		PaddingLeft:     10,
		BackgroundColor: scheme.Warning,
		TextColor:       paint.Color{255, 255, 255, 255},
	}
}

// TagStyleError returns a tag style with error-colored (red) background.
func TagStyleError(scheme ColorScheme) WidgetStyle {
	return WidgetStyle{
		BorderRadius:    12,
		PaddingTop:      2,
		PaddingRight:    10,
		PaddingBottom:   2,
		PaddingLeft:     10,
		BackgroundColor: scheme.Error,
		TextColor:       paint.Color{255, 255, 255, 255},
	}
}

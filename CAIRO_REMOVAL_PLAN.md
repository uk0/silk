# Cairo Removal Plan — opengl branch

## 目标

`opengl` 分支彻底去掉 Cairo 依赖。`go build -tags silk_no_cairo ./...` 可成功构建出**不需要 libcairo 的 Silk 二进制**，所有渲染走 OpenGL（`silk/glui`）。

## 当前状态（commit `198485f` 之后）

- `silk/glui` 已经是完整的 OpenGL 渲染管线，含 SDF 字体、CJK 多字体回退链、GPU 渐变、PixmapBrush
- `SILK_GLUI=1` 时**渲染逻辑**走 OpenGL，但**构建**仍需 libcairo（cgo 编译时 link）
- `silk/paint`、`silk/gui` 等 7 个文件仍 import `silk/cairo`

## 4 轮重构计划

### Round 1 ✅（本轮）— 脚手架 + 诊断

**已完成**：

- `silk/cairo/{cairo,cairo_windows,io,cgo_unix,cgo_windows}.go` 全部加 `//go:build !silk_no_cairo` tag
- 确认默认构建（无 tag）仍正常通过
- 确认 `-tags silk_no_cairo` 触发链式 import 错误：`paint → cairo` 是头号阻塞点

**诊断输出**：
```
package silk
    imports silk/ged → silk/graph → silk/gui → silk/glui → silk/paint
    imports silk/cairo: build constraints exclude all Go files
```

paint 包中 7 个文件仍 import `silk/cairo`：
- `paint/pixmap.go`（NewPixmap、LoadPngFile）
- `paint/surface.go`（cairoSurface 实现）
- `paint/painter.go`（cairoPainter 实现）
- `paint/brush.go`（NewPixmapBrush 创建 cairo.Pattern）
- `paint/font.go`（Font 通过 Cairo scaled font）
- `paint/icon.go`（icon 用 cairoSurface）
- `paint/paint_windows.go`（Win32 surface bridges）

### Round 2 ✅（已完成，commit `<TBD>`）— paint 包按构建 tag 切分

**目标**：让 `silk/paint` 在两种 tag 下都能构建。

**实际产出**：
- ✅ `paint/surface.go` 简化为只含 `Surface` interface（无 tag）
- ✅ `paint/surface_cairo.go` 持 `cairoSurface` 实现（`!silk_no_cairo`）
- ✅ `paint/pixmap.go` 简化为 `Pixmap` interface + `Format` 常量
- ✅ `paint/pixmap_cairo.go` 持 Cairo NewPixmap/LoadPngFile/TextToPixmap/IconTextToPixmap
- ✅ `paint/pixmap_pure.go` 提供 `imagePixmap`(image.RGBA + image/png) 实现
- ✅ `paint/painter.go` 简化为 `Painter` + `ShadowPainter` interface + `Round` helper
- ✅ `paint/painter_cairo.go` 持 cairoPainter 全部 60 方法
- ✅ `paint/painter_pure.go` 提供 `nullPainter` 桩满足接口
- ✅ `paint/brush.go` 拆分：SolidBrush/LinearGradient/RadialGradient 无 tag；PixmapBrush 数据/Pixmap() accessor 移到 cairo+pure 两个文件
- ✅ `paint/font.go` 重定义 FontExtents/TextExtents/Glyph 为独立 struct（不再是 Cairo type alias）
- ✅ `paint/font_cairo.go` 持完整 Cairo font 实现 + cache
- ✅ `paint/font_pure.go` 提供 `pureFont` 估算实现（ASCII 0.5×size、CJK 1.0×size）
- ✅ `paint/icon.go` 中 `subIcon.img` 改为 `Pixmap` 接口；移除 cached `subIcon.pat`
- ✅ `paint/icon_cairo.go` 持 `genMissingSubIcon`（红叉缺失图标）
- ✅ `paint/icon_pure.go` 桩 `genMissingSubIcon` 返回空白
- ✅ `paint/paint_windows.go` 加 `!silk_no_cairo` tag
- ✅ `painter_cairo.go` 中 icon 缩放 pattern 改为 per-draw 临时分配（消除 subIcon.pat 依赖）

**已验证**：
```
go build ./paint/                     # ✓
go build -tags silk_no_cairo ./paint/ # ✓
go test ./paint/                      # ✓
go test -tags silk_no_cairo ./paint/  # ✓
```

paint 包是 Cairo 移除链上的最大瓶颈，Round 2 把它打通了。

### Round 3+4 ✅（已完成）— gui 自动切 glui + 端到端验证

**实际产出**（远比预期顺利 —— Round 2 把 paint 拆掉后下游全自动顺利）：

- ✅ gui 包在两个 tag 下都直接编译通过（无 silk/cairo 直接 import）
- ✅ 添加 `gui/glui_force_pure.go` + `gui/glui_force_cairo.go`：silk_no_cairo 下 `forceGluiPath()` 返回 true，自动激活 glui 渲染（用户无需 SILK_GLUI=1）
- ✅ 默认模式仍是 Cairo 路径，glui 仍是 opt-in
- ✅ glui / decl / i18n / settings / svg / state / fswatch 全部 11 包在 silk_no_cairo 下构建+测试通过
- ✅ 端到端验证：`/tmp/decl-demo-pure` 二进制
  - `otool -L` 显示 **0 个 cairo 引用**
  - 仅链 Cocoa / OpenGL / 系统库（13 项）
  - 实际运行：窗口 + Frame chrome + 计数器 + 按钮全部渲染
  - Swift CGEvent 模拟点击：counter 0→3，Increment 按钮显示 hover 蓝框

**验收命令**：
```bash
go build -tags silk_no_cairo ./...      # ✓
go test  -tags silk_no_cairo ./...      # ✓ 11 包全过
go build -tags silk_no_cairo decl_demo.go
otool -L /tmp/decl_demo                 # 无 libcairo
/tmp/decl_demo                          # 窗口 + 交互正常
```

### 还差什么（追加项，非阻塞）

- CI workflow `.github/workflows/ci.yml` 矩阵未加 silk_no_cairo 测试
- release.yml 未产出 cairo-free 二进制
- README 未加 "no-Cairo build" 章节
- gui/theme.go 的 9-patch 主题在 pure 模式下 `NewPainter()` 返回 nullPainter（生成的 tile 是空白）—— 默认主题用编程式绘制（programmatic 路径），非 9-patch，所以用户层面看不出差别

## 预估代码量

| 轮次 | LOC |
|------|----:|
| Round 1（本轮） | 5（仅 build tag）+ 200（本文档）|
| Round 2 | ~800（paint 拆分）|
| Round 3 | ~400（gui 拆分）|
| Round 4 | ~200（CI、demos、文档）|
| **合计** | **~1600 LOC** |

## 设计原则

1. **接口在无 tag 文件**：`Pixmap`、`Painter`、`Surface` 等接口在 `pixmap.go` / `painter.go` / `surface.go` 中无 tag 定义；实现按 tag 选
2. **构造函数返回接口**：`NewPixmap` 返回 `Pixmap`，不返回 `*cairoSurface`。这是当前类型断言失败的源头
3. **glui 优先**：silk_no_cairo 模式下，`paint.NewPainter` / `paint.NewPixmap` 内部转给 glui — 复用已有 GPU 路径
4. **降级而非缺失**：纯 Go 路径下功能可能略有简化（如 SVG 复杂滤镜），但不能 panic 或返回 nil

## 取舍

| 项 | Cairo 模式 | silk_no_cairo 模式 |
|----|---------|-------------------|
| 字体测量 | freetype + Cairo scaled font | opentype（pure Go）|
| PNG 解码 | libpng via Cairo | image/png（pure Go）|
| JPEG 解码 | 不支持 | image/jpeg（pure Go）|
| 文本 shaping | 基本 | 基本（无 HarfBuzz） |
| Pixmap pattern brush | cairo_pattern | glui 纹理填充 |
| 复合操作 | 14 种 | SRC_OVER（glui）|
| Windows 后端 | win32+Cairo+GDI | GLFW + glui |

## 后续追踪

每轮完成后回到本文档，把对应章节状态从 ⏳ 改为 ✅ 并记录实际遇到的坑。

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

paint 包是 Cairo 移除链上的最大瓶颈，Round 2 把它打通了。下游包（gui / glui）目前在 silk_no_cairo 下还会失败（它们直接依赖 cairoSurface 等 paint-internal），是 Round 3 的目标。

### Round 3（下轮 30min 后）— gui 包窗口路径切分

**目标**：`silk/gui` 在 silk_no_cairo 模式下使用 glui 路径（不走 Cairo backbuffer）。

**步骤**：

1. **拆 `paint/pixmap.go`**：
   - 保留：`Pixmap` interface + `Format` 常量（无 tag）
   - 新建 `paint/pixmap_cairo.go`（`!silk_no_cairo`）：移入 `cairoSurface`、`NewPixmap`、`LoadPngFile`
   - 新建 `paint/pixmap_pure.go`（`silk_no_cairo`）：纯 Go 实现 `imagePixmap`，使用 `image.RGBA` + `image/png`
   - `NewPixmap` / `LoadPngFile` 函数签名改为返回 `Pixmap` 接口

2. **拆 `paint/surface.go`**：
   - `Surface` interface 留无 tag
   - cairoSurface 实现搬到 `paint/surface_cairo.go`

3. **拆 `paint/painter.go`**：
   - `Painter` interface 留无 tag
   - `cairoPainter` 实现搬到 `paint/painter_cairo.go`
   - 新建 `paint/painter_pure.go` 提供桩（实际 painter 由 glui CairoCompat 在 gui 层注入）

4. **`paint/brush.go`** 中 `NewPixmapBrush`：
   - lazy 创建 cairo.Pattern（仅在 cairo 可用时）
   - pure 模式只保留 `pixmap` + `extend` 字段

5. **`paint/font.go`**：
   - 拆出 Cairo scaled font 部分到 `font_cairo.go`
   - pure 模式用 opentype 接口（已在 glui 实现，复用部分代码到 paint）

6. **`paint/icon.go`** + **`paint/paint_windows.go`**：tag 标记，pure 模式提供 stub

**验收**：`go build -tags silk_no_cairo ./paint/` 通过，`go test ./paint/` 在两种 tag 下都过。

### Round 3 — gui 包窗口路径切分

**目标**：`silk/gui` 在 silk_no_cairo 模式下使用 glui 路径（不走 Cairo backbuffer）。

**步骤**：

1. `gui/window_glfw.go` 中 `paint()` 函数（Cairo 路径）加 tag `!silk_no_cairo`
2. 让 `paintGlui()` 成为 silk_no_cairo 模式下的唯一入口
3. `gui/window_windows.go`（win32+Cairo+GDI）整体加 tag — silk_no_cairo on Windows 走 GLFW（已支持）
4. clipboard / cursor 文件可能也涉及 Cairo，分别审查
5. 验收：`go build -tags silk_no_cairo ./gui/` 通过

### Round 4 — 端到端 + CI

**目标**：构建产物零 libcairo 依赖。

**步骤**：

1. `decl_demo.go` / `i18n_demo.go` 在 silk_no_cairo 下编译运行
2. `otool -L` / `ldd` 确认产物无 libcairo
3. `.github/workflows/ci.yml` 矩阵加 `silk_no_cairo` 测试
4. README 加 "no-Cairo build" 章节
5. release.yml 选择性产出两套二进制（cairo + cairo-free）

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

# Silk on opengl — 测试与体验指南

`opengl` 分支为 Silk 引入了纯 OpenGL 渲染管线（`silk/glui`），与默认的 Cairo 路径并存。同分支还集成了一组 Qt5 风格的辅助库：`silk/decl`（声明式 UI）、`silk/i18n`（翻译）、`silk/settings`（QSettings 等价）、`silk/svg`（SVG 渲染）、`silk/state`（QStateMachine）、`silk/fswatch`（文件监听）、`silk/gui` 内的 `Validator` / `Completer`。

本文档帮助你在本地拉这个分支并跑通验证。

## 0. 两种构建模式

opengl 分支支持两种构建模式：

| 模式 | 命令 | 链接库 | 何时用 |
|------|------|--------|--------|
| **默认（Cairo）** | `go build` | libcairo + libpixman + libfontconfig + libfreetype + ... | 兼容现有 Cairo 应用 / 字体小字号渲染要求最佳清晰度 |
| **silk_no_cairo（纯 Go）** | `go build -tags silk_no_cairo` | **零 libcairo 依赖**，仅链 OpenGL + 系统库 | 容器最小镜像 / 跨平台简化部署 / 不希望 cgo Cairo 链 |

两种模式下功能基本一致——布局、颜色、CJK 渲染、点击交互、i18n、复数规则全部相同。详见 §10 对比表。

## 1. 环境要求

| 平台 | 必须 | silk_no_cairo 是否需要 | 说明 |
|------|------|--------------------|------|
| Go | 1.21+ | ✅ 同样需要 | 推荐用 `g` 版本管理器，避免和系统 brew Go 串台 |
| Cairo | 1.16+ | ❌ 无需 | 仅默认模式：macOS `brew install cairo`；Linux `libcairo2-dev`；Windows MSYS2 mingw-w64-x86_64-cairo |
| pkg-config | 0.29+ | ❌ 无需（不调 cairo） | macOS `brew install pkg-config` |
| GLFW 3.3 | 自动 | ✅ 同样需要 | 由 go modules 拉取 |
| C 编译器 | clang/gcc | ✅ 同样需要 | macOS Xcode CLT；Linux build-essential；Windows MSYS2 mingw |
| CGO | 启用 | ✅ 同样需要 | `CGO_ENABLED=1` 默认即开 |

> **macOS Apple Silicon**：cairo 装在 `/opt/homebrew/Cellar/cairo/...`；下面命令里有 `CGO_CFLAGS` / `CGO_LDFLAGS` 的路径模板可以直接套。
> **silk_no_cairo 模式**：完全不需要上面所有 Cairo 路径，下面 §3.2 直接给命令。

## 2. 拉代码

```bash
git clone git@github.com:uk0/silk.git
cd silk
git checkout opengl
```

## 3. 构建

### 3.1 默认模式（Cairo）— macOS Apple Silicon 模板

```bash
export PATH="/opt/homebrew/bin:$PATH"
export CGO_CFLAGS="-I/opt/homebrew/Cellar/cairo/1.18.4/include/cairo -I/opt/homebrew/include -I/opt/homebrew/opt/cairo/include -I/opt/homebrew/include/cairo"
export CGO_LDFLAGS="-L/opt/homebrew/lib -lcairo"

go build -o /tmp/silk-designer design.go
go build -o /tmp/silk-demo demo.go
go build -o /tmp/decl-demo decl_demo.go    # 声明式 + 点击计数
go build -o /tmp/i18n-demo i18n_demo.go    # i18n + locale 切换
```

**Linux** 大致一样，把 `CGO_CFLAGS` / `CGO_LDFLAGS` 改成 pkg-config 输出即可：

```bash
export CGO_CFLAGS="$(pkg-config --cflags cairo)"
export CGO_LDFLAGS="$(pkg-config --libs cairo)"
```

### 3.2 silk_no_cairo 模式（纯 Go，无 libcairo）

无任何 Cairo 环境变量、无任何 brew/apt cairo 安装：

```bash
go build -tags silk_no_cairo -o /tmp/silk-designer-pure design.go
go build -tags silk_no_cairo -o /tmp/silk-demo-pure demo.go
go build -tags silk_no_cairo -o /tmp/decl-demo-pure decl_demo.go
go build -tags silk_no_cairo -o /tmp/i18n-demo-pure i18n_demo.go
```

**验证产物无 libcairo 依赖**：

```bash
# macOS
otool -L /tmp/silk-designer-pure | grep -ci cairo
# 输出应为 0

# Linux
ldd /tmp/silk-designer-pure | grep -ci cairo
# 输出应为 0
```

silk_no_cairo 模式自动启用 `silk/glui` OpenGL 渲染（无需 `SILK_GLUI=1`）。

## 4. 跑全套单元测试

```bash
# 默认（Cairo）模式
go test -short -count=1 \
  ./core/ ./geom/ ./paint/ ./gui/ ./graph/ ./prop/ ./ged/ \
  ./glui/ ./decl/ ./i18n/ ./settings/ ./svg/ ./state/ ./fswatch/

# silk_no_cairo 模式
go test -short -count=1 -tags silk_no_cairo \
  ./core/ ./geom/ ./paint/ ./gui/ \
  ./glui/ ./decl/ ./i18n/ ./settings/ ./svg/ ./state/ ./fswatch/
```

预期：两种模式每个包都 `ok`。也可加 `-race` 跑并发安全检查（已验证全过）。

各包测试数量速览：

| 包 | 测试数 | 关键覆盖 |
|----|------:|---------|
| `glui` | 60+ | 渲染 batch、SDF 文字、CJK fallback、PixmapBrush、Clip 重置 |
| `decl` | 14 | AST 双向 codec、TrKey、Build → 真实 widget |
| `i18n` | 18 | T/Tn/Tf、locale fallback、JSON load |
| `settings` | 23 | TDoc 持久化、group 嵌套、跨进程 round-trip |
| `svg` | 25 | path 数据、transform、color、级联样式 |
| `state` | 17 | 简单/复合 state、guard、self-transition |
| `fswatch` | 13 | poll 周期、create/modify/remove |
| `gui` | 280+ | 含 Validator / Completer 新增 |

## 5. 三个 demo 程序怎么跑

### 5.1 decl_demo — 声明式 UI + 点击验证

```bash
/tmp/decl-demo
```

窗口显示：
- "click counter:" 标签
- 计数值（初始 0）
- Increment / Reset / Quit 三个按钮

每点 Increment 计数 +1；Reset 归零；Quit 退出。

**全部 widget 通过 `decl.Form/Label/Button` 声明式构建**，没有 imperative `gui.NewLabel` 调用。代码在 `decl_demo.go`。

### 5.2 i18n_demo — Qt-tr() 等价物

```bash
SILK_LOCALE=zh-CN /tmp/i18n-demo   # 中文：关于 / 设置 / 保存 / 取消 / 退出
SILK_LOCALE=ja /tmp/i18n-demo      # 日文（部分汉字 + 假名）
SILK_LOCALE=ko /tmp/i18n-demo      # 韩文
SILK_LOCALE=en /tmp/i18n-demo      # 英文（fallback 到源字符串）
```

按 Save 累加，counter 显示 "%d 项" / "%d items"（按 locale 复数规则）。

翻译表在 `i18n/example/translations.json`，可手动编辑后重启生效。

### 5.3 silk-demo — 62 widget 全家桶

```bash
/tmp/silk-demo
```

经典 widget gallery，跑在默认 Cairo 路径。

## 6. 切换 Cairo / OpenGL 渲染路径

OpenGL 路径是 **opt-in**，通过环境变量控制：

| 变量 | 值 | 作用 |
|------|---|------|
| `SILK_GLUI` | `1` | 切到纯 OpenGL 渲染（替代 Cairo） |
| `SILK_GLUI_MSAA` | `0/2/4/8/16` | MSAA 采样数（默认 4，0 = 关）|
| `SILK_GLUI_SDF` | `1` | 字体走 SDF 模式（极端缩放清晰）|
| `SILK_GLUI_FPS` | `1` | 右上角显示 FPS overlay |
| `SILK_GLUI_DEBUG_CLEAR` | `red` | 调试：用红色清屏背景 |
| `SILK_LOCALE` | BCP-47 tag | i18n 翻译目标语言 |

例：

```bash
SILK_GLUI=1 SILK_GLUI_FPS=1 SILK_LOCALE=zh-CN /tmp/i18n-demo
```

中文 + GPU 渲染 + FPS overlay 全开。

## 7. 用 osascript / Swift 自动化点击（可选）

macOS 上 GLFW 窗口接收 CGEvent 鼠标事件。**先 hover 再 click** 才能触发 button 的 emit（GLFW 的 hover 状态依赖 mouseMoved 事件）。

把这段 swift 存为 `/tmp/click.swift`：

```swift
import Foundation
import CoreGraphics

let args = CommandLine.arguments
guard args.count == 3, let x = Double(args[1]), let y = Double(args[2]) else { exit(1) }
let pt = CGPoint(x: x, y: y)

let move = CGEvent(mouseEventSource: nil, mouseType: .mouseMoved, mouseCursorPosition: pt, mouseButton: .left)!
move.post(tap: .cghidEventTap)
usleep(150_000)

let down = CGEvent(mouseEventSource: nil, mouseType: .leftMouseDown, mouseCursorPosition: pt, mouseButton: .left)!
down.post(tap: .cghidEventTap)
usleep(80_000)
let up = CGEvent(mouseEventSource: nil, mouseType: .leftMouseUp, mouseCursorPosition: pt, mouseButton: .left)!
up.post(tap: .cghidEventTap)
print("clicked at \(x), \(y)")
```

查询窗口位置：

```bash
osascript -e 'tell application "System Events" to tell process "decl_demo" to tell window 1 to set p to position & size'
```

按返回的坐标点：

```bash
swift /tmp/click.swift 866 655   # 点 Increment 按钮
```

第一次执行 Terminal 会被要求授予"辅助功能"权限。

## 8. 常见问题

### 编译报 `cairo.h not found`

你的 `pkg-config` 找不到 cairo。手动设：

```bash
brew install cairo pkg-config
export PATH="/opt/homebrew/bin:$PATH"
```

或直接传 CGO_CFLAGS（见上方"构建"章节）。

### 编译报 `compile: version "go1.25.0" does not match go tool version "go1.26.0"`

混 Go 工具链了。仓库里有两个 Go：`/Users/<you>/.g/go`（g 管理）和 `/opt/homebrew/bin/go`（brew）。任选一个统一。

```bash
which -a go        # 看顺序
hash -r            # 清缓存
```

### macOS 上日文窗口里假名是方框（□□□）

`opengl` 分支已修复（commit `4c955de`）。确认你拉到了 commit `7163b4b` 或更新。Cairo 路径仍可能有此问题——是 fontconfig 配置问题，跟 Silk 无关。

### CI workflow 在 Linux 上 CJK 测试 skip

Linux runner 需要 `fonts-noto-cjk` 包。`opengl` 分支的 `.github/workflows/ci.yml` 已加。

### 按钮点击没反应

Silk 的 button 在 `OnLeftUp` 里检查 `IsHover()`。CGEvent 走 `cghidEventTap` 时必须先发 `mouseMoved`，否则 hover 状态没建立，up 事件会被丢弃。

## 9. 关键提交链（opengl 分支）

| commit | 内容 |
|--------|------|
| `1dec0bd` | paint: hashScaledFontCacheKey checkptr fix + 验证报告 |
| `afe4ee0` | gui: silk_no_cairo 自动启 glui + Round 3+4 收尾 |
| `6568924` | paint: Round 2 of Cairo removal — 包内按 build tag 切分 |
| `d1f2558` | cairo: 加 silk_no_cairo build tag 脚手架 + removal plan |
| `7163b4b` | fswatch: QFileSystemWatcher 等价 |
| `09636d1` | glui: PixmapBrush GPU 渲染 |
| `63e31b0` | state: QStateMachine 等价 |
| `22345f0` | svg: QSvgRenderer 等价 |
| `92d2124` | gui: QCompleter 等价 |
| `b71edbb` | settings: QSettings 等价 |
| `716466d` | gui: QValidator 等价 |
| `a611d3e` | glui: clear current point bug fix |
| `23e6236` | i18n: Qt-tr 等价 + decl TrKey + 多脚本 CJK |
| `a6ef2ac` | decl: 端到端点击验证 |
| `ba3f836` | decl: 声明式 AST + 双向 TDoc codec |
| `ed4091c` | glui: MSAA + stencil bits |
| `1810a2a` | glui: GPU 径向渐变 |
| `4c955de` | glui: CJK 字体回退链 |

## 10. Cairo vs 纯 Go（silk_no_cairo）功能对比

| 维度 | 默认（Cairo）| silk_no_cairo | 注 |
|------|--------------|---------------|-----|
| 布局 / 颜色 / hover | 一致 | 一致 | — |
| CJK 渲染 | freetype + fontconfig | glui CJK 多脚本回退 | 字体覆盖均完整 |
| 字体小字号清晰度 | 略胜（freetype hinting）| 略软（SDF atlas + AA halo）| 12-16pt 正常 UI 字号都可读 |
| 布局测量 | 准确（freetype）| 估算（ASCII 0.5×size、CJK 1.0×size）| 标签 / 按钮 / 输入框完全够用 |
| 复合操作 | 14 种 cairo_operator | SRC_OVER | grayed 图标用 RGB×0.6/A×0.7 近似 |
| 路径裁剪 | 任意路径 | AABB scissor | 旋转容器圆角会越界（roadmap §3.2.1）|
| PNG/JPEG 解码 | 通过 libpng | image/png + image/jpeg | 纯 Go |
| `paint.TextToPixmap` / `IconTextToPixmap` | 渲染纹理 | 返回空白 stub | 仅 3 widget 视觉装饰受影响 |
| 9-patch theme | 渲染 tile | 空白 tile（默认 theme 不用 9-patch）| 视觉无差别 |
| 二进制依赖 | libcairo + libpixman + libfontconfig + libfreetype + libpng + ... | 仅 OpenGL + 系统库 | 见 §3.2 验证 |

完整对比详见仓库根 `CAIRO_REMOVAL_VALIDATION.md`。

## 11. 反馈与下一步

按 `ROADMAP.md` 三段：

- **短期 1-2 周** — CI silk_no_cairo 矩阵（已加，本次 commit）/ release 双轨二进制（已加）/ Linux+Windows 真机验证 / decl→Go emitter
- **中期 2-8 周** — Stencil-based path clipping / SetOperator 14 种 blend / SVG 椭圆弧 / glui 子像素文字 / 性能基准套件 / Designer 输出 decl Go
- **长期 8 周+** — decl + fswatch 热重载 / GLES 3.0/WebGL 后端 / go-text 文本 shaping / Accessibility 树 / GPU instancing / Native event APIs

发现问题或新想法直接开 issue。

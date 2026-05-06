# Silk on opengl — 测试与体验指南

`opengl` 分支为 Silk 引入了纯 OpenGL 渲染管线（`silk/glui`），与默认的 Cairo 路径并存。同分支还集成了一组 Qt5 风格的辅助库：`silk/decl`（声明式 UI）、`silk/i18n`（翻译）、`silk/settings`（QSettings 等价）、`silk/svg`（SVG 渲染）、`silk/state`（QStateMachine）、`silk/fswatch`（文件监听）、`silk/gui` 内的 `Validator` / `Completer`。

本文档帮助你在本地拉这个分支并跑通验证。

## 1. 环境要求

| 平台 | 必须 | 说明 |
|------|------|------|
| Go | 1.21+ | 推荐用 `g` 版本管理器，避免和系统 brew Go 串台 |
| Cairo | 1.16+ | macOS `brew install cairo`；Linux `libcairo2-dev`；Windows MSYS2 mingw-w64-x86_64-cairo |
| pkg-config | 0.29+ | macOS `brew install pkg-config` |
| GLFW 3.3 | 自动 | 由 go modules 拉取 |
| C 编译器 | clang/gcc | macOS Xcode CLT；Linux build-essential；Windows MSYS2 mingw |
| CGO | 启用 | `CGO_ENABLED=1` 默认即开 |

> **macOS Apple Silicon**：cairo 装在 `/opt/homebrew/Cellar/cairo/...`；下面命令里有 `CGO_CFLAGS` / `CGO_LDFLAGS` 的路径模板可以直接套。

## 2. 拉代码

```bash
git clone git@github.com:uk0/silk.git
cd silk
git checkout opengl
```

## 3. 构建（macOS Apple Silicon 模板）

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

## 4. 跑全套单元测试

```bash
go test -short -count=1 \
  ./core/ ./geom/ ./paint/ ./gui/ ./graph/ ./prop/ ./ged/ \
  ./glui/ ./decl/ ./i18n/ ./settings/ ./svg/ ./state/ ./fswatch/
```

预期：每个包都 `ok`。

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

## 10. 反馈与下一步

剩余 gap（按优先级）：

- **Stencil-based path clipping** — 旋转容器才用得上，是真正的非 AABB 限制
- **SetOperator / 复合模式** — 14 种 Cairo blend mode 当前都映射成 SRC_OVER
- **decl→Go source emitter** — 让设计器输出可读 .silk.go，闭合 round-trip
- **跨平台 release pipeline 在 Windows 真机端到端验证**

发现问题或新想法直接开 issue。

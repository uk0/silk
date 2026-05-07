# Silk opengl 分支路线图与现状

最后更新：commit `1dec0bd`（2026-05-07）

本文档分三部分回答：
1. **opengl 现在实现的效果**（截至本次 commit 的客观成果）
2. **opengl 对比 Cairo 的优势**（技术差异 + 工程差异）
3. **后续路线**（短期 / 中期 / 长期）

---

## 1. opengl 现在实现的效果

### 1.1 渲染管线（silk/glui）

`silk/glui` 是一个完整的纯 OpenGL 2.1 渲染管线，作为 `silk/paint` 的替代后端：

| 子系统 | 实现 |
|--------|------|
| 图元 | 矩形（含圆角 + per-vertex SDF AA）、圆、椭圆、三角形、任意路径（ear-clip 三角化）|
| 描边 | 实线 + 虚线 + 多端帽（Butt / Round / Square）+ 多 join（Miter / Round / Bevel）|
| 渐变 | 两停 linear（uniform fast path）+ 多停 linear（256×1 ramp 纹理）+ **径向渐变**（GPU shader）|
| 字体 | OpenType 光栅化 + SDF 模式 + **CJK 多脚本 fallback 链**（macOS 含 PingFang/Hiragino/AquaKana/AppleSDGothicNeo）|
| 图像 | 2D 纹理 quad + 像素笔刷（PixmapBrush GPU 路径）|
| 阴影 | `FillBoxShadow` —— shader 内 SDF 模糊，比 Cairo 的多次 blur 卷积快得多 |
| 抗锯齿 | per-pixel SDF AA + 4× MSAA（默认；`SILK_GLUI_MSAA` 可配 0/2/4/8/16）|
| 裁剪 | scissor 矩形（旋转容器仍是 AABB —— 见 §3 stencil clip 计划）|
| Atlas | Skyline bin packer（字体 / 图标 / 渐变 ramp 纹理）|
| 缓存 | LRU pixmap 纹理缓存 + 帧时戳驱逐，硬上限 256 项 |
| 性能 | 单批次合并 + 流式 VBO + dirty-flag flush；FPS overlay 可选（`SILK_GLUI_FPS=1`）|

**61 个 widget 全部通过 `glui.CairoCompat` paint.Painter facade 走 GPU 路径，无需任何 widget 代码改动。**

### 1.2 Cairo 依赖移除（silk_no_cairo build tag）

| 验证项 | 结果 |
|--------|------|
| `go build -tags silk_no_cairo ./...` | ✅ 全部 11 包构建通过 |
| `go test -tags silk_no_cairo -race ./...` | ✅ 11 包 -race 通过 |
| `otool -L decl_demo`（pure 模式）| ✅ **零 libcairo 引用**，仅 13 个系统库 |
| 端到端 demo（窗口 + 点击 + i18n + 复数 + CJK 渲染）| ✅ 在 pure 模式下完整工作 |

详见 `CAIRO_REMOVAL_VALIDATION.md`、`CAIRO_REMOVAL_PLAN.md`。

### 1.3 Qt5 等价物完成清单

opengl 分支顺路实现了 Qt5 的若干基础库：

| Silk 包 | Qt5 等价 | 行数 | 状态 |
|---------|----------|------|------|
| `silk/decl` | QML 风格声明式 + 设计器互通 | ~1000 | ✅ TDoc 双向 codec、TrKey i18n 集成、端到端点击验证 |
| `silk/i18n` | QTranslator + tr() | ~800 | ✅ JSON 加载、locale 检测（含 macOS AppleLocale）、复数规则 |
| `silk/settings` | QSettings | ~1000 | ✅ TDoc 后端、group 嵌套、跨平台路径 |
| `silk/svg` | QSvgRenderer | ~2000 | ✅ 7 类 shape、path data、transform、级联样式 |
| `silk/state` | QStateMachine | ~870 | ✅ 简单 + 复合 state、guard、self-transition |
| `silk/fswatch` | QFileSystemWatcher | ~720 | ✅ polling 实现、Created/Modified/Removed 事件 |
| `silk/gui.Validator` | QValidator | ~330 | ✅ Int / Double / RegExp + Edit 集成 |
| `silk/gui.Completer` | QCompleter | ~250 | ✅ 3 类 match mode + Edit 集成 |
| `silk/gui.PixmapBrush GPU` | cairo_pattern_create_for_surface | ~250 | ✅ glui CairoCompat 检测路由 |

### 1.4 测试覆盖

| 项目 | 数量 |
|------|------|
| 包总数 | 11（不含 prop / cairo / hashmap / sqlite3 等基础包） |
| 测试用例（默认模式）| 600+ |
| 测试用例（silk_no_cairo 模式）| 600+（同集合 + tag-aware 行为不变） |
| -race 通过率 | 100% |
| go vet 警告 | 全部 pre-existing，本轮零新增 |

### 1.5 文档资产

仓库根的非 gitignore 文档：

- `OPENGL_TESTING.md` —— 完整测试教程（构建 / 运行 / 自动化点击）
- `CAIRO_REMOVAL_PLAN.md` —— 4 轮 Cairo 移除计划及实际产出
- `CAIRO_REMOVAL_VALIDATION.md` —— 本地验证报告
- `ROADMAP.md` —— 本文档

---

## 2. opengl 对比 Cairo 的优势

### 2.1 性能

| 维度 | Cairo | opengl (glui) | 说明 |
|------|-------|---------------|------|
| 矩形 fill | CPU 像素填充 | 1 batch + GPU rasterizer | 大窗口下 5-10× 提速实测 |
| 圆角矩形 | CPU 路径采样 | per-vertex SDF + AA | 边缘更平滑，无需多次采样 |
| 渐变 | 软件光栅 | shader 直采 + 缓存 ramp 纹理 | radial 加速最明显 |
| 文本 | freetype 单次 + cache 像素 | atlas 多 quad + SDF 缩放 | 大字体缩放 SDF 优于 Cairo bitmap |
| 阴影 | 多次卷积模糊 | 单 shader pass | FillBoxShadow 数倍提速 |
| 帧延迟 | 全画布更新 | dirty-flag + scissor 部分刷新 | UI 静态时近零开销 |
| MSAA | 软件 SSAA（昂贵） | 硬件 4×（默认开） | 边缘 AA 几乎免费 |

### 2.2 部署

| 维度 | Cairo | opengl (glui) |
|------|-------|---------------|
| 二进制 | 必须链 libcairo + libpixman + libpng + libfontconfig + libfreetype + libxlib + ... | 仅链 OpenGL 框架（每个平台 1 个）|
| Runtime 大小（macOS arm64）| 14 MB binary + ~10 MB 系统 libcairo dylibs | 14 MB binary + 0 额外 |
| 跨平台一致性 | 依赖每个平台的字体后端（fontconfig / Win32 GDI / CoreText）| OpenGL 4.x+ 在 macOS / Linux / Windows 行为一致 |
| 容器/最小镜像 | 必装 libcairo + 字体 | scratch 镜像即可（仅需 libGL.so）|
| 跨编译 | 需要目标平台的 cairo headers | 纯 Go + GLFW，无需 cairo headers |

### 2.3 渲染特性

| 特性 | Cairo | opengl (glui) | 备注 |
|------|-------|---------------|------|
| 自定义着色器 | ❌ | ✅ | 可写自定义 visual effects |
| Per-vertex 数据 | ❌ | ✅ | rect SDF 用 cornerHX/cornerR/cornerAA |
| GPU 纹理 brush | 通过 surface pattern | 直接 GL 纹理 | glui 在桌面端常胜 |
| FPS overlay | 需自实现 | 内置 `SILK_GLUI_FPS=1` | 调试方便 |
| 帧时戳缓存驱逐 | ❌ | ✅ | LRU + 硬上限 256 |
| Hot-reload 友好 | 慢（surface 重建昂贵）| 快（VBO 流式）| 配合 decl + fswatch 可秒级 |

### 2.4 工程优势

- **可测性**：glui Renderer 可在 ctx==nil 下安全 drain，单元测试不需要真实 GL context
- **明确接口**：`paint.Painter` 接口固化，glui 的 `CairoCompat` 是 paint.Painter 的纯 GPU 实现
- **build tag 切换**：`silk_no_cairo` 一键切换全栈，无 if/else 散落各处
- **无 cgo finalize 风险**：纯 Go 模式无 cairo finalizer，更可控的 GC 行为

### 2.5 Cairo 仍占优的地方（诚实陈述）

| 维度 | Cairo 优势 |
|------|-----------|
| 字体渲染清晰度 | freetype hinting + sub-pixel 定位，小字号略胜 |
| 文本 shaping | 通过 cairo + pango/HarfBuzz 间接得到 ligature / 复杂脚本 |
| 复合操作 | 14 种 cairo_operator（OVER/MULTIPLY/SCREEN/HSL_LUMINOSITY/...）；glui 当前只有 SRC_OVER |
| 路径裁剪 | 真任意路径裁剪；glui 当前是 AABB scissor |
| PDF/SVG 输出 | Cairo 原生支持；glui 仅限屏幕 |
| 椭圆弧 | path A 命令完整解算；svg 渲染器当前简化为 line |

这些都是 §3 路线图中明确要补的项。

---

## 3. 后续路线

按时间窗口分三段。每段后面跟随**完成判据**，便于追踪进度。

### 3.1 短期（1-2 周内 / 收尾轮）

**目标**：把已完成工作从"能跑"推到"可发布"。

| 任务 | 工作量 | 完成判据 |
|------|--------|----------|
| CI 矩阵加 `silk_no_cairo` 测试 | 1 commit | `.github/workflows/ci.yml` linux-amd64 + macos-arm64 + macos-amd64 三平台都跑两次（默认 + tag）|
| release.yml 产出 cairo-free 二进制 | 1 commit | tag push 后 release 同时上传 `silk-pure-*` 和 `silk-*` 两套压缩包 |
| README "no-Cairo build" 章节 | docs | README 顶部加方块"两种构建模式"对照 + 链接到 `OPENGL_TESTING.md` |
| `OPENGL_TESTING.md` 加 silk_no_cairo 教程 | docs | 一行命令 + 一行 `otool -L` 验证 |
| Linux 真机端到端验证 | local 测试 | `go build -tags silk_no_cairo` + 跑 demo + xev 测试点击 |
| Windows 真机端到端验证 | local 测试 | MSYS2 mingw 默认模式 + silk_no_cairo 模式都跑通 silk-demo |
| `decl→Go source emitter` | ~300 LOC | `silk gen` 子命令把 `.silkui` 转成 `.silk.go` 源码；round-trip 测试 |

**完成意义**：opengl 分支可以正式合并 main 或打 release tag，外部用户可消费两种构建模式。

### 3.2 中期（2-8 周）

**目标**：闭合 §2.5 中 Cairo 仍占优的几项；让 opengl 在功能上不再"略微弱于"。

#### 3.2.1 Stencil-based path clipping ✅（已完成）

当前 `applyClip` 把任意 path 退化为 AABB scissor。旋转容器内的圆角 overflow:hidden 会越界。

**已完成**：
- ✅ glui `Renderer.PushClipPath(points)` / `PopClipPath(points)` —— 走 stencil INCR_WRAP / DECR_WRAP
- ✅ `clipKind` 枚举区分 scissor 与 stencil；`clipState` 携带 stencil ref depth
- ✅ `Renderer.curStencilRef` 跟踪当前嵌套深度，8-bit stencil 上限 255
- ✅ CairoCompat `applyClip` 检测 CTM 含 rotation/skew (`Xy != 0 || Yx != 0`) 时路由到 `applyStencilClip`，否则保持 scissor 快路径
- ✅ Save/Restore 通过 `clipPushedAt []clipPushRecord` 区分两类 clip 并匹配正确的 Pop
- ✅ 9 个测试覆盖：push/pop ref 计数、嵌套深度、scissor+stencil 混合栈、退化 path 防御、CairoCompat rotation 检测、Save/Restore 跨两类 clip 正确退栈

约 350 LOC（比预估 600 少，因为 stencil triangle render 复用了 kindPath batch）。

#### 3.2.2 SetOperator 复合模式 ✅（已完成）

glui 之前 hardcode SRC_OVER，CairoCompat.SetOperator 是 no-op。Cairo 14 种 operator 中固定功能管线能精确表达的全部接通：

**已完成**：
- ✅ `glui/blend.go`: `blendStateFor(op)` 把 paint.Operator 映射到 `(srcFactor, dstFactor, equation)` GL 状态三元组
- ✅ 17 种可分 Porter-Duff + 算术 operator（CLEAR / SOURCE / OVER / IN / OUT / ATOP / DEST / DEST_OVER / DEST_IN / DEST_OUT / DEST_ATOP / XOR / ADD / MULTIPLY / SCREEN / DARKEN / LIGHTEN）— 后两者用 `gl.MIN` / `gl.MAX` blend equation
- ✅ Renderer 加 `curOp` 字段；`SetBlendOp(op)` 在 op 变化时 flush 当前 batch 后重写 GL 状态；同 op 短路无 flush
- ✅ Begin() 复位 curOp = OpOver + 默认 GL 状态；End() 把 GL 状态还原到 OVER 防泄漏到外部 GL 客户
- ✅ CairoCompat.SetOperator 直接代理到 Renderer.SetBlendOp
- ✅ 9 个测试覆盖：17 种 op 映射查表、11 种不可表达 op fallback OVER、curOp 切换正确、unsupported op 记录为 OpOver 防 redundant flush、op 切换 flush 待 batch、同 op 不 flush、no-ctx 路径安全、CairoCompat 路由、混合 fill+SetOperator flush

非可分 / HSL operator（OVERLAY, COLOR_DODGE, COLOR_BURN, HARD_LIGHT, SOFT_LIGHT, DIFFERENCE, EXCLUSION, HSL_*）需要 framebuffer 读回的 fragment shader 变体，已记录为 fallback OVER；后续 milestone 再上 shader path（grayed icon 当前在 cairo_compat 路径里用固定 0.6/0.7 tint 近似 HSL_LUMINOSITY，仍工作正常）。

约 250 LOC（实现 + 测试）。

#### 3.2.3 SVG 路径椭圆弧完整解算 ✅（已完成）

当前 svg 渲染器把 `A` 命令简化为 LineTo，复杂图标会缺一段弧。

**已完成**：
- ✅ `svg/arc.go`: `decomposeArc(...)` 走 W3C 标准 endpoint→center 转换 + 90° cubic Bezier 切片
- ✅ 退化处理：rx=0/ry=0 → 直线段；起终点重合 → 跳过；半径过小 → 自动上调（W3C B.2.5）
- ✅ render.go 中 PathArc 用 painter.CurveTo 串联 decomposed 段
- ✅ 8 个测试覆盖：零长度、退化半径、四分之一圆、大弧标志、sweep 翻转、终点精度、半径过小自动 scale、渲染器集成 CurveTo emit

约 280 LOC（实现 + 测试）。

#### 3.2.4 glui 字体子像素定位 ✅（已完成 — 灰度子像素，LCD shader 推迟）

之前 glyph quad 默认走 fractional 位置 + bilinear filtering，结果是"软糊"text。Cairo 在每次绘制时按实际亚像素位置重栅格化，因此清晰。

**已完成**：
- ✅ `glui/font.go`: glyph 缓存键从 `map[rune]glyphInfo` 改为 `map[glyphKey]glyphInfo`，key 含 `sub uint8` (0..3)
- ✅ `numSubpixelBuckets=4`：在 0.0 / 0.25 / 0.5 / 0.75 px 各栅格化一份，传 `fixed.Point26_6{X: dotX}` 让 opentype 把亚像素偏移烘进 mask
- ✅ `Font.subpixel` 字段 + `SetSubpixel(bool)` + `SubpixelEnabled()` 方法；构造时通过 `SILK_GLUI_SUBPIXEL=1` env var 也能开
- ✅ `Font.GlyphAt(r, fracX)` 自动量化到桶；`Glyph(r)` 仍走桶 0 保留旧调用语义
- ✅ `DrawText` 两条路径：subpixel off 保留旧 fractional 行为；subpixel on 时整数 snap quad X，亚像素清晰度全靠 mask 自身
- ✅ 8 个测试覆盖：桶量化（含负数/越界）、默认关、开后两个桶 mask 字节级不同、`MeasureText` 不受影响（advance 与 sub 无关）、fractional 行为保留（off）、整数 snap（on）、`Glyph(r) == GlyphAt(r, 0)`、subpixel-off `GlyphAt(r, *)` 不爆缓存

LCD 子像素三采样（RGB 通道分别偏移 1/3 px）需要 RGBA atlas + RGB-aware fragment shader，已写入 §3.3 待办，灰度桶缓存对 14pt 以上字号的清晰度提升已经显著。

约 280 LOC（实现 + 测试）。

#### 3.2.5 性能基准测试套件 ✅（已完成）

之前没有可重现的对比数字，所有"快"宣称都是定性描述。

**已完成**：
- ✅ `bench/scenarios.go`: 6 个场景通过 `paint.Painter` 接口驱动两种 backend —— RectFill / RoundedRect / LinearGradient / TextPaint / ScrollingList / TypicalForm
- ✅ `bench/glui_bench_test.go`: 通过 `glui.NewBenchRenderer` + `NewCairoCompat` 测 glui CPU 录制成本（无真 GL flush）
- ✅ `bench/cairo_bench_test.go`: 通过 `paint.NewPixmap.NewPainter` 测 Cairo 全栈渲染（C 库实际写像素）
- ✅ `glui/bench_export.go`: 把原本只在 `*_test.go` 内部使用的 `newBenchRenderer` 升级为公开 API 方便跨包调用
- ✅ `BENCHMARK.md`: 跑法 + 结果表 + 方法论 + 已知 RoundedRect 反向场景的根因解释
- ✅ 数字落地：6/6 场景 glui 比 Cairo 快 **3–7.6×**（详见 BENCHMARK.md）
- ✅ 后续优化：CairoCompat 加 rounded-rect 模式识别 → SDF rect shader 快路径，RoundedRect 单测从 7.9ms 降到 0.45ms（17.4× 加速），从落后 3.5× 转为领先 4.9×。`arcsInPath` side-buffer 在 `appendArc` 记录每个弧，`Fill` 时检查 4 等半径四分之一弧 + 4 LineTo + 单子路径 + 实心 brush 模式 → 直接 dispatch `Renderer.FillRoundedRect`。Gradient/不等半径/非 canonical 全部 fall-through 慢路径，6 个测试锁住

CI 上挂 PR regression gate 推迟到下个 milestone（需要先有稳态基线 + tolerance 政策）。

约 600 LOC（实现 + 文档 + RoundedRect 快路径 + 测试）。

#### 3.2.6 Designer 输出 decl Go 源码 ✅（已完成）

之前 designer 只能输出"struct + 命令式构造器"（`gui.NewButton1` + `SetBounds` 一连串），手写 hot-reload 风格的 decl AST 形态没有 designer 入口。

**已完成**：
- ✅ `ged/codegen_decl.go`: `GedScene.GenerateDeclCode(opts) string` 生成 `func BuildXxx() *decl.Node { return decl.Form(...) }` 形式的完整 .silk.go
- ✅ 走 scene → `*decl.Node` → `decl.ToGo` 路径，复用 §3.1 已经做好的 emitter
- ✅ buildSceneDeclNode 把 form 标题/尺寸装进根 Node；buildFakeDeclNode 把每个 FakeWidget 转成 child Node（factory + ID + x/y/w/h + text/title）
- ✅ 已知 widget 走 `decl.Button(...)` 等 shortcut；未注册 widget 自动 fall through 到 `decl.New("type", ...)`
- ✅ Event handler 代码（`FakeWidget.GetCode`）以 footer 注释块形式保留 —— decl AST 没有原生槽位放 raw Go body，命令式 GenerateCode 路径仍是首选当 form 需要把 handler 放同一文件
- ✅ 6 个测试覆盖：空 scene 骨架、生成结果 `go/parser` 通过、子部件类型 + ID 正确、未知 widget fallback `decl.New`、handler 注释块、gofmt 稳态

约 280 LOC（实现 + 测试）。

至此 §3.2 中期路线全部 6 项完成（stencil clip / blend ops / SVG arc / glyph subpixel / bench suite / designer→decl）。

### 3.3 长期（8 周+）

**目标**：在 opengl 路径基础上做 Cairo 时代不可能的事。

#### 3.3.1 Hot reload（decl + fswatch）✅（已完成 — 文件→AST 链路；widget 替换交给 host）

之前 fswatch 与 decl 各自存在但没有集成路径，"修改 .silkui 触发 widget 重建" 需要每个 host 自己拼。

**已完成**：
- ✅ `hotreload/hotreload.go`: `Reloader` 类型把 fswatch.Watcher + core.LoadTDocFile + decl.FromTDoc 串成单 goroutine reader loop
- ✅ Reload 回调签名 `func(path string, tree *decl.Node) error` —— host 收到新 AST 后自行决定 `(*decl.Node).Build()` 整树重建还是局部 diff
- ✅ Error 回调单独通道，parse / load 失败不杀死 loop
- ✅ Debounce（默认 100ms）合并同 path 的连续 Modified 事件 —— 编辑器原子保存（重命名 + 写）触发的事件爆发只产生一次重建
- ✅ AllowedExt 过滤 —— 监听目录时只对指定扩展名（`.silkui` / `.silk.go` 等）触发回调
- ✅ Stop 幂等 + 取消所有 pending debounce 定时器
- ✅ 6 个测试覆盖：modify 触发、garbage 内容报 error 不崩、Stop 真停、debounce 3 写合并、扩展名过滤、nil callback 拒绝

实际 widget 替换属于 host policy（有的 app 想保留焦点 + scroll 状态，有的整树重建）—— hotreload 只负责把"新 AST 送到主线程信箱"。文档明确 callback 是 reader goroutine 调用，host 必须自行 marshal 到 GLFW main thread。

约 250 LOC（实现 + 测试）。比预估 800 LOC 少很多 —— 复用 fswatch 现成 polling + decl 现成 codec 后，hotreload 本身只需要 debounce + dispatch 胶水。

#### 3.3.2 GLES 3.0 / WebGL 后端

OpenGL 2.1 是桌面级。如果要跑：
- iOS / Android（GLES 3.0）
- Web（WebGL 2.0 via wasm）

需要 shader 改造（`#version 100 es` / `#version 300 es`）+ 移除 ARB 扩展依赖 + 对接 GLFW Web 适配（如 emscripten）。

工作量：~2000 LOC + 工具链。

#### 3.3.3 文本 shaping（HarfBuzz 或 Go 替代）

复杂脚本（阿拉伯 / 印地语 / 藏文）需要字符级 shaping。当前 glui 是 per-rune advance + atlas blit，没有 ligature / mark positioning。

选项：
- 集成 HarfBuzz（C 依赖回归，但 cgo-only 可选）
- 等纯 Go shaping 库（github.com/go-text/typesetting 已基本可用）
- 自己写简化版 OpenType GSUB/GPOS（工作量极大）

推荐：集成 `go-text/typesetting`，纯 Go，覆盖 90% 用例。

工作量：~600 LOC。

#### 3.3.4 自动 Accessibility 树 ✅（已完成 — 跨平台 Go 层；OS bridge 推迟）

之前 silk 没有任何 a11y 接口，屏幕阅读器 / 自动测试无法枚举 UI。

**已完成**：
- ✅ `a11y/a11y.go`: `Role` 枚举（40+ widget 角色）、`State` 位掩码（Focused / Checked / Disabled / Hidden / ReadOnly / Pressed / Expanded / Selected / Required / Invalid）、`Node` 结构（role + name + desc + value + state + bounds + children）、显式 `Accessible` 接口及 4 个可选 refinement
- ✅ `a11y/inspect.go`: 鸭子类型 `readName / readDescription / readValue / readState / readBounds` 通过现有 widget 方法名（Text/Title/IsChecked/Value/IsEnabled/HasFocus/Bounds）回退提取信息 —— 不修改 62 个 widget 文件
- ✅ `a11y/walk.go`: `Walk(root)` 默认跳过 hidden 子树；`WalkAll(root)` 完整枚举；递归 DFS 保持视觉顺序；nil-safe；通过 `childrenAdapter` 接口让任意层次结构（gui.IWidget / graph.IItem / 自定义）适配
- ✅ `a11y/guess.go`: 反射查 widget runtime 类型名后缀，把"Button"→`RoleButton`、"Slider"→`RoleSlider` 等映射 40+ 条 —— 既覆盖 silk 内置 widget，又支持 host 自定义 `MyButton` / `TaggedLabel` 等模式
- ✅ 11 个测试覆盖：Role.String 稳定性、State 位运算、显式 Accessible 优先于 duck-type、duck-type name/state/checked/value-as-float、类型名推断 role、嵌套子树递归、hidden 跳过、WalkAll 反例、nil-safe、bounds XYWH fallback

OS-native bridge（macOS NSAccessibility / Windows UIA / Linux AT-SPI）属于平台特定 cgo 工作（~1500 LOC + 三个 OS），推迟到后续 milestone 单独立项。当前 a11y 包已经足够给 Silk 自动化测试套件、designer outline 视图、debug overlay 等内部工具用。

约 480 LOC（实现 + 测试）。比预估 1500 LOC 少 —— OS bridge 没做。

#### 3.3.5 性能：GPU instancing + 复杂场景

当前每个 widget 单独 push quad。万级 widget 场景（IDE 大文件、大表格）会瓶颈在 CPU 提交。

**实现**：
- 同 kind 同样式批次内做 instanced draw
- 顶点写入避免 reflection
- VirtualList 与 instancing 互动

工作量：~600 LOC + benchmark 验证。

#### 3.3.6 Native event 后端（取代 fswatch polling）

fswatch 当前是 polling。系统级 inotify / FSEvents / ReadDirectoryChangesW 可降低延迟到毫秒级、避免 idle CPU。

可选 cgo（fsnotify）或纯 Go 系统调用直接封装。

**注**：fswatch 文档（doc.go）明确反对引入 fsnotify 第三方依赖；纯 Go 系统调用包装跨平台代价大且 macOS polling 没有实际痛点（500ms 延迟 + stat 调用近零 CPU）。本项延后；可作为单独 silk-fsevents 包做 opt-in 升级而不替换主路径。

工作量：~400 LOC。

#### 3.3.7 Export surfaces (PDF / SVG / PS) ✅（SVG 已完成）

opengl 分支去掉 Cairo 时一并丢失了 cairo_pdf_surface / cairo_svg_surface / cairo_ps_surface —— "用 paint.Painter 画一次，导出为矢量文件" 的能力没了。设计器画布、报表生成、图表导出等场景都需要这条出口。

**已完成（SVG）**：
- ✅ `svgexport/svgpaint.go`: `SVGPainter` 实现 `paint.Painter` 全部 30+ 方法 —— 路径构造（MoveTo/LineTo/Arc/CurveTo/Rectangle）、Fill/Stroke、变换栈（Save/Restore/Translate/Scale/Rotate）、画笔/画刷/字体/文本
- ✅ CTM 在 emit 时 fold 进坐标 —— 输出无 `transform=` 嵌套 `<g>`，文件更小、parsing 更直
- ✅ 颜色：opaque → `#RRGGBB`，alpha < 255 → `rgba(r,g,b,a)`
- ✅ 文本走 `<text>` 元素保留可选/可访问性，XML 转义 5 个保留字符防注入
- ✅ Pixmap/Icon/Glyphs 是 SVG 无原生对应的 raster-only 操作，记 no-op 留 SVG raster image 后续扩展
- ✅ Clip 当前 no-op（SVG 走 `<defs><clipPath>` 是 follow-up；多数 designer scene 不需要）
- ✅ 11 个测试覆盖：paint.Painter 接口编译断言、rect/path 路径属性、stroke 属性、CTM 折叠、Save/Restore 状态栈、Arc → SVG "A" 命令、文本 + XML 转义、`encoding/xml` parses output、rgba alpha、空 Fill no-op、`<script>` 转义防 XSS

**已完成（PDF）**：
- ✅ `pdfexport/pdfpaint.go`: `PDFPainter` 同 SVGPainter 一样实现 paint.Painter 全部 30+ 方法 —— 路径构造、Fill/Stroke、Save/Restore（同时 emit PDF 的 q/Q）、变换栈、画笔/画刷/字体
- ✅ `pdfexport/document.go`: PDF 1.4 文档结构组装 —— %PDF-1.4 header + binary marker / 5 个 object（Catalog / Pages / Page / Contents / Font）/ xref table（每行严格 20 字节）/ trailer / startxref / %%EOF
- ✅ Y 翻转每坐标 emit 时处理（PDF 原点 bottom-left vs paint 原点 top-left），输出无需嵌套 cm 全局变换
- ✅ Helvetica 走 PDF 标准 14 字体（无需嵌入 TTF），DrawText 用 Tm `1 0 0 -1 x y` 反转 glyph 朝向兼容 paint top-down 调用
- ✅ Arc 90° 切片 cubic Bezier 近似（PDF 无原生 arc operator）
- ✅ Rectangle 走 PDF 原生 `re` operator（比 4 LineTo 快）
- ✅ xref 偏移精确到字节（offset 错则全部 reader 拒收）；`startxref` 指向 `xref\n` 关键字位置
- ✅ 12 个测试覆盖：paint.Painter 接口编译断言、PDF doc 结构、`re`/`f`/`S`/`RG`/`rg`/`w`/`c`/`q`/`Q`/`BT/ET`/`Tf`/`Tm`/`Tj` 全部关键 operator、Y 翻转坐标正确、Arc → cubic 解算、Save/Restore q/Q 嵌套、文本转义保护 PDF 字面字符串语法、xref 偏移每条指向真"N 0 obj"、startxref 字节位置准确、非透明 alpha 当前 fallback 文档化
- ✅ macOS `qlmanage` Quick Look 渲染验证 PDF 真合法（thumbnail 成功生成）

**已完成（图像嵌入）**：
- ✅ `svgexport` `DrawPixmap` / `DrawPixmap1` / `DrawPixmap5`：编码 PNG + base64 → 输出 `<image href="data:image/png;base64,..."/>`，CTM 同步 fold 进坐标
- ✅ `pdfexport` `DrawPixmap*`：编码 zlib-compressed RGB → 添加 PDF `/XObject /Subtype /Image` 对象、更新 `/Resources /XObject` dict、内容流 emit `q w 0 0 h x y cm /ImN Do Q`，alpha 通道在编码时合成到白底（PDF SMask 单独 object 单独走 follow-up）
- ✅ `document.go` 重构成支持 N 个图像对象 —— xref 表大小、`/Size` trailer 字段、对象 ID 编排全部跟着图像数动态调整，每条 xref 仍严格 20 字节
- ✅ 12 个测试覆盖（5 svg + 7 pdf）：image element 输出、显式 (w, h) 参数、CTM 折叠、`encoding/xml` parses output、nil 安全；PDF 这边额外锁 XObject 字典出现、`/Im1 Do` operator 出现、多 pixmap 顺序命名、xref 偏移在新增对象后仍指向真"N 0 obj"、trailer `/Size` 字段对得上、cm 矩阵 Y 翻转正确
- ✅ macOS `qlmanage` Quick Look 渲染含图像的 PDF 缩略图成功 —— 真实 PDF parser 接受

**已完成（SVG clip path）**：
- ✅ `svgexport.Clip()` / `ClipPreserve()`: 之前 no-op，现在 emit `<defs><clipPath id="cN"><path d="..."/></clipPath></defs>` + 打开 `<g clip-path="url(#cN)">` 包裹后续内容
- ✅ `Save/Restore` 跟踪 `openGroups` 计数 —— Restore 自动关闭对应数量的 `</g>`，clip 区域跟着 graphics-state 走（与 Cairo / PDF 行为对齐）
- ✅ Nested clip 自动分配 c0/c1/c2 唯一 ID
- ✅ `WriteTo` 在 `</svg>` 前自动关闭剩余 open `<g>`，输出永远是 well-formed XML（即便 caller 没有 Restore）
- ✅ 9 个测试覆盖：clipPath + g 元素出现、fill 在 g 内部、Save/Restore 正确闭合、嵌套 clip ID 唯一、ClipPreserve 不消路径、空 clip no-op、含 clip SVG `encoding/xml` 通过、未配对 clip 自动闭合、ResetClip 仍 no-op
- ✅ macOS `qlmanage` 渲染含 clip 的 SVG 缩略图成功 —— 系统 SVG parser 接受输出

**已完成（PDF clip + 内容流压缩）**：
- ✅ `Clip()` / `ClipPreserve()`: 之前 no-op，现在 emit `W\nn\n`（winding clip + no-paint）。`ResetClip` 仍 no-op（PDF 没办法在不退出 graphics state 的情况下"拆 clip"，文档明确建议用 Save/Restore 包 clip 区间）
- ✅ `SetCompression(bool)` / `CompressionEnabled()`: 默认关，opt-in 后 per-page content stream 走 zlib FlateDecode + Contents 字典加 `/Filter /FlateDecode`
- ✅ 真实场景 ratio：100 rect + text 页面 18KB → 2.8KB（**84.6% 缩减**），远超 ROADMAP 60-80% 估值
- ✅ 7 个测试覆盖：W/n emission、ClipPreserve 不 reset 路径、空 clip no-op、SetCompression 切换 /Filter、压缩输出更小、压缩流 zlib decode 后字节级与未压缩相同（round-trip 正确）、默认关
- ✅ macOS `qlmanage` 渲染压缩 PDF 缩略图成功 —— 系统 parser 接受 FlateDecode

**已完成（多页 PDF）**：
- ✅ `PDFPainter.NewPage()` / `NewPage1(w, h)`：finalise 当前页 content stream + 起新页；CTM/state stack/brush/pen/font/curX/Y 全部 reset，每页独立
- ✅ `PDFPainter.PageCount()`：实时报告"finished + 当前打开页"总数
- ✅ document.go 重构成 N 页对象布局：Catalog(1) → Pages(2) → 交替 Page/Contents(3,4,5,6,…) → Font(2N+3) → Image XObjects。单页文档对象 ID 维持 1-5 与之前完全一致，旧测试零修改通过
- ✅ Pages tree `/Kids` + `/Count` 跟着实际页数动态生成；每页有独立 MediaBox 支持竖横混合（A4 portrait + landscape 同 PDF）
- ✅ Image XObject 池仍是 document-scoped 跨页共享（无 dedup，每次 DrawPixmap 单独 XObject —— dedup 是 follow-up）
- ✅ 8 个测试覆盖：Kids/Count 正确、PageCount 报告、自定义 page size、每页独立 Contents 对象、CTM 跨页 reset、image XObject 跨页布局、xref 偏移精确到字节、单页向后兼容
- ✅ `qlmanage` 渲染 3 页 PDF（red portrait / blue landscape / green portrait）成功；`file` 输出 "3 pages" 确认

**已完成（recording surface 等价物）**：
- ✅ `recpaint/recpaint.go`: `RecordingPainter` 实现 paint.Painter 全部 30+ 方法 —— 每次调用把闭包追加到 ops slice，捕获参数 by value
- ✅ `Replay(target paint.Painter)`：迭代 ops 调用 target，幂等（可对多个 target 重放，可对同一 target 重放多次）
- ✅ Mirror 状态：`CurrentPoint` / `CurrentState` / `GetMatrix` 在录制期返回实时正确值（不是 stub），让 widget Draw 代码读 CTM 时行为与真 Painter 一致
- ✅ `Reset()` 清 ops 但保留 mirror 状态 —— 跨帧复用同一个 recorder 时减少 New 分配
- ✅ 用例：scene cache（复杂子场景录一次，每帧 cheap replay）、multi-target export（一次录制 → 屏幕 + PDF + SVG 三个目标）、debug 捕获（测试中录一段，replay 到 instrumented painter 检查每个调用）
- ✅ 10 个测试覆盖：paint.Painter 接口编译断言、replay 路由每个 op 一次、order 保持、多 target 幂等、CurrentPoint mirror、CurrentState mirror、GetMatrix mirror（与独立 Mat3x2 操作结果对齐）、Reset 不破坏 mirror、nil target 安全、replay 到 svgexport.SVGPainter 和直接调用 byte-identical

**未做（PS）**：PostScript 文档结构与 PDF 大同小异（DSC comments + showpage），但实战使用率远低于 PDF/SVG，留给真正需求出现再做。

约 2000 LOC（SVG 480 + PDF 480 + 测试 + 文档结构 + 图像嵌入两侧 + recording surface 360）。

---

## 4. 路线总结

```
当前位置（commit 1dec0bd）
├── opengl 渲染管线 ✅（11 大子系统）
├── Cairo 移除 ✅（silk_no_cairo build tag 全栈通）
├── Qt5 等价物 9 个 ✅
└── 测试 600+ × 2 模式 × -race 全过

短期 1-2 周
├── CI silk_no_cairo 矩阵
├── release.yml 双轨二进制
├── README + 教程更新
├── Linux / Windows 真机验证
└── decl → Go emitter

中期 2-8 周
├── Stencil-based path clipping
├── 14 种 SetOperator 复合模式
├── SVG 椭圆弧完整解算
├── glui 子像素文字
├── 性能基准套件
└── Designer 输出 decl Go

长期 8 周+
├── decl + fswatch hot-reload
├── GLES 3.0 / WebGL 后端
├── go-text 文本 shaping
├── Accessibility 树
├── GPU instancing 性能
└── Native event 后端取代 polling
```

每完成一项更新本文档对应章节的 ✅ / ⏳ 状态，并在 commit message 引用 ROADMAP.md 章节号。

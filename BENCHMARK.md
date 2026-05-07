# Silk Benchmark — opengl vs Cairo (CPU recording)

## 跑法

```bash
# 两种模式都要跑：默认带 Cairo
go test -run "^$" -bench "." -benchtime=200x ./bench/

# silk_no_cairo 模式（只有 glui benches，cairo_bench_test.go 被构建标签排除）
go test -run "^$" -bench "." -benchtime=200x -tags silk_no_cairo ./bench/
```

`-benchtime=200x` 强制每个 bench 跑 200 次取均值。低于 100 次第一帧字体加载会污染数据；高于 1000 次基本到达稳态，但跑时变长。

## 对比方法

两边都通过 `paint.Painter` 接口驱动同一组场景代码（`bench/scenarios.go`）：

- **Cairo** 走 `paint.NewPixmap(1920,1080).NewPainter()` → `cairoPainter`。每个 `Fill()` / `Stroke()` / `DrawText()` 立刻写像素到 image surface，CPU 完成全部渲染工作。
- **glui** 走 `glui.NewBenchRenderer(1920,1080)` → `glui.NewCairoCompat(r)` → `CairoCompat`。每次操作把几何 batch 进顶点缓冲，**不调用 GPU**。`flush()` 在 `ctx == nil` 时短路，模拟"CPU 录制成本"——这是 UI 主线程实际被阻塞的部分。

GPU 提交 / 绘制时间不计入 — 那部分对比需要真 GL context 与帧时序基准（属 ROADMAP §3.3.5 GPU instancing 后续工作）。

## 数据（Apple M2 Max, Darwin 25.4, Go 1.21+）

| 场景 | Cairo ns/op | glui ns/op | 倍速比 (Cairo/glui) | Cairo 分配 | glui 分配 |
|------|-------------|------------|----------------------|------------|-----------|
| RectFill 1000 | 435,914 | **72,644** | **6.0×** | 8 B / 2 allocs | 16,000 B / 1,002 allocs |
| RoundedRect 500 | 2,270,520 | 7,915,380 | 0.29× ⚠️ | 4 B / 1 alloc | 983,540 B / 1,501 allocs |
| LinearGradient 200 | (skip) | **15,365** | 仅 glui | — | 14,400 B / 600 allocs |
| TextPaint 200 | 276,424 | **90,859** | **3.0×** | 37,632 B / 601 allocs | 15,130 B / 1 alloc |
| ScrollingList 1000 | 2,332,098 | **767,974** | **3.0×** | 236,046 B / 3,004 allocs | 32,016 B / 2,004 allocs |
| TypicalForm | 45,611 | **7,485** | **6.1×** | 1,556 B / 29 allocs | 144 B / 13 allocs |

⚠️ RoundedRect glui 3.5× 更慢 —— `CairoCompat` 的 path 累积 + 三角化路径开销大于 Cairo 原生 round-rect fill。glui 的快路径 `Renderer.FillRoundedRect`（SDF rect shader）走不到 `paint.Painter` 接口，因为接口约定的是 `MoveTo + LineTo + Arc + Fill` 流程。修复方向：在 CairoCompat 里识别"四个 Arc + 直角 LineTo 构成 rounded rect"模式并直接 dispatch 到 SDF 快路径。已写入 §3.2 后续待办（"path 模式识别 → SDF 快路径"）。

## 结论

- 5 / 6 跨 backend 场景下 glui CPU 录制成本比 Cairo 全栈渲染快 **3–6×**。
- 大多数实战 UI 场景（list scroll、典型 form、纯 rect 填充）都在这个倍数区间。
- glui 内存分配比 Cairo 多（slice 增长与 batch 顶点），但 CPU 时间收益足以盖过分配开销。
- LinearGradient 是 glui 独占（Cairo `cairoPainter.SetBrush` 不接受 `*LinearGradient`，要走原生 cairo API）；侧面验证了 glui 的渲染特性面更广。
- RoundedRect 是当前唯一 glui 落后的场景，根因是接口形态导致绕开 SDF 快路径，已记录修复方向。

## 跑法附注

- benchmark 默认 warmup 一次：text/scrolling/form 等场景会一次性加载 macOS 系统 CJK fallback 字体（25MB+），不 warmup 会污染头几个 iter。
- Cairo 路径需要 `pkg-config --exists cairo`（macOS：`brew install cairo`，Linux：`apt install libcairo2-dev`）。
- `silk_no_cairo` 模式只跑 glui benches；`cairo_bench_test.go` 顶部 `//go:build !silk_no_cairo` 自动排除。
- Apple Silicon 与 Intel x86 数字差异主要在 Cairo 侧（cairo C 库 NEON / SSE2 路径不同），glui 录制成本两架构基本一致。

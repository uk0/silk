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
| RectFill 1000 | 492,085 | **75,158** | **6.5×** | 8 B / 2 allocs | 16,000 B / 1,002 allocs |
| RoundedRect 500 | 2,215,696 | **453,187** | **4.9×** | 4 B / 1 alloc | 15 B / 1 alloc |
| LinearGradient 200 | (skip) | **15,049** | 仅 glui | — | 14,400 B / 600 allocs |
| TextPaint 200 | 281,355 | **92,552** | **3.0×** | 37,604 B / 601 allocs | 15,130 B / 1 alloc |
| ScrollingList 1000 | 2,277,061 | **756,648** | **3.0×** | 236,047 B / 3,004 allocs | 32,016 B / 2,004 allocs |
| TypicalForm | 44,536 | **5,880** | **7.6×** | 1,556 B / 29 allocs | 144 B / 13 allocs |

### RoundedRect 路径优化（2026-05-07 更新）

之前 RoundedRect 是 glui 落后场景（**3.5× 慢于 Cairo**），根因是 paint.Painter 接口下的 `MoveTo + LineTo + Arc × 4` 调用栈被 CairoCompat 累积成 ~64 段直线后送进通用 path 三角化，绕开了 `Renderer.FillRoundedRect` 的 SDF rect shader 快路径。

修复：CairoCompat 在 `appendArc` 里挂一个 side-buffer `arcsInPath`，在 `Fill()` 时检查"4 个等半径四分之一弧 + 4 条 LineTo + 一个 sub-path + 实心 brush"模式 → 直接 dispatch 到 `r.FillRoundedRect`（SDF rect 单 quad，无需三角化）。

数字变化（同一硬件、同一 -benchtime=200x）：

```
Before:   7,915,380 ns/op   1,501 allocs   ← glui 3.5× 慢于 Cairo
After:      453,187 ns/op       1 alloc    ← glui 4.9× 快于 Cairo
单测 17.4× 加速；从反例转为强项。
```

Gradient brush / 不等半径 / 任意 4 弧分布 / 非 canonical 路径全部走 fall-through 慢路径，被 6 个测试锁定。

## 结论

- **6 / 6 跨 backend 场景** glui CPU 录制成本比 Cairo 全栈渲染快 **3–7.6×**。
- 之前 RoundedRect 反例已修复，目前没有 glui 落后场景。
- glui 内存分配比 Cairo 多（slice 增长与 batch 顶点），但 CPU 时间收益足以盖过分配开销 — RoundedRect 优化后 glui 单 quad 1 alloc 反而比 Cairo 还少。
- LinearGradient 是 glui 独占（Cairo `cairoPainter.SetBrush` 不接受 `*LinearGradient`，要走原生 cairo API）；侧面验证 glui 的 paint.Painter 渲染特性面更广。

## 跑法附注

- benchmark 默认 warmup 一次：text/scrolling/form 等场景会一次性加载 macOS 系统 CJK fallback 字体（25MB+），不 warmup 会污染头几个 iter。
- Cairo 路径需要 `pkg-config --exists cairo`（macOS：`brew install cairo`，Linux：`apt install libcairo2-dev`）。
- `silk_no_cairo` 模式只跑 glui benches；`cairo_bench_test.go` 顶部 `//go:build !silk_no_cairo` 自动排除。
- Apple Silicon 与 Intel x86 数字差异主要在 Cairo 侧（cairo C 库 NEON / SSE2 路径不同），glui 录制成本两架构基本一致。

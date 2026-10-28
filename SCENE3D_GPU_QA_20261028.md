# OpenGL 场景后端与原生生命周期验证 — 2026-10-28

基线 `14ab7b4`。这次增加真实 OpenGL 光栅化与平台资源管理，不把 CPU 后端改名为 GPU；
也不是整个工业 3D / glTF / 插件目标完成或正式版本发布。

## 变更

- SDK 增加只读 `FrameView`、网格顶点/索引复制与快照实例读取，CPU/GPU 共享相机契约。
- `scene3d/glrender`：GLSL 平面光照、VBO/索引缓冲、双颜色附件（颜色/ID）及深度附件。
  不可变网格缓存、模型替换回收、像素/驻留预算；未变化帧不重复 draw 或 readback。
- `SceneView` 支持 `auto/cpu/opengl`、实际后端和回退诊断。模式切换正确刷新 Cairo 图像；
  设计器保存/回读、预览与生成代码保留后端属性。
- Unix 用一个隐藏 GLFW 资源 context，Windows 用独立原生 WGL / `CS_OWNDC` 资源窗口。
  资源窗口永不显示，不是悬空的可见子控件；每个视口单独拥有 Renderer，设备共享。
- 每次绘制恢复宿主 current context（Windows 包括 DC），关闭视口和退出消息循环释放
  GPU 对象与资源窗口。Windows 不初始化第二套 GLFW，不改宿主 DPI 策略或事件泵。
- GPU 结果仍同步读回并进入普通 Cairo 控件树，保持后画控件/菜单遮挡与裁剪关系。

## macOS 自动与原生验证

环境：Apple M2 Max，Go 1.26.10，驱动报告 `Apple / Apple M2 Max / 2.1 Metal - 91.7`。

| 检查 | 已验证结果 |
| --- | --- |
| 10 个有测试包 `-race -short -count=1`，另含示例包 | **6313 pass / 51 skip / 0 fail**（含子测试；示例无测试另记 package skip） |
| `scene3d` 完整测试与 `TestScene*` GUI/设计器 | 通过；含后端模式错误、快照/网格隔离及生成代码编译 |
| `silk_pure_go` 五包 | 通过；glrender 无 cgo 时明确返回不可用，不声称 GPU 成功 |
| `go vet -composites=false` 六包 | 通过 |
| 实际驱动探针，普通与 `-race` | 三种尺寸稳定内部/背景像素与 CPU 对照：17562 / 27950 / 73498 像素，颜色容差 2，ID 完全一致，均零不匹配 |
| 实际 GUI native composition 探针，`-race` | 正常 painter 后续矩形保持在上层；宿主 context 恢复、两个视口共享设备、关闭一个不影响另一个、后端切换/退出/重复 Close 通过 |
| Windows amd64 CGO=0 桩及 WGL API overlay 类型编译 | 通过；只是类型检查，不代替 Windows 原生运行 |

驱动探针还验证深度遮挡、隐藏不可拾取、缓存命中、相机变化不重传网格、20 次替换不
累积旧网格、超尺寸拒绝、重复关闭。默认测试明确 skip opt-in native probe，单独开启
环境变量才算驱动测试；没有把 51 个 skip 算成通过。

## macOS 真实 UI 闭环

独立测试应用 `org.silk.qa.scene3d.gpu`，测试二进制 SHA-256：
`af14ca15dccbe1ca0b3daf2e2dbf38e19ec0239d1bb33c366e043c7853d59c4d`。

- Computer Use 点击 Snapshot，回读实际 `backend=opengl` 与 Apple 驱动，未出现渲染错误。
- Publish 1000 values：后台 0…999 样本在 UI 线程落到 Y=0.999。
- 清空选择后点击模型，选择 `"" → "model"`，两次 `renders=398`，证明拾取复用光栅缓存。
- Load OBJ 回读 `loaded=2`、`node=model`、`busy=false`、`on_ui=true`、`error=""`、Y=0.999。
- 中途用户手动操作了相机/模型；这些操作与渲染计数不记作自动回归或空闲性能基线。
  当前窗口留供用户试用，关闭资源用独立 native probe 验证，没有擅自关闭用户正在操作的窗口。

## Windows 原生验证

最终 WGL 源码使用独立目录 `C:\silk_scene_wgl_20261028`，不覆盖早期 GLFW 实验目录。
上传后回读关键源文件 SHA-256；采用用户指定 win10testcomputer 的交互式计划任务，
实际 session 1 / 1600×1200，不是在 SSH 会话 0 假装点击。

- 最终 WGL 示例、GUI 测试程序与驱动测试程序在 Windows 原生 cgo 构建通过。
- 机器报告 Todesk Virtual Display Adapter / Microsoft Basic Display Adapter。
  严格驱动测试退出 **1**：`WGL: The driver does not appear to support OpenGL`；
  WGL 生产后端诊断 `OpenGL backend unavailable: glActiveTexture`。
  **这不是 Windows GPU 通过**，没有更换系统显示驱动或伪装硬件能力。
- `TestSceneNativeWindowsWGLCompositionAndFallback` 通过：CPU 回退有诊断、失败不留
  资源设备、不逐帧重试、拾取缓存、后续 painter 遮挡、宿主 HDC/HGLRC 恢复、强制 GPU
  明确失败、切换 CPU/auto 恢复、关闭/重复关闭。GPU 成功分支在该机器上未执行。
- 原生按钮/鼠标：Snapshot 回读实际 CPU 与回退原因；tank/pump 选择保持 `renders=2`；
  Publish 1000 后 Y=0.999；16 步拖拽改变 yaw/pitch；单滚轮距离 6→5.429024508；
  Reset camera 恢复 yaw=.65 / pitch=.35 / distance=6。
- Load OBJ 回读 `loaded=1`、`node=model`、`on_ui=true`、`busy=false`、`error=""`、Y=.999。
  点击导入模型得到 ID `model`，`renders=22` 不增加。点位平移后模型可能离开视野；
  裁剪有效、不会覆盖工具栏，但自动跟踪/重新适配相机尚未实现。
- 静置 **15.013 s**，`renders=22` 不变，进程 CPU 时间增加 **0.046875 s**，约单核
  **0.312%**，Responding=true。只是这个 CPU 回退示例的短采样；不代表整个 IDE，
  当时同机还有编译任务，也不能当 GPU 性能或长期资源压力结论。
- 新启动基线点击真实关闭按钮，保留 native process handle 后 `GetExitCodeProcess=0`，
  stderr 为空，进程退出。初版脚本对已退出的外部 Process 读取到空 ExitCode，断言失败；
  保留该失败日志，改用保留句柄并从新进程重测，未将脚本失误记作程序通过。
- 初次截图误取启动时的控制台句柄；之后隐藏控制台，检查 PID/程序路径/窗口标题、
  `GetWindowRect` 与点击归属，重新捕获实际 GUI，未据错窗口截图作布局结论。

最终原生示例 SHA-256：
`f8ba171bfbf5bbcfeab44ffa110123b97767fcee6bdd47d2c57a959e252d2a7b`。
[真实 Windows 窗口截图](screenshots/scene3d-windows-fallback-20261028.png) 展示的是 CPU 回退。
Windows 原生 cgo 十个有测试包 `-short -count=1`：**6101 pass / 77 skip / 0 fail**，
示例包无测试另记 package skip；同一最终 WGL 源码。源码/布局/UI 不因回退而跳过。
随后对最终 WGL 工作树重跑 `-race -short -count=1 ./scene3d ./gui ./ged ./decl`，
四包均通过（GUI 140.082 s，设计器 53.884 s）；不是把早期 GLFW 实验树的结果拿来充数。
Windows native WGL opt-in 桌面探针使用非 race 测试程序；macOS native 探针另已跑 race。

第一轮 PowerShell 5.1 的原生 stdout 解码损坏了少数 Unicode 测试名的 JSON 日志；
退出码为 0，但日志不可用作完整结构化证据。随后用 `cmd /c` 原生字节重定向完整重跑，
UTF-8 JSONL 全部可解析，以上计数来自后一轮，不删除旧记录。
Windows GPU 与 Linux native 仍是明确的验收缺口。

## 性能与未完成事项

实际 Apple 驱动短对照：1000 个共享圆柱、256000 三角形、640×480、6 个变化帧，
含初次分配/上传与颜色+ID 回读，CPU **148.641 ms**、GPU **18.946 ms**。
这是一次短探针，不是稳定 FPS、长测、整个 IDE 性能或 Windows 性能承诺；`-race` 计时另存，
不与非 race 结果混用。重复实例目前仍每节点一个 draw，不是单 draw 硬件实例化。

尚未验收/实现：Linux native UI、三平台全 IDE 场景编辑、长期多视口资源占用、大坐标精度、
抗锯齿/平滑法线/透明材质、glTF/PBR、硬件实例化/视锥/BVH/LOD、零读回有序合成。
简单场景可能受同步读回影响比 CPU 慢；驱动报告 OpenGL 也可能是软件实现。

Windows 首次编译现有 go-gl 2.1 大型 cgo 绑定耗时明显，属于构建成本，不是 GUI 空闲
CPU 证据。后续可评估精简的按 context 入口绑定，减少冷构建成本及全局入口表限制。

## 重放与证据

```sh
go test -json -race -short -count=1 -p 1 ./scene3d ./scene3d/glrender ./gui ./ged ./decl ./graph ./paint ./async ./prop ./cmd/silkide ./examples/scene3d
SILK_TEST_SCENE_GPU=1 go test -count=1 -v ./scene3d/glrender
SILK_TEST_SCENE_GPU=1 go test -race -count=1 -v -run '^TestScene' ./gui
go test -tags silk_pure_go -short -count=1 -p 1 ./scene3d ./scene3d/glrender ./gui ./ged ./decl
go vet -composites=false ./scene3d ./scene3d/glrender ./gui ./ged ./decl ./examples/scene3d
# Windows interactive desktop (PowerShell):
# $env:SILK_TEST_SCENE_WGL='1'; .\gui-test.exe -test.v -test.run '^TestSceneNativeWindowsWGL'
# Raw Windows JSON logs: cmd /c "go test -json ... > result.jsonl 2> result.stderr"
```

本地证据位于 `bin/qa-artifacts/scene3d-gpu-20261028/`：测试 JSONL/退出码、驱动日志、
源文件与二进制哈希、原生 stdout/断言、Windows 原生任务/截图。该目录是忽略产物，
不上传测试二进制。早期 CPU 验证保留在 [历史报告](SCENE3D_QA_20261028.md)。

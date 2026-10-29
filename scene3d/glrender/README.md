# OpenGL 场景后端

`glrender` 是真实 OpenGL 光栅化路径（驱动也可能是软件实现），不是给 CPU 参考渲染改名。顶点/索引上传到缓冲对象，
顶点着色器处理世界/相机变换，片元着色器输出平面光照与独立 ID，深度附件完成遮挡。

## 使用与资源归属

- 要求 cgo、OpenGL 2.1 compatibility、`GL_EXT_framebuffer_object` 和至少两个 draw buffers。
  `!cgo` / `silk_pure_go` 编译为明确返回 `ErrUnavailable` 的桩，不偷偷宣称 GPU 可用。
- 宿主创建**独占**上下文，在绑定该上下文的同一 OS 线程调用 `NewCurrent`、`Render`、
  `CloseCurrent`。每个 Renderer 现在拥有独立的 **59 项必需 + 3 项可选 GL 函数表**，不再初始化/
  改写 go-gl 的进程全局入口。不同 context 的 Renderer 可以在各自锁定的 OS 线程并行；
  同一个 Renderer 仍不可跨线程使用，宿主仍需 `runtime.LockOSThread` 并保持正确绑定。
  后端会改变其独占 context 的 GL 状态，不负责恢复该 context 内任意外部状态。
- `Render` 返回 `Frame`（实现 `scene3d.FrameView`），RGBA 是顶端起始，拾取坐标也是顶端
  起始。缓存不重复 GL draw/readback；新 Frame 的 `Sequence` 才增长。
- 同一个 Mesh 在该 Renderer 内只上传一次，实例复用 VBO/索引缓冲；支持时按原顺序合并连续的共享 mesh 实例，
  每批最多 1024 个，一次 instanced draw。单个实例/缺失能力仍走原 uniform 普通 draw。
- 新场景绘制前回收不再引用的 mesh。网格缓冲预算 64 MiB，帧还受 `scene3d.MaxPixels`、
  驱动 texture/renderbuffer 上限约束。多个 Renderer 的预算不共享。
- 返回图像/ID 缓冲只读，下一次 Render 可能覆盖；失败后不使用旧 Frame。Close 必须在
  native context 销毁前调用，不依赖 Go finalizer 进入 GL。
- `Render` 和 `CloseCurrent() error` 在错误 OS 线程/context 上返回 `ErrContext`，
  不返回缓存帧、不删除其它 context 的同名对象。错误线程先检查不可变归属，不触碰
  可能已由 owner 释放的 C 表。`Close()` 保留兼容便利接口并忽略错误，必须在 owner
  context 上调用；需要确认释放结果时用 `CloseCurrent`。关闭后仍需宿主销毁 native context。
- 入口由 WGL / CGL / GLX 加载，Windows 仅使用系统 `opengl32.dll`；目前不提供 EGL loader。
  Windows 的 SceneView 生产依赖树已不再包含 go-gl/glfw；Unix 既有 2D 呈现仍可能依赖它们。
  [内部绑定与生成说明](internal/gl21/README.md) 说明 ABI、资源归属与可重放验证。
- `Info` 是驱动原样报告，不据此保证硬件：远程桌面、VM、软件 GL 或缺少能力会不同。

## GUI 集成

`SceneView.SetRenderBackend("auto" | "cpu" | "opengl")`：

- **auto**：实际 native 视口在 UI 线程第一次绘制时尝试 GPU；失败后固定回退 CPU，
  `GPUError()` 可查询原因，不在每帧反复创建上下文。普通离屏工具不隐式建 native 窗口。
- **cpu**：强制参考渲染。**opengl**：强制 GPU；不可用时返回/显示错误，不静默回退。
- `ActiveRenderBackend()` / `GPUInfo()` 给出实际后端。切换另一模式再切回可显式重试。
- 多视口共享一个不可见的资源上下文，但各自拥有 Renderer/缓存。Unix 复用已初始化
  GLFW；Windows 用 `CS_OWNDC` 隐藏资源窗口和原生 WGL，不再初始化另一套 GLFW，
  避免影响既有 Win32 DPI 策略、系统 foreground-lock 设置和消息循环。
- GPU 颜色结果经 readback 后由正常 Cairo 控件树合成。因此普通后续控件、裁剪与菜单
  顺序不被最后一层 GL overlay 覆盖。资源窗口永不显示，不承担可见 UI。
- 显式 Close、更换父级、切换渲染后端释放客户端；事件循环在终止图形系统前关闭设备，
  宿主之后再次 Close 安全。底层 SDK 新增 WGL 当前 context/DC 查询以恢复宿主绑定。

## 验证与性能边界

```sh
SILK_TEST_SCENE_GPU=1 go test -count=1 -v ./scene3d/glrender
SILK_TEST_SCENE_GPU=1 go test -race -count=1 -v -run '^TestScene' ./gui  # macOS/Linux native probe
# Windows: run from an interactive desktop, not an SSH session-0 GUI test.
# $env:SILK_TEST_SCENE_WGL='1'; go test -count=1 -v -run '^TestSceneNativeWindowsWGL' ./gui
```

默认测试会明确 skip 驱动测试；指定环境变量后没有 context/能力就失败，不能用 skip
当 GPU 通过。驱动测试比对 CPU 的稳定内部像素/ID、深度、三种尺寸、网格复用/替换/关闭。
Windows WGL 探针区分 GPU 成功与驱动不支持时的 CPU 回退，验证宿主 context/DC 恢复、
拾取缓存、普通绘制叠加与销毁。回退通过不等于 GPU 通过。
底层 glrender 驱动探针还验证错误 context/线程拒绝、并发错误 Close、两个独立 context 同时渲染
128 帧，以及 CPU/GPU 使用相同相机序列。还有含读回的 25.6 万三角形对照，不是纯 GPU timer、桌面 FPS 或普遍性能承诺。

限制：顶点/矩阵使用 float32；暂不支持大坐标相机相对精度方案、纹理/PBR、透明材质、
平滑法线、MSAA、跨不相邻 mesh 的重排批处理、SAH/遮挡裁剪/LOD、零读回合成。每变化帧同步读取颜色+ID
两个附件，在简单场景中可能比 CPU 慢。后续需要避免读回并维持正确的控件合成顺序，
不能为了性能改回悬空 overlay。参见 [总路线](../../SCENE3D_ROADMAP.md)。

规范依据：[Khronos EXT framebuffer objects](https://registry.khronos.org/OpenGL/extensions/EXT/EXT_framebuffer_object.txt)、
[GLSL 1.20](https://registry.khronos.org/OpenGL/specs/gl/GLSLangSpec.1.20.pdf)、
[GLFW context guide](https://www.glfw.org/docs/latest/context_guide.html)、
[Microsoft WGL current context](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-wglgetcurrentcontext)。

### 屏外实例与缓存

CPU/GPU 共用 `scene3d.Frustum` 做保守 AABB / 六平面检查。完全屏外的实例不上传首次使用
的网格，也不提交 draw；进入视野后再上传。仍被场景引用的已有缓冲保持驻留，避免相机
来回移动时反复上传；从场景移除后照常回收。输入矩阵与保守内存预算在裁剪前验证，不能
靠把巨大/非法模型放到屏外绕过错误或预算。拾取 ID 不重新编号，帧缓存继续跳过全部工作。

`Stats.CulledInstances` / `CulledTriangles` 为原始被裁剪工作量；`DrawCalls` / `Triangles`
仅计实际提交，缓存帧保留上次统计。这不减少每帧 RGBA / ID readback，也不等于 GPU
硬件实例化、遮挡剔除或零拷贝合成。真实驱动像素/拾取、冷上传、移入、关闭与性能证据
见 [视锥裁剪 QA](../../SCENE_FRUSTUM_QA_20261028.md)。

## 有序 GPU 硬件实例化（开发候选）

`NewCurrent()` 默认 `InstancingAuto`。同一可见运行段共享 mesh 时，将各自世界矩阵、颜色、
原始拾取 ID 放入实例属性缓冲，以一次硬件实例化命令提交；不是复制几何后冒充批处理。
不按 mesh 全局排序，不跨仍可见的不同 mesh 合并，保持等深度先绘制者和节点顺序。视锥
裁剪后的空洞不会重新编号。单个实例仍走原 ordinary shader；帧缓存跳过全部提交/上传。

```go
r, err := glrender.NewCurrentWithOptions(glrender.Options{
    Instancing: glrender.InstancingRequired, // Auto / Disabled / Required
})
// 仍需独占 current context、锁定 OS 线程，并在 context 销毁前 CloseCurrent。
// Required 在缺少能力时明确失败；Disabled 适合对照/驱动性能调优。
_ = err
if r != nil {
    caps := r.Capabilities() // InstancedDrawing
    _ = caps
}
```

- 支持判定同时检查版本/扩展、7 个 vertex attributes、两个非空入口以及 flat varying
  能力；GLX 返回一个地址不等于支持。可选入口缺失不会破坏旧的 GL 2.1 普通 GPU 路径。
- GL 2.1 使用 `GL_EXT_gpu_shader4` 的 `flat varying`，GL 3.x 可用 GLSL 1.30；颜色
  与拾取 ID 不做平滑插值。Linux Mesa 测试确实发现过颜色 1 LSB 差异，修复后仍按完整
  RGBA / ID 字节对照，未放宽断言。`worldPosition` 保持平滑以保留原法线计算。
- 实例 staging 与 GPU buffer 单批最大各 **96 KiB**；每实例 96 B。缓冲复用，不保存
  Go 指针到 C，Close 释放；网格预算和绘制前矩阵验证仍生效，不因实例化绕过。
- `Stats.Instances` 为实际提交实例数；`InstancedDrawCalls` / `InstancedInstances` 为
  真实批量 draw 工作量；`InstanceUploadBytes` 为本帧传输，`InstanceBufferBytes` 为实例
  buffer 当前大小，与原 `ResidentBytes` 网格预算分开。缓存帧保留上次统计。
- `SceneView.GPUCapabilities()` 只查询已建立的后端，CPU 回退、未绘制或关闭后返回空
  能力，不隐式创建 context；真实 GUI 测试覆盖 1000 个设备经正常控件树合成和拾取。

`SILK_TEST_SCENE_GPU=1 SILK_TEST_REQUIRE_INSTANCING=1` 启用严格驱动测试，不允许缺失能力
被 skip 掩盖。物理 Apple GPU 与 Linux llvmpipe 的验证含义不同：API 的实例化命令在软件
驱动也可以运行。软件 GL 下该基准不保证更快；可用 Disabled 对照。readback 仍存在，不能
把减少 draw 数当作整个应用的统一提速倍数。见 [实例化 QA](../../SCENE_INSTANCING_QA_20261028.md)。

参考：[ARB instanced arrays](https://registry.khronos.org/OpenGL/extensions/ARB/ARB_instanced_arrays.txt)、
[ARB draw instanced](https://registry.khronos.org/OpenGL/extensions/ARB/ARB_draw_instanced.txt)、
[EXT gpu shader4](https://registry.khronos.org/OpenGL/extensions/EXT/EXT_gpu_shader4.txt)。

## 空间索引与不可变快照准备缓存

CPU/GPU 同用 `scene3d.Culler` 的 Morton BVH，只把可见的原始索引送入有序实例化。
相机移动只查询已有空间树，不为每个屏外设备重复完整 mesh-frustum 测试。
GPU 的矩阵可表示性、完整场景预算与驻留集合验证按不可变 Snapshot 缓存；
`Stats.ValidatedInstances` 在相机独立变化帧为 0，在新的 Snapshot 上重新验证所有实例。
这不会绕过非法新快照：验证仍在裁剪/修改上一帧之前执行。

`Stats.Culling` 给出索引构建、增量 refit、包围盒/实例测试及线性回退统计。新快照只改
颜色时不重建空间树；移入视野的冷网格仍按需上传；移除的网格仍被回收；关闭时释放
空间 scratch 和缓存快照。`Render` 仍返回顶端起始 CPU 图像，颜色和 ID 同步 readback
没有在本阶段移除，不能据此宣称解决了所有驱动/所有场景的呈现瓶颈。

## GPU 驻留帧与显式传输（开发增量）

`RenderGPU` 与旧 `Render` 共用相同几何/实例化/裁剪/深度路径，但把颜色和 ID 保留在 FBO。
不调用 `glReadPixels`，也不创建全帧 Image、ID 像素或翻转缓冲。`GPUFrame` 是不暴露 GL 名称的
不可变借用令牌；变化绘制/resize/Close 后失效，相同缓存命中不使其失效。错误线程、错误 context、
其他 Renderer 的令牌、零值和过期令牌在调用 GL 前拒绝。参数验证在触碰旧帧之前失败时保留旧令牌。

```go
frame, err := renderer.RenderGPU(snapshot, camera, width, height, background)
if err != nil { return err }
if !renderer.CanBlitColor() { return glrender.ErrUnavailable }
// The host configures targetFBO's draw buffers and supplies its exact pixel size.
// Rectangles use top-left physical pixels; scaling is nearest, clip is explicit.
err = renderer.BlitColor(frame, targetFBO, targetWidth, targetHeight, destination, clip)
// Only when input requires a raster ID:
node, err := renderer.PickGPU(frame, pixelX, pixelY) // exactly four ID bytes
// Only for CPU composition, export or explicit diagnostics:
capture, err := renderer.Readback(frame) // both attachments, lazy/reused host buffers
_ = node; _ = capture
return err
```

- `BlitColor` 要求可选 EXT/core framebuffer blit；是**不透明复制**，不做 alpha 混合或控件排序。
  目标已配置的所有 draw buffers 接收颜色。宿主不得将源附件别名绑定到目标。保存/恢复 read/draw
  framebuffer、源 read selector 和 scissor 状态；它不会破坏目标的 MRT draw-buffer 配置。
- `PickGPU` 与全量 Readback 使用同一 raster ID，不以几何射线冒充光栅可见性；界外查询零传输。
  调用是同步的，单像素仍可能等待 GPU 完成，不是“零延迟”。Readback/Pick 显式重置 pack buffer、
  alignment、row length 和 skip rows/pixels，以免宿主残余状态越过目标切片；它们不保存该 GL 状态。
- `Transfers()` 为所属线程上的累计成功调用/字节计数，区分 ColorBlits、ColorReads、IDReads 和
  字节数；不是驱动计时。`GPUFrame.Statistics().ReadbackBytes` 为 0；CPU Frame 保持旧统计语义。
- `Render` 仍返回 `FrameView`，等价于 GPU render + 显式完整 readback，兼容当前 Cairo 控件树。
  **SceneView 尚未接入驻留帧**：不能在最终 2D 图像上无条件叠一张 GPU 图，那会覆盖菜单、同级控件
  与 toast。后续必须完成有序合成、截图/导出、所有权迁移与原生控件生命周期后，才称 IDE 零读回。
- `SILK_TEST_SCENE_GPU=1 SILK_TEST_REQUIRE_BLIT=1` 启用实际 native resident 探针，检查零全帧
  CPU 缓冲、4 字节拾取、裁剪/负偏移/缩放逐像素结果、GL 状态恢复和线程/关闭/过期拒绝。

依据：[Khronos 同步语义](https://wikis.khronos.org/opengl/Synchronization)、
[EXT framebuffer blit](https://registry.khronos.org/OpenGL/extensions/EXT/EXT_framebuffer_blit.txt)。
使用 PBO 后立即映射并不能消除等待，故本阶段先拆开驻留帧和真正需要的读回，不增加空闲绘制循环。

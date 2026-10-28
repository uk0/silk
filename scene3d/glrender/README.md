# OpenGL 场景后端

`glrender` 是真实 GPU 光栅化路径，不是给 CPU 渲染改名。顶点/索引上传到缓冲对象，
顶点着色器处理世界/相机变换，片元着色器输出平面光照与独立 ID，深度附件完成遮挡。

## 使用与资源归属

- 要求 cgo、OpenGL 2.1 compatibility、`GL_EXT_framebuffer_object` 和至少两个 draw buffers。
  `!cgo` / `silk_pure_go` 编译为明确返回 `ErrUnavailable` 的桩，不偷偷宣称 GPU 可用。
- 宿主创建**独占**上下文，在绑定该上下文的同一 OS 线程调用 `NewCurrent`、`Render`、
  `Close`；当前 go-gl 入口表为进程全局，因此多个 Renderer 的 GL 调用/初始化也应
  串行安排在同一 OS 线程，不支持后台并行 GL context。后端会改变 GL 状态，
  不负责保存共享上下文的任意外部状态。
- `Render` 返回 `Frame`（实现 `scene3d.FrameView`），RGBA 是顶端起始，拾取坐标也是顶端
  起始。缓存不重复 GL draw/readback；新 Frame 的 `Sequence` 才增长。
- 同一个 Mesh 在该 Renderer 内只上传一次，实例复用 VBO/索引缓冲；相机、颜色和节点
  变换通过 uniform 更新。当前不是单次 draw 的硬件实例化；每可见节点一次 draw。
- 新场景绘制前回收不再引用的 mesh。网格缓冲预算 64 MiB，帧还受 `scene3d.MaxPixels`、
  驱动 texture/renderbuffer 上限约束。多个 Renderer 的预算不共享。
- 返回图像/ID 缓冲只读，下一次 Render 可能覆盖；失败后不使用旧 Frame。Close 必须在
  native context 销毁前调用，不依赖 Go finalizer 进入 GL。
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
还有含读回的 25.6 万三角形对照，不是纯 GPU timer、桌面 FPS 或普遍性能承诺。

限制：顶点/矩阵使用 float32；暂不支持大坐标相机相对精度方案、纹理/PBR、透明材质、
平滑法线、MSAA、单 draw 硬件实例化、BVH/LOD、零读回合成。每变化帧同步读取颜色+ID
两个附件，在简单场景中可能比 CPU 慢。后续需要避免读回并维持正确的控件合成顺序，
不能为了性能改回悬空 overlay。参见 [总路线](../../SCENE3D_ROADMAP.md)。

规范依据：[Khronos EXT framebuffer objects](https://registry.khronos.org/OpenGL/extensions/EXT/EXT_framebuffer_object.txt)、
[GLSL 1.20](https://registry.khronos.org/OpenGL/specs/gl/GLSLangSpec.1.20.pdf)、
[GLFW context guide](https://www.glfw.org/docs/latest/context_guide.html)、
[Microsoft WGL current context](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-wglgetcurrentcontext)。

# 嵌入式工业 3D 场景

从仓库根目录执行：

```sh
go run ./examples/scene3d -model examples/scene3d/pump.obj
go run ./examples/scene3d -model examples/scene3d/assembly.gltf
# 强制真实 GPU：不满足驱动能力则明确诊断，不静默回退。
go run ./examples/scene3d -backend opengl
# 离屏参考图；仍需本机 gui/Cairo 构建依赖，不是独立无显示 CLI。
go run ./examples/scene3d -render /tmp/silk-scene3d.png
```

- **Publish 1000 values**：后台发布 0…999，UI 合并更新 tank 的 Y 坐标为 0.999。
- **Load model**：显式后台加载 OBJ/glTF/GLB，保留 `model` 根节点并重新绑定相同点位。
  glTF 保留组装件的层级、局部变换和共享网格，点击可选择 primitive 子节点。
  加载成功自动对准模型初始包围盒；之后的点位移动不会自动追踪相机。
- **Reset camera**：恢复环绕角和距离，保留当前模型的观察中心。
- **Fit scene**：按当前世界包围盒重新对准组装件，适用于点位移动后；不启动相机跟随循环。
- **Snapshot**：状态栏显示摘要，stdout 的 `SCENE_STATE` JSON 可回读线程、位置、
  选择、加载、相机、实际光栅化次数、警告及错误。统计在点击时采样，无定时刷新。
- **Inspect center**：显式后台构建/更新两级 BVH，再在 UI 线程查询视口中心表面。
  底部显示节点、原始三角索引、世界坐标和法线；`SURFACE_QUERY` JSON 还包含局部坐标、
  重心权重、访问计数、索引复用统计及查询前后渲染次数。准备期间禁用重复点击，关闭取消；
  场景在准备期间改变会报告 stale，不把旧表面当当前表面，不自动轮询重试。
  再次查询复用快照；点位更新后重建/复用由下一次显式点击触发。它不改变选择或场景文件。
- 拖拽环绕、滚轮缩放、点击选择节点。没有动画/轮询定时器，静态场景不重复光栅化。

`pump.obj` 与 `assembly.gltf` 是本仓库自建几何夹具，无外部素材依赖。默认 auto 在 native 视口尝试 OpenGL 并可查询回退原因；`-backend cpu` 强制 CPU。
当前都是平面着色，不展示纹理/透明材质。窗口关闭时取消上下文、绑定与加载并释放视口。

## 在 SDK 中组装

在 UI 线程先设置父控件，再设置 Scene、绑定 tag：

```go
view := gui.NewSceneView()
view.SetParent(form)
view.SetBounds(12, 80, 640, 480)
// 初始预览已有 model 节点；也可 SetScene 自己构造的层级场景。
stop, err := view.BindNodePosition(tag, "model", 1, 0.001, 0)
if err != nil { return err }
// stop 可在任意 goroutine 调用，取消排队但尚未开始的交付。
_ = stop
view.SetModelSource("assets/pump.obj") // 只配置，不隐式进行 I/O
view.SigModelLoaded(func() {
    // SetScene / LoadModel 成功会取消旧绑定；按新节点重新建立绑定。
})
if err := view.LoadModel(ctx); err != nil { return err }
// 宿主销毁时在 UI 线程调用 view.Close()。
```

运行时错误通过 `SigError(func(string))` 和 `LastError()` 查询；开始加载前的参数/忙碌
错误直接返回。绘制错误只更新 `LastError()` 并在控件内显示，不在 Draw 里调用宿主回调。
`ModelWarnings()` 返回副本。相对模型路径相对进程 cwd，不会自动相对 `.silkui` 文件解析。

`BindNodePosition` 支持 `float64` / `int64` / `int`；`BindNodeVisibility` 支持 bool。
仅应用 Good 质量；Bad/Uncertain/不匹配类型保留最近有效值。高频合并用于视觉状态，
不替代无损报警/历史记录。更改父级、替换场景、Close 会注销绑定和过期加载。

## 设计器接入

控件面板 **组态 (SCADA) → SceneView**，有矢量立方体图标。拖入后可设置 Model Source、
Camera Yaw/Pitch/Distance、Render Backend（auto/cpu/opengl）；保存、重载、预览和生成 Go 代码保留这些配置。
OnSelected / OnModelLoaded / OnError 可生成事件处理函数；也可使用 `decl.SceneView(...)`。

构造、预览及读取设计文件**不会自动打开模型文件**；宿主必须调用 `LoadModel`。
当前不把完整运行时 Scene、相机观察中心或 tag 绑定序列化到设计文件，场景节点/绑定通过独立 `.silkscene` 文件及其结构化编辑器保存。

## 场景文档与实时绑定验证

```sh
go run ./examples/scene3d -model examples/scene3d/assembly.gltf -scene-output /tmp/plant.silkscene
```

显式提供 `-scene-output` 才增加保存/回读操作，不在默认启动时写文件：

1. **Load model** 加载组装件并安装高度、旋转及单个 mesh 的告警颜色绑定。
2. **Publish 1000 values**、**Rotate 90 degrees**、**Toggle alarm** 驱动可见状态。
3. **Reconnect bindings** 验证原地重绑不重复叠加旋转；**Save scene** 在后台保存创作基线、共享几何、警告及绑定定义；等待 `document_saving=false`
   和 `SCENE_DOCUMENT_SAVED`，不能把按钮点击当作保存成功。
4. **Reload saved scene** 通过同一异步加载链回读，绑定来自文件且使用本例显式选择的 TagDB。
   最新点位重新应用一次；旋转/缩放不在每次保存后累积。

`SCENE_STATE` 增加绑定数量、保存状态/次数、旋转、告警值及目标矩阵/颜色。目标路径应使用 `.silkscene`；
本示例不是完整的 IDE 场景编辑器，也不会自动配置现场驱动或相机跟随。

在 SilkIDE 打开 `examples/scene3d/assembly.silkscene`，通过 Tools → Edit 3D Scene...
编辑节点与绑定。Use changes 返回 JSON 缓冲，Ctrl/Cmd+S 保存，单次 Undo 撤销整批修改。
该自有示例包含 2 个共享网格、8 个节点和 3 个惰性绑定；单纯打开不会连接现场点位。

`Snapshot` 的 `SCENE_STATE` 日志包含 `gpu_transfers`（累计 ColorReads/IDReads 与字节数）；
CPU 回退时为零。这是显式按钮/事件诊断，不安装采样定时器。可对比拖动前后与点击前后的计数：
GPU 绘制只读颜色，点击单次读取 4 字节 ID，静止缓存不持续传输。


## 实验性 GPU 直接合成（默认关闭）

macOS / Linux GLFW 窗口可显式试用：

```sh
go run ./examples/scene3d -gpu-compose -model examples/scene3d/assembly.gltf
```

SDK 在 UI 线程、窗口 attach 后、Draw 之外调用
`form.Window().SetSceneGPUComposition(true)`；用 `SceneGPUCompositionStatus()`
查询启用状态与回退原因。普通 `SceneView.Draw`、离屏导出与未启用窗口保持原来的 Cairo 路径。
Windows/GDI 与 pure-Go 明确返回 `glrender.ErrUnavailable`；示例打印诊断后正常使用原路径，
不代表 Windows 没有 GPU 光栅化能力。

- 合格视口直接在所属窗口的上下文渲染和 blit，`GPUTransfers().ColorReads` 不增加；
  光栅点击仍只读 4 字节。Cairo UI 纹理仍需要上传，不是全应用零传输。
- 仅接受正向轴对齐、整数物理像素位置、精确单矩形 clip；曲线/多区 clip、旋转、
  亚像素位置局部退回 Cairo，不用 clip 包围盒冒充遮罩。
- 按控件树顺序处理不透明场景与 CPU 像素，后绘菜单、提示与编辑手柄仍遮挡场景；
  重叠场景保持先后顺序。预乘透明叠加允许与全 Cairo 参考最多 1 个 8-bit 色阶的舍入差异。
- 场景之后遇到非 OVER 算子、图标回调或 Target 检查，以及过期 GPU 帧，拒绝提交该帧、
  排队完整 Cairo 重绘并关闭此路径；不会自动反复重试。图标限制是保守回退，不是缺失图标。
- 首版每次实际重绘采用完整树/整窗 UI 纹理上传；不新增定时器，空闲不重绘。
  小脏区或复杂 UI 不保证更快，因此尚未默认开启或启用到 SilkIDE 全部窗口。
- 所有 window-owned GPU 资源在原生上下文销毁前释放；工作线程不能渲染/拾取/释放它们。

`SCENE_STATE` 新增 `gpu_composition` / `gpu_composition_fallback`，可与传输计数一起核验实际路径。
裁剪查询遵循 [Cairo 的精确 rectangle-list 契约](https://www.cairographics.org/manual/cairo-cairo-t.html#cairo-copy-clip-rectangle-list)，
而不是仅使用 `ClipBounds`。

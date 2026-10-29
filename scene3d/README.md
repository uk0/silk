# scene3d：工业场景 SDK 基础

纯 Go、无第三方依赖的场景数据、受限 OBJ/glTF/GLB 加载器和 **CPU 参考渲染器**。
本包仍不依赖 GL；可选 [glrender](glrender/README.md) 子包实现 GPU 后端。CPU 路径保留
为正确性对照，不是完整工业 3D 引擎或 Qt Creator 功能对等声明。

## 快速使用

```go
s := scene3d.NewScene()
err := s.Add(scene3d.Node{
    ID: "pump", Mesh: scene3d.Box(),
    Transform: scene3d.Translation(scene3d.Vec3{X: 1}),
    Color: color.RGBA{R: 245, G: 160, B: 65, A: 255},
})
if err != nil { return err }
snapshot, err := s.Snapshot()
if err != nil { return err }
var renderer scene3d.Renderer
frame, err := renderer.Render(snapshot, scene3d.DefaultCamera(), 640, 480, color.RGBA{})
if err != nil { return err }
// frame.Image 可交给 png.Encode；frame.Pick(x, y) 返回稳定节点 ID。
```

完整可运行的原生控件示例：从仓库根目录执行
`go run ./examples/scene3d -model examples/scene3d/pump.obj`。

## 数据与线程契约

- `NewMesh` 验证并复制顶点/索引；不可变 `Mesh` 可多节点、跨 goroutine 共享。
  内置 `Box()` 和 Y 轴闭合 `Cylinder(radius, height, segments)`。
- `Scene` 和 `Renderer` 是 **单所有者** 对象，不可并发读写。`Scene.Snapshot()`
  生成不可变状态；后台可用自己的 `Renderer` 渲染旧快照，同时 UI 修改原 Scene。
- 节点稳定 ID、父节点、局部变换、可见性、颜色；先添加父节点，再添加子节点。
  隐藏/删除父节点影响整个子树。父颜色不覆盖子材质；创作时使用
  `EditSession.Reparent` 重挂父节点，可选择保留世界变换并支持撤销/重做。
- 矩阵列主序、列向量；`a.Mul(b)` 表示 `a*b`。`RotationXYZ` 使用弧度，依次 X、Y、Z。
  Node 的零矩阵初始化为单位阵，零颜色初始化为蓝色。材质目前必须不透明。
- `Frame`/`Image` 属于 Renderer，下次 Render 可改写；按只读处理，跨线程前自行复制。
  相同快照、相机、尺寸、背景会复用帧；`Sequence` 只在成功光栅化后递增，`Reset` 清零。
  出错可能留下部分内部缓冲，不可继续使用旧 Frame；再次成功 Render 后才可读取。
- 六平面齐次裁剪、深度缓冲、ID 拾取、双面平面光照；暂无纹理、平滑法线、透明度、
  阴影、抗锯齿、LOD 或多线程分块。`Pixels` 统计深度测试通过的写入次数，
  不是唯一可见像素数；缓存查询保留上一次渲染计数并置 `Cached=true`。

## 输入与工作量限制

- 每网格最多 250,000 顶点 / 500,000 三角形；每场景最多 10,000 节点。
- 每个快照最多 1,000,000 可见实例顶点 / 1,000,000 三角形，**重复实例也计数**。
  超限返回 `ErrLimit`，不是静默跳过模型。这些上限不是交互延迟保证。
- 单维最多 8,192，帧最多 16,777,216 像素；RGBA、深度、ID 各占约 4 字节/像素，
  GUI 合成另持有一份 Cairo 图像。大窗口、多个视口的宿主应设置更低尺寸预算。
- `LoadOBJ(ctx, reader, LoadLimits{})` 默认最多 32 MiB 输入、1 MiB 单行；显式预算有上限。
  支持 `v`、三角形、凸且共面的四边形、正/负索引和 `v/vt/vn` 引用检查。
  不支持凹面、大多边形、曲线；未知指令明确报错。
- OBJ 法线/UV 被检查但不用于当前渲染，分组扁平化，MTL 不加载；读取 `Warnings`，
  不要把成功加载几何当作材质/层级完整导入。**不自动打开任何外部 MTL/贴图路径**。
- Reader 取消在读/解析边界检查；调用者须负责关闭或设置期限以打断阻塞 Reader。

## glTF / GLB 静态场景

```go
// 明确的文件 I/O；外部 buffer 仅能来自模型目录及其子目录。
loaded, err := scene3d.LoadModelFile(ctx, "assets/assembly.glb", scene3d.LoadLimits{})
if err != nil { return err }
// loaded.Scene 是单所有者对象；可交给 UI 线程的 view.SetScene。
// loaded.Warnings 必须展示/记录，不能把几何预览当完整 PBR 导入。
```

- `LoadGLTF(ctx, reader, GLTFOptions{})` 自动识别 JSON/GLB，不自动打开路径或网络。
  外部 buffer 需要显式 `Buffers fs.FS`，本机目录应使用 `os.OpenRoot(dir).FS()`；
  `os.DirFS` 不限制符号链接逃逸。`LoadModelFile` 已使用 Root 并只接受普通文件。
- 保留选择的 scene、父子层级、matrix / T×R×S、四元数、负缩放、共享 mesh、
  多 primitive；支持索引/无索引 TRIANGLES、TRIANGLE_STRIP、TRIANGLE_FAN，
  交错顶点与 sparse accessor。POSITION 为 FLOAT VEC3，索引为 UBYTE/USHORT/UINT。
- 基础色从 glTF 线性颜色转换为现有 RGBA 预览；当前双面平面着色不是 PBR。
  纹理、法线/顶点颜色、相机与可选扩展明确警告；不读取 image URI。
  required extension、蒙皮、动画、morph、MASK/BLEND 等明确拒绝。
- 默认使用文件的 default scene；未声明时选择 scene 0 并警告。无 scenes 时选父节点为空
  的全部节点并警告。`SceneIndex` 可覆盖选择。所有节点均检查循环/多父/无效引用。
- `GLTFModel.Nodes()` / `Warnings()` 返回副本；`NewScene("device")` 创建独立可变场景，
  Mesh 仍不可变共享。稳定 ID 为 `device/node/N` 及 `device/node/N/primitive/P`，不依赖名字。
  把 tag 绑定在 `device` 根节点可移动整个组装件而不破坏内部局部变换。
- 顶点/三角预算按导入的全部唯一 primitive 合计；重复节点另受 Snapshot 的实例预算约束。
  字节预算包含主文件与额外 buffer/解码数据，GLB BIN 不重复计费。另限制 JSON 元数据深度、
  token/容器数量和每 primitive 32 个属性的验证工作量；拒绝重复键、无穷值、越界和整数溢出，不是完整 glTF schema validator。
- `Snapshot.Bounds()` 计算可见实例的世界 AABB；`view.FitScene()` 显式重置观察中心/距离，
  可在点位移动后调用。没有自动跟踪定时器或空闲重绘循环。

格式依据：[Khronos glTF 2.0 specification](https://registry.khronos.org/glTF/specs/2.0/glTF-2.0.html)。

完整目标与剩余验收：[路线图](../SCENE3D_ROADMAP.md)。

## 可保存的场景与编译式绑定

`.silkscene` v1 保存当前引擎支持的静态几何、稳定节点 ID、层级、共享网格、变换、
不透明颜色、可见性、导入警告与点位绑定配置。它不是原始 glTF/PBR 的无损备份，
也不包含宿主相机、窗口布局、驱动凭据或可执行脚本。几何按共享 Mesh 去重，文档
无外部文件引用；读取或配置控件不会自动订阅点位。

```go
bindings := []scene3d.Binding{
    {Node: "pump", Tag: "pump.height", Property: scene3d.PositionY, Gain: .001},
    {Node: "pump", Tag: "pump.angle", Property: scene3d.RotationZ, Gain: math.Pi / 180},
    {Node: "pump", Tag: "pump.enabled", Property: scene3d.Visibility},
}
doc, err := scene3d.CaptureDocument(authoredScene, bindings)
if err != nil { return err }
// 显式文件 I/O；放在后台任务，不要在 Draw 中保存。
if err := scene3d.SaveDocumentFile(ctx, "plant.silkscene", doc); err != nil { return err }
loaded, err := scene3d.LoadModelFile(ctx, "plant.silkscene", scene3d.LoadLimits{})
if err != nil { return err }
// 在 UI 线程安装；TagDB 必须由应用显式选择并已具有所引用的点位。
if err := view.SetScene(loaded.Scene); err != nil { return err }
stop, err := view.SetSceneBindings(db, loaded.Bindings)
if err != nil { return err }
// stop 可从任何 goroutine 调用；关闭/换场景/重挂父控件也会取消。
```

- `LoadDocument` / `SaveDocument` 使用 Reader/Writer；`Document.Build` 创建独立场景，
  `Scene.Nodes` 返回节点副本。保存/回读保持合法的原绘制顺序，避免共面像素的拾取 ID 改变。
- 输入上限 32 MiB；另有元数据深度/token/容器、节点、唯一几何、绑定和文本预算。
  循环/重复 ID、越界网格、非仿射矩阵、重复 JSON 键（含大小写别名）、未知字段、
  尾随 JSON、无效版本和无效绑定均拒绝。绑定目标必须存在，同一节点属性不能重复绑定。
- `BindingSet` 是不依赖 GUI/core 的单所有者编译计划；支持位置 X/Y/Z、相对旋转 X/Y/Z、
  相对缩放 X/Y/Z、布尔可见性及布尔告警颜色。旋转单位弧度，旋转/缩放按
  `authored * RotationXYZ(delta) * Scaling(factor)` 组合，再覆盖绑定的位置分量；
  不对原始仿射矩阵进行有损分解。数值映射为 `value*Gain+Offset`，恒等映射须设 `Gain: 1`，
  `Gain: 0` 是合法常量映射。告警颜色要求两个显式不透明颜色且目标节点自身有 Mesh，
  **不假称组节点颜色会自动继承到所有子节点**。
- `BindingSet.Apply` 在验证输入及局部矩阵后整批提交一个点位的所有目标。未知 tag、类型
  不符、非有限数值或已删除节点不会部分提交。世界层级溢出仍由 `Scene.Snapshot` 报错。
  计划内复用 scratch 缓冲，正常数值更新不分配堆对象；节点矩阵不能跨 goroutine 并发写。
- `SceneView.SetSceneBindings` 先验证全部配置及 TagDB 引用，成功才替换所有旧绑定。
  所有点位共用一个合并 UI 投递，每轮最多 256 个样本，质量差/类型错误保持最后已应用的
  正常值；中间样本可能被合并，不能替代事件/告警历史。成功载入新文档后，应显式调用
  `SetSceneBindings(db, view.SceneBindings())`，不会自动创建不存在的点位。
- 活跃绑定时使用 `BindingSet.CaptureDocument` / `SceneView.CaptureSceneDocument` 导出
  **创作基线**，而不是把运行时旋转/缩放再次保存成基线，避免保存、重载、重绑后重复叠加。
  `BindingSet.Reconfigure` / 再次 `SetSceneBindings` 在原场景上恢复旧绑定的创作基线后
  验证新计划；失败不改变场景，移除绑定恢复对应基线。停止订阅本身保持当前显示值。
  旧式 `BindNodePosition` / `BindNodeVisibility` 不携带可持久化配置，捕获时保存其当前值。
- `SaveDocumentFile` 先编码验证，再写同目录临时文件、sync/close、rename；拒绝已有符号
  链接/目录目标，不自动创建父目录。rename 前的错误/取消不截断原文件，原文件普通权限
  得到保留。宿主仍须串行保存并控制目录写权限；不是跨平台掉电持久性或 compare-and-swap 保证。

## SDK / IDE 结构化场景编辑器

- `gui.NewSceneDocumentEditor(doc)` 提供节点树、可拾取预览、节点表单和绑定列表；
  输入与导出的文档均为独立副本，显式编辑才重新构建场景，空闲不轮询点位。
- `gui.ShowSceneDocumentEditor(parent, doc, commit)` 暂存修改；取消不调用 commit，
  commit 错误保留窗口。宿主负责检查源版本并实现撤销/保存事务。
- SilkIDE 打开 `.silkscene` 后使用 **Tools → Edit 3D Scene...** 或命令面板同名命令。
  支持名称、父 ID、局部 XYZ、网格颜色、子树显隐；已有旋转/缩放矩阵保留。
  拾取模型会展开祖先并同步选中节点树；组节点不能修改不存在的自身材质。
- 绑定表单支持 11 种属性、点位名、数值映射与布尔双色告警；按节点/属性新增或更新，
  改选属性不隐式删除旧属性绑定，删除使用独立按钮。预览只显示创作基线，不连接 TagDB。
- **Use changes** 只替换当前文件缓冲一次，复用正常 Undo/Redo、dirty、Save/Save All、
  关闭保护与会话恢复；旧异步结果和过期源版本不得覆盖新文本。CRLF 文档保持换行风格。
  `.silkscene` 使用 JSON 高亮，不冒充 Go 文件启动 LSP。

示例资产：`examples/scene3d/assembly.silkscene`，由仓库自有几何生成。当前不包含
交互变换 gizmo、完整 TRS 表单、PBR/纹理或现场驱动配置。节点新增/复制/删除及改父级
已由下文 `EditSession` 和 Node actions 补齐；早期文档编辑链的阶段测试记录见
[场景文档 QA](../SCENE_DOCUMENT_QA_20261028.md)，不代表完整工业编辑器已经完成。

## 设备模板与批量产线组装

`Template` 把已验证的创作场景编译成可复用设备定义。加载/验证网格一次，后续所有实例
共享不可变 `Mesh`，节点状态和点位目标彼此独立。这是数据层实例化，**不是 GPU 硬件
实例化**；可见实例仍逐个计算快照预算和绘制工作量。

```go
// source 是 LoadDocument 或 CaptureDocument 得到的创作基线。
template, err := scene3d.NewTemplate(ctx, source)
if err != nil { return err }
plant := scene3d.NewScene()
instances, err := template.InstantiateBatch(ctx, plant, []scene3d.InstanceOptions{
    {ID: "pump-01", Transform: scene3d.Translation(scene3d.Vec3{X: -3}), TagPrefix: "pump01."},
    {ID: "pump-02", Transform: scene3d.Translation(scene3d.Vec3{X:  3}), TagPrefix: "pump02.",
        TagMap: map[string]string{"alarm": "line.sharedAlarm"}}, // key 必须是源模板的真实 tag
})
if err != nil { return err }
var bindings []scene3d.Binding
for _, instance := range instances { bindings = append(bindings, instance.Bindings()...) }
// UI 线程：view.SetScene(plant)，然后显式选择注册表安装 bindings。
// 不自动创建点位，也不自动连接任何设备。
```

- 每次放置增加一个无网格根节点；源节点 ID 映射为 `实例 ID + "/" + 源 ID`，保留原层级、
  合法绘制顺序和共享网格。`RootID()` / `NodeID(sourceID)` 不依赖显示名称；组合 ID 仍须
  在 256 字节内。`Parent` 可以是目标中已有节点或同批次更早放置的节点。
- `TagMap` 精确覆盖源点位名称，否则原名加 `TagPrefix`；未知 map key、空名称、非法文本、
  超限及同名数值/布尔混用均拒绝。多个数值通道可明确映射到同一个数值点位。
  `Bindings()` / `Warnings()` 返回副本；描述符不会创建订阅。
- `InstantiateBatch` 只复制一次目标节点元数据，不按实例复制网格数组；整个批次验证成功
  后仅增加一次 revision。失败、取消、ID 冲突、世界矩阵溢出及快照超限均不改变目标节点、
  revision 或旧缓存快照。空批次不修改场景。批量组装优先用此方法而非循环单次添加。
- 模板可跨 goroutine 共享，但每个目标 `Scene` 仍严格单所有者；不要同时改 options/map。
  原场景的外部绑定计划不会自动重配，调用者应显式合并所需配置；用 `Remove(RootID())`
  可删除整台设备，之后调用者负责停止/重配相关绑定，描述符不会跟踪删除状态。
- 活跃场景应先用 `BindingSet.CaptureDocument` 取得创作基线，避免把运行时变化烘焙进模板。
  不同模型可编译各自模板后装入同一目标；共享只发生在同一模板的网格，不做跨文件内容哈希
  去重。导出仍受 `.silkscene` 自身的唯一几何、文本和 32 MiB 文件预算约束。

纯 Go 产线生成示例见 [`examples/scene3dplant`](../examples/scene3dplant/README.md)。

## 轴向移动交互 SDK

`Camera.Project` 把世界坐标映射到左上原点的视口 XY，Z 为 0（近）到 1（远）；
返回的 `visible` 检查完整视锥。`Camera.Ray` 使用同一相机/视口生成归一化世界射线，
允许捕获指针越出视口。两者必须使用同一种单位：GUI 逻辑像素或物理像素，不能混用。
`Ray.IntersectPlane` 仅接受射线前方交点，拒绝无效、近乎平行和溢出输入。

```go
// 鼠标按下时；edit 是单所有者 EditSession，preview 是独立的预览 Scene。
world, err := edit.WorldTransform(nodeID)
if err != nil { return err }
ray, err := camera.Ray(x, y, width, height)
if err != nil { return err }
axis, err := scene3d.NewAxisDrag(ray, world.Point(scene3d.Vec3{}), scene3d.Vec3{X: 1})
if err != nil { return err }
gesture, err := edit.BeginTranslation(ctx, nodeID)
if err != nil { return err }
defer gesture.Cancel()
// 后续移动使用按下时冻结的 camera/width/height；delta 是从起点算起的绝对位移。
nextRay, err := camera.Ray(nextX, nextY, width, height)
if err != nil { return err }
delta, err := axis.Offset(nextRay, .5) // 0 = 自由移动；.5 = 世界位移吸附
if err != nil { return err }
local, err := gesture.Transform(delta)
if err != nil { return err }
if err := preview.SetTransform(nodeID, local); err != nil { return err }
// 鼠标释放时调用一次。取消则丢弃/恢复 preview，不调用 Commit。
if err := gesture.Commit(ctx, delta); err != nil { return err }
```

- 固定拖拽平面与起始抓取偏移，避免按下跳变和累计漂移；几乎正对视线的轴拒绝拖动。
- `Transform` 不修改 session、绑定或历史，O(1)、零分配；通过父级逆线性基保持局部旋转、
  缩放、镜像和剪切。奇异父级拒绝，隐藏节点/组可由 SDK 编辑。
- `Commit` 完整验证后最多追加一个历史项；无变化不改矩阵编码、不清空 redo。
  期间发生任何 session revision 变化（包括 undo/redo）返回 `ErrEditConflict`，避免覆盖新编辑。
  `Cancel` 只释放事务引用，**不会替宿主恢复其独立 preview Scene**。
- GUI `SceneDocumentEditor.SetTranslationEnabled(true)` 开启 XYZ 工具；
  `SetTranslationSnapStep(.5)` 设置步长，Shift 临时使用 1 单位。未命中手柄仍可旋转/拾取。
  手柄只在自身画布绘制与裁剪，静止帧继续使用缓存，不创建动画定时器。
- GUI 对焦点/捕获丢失、Escape、resize、禁用、相机/场景变化取消正在进行的手势。
  `IEventPointerCancel.OnPointerCancel` 在 UI 线程调用；实现方必须先退休自己的手势，
  再释放捕获，不能在回调中提交或重新捕获。其他线程须使用现有 UI 调度接口。
- 当前仅世界坐标单节点/组平移，尚无旋转、缩放、多选或局部坐标模式。

## CPU / GPU 共享视锥裁剪

`NewFrustum` / `Frustum.Intersects` 使用不可变网格的缓存 AABB，不遍历
顶点、不分配堆对象。将六个齐次裁剪平面变换到模型空间，只拒绝完全处于某个平面外的
包围盒；旋转、负缩放、剪切、零缩放、相机位于盒内和穿越近裁剪面均保留原逐三角裁剪。
零值/不确定或超出 float32 算术范围的输入保守放行，不能用它代替模型/相机验证。
内部误差裕量基于绝对中间量，避免大数抵消或 CPU float64 / GPU float32 差异造成边缘跳变。

```go
vp, err := camera.ViewProjection(float64(width) / float64(height))
if err != nil { return err }
frustum := scene3d.NewFrustum(vp)
if frustum.Intersects(mesh, world) {
    // 可能可见；不是精确网格相交，也不是遮挡判定。
}
```

CPU 与 OpenGL 后端均自动使用它：屏外实例不再变换全部顶点/扫描三角形或提交 GPU draw。
Snapshot 及实例顺序不改写，拾取 ID 保持原索引；点位把设备移回视野时正常重绘。
`CulledInstances` / `CulledTriangles` 记录拒绝的实例与原索引三角形（含退化三角形）。
`Triangles` 仍表示实际进入后续处理的工作；缓存帧保留上次实际绘制的统计，并标记 Cached。
逐实例视锥测试不会放宽导入、Snapshot 或 GPU 内存预算；分层加速见下文的 Culler，
遮挡剔除和硬件实例化仍是不同能力。

裁剪体依据：[OpenGL 2.1 specification §2.12](https://registry.khronos.org/OpenGL/specs/gl/glspec21.pdf)。
当前隔离开发与验证记录见 [视锥裁剪 QA](../SCENE_FRUSTUM_QA_20261028.md)。


### GPU 实例化与控件能力查询

`glrender.NewCurrentWithOptions` 提供 Auto / Disabled / Required 有序实例化策略；
支持的 context 对连续共享 mesh 设备批量提交，保留原节点 ID、深度顺序、视锥裁剪和
普通控件树合成。`SceneView.GPUCapabilities()` 暴露当前已启用能力，不触发绘制/建窗。
仍有独占 OS 线程/context 契约，未消除 readback，也不保证所有驱动/模型都会提速。
详细 API、内存预算与验收边界见 [GPU 后端](glrender/README.md)。

### Inert scene authoring and bounded history

`NewEditSession(ctx, document)` owns a private copy for authoring tools/plugins.
Methods are single-owner, just like `Scene`; immutable meshes/snapshots can be
shared with render owners. `Document()` and `Scene()` return independent mutable
copies. Editing never opens a tag database, driver, file or graphics context.

```go
edit, err := scene3d.NewEditSession(ctx, document)
if err != nil { return err }
defer edit.Close()
if err := edit.AddNode(ctx, scene3d.DocumentNode{ID: "line-b"}); err != nil { return err }
ids, err := edit.DuplicateSubtree(ctx, "pump-a", scene3d.DuplicateOptions{
    RootID: "pump-b", Offset: scene3d.Vec3{X: 3},
    CopyBindings: true, Tags: map[string]string{"pump-a.level": "pump-b.level"},
})
if err != nil { return err }
if err := edit.Reparent(ctx, ids["pump-a"], "line-b", true); err != nil { return err }
if err := edit.Undo(ctx); err != nil { return err }
updated, err := edit.Document() // the host decides when/how to commit or save
```

`AddMeshNode` adds a new immutable mesh and its first node atomically;
`AddNode` references an existing mesh ID. `UpdateNode`, `SetBinding`,
`RemoveBinding`, `RemoveSubtree`, `Undo` and `Redo` share the same transaction
boundary. All failures/cancellation preserve state and history. Duplicate bindings
are opt-in; unmapped tags stay the same process points. Removing a subtree removes
its binding declarations but retains mesh resources. Keep-world reparent rejects
unsafe/singular inverses instead of making geometry jump (1e-9 matrix-element
relative/absolute tolerance). It preserves authored transforms, not live tag state.

History retains at most 32 states / 32 MiB estimated metadata, excluding shared
geometry and current/initial state; normal document geometry limits still apply.
Renames/inert binding edits reuse unchanged draw snapshots and compiled meshes.
See `SCENE_AUTHORING_QA_20261028.md` for test contracts and acceptance boundaries.

## Morton BVH 与稳定顺序可见性查询

`Culler` 为 CPU 和 GPU 共用的纯 Go 空间索引。内部按包围盒中心的 Morton 编码排序、
建立平衡 BVH；使用位集合恢复原始实例顺序，因此不改变拾取 ID、等深度先绘制者或
连续共享 mesh 的实例化分组。内部排序不修改 Snapshot、节点或 mesh。

```go
var culler scene3d.Culler // 每个调用线程/渲染 owner 单独持有
snapshot, err := scene.Snapshot()
if err != nil { return err }
vp, err := camera.ViewProjection(16.0 / 9.0)
if err != nil { return err }
indices, stats := culler.Cull(nil, snapshot, scene3d.NewFrustum(vp))
for _, index := range indices {
    instance, _ := snapshot.Instance(index)
    _ = instance // 供自定义后端使用；顺序与原 Snapshot 相同
}
_ = stats.BoundsTests
// 下一次调用可以复用 indices 的容量；返回切片归调用者所有。
indices, _ = culler.Cull(indices, snapshot, scene3d.NewFrustum(vp))
culler.Reset() // 释放上一快照、BVH、排序/位集合 scratch
```

- 64 个实例以下走线性路径；最大实例数仍受 `MaxNodes` 和原 Snapshot 预算限制。
  每叶最多 8 个实例，树深有界，查询使用固定栈；容量充分的热查询不分配内存。
- 同一 Snapshot 的相机变换不重建索引。新的同长度 Snapshot 比较 mesh/世界变换：
  颜色/ID 编辑不改空间树；几何变更只重算相关叶和祖先。实例数变化重建空间排序。
  这不是任意动态场景的 O(1) 更新：新快照仍有 O(N) 比较，初建还需要整数排序。
- 反射、剪切、退化缩放、边界附近和消减误差沿用保守浮点语义；不确定包围盒、
  超出 float32 中间范围的变换不得被分层裁剪隐藏。渲染器依然验证非法几何与预算。
- 大量重叠包围盒或长期移动可能降低索引质量；每次查询有节点访问预算，达到上限
  后对剩余不相交索引区间线性测试，不持续支付低效的完整树遍历开销。
- `CullingStats` 报告 BoundsTests、InstanceTests、RefittedNodes、HierarchyNodes、
  LinearFallbackInstances 和被裁剪实例/三角形；CPU/GPU Frame 的 `Stats.Culling`
  同样暴露本次实际绘制的统计，缓存帧保留上次统计，不代表再次执行了这些工作。
- 不是 SAH 树、三角形级加速、遮挡检测、LOD、多线程渲染或零读回合成。
  查询微基准不等于完整 GUI FPS；构建/更新成本也必须单独考虑。

概念参考：[PBRT 的 BVH 与 Morton 空间排序](https://www.pbr-book.org/4ed/Primitives_and_Intersection_Acceleration/Bounding_Volume_Hierarchies)。
本实现使用普通 Go 整数排序、平衡分组及稳定原序输出，不复用 PBRT 的 HLBVH/SAH 实现。

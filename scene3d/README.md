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
  隐藏/删除父节点影响整个子树。父颜色不覆盖子材质；尚无重挂父节点 API。
- 矩阵列主序、列向量；`a.Mul(b)` 表示 `a*b`。`RotationXYZ` 使用弧度，依次 X、Y、Z。
  Node 的零矩阵初始化为单位阵，零颜色初始化为蓝色。材质目前必须不透明。
- `Frame`/`Image` 属于 Renderer，下次 Render 可改写；按只读处理，跨线程前自行复制。
  相同快照、相机、尺寸、背景会复用帧；`Sequence` 只在成功光栅化后递增，`Reset` 清零。
  出错可能留下部分内部缓冲，不可继续使用旧 Frame；再次成功 Render 后才可读取。
- 六平面齐次裁剪、深度缓冲、ID 拾取、双面平面光照；暂无纹理、平滑法线、透明度、
  阴影、抗锯齿、BVH/LOD 或多线程分块。`Pixels` 统计深度测试通过的写入次数，
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
节点新增/删除、交互变换 gizmo、完整 TRS 表单、PBR/纹理或现场驱动配置。不能把这个
阶段等同于完整工业编辑器；测试记录见 [本轮 QA](../SCENE_DOCUMENT_QA_20261028.md)。

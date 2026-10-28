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

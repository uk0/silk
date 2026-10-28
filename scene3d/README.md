# scene3d：工业场景 SDK 基础

纯 Go、无第三方依赖的场景数据、受限 OBJ 加载器和 **CPU 参考渲染器**。
这是后续 GPU 后端的正确性对照，不是完整工业 3D 引擎或 Qt Creator 功能对等声明。

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
  阴影、抗锯齿、GPU、BVH/LOD 或多线程分块。`Pixels` 统计深度测试通过的写入次数，
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

完整目标与剩余验收：[路线图](../SCENE3D_ROADMAP.md)。

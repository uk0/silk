# 工业组态 + 3D 场景 + 跨平台控件开发路线

目标保持完整：IDE 同时支持工业组态、代码开发、模型加载/数据绑定、高性能 3D 场景和跨平台插件控件。此文是实施与验收清单，**不是完成声明**。

起始基线 `a4ff0d3`：主线只有 Cairo 2D + GLFW/OpenGL 呈现；Win32 没有 GL 呈现上下文。已有 tag、驱动、组态服务和编译链接式控件插件注册，可复用而不另造一套 UI/数据系统。

## 实施阶段与验收

1. 独立 `scene3d` SDK：不可变共享网格、稳定节点 ID、层级变换、可见性/颜色、相机、模型输入预算、深度缓冲、拾取、渲染缓存。用几何/像素/错误输入测试和基准证明。
2. `gui.SceneView`：真正嵌入同一控件树，正常裁剪/遮挡/缩放，不建悬空子窗口；按需重绘、相机交互、选择事件、异步模型加载、取消及资源释放。通过真实桌面操作与保存回读验收。
3. 组态：tag 驱动节点位置/旋转/缩放/可见性/告警颜色；订阅合并、质量状态、销毁取消；设计器属性、持久化、预览、生成代码端到端一致。
4. 高性能引擎：GPU 深度/材质渲染、共享顶点/索引缓冲、实例化、视锥裁剪、资源预算/复用、后台加载；保留软件正确性对照路径。Windows/macOS/Linux 分别运行对照、性能与生命周期测试，不能只看平均 FPS。
5. 模型：OBJ 初始几何路径；glTF/GLB 的层级、变换、材质/纹理逐项支持；对未支持特性明确报错/诊断，不静默声称模型完整。验证恶意大小、截断、索引、NaN、循环与取消。
6. 插件控件：复用并完善跨平台注册、能力/版本描述、设计器与生成代码集成；明确静态链接与动态扩展边界，不依赖不支持所有目标平台的 Go plugin ABI。
7. 工业场景示例：模型加载、设备状态、实时点位与代码事件协作；Windows/macOS/Linux 原生 UI、线程/race、输入/布局/关闭、静默 CPU、性能基准和回归矩阵。

当前阶段的 CPU 参考渲染器不是 GPU 高性能目标的替代品；完整材质/纹理、高性能 GPU 管线、大场景性能与三平台桌面验收未完成前，整个目标保持 active。

## 早期阶段证据（后续进展见文末）

- `14ab7b4` 已提交 CPU 参考、OBJ、嵌入式控件、位置/可见性绑定与设计器接入。
- 本阶段增加真实 OpenGL FBO/深度/颜色与 ID、共享网格缓冲和后端诊断。
  macOS 真实 GPU 对照与控件合成/生命周期探针通过；Windows 真机 WGL 不支持时的
  CPU 回退、宿主 context/DC 恢复、交互与静默缓存通过。该测试机缺少所需 OpenGL，
  严格 GPU 探针失败，因此 Windows GPU 不能标为通过。见 [本轮报告](SCENE3D_GPU_QA_20261028.md)。
- 当前 GPU 仍同步读回到 Cairo；硬件实例化、视锥/BVH/LOD、大坐标精度、材质/纹理/抗锯齿、
  GPU 直接合成以及三平台完整 UI 验收仍未完成。不能用这个阶段代替完整高性能目标。

精简的按 context GL 绑定现已实现：58 个入口，避免场景后端依赖可变的进程全局 GL 表。
增加错误线程/context 拒绝、并发关闭与两个 context 各自 64 帧的验证；Windows 使用系统
DLL 加载策略。Linux Xvfb/Mesa 软件 GL 的实际渲染与控件合成已验证，不代替真实 GPU 或
物理桌面交互。完整回归与 Linux 分数缩放修复见 [本轮报告](GL_DISPATCH_QA_20261028.md)。

下一步继续有序合成、场景绑定编辑和完整材质导入；按 context 入口独立不等于整个 GUI 可以从
任意 goroutine 绘制，原生控件仍严格遵守 UI 线程契约。

## glTF 静态组装件（基础导入阶段记录）

现有异步控件链路接入 glTF/GLB：层级、共享网格、变换、基础色、稳定子节点 ID、根节点
点位绑定和显式 FitScene。外部 buffer 由 os.Root 限制到模型目录，失败/取消不替换当前场景。
这仍是几何预览：PBR/纹理/动画/蒙皮、GPU 实例化和有序合成未完成。
本阶段三平台主模块普通/race、地图模块 race 已通过；macOS/Windows 原生控件工作流已验证，
Windows 为 CPU 回退，Linux 为软件 GL，不代替 Windows GPU 或 Linux 物理桌面验收。
跳过项、源码校验、失败重测与限制见 [glTF 验证记录](GLTF_QA_20261028.md)。

## 场景文档与组态编辑链（阶段记录）

当前补齐 `.silkscene` 静态场景持久化、共享 mesh/节点 ID 回读、创作基线导出及编译式
位置/旋转/缩放/可见性/告警颜色绑定；SceneView 使用一条合并投递，分批让出 UI 调度。
新增 SDK 结构化节点/绑定编辑器，SilkIDE Tools 与命令面板复用文本缓冲、撤销和保存。
嵌套原生对话框立即隐藏、延迟释放，关闭子窗时把焦点交回仍在运行的父模态。
本轮验证及平台边界见 [场景文档 QA](SCENE_DOCUMENT_QA_20261028.md)。
后续仍需节点增删/gizmo、材质、GPU 直接合成、实例化及大场景性能验证；整体目标未完成。

## 设备模板与批量组装（初始阶段记录）

新增可复用的不可变 `Template`、原子批量放置、独立节点 ID 与点位映射，复用网格而不按设备
拷贝顶点数组；纯 Go `scene3dplant` 示例可把一个设备装配成可在现有 IDE 中打开的产线文档。
它是设备库/复制拖入的 SDK 基础，不是 GPU 硬件实例化，也没有把交互式设备库、节点增删、
多模型拖入、gizmo、材质或大场景渲染性能验收标为完成。新一轮验证与编辑器接入持续推进。


## 有序硬件实例化（实现与验证范围）

增加可选的 per-context instancing 入口、连续共享 mesh 批量 draw、保留 ID / 等深度顺序、
有界 staging、SDK 策略和 SceneView 能力查询。Apple 真实 GPU 与 Mesa 软件 GL 对照已
发现并修复每实例颜色插值精度差异；最终合并候选的全量回归、Windows 与真实桌面验收
统一记录在 [最终验收矩阵](SCENE_RELEASE_QA_20261028.md)，不以旧候选结果替代。
不因此将材质/纹理、直接 GPU 合成、设备库/gizmo 或完整工业 IDE 目标标成完成。

## 原子场景创作 SDK 与编辑器

- Added inert authoring sessions, resource-sharing node creation, subtree copy /
  delete, safe keep-world reparent and bounded undo/redo.
- Integrated owned scene-editor menus/forms, primitive creation, history controls,
  exact one-transaction host commit/save and small-host bounds regression coverage.
- Scope remains opaque static geometry and authored binding declarations, not
  animation, physics, a general CAD editor or completed Qt Creator parity.
- Safety contracts are documented in `SCENE_AUTHORING_QA_20261028.md`; the
  final frozen-source regression matrix is in `SCENE_RELEASE_QA_20261028.md`.

## 本次整合后的能力边界

设备模板/批量放置、保守视锥裁剪、有序实例化，以及节点新增/复制/删除/改父级和有界历史
已实现；上文按阶段保留的“未完成”描述是当时的历史状态，当前证据以
[最终验收矩阵](SCENE_RELEASE_QA_20261028.md) 为准。

仍未完成：交互式设备库拖入/gizmo、PBR/纹理/动画/蒙皮、三角级 BVH/LOD、GPU 直接合成、
Windows 硬件 GPU 和 Linux 物理桌面验收、全应用高负载性能指标、完整工业 IDE 与 Qt
Creator 功能对等。当前实例化不保证所有驱动更快，原生控件仍限定 UI 所属线程。

## 实例级空间 BVH 与快照准备复用

新增零值可用、单所有者的 `scene3d.Culler`。Morton 空间排序在内部聚合同位置邻居，
输出仍按原实例索引递增，保持等深度顺序、拾取 ID 和有序实例化。变换变化 refit
叶及祖先，颜色/ID 独立变化不重建；新快照仍需 O(N) 检查，数量变化重建排序。
层级重叠退化时有界回退到线性查询，不把不确定/溢出数值误裁掉。
CPU/GPU 后端共用查询器，GPU 相机独立变化复用快照矩阵/预算验证。
这是实例包围盒的视锥查询，不是三角 BVH、遮挡剔除、SAH、LOD 或 GPU 直接合成。
完整回归、真实 UI、合成微基准及未验证范围见 [BVH 验收](SCENE_BVH_QA_20261029.md)。

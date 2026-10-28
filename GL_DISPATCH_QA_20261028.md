# GL 入口隔离、跨平台布局与完整回归 — 2026-10-28

基线 `2727166`。本轮是 SDK/rendering 增量，不是整个工业 3D 引擎完成，也不创建正式版本 tag。

## 改动与契约

- 以私有、生成式的 **58 个 GL 入口**替代场景后端的大型全局 go-gl 表。每个 Renderer
  拥有独立的 C 表、库引用、线程/context 身份。失败构造释放表，正常关闭先回收 GL 对象。
- 新增 `ErrContext` 与 `CloseCurrent() error`；在缓存命中、绘制、删除对象之前检查所有权。
  错误线程在读取可变状态/已释放表之前被拒绝。`Close()` 保持兼容，不再删除别的 context
  中恰好同名的 GL 对象。独立 context 可并行；同一 Renderer 和 GUI 不因此变成任意线程安全。
- Windows 用系统目录的 `opengl32.dll`，WGL 无效入口 sentinel 被拒绝，嵌入 NUL 的入口名
  返回 0 而非 panic。macOS CGL、Linux GLX；没有 EGL、现代 GL core-only 或动态插件 ABI。
- 生成器是 ABI 的唯一声明源，Go/C/load-list 一起生成；C 固定宽度/调用约定检查，offset
  在 C 端转换，Go slice 只在同步调用期间借用。不增加后台绘制循环或可见独立子窗口。
- 全量 Linux 回归发现既有分数缩放文字不一致：FreeType 默认将 advance 舍入到设备像素，
  TextBlock 在逻辑尺度缓存的位置与 1.5× 即时排版不同。Linux 改为不舍入的逻辑度量，
  不关闭轮廓 hinting/AA、不修改 Windows 字体策略。新增五种缩放×两类字体×三种字号验证。
  机制依据 [Cairo font options](https://www.cairographics.org/manual/cairo-cairo-font-options-t.html#cairo-hint-metrics-t)。
- 工具栏测试此前假设 180px 必溢出；Linux 实际首组 preferred=179px。改为按实际 preferred
  两侧验证能放下/必须溢出，保留小宽度、字体/图标矩阵和逐控件边界断言；没有削弱溢出功能。
- `mod/map` 改为可独立导入的 `github.com/uk0/silk/mod/map`，新增外部包组装测试与使用说明。
  依赖版本逐项比较未变；CI 直接扫描真实模块，移除临时重命名 manifest 的旧规避路径。

## 验证范围

最终结果见下方测试矩阵；各平台正常测试均不使用 `-short`，并指定 `-count=1`。
主模块 `./...` 不包含嵌套模块，因此地图单独测试；工具模块用实际 bundled gopls 集成验证。
默认跳过的真实驱动测试另用 opt-in 开关执行，未将 skip 记成 pass。

| 最终源码检查 | 结果 |
| --- | --- |
| macOS 主模块，完整普通测试 | 8139 pass / 22 skip，90 有测试包通过，20 无测试包 |
| macOS 主模块，完整 race | 8139 pass / 22 skip，90 有测试包通过，20 无测试包 |
| Linux 主模块，完整普通测试 | 8077 pass / 31 skip，90 有测试包通过，20 无测试包 |
| Linux 主模块，完整 race | 8077 pass / 31 skip，90 有测试包通过，20 无测试包 |
| Windows 主模块，完整普通测试 | 7923 pass / 74 skip，91 有测试包通过，19 无测试包 |
| Windows 主模块，完整 race | 7923 pass / 74 skip，91 有测试包通过，19 无测试包 |
| 地图独立模块 race | macOS、Linux、Windows 全部通过 |
| macOS bundled gopls 三个真实集成测试 | 全部通过：Unicode 补全/格式化、未保存定义重命名、类型/实现跳转与返回 |
| Windows bundled gopls，原测试不修改，连续三轮 | 9/9 通过；首次独立运行存在一次 8 秒补全超时，见下文边界 |
| pure-Go paint/gui/ged/decl/scene3d、定点 vet、生成器字节稳定性 | 全部通过 |
| 平台/品牌打包脚本、版本对齐脚本 | 全部通过（负例的错误输出为预期断言） |

计数含子测试。skip 包含平台专属分支、需外部 debugger/font/native-window 环境的 opt-in
夹具和另行启用的 gopls/driver 用例。Linux 缺少覆盖全部 CJK/emoji 的测试字体，相关
3 个 fallback 像素用例跳过；分数缩放和新增逻辑度量回归实际执行且通过。

## 实际渲染与桌面闭环

### macOS / Apple M2 Max

- 实际驱动 `Apple / Apple M2 Max / 2.1 Metal - 91.7`。普通、race 与 `cgocheck2` 探针验证
  三种尺寸与 CPU 对照，稳定像素 17562/27950/73498，颜色容差 2、ID 完全一致，零不匹配。
- 同 context 缓存、错误 context Render/Close、owner 恢复、错误线程与并发 owner Close、
  关闭后错误线程调用，以及两个 OS 线程/context 各 64 帧通过。不是只做编译检查。
- native widget race 探针验证普通 painter 遮挡、共享资源 context、宿主 context 恢复、
  后端切换、拾取缓存和退出清理。
- 独立应用 `org.silk.qa.scene3d.gl21` 的 Computer Use：Snapshot 确认真实 OpenGL；点击 tank
  不增加 renders=1；Publish 1000 后 Y=.999；导入 OBJ 后 loaded=1/on_ui=true/error=""；
  点击 model 保持 renders=4，随后实际退出码 0、stderr 为空。
  最终二进制 SHA-256：`6b17b704530e0d74eaed8ef1faaf0e53d0717af02c072f1362c78476a767d53d`。
  这不是完整 IDE UI 验收，模型随点位平移后可能离开相机视野，自动跟踪尚未实现。
- 普通探针短对照：1000 个共享圆柱、256000 三角形、640×480、6 个变化帧，包含初次
  上传和颜色/ID 读回：CPU 154.682 ms，OpenGL 23.651 ms。只是一轮短样本，不能换算成
  稳定 FPS、整机 IDE 性能或 Windows 加速承诺。race/cgocheck2 计时不与此混用。

### Linux / 本地隔离容器

- Go 1.26.10 / Debian bookworm arm64 / Xvfb，Mesa 22.3.6 llvmpipe LLVM 15。
  这是 **软件 GL**，不是硬件 GPU 或物理桌面的输入/窗口管理验收。
- 实际 GL driver race 探针和 native SceneView race 合成/生命周期探针通过，包含上述
  128 帧并行 context、像素/ID 对照和宿主恢复。
- 首轮完整测试两项失败和定点原样复现日志保留。单变量关闭 Linux 设备度量舍入后文字
  测试通过；工具栏按实际测量修正测试假设后通过，再运行全量普通/race 回归。

### Windows / 用户指定真机

- 使用用户指定 win10testcomputer skill，通过交互式计划任务在 session 1 / 1600×1200
  执行；先验证最终源码 SHA-256、二进制路径、PID/窗口标题与点击归属，不在 SSH session 0
  假装 GUI 测试。源码目录 `C:\silk_gl_dispatch_20261028`。
- 严格 GLFW GPU 探针退出 1：驱动不支持所需 OpenGL；生产 WGL 后端明确报告
  `desktop OpenGL 2.1 required; driver reports 1.1.0`。**Windows GPU 未通过**。
- 原生 WGL 控件探针（普通构建，不是 native race）通过：CPU fallback、失败资源回收/不逐帧重试、宿主 DC/context
  恢复、painter 遮挡、拾取缓存、强制 GPU 报错、模式切换和关闭。
- 两个 WGL 系统 DLL 测试在 Windows `-race` 下通过；同名工程文件没有被加载，实际
  模块路径回读 `C:\Windows\SYSTEM32\opengl32.dll`，NUL 名称不再 panic。
- 原生按钮/鼠标：tank/pump 拾取保持 renders=2，1000 点位更新到 Y=.999，16 步拖拽改变
  yaw/pitch，单滚轮距离 6→5.429024508，Reset 恢复默认相机。导入 OBJ 后 loaded=1、
  node=model、Y=.999、busy=false、on_ui=true、error=""。
- 静置 15.028 s：renders=22 不变、Responding=true，进程 CPU 时间 +0.078125 s，约
  单核 0.520%。只代表该示例的短采样；同机存在构建任务，不是整个 IDE 或长期压力结论。
- 点击导入后的 model，renders=22 保持不变；保留原生进程句柄后点击真实关闭按钮，
  `GetExitCodeProcess=0`，stderr 为空，测试窗口已退出。
- bundled gopls 初次补全集成测试在并行构建期间触发既有 8 秒 deadline，并出现临时目录
  被占用的清理错误。**没有把这次失败抹掉或通过加大客户端超时变绿**。同一测试二进制、
  同一 gopls、原 8 秒 deadline 连续三轮重测 9/9 通过，补全用例耗时 4.31/2.17/1.78 s。
  冷启动/负载相关只是待验证推断，根因尚未完成隔离；此次未修改 LSP 客户端，不能宣称
  这个冷启动/清理问题已修复，需后续专项压力与进程树关闭验证。
- 最终示例 SHA-256：`0cb4810e7a62e51cbfaf1704d9ab14454f2dd881308c17aa9c5fb9cc6d48c125`。
  同一工具链/默认构建参数下，之前示例 29,996,258 bytes，本轮 27,214,785 bytes，
  减少 2,781,473 bytes（约 9.27%）；不是整个 IDE 安装包体积或运行内存的对比。

## 构建、扫描与证据边界

- 生产 Windows scene example 的依赖树不再包含 `github.com/go-gl/*`；Unix 的现有 2D
  presenter 仍使用原 go-gl，不能宣称全仓移除了它。没有新增第三方依赖或改变 go.sum。
- 首次精简入口包构建+100 次无 context 失败测试 21.816 s，示例构建 70.814 s；复用了
  已有 Go/Cairo 工具链缓存，不是完全冷机，也不把历史非精确计时写成速度提升百分比。
- `govulncheck@v1.9.0 -scan module` 与源代码扫描在 macOS 对实际地图模块通过（无发现）；这是
  该模块与当时漏洞数据库的扫描结果，不是整个平台永无 CVE 的保证。
- 首轮新增并发测试夹具曾共享并修改 camera，race 报告已保留；改为独立 workerCamera
  后重跑通过。一次 `-p1` 命令拼写导致测试参数拒绝，改用 `-p 1` 完整重跑，不作产品失败
  或通过证据。Windows 脚本执行策略/未加引号的点号参数错误也单独保留；修复调用后重测，
  未更改系统持久执行策略。
- 原工作区忽略目录 `bin/local` 不可遍历，完整 `./...` 在受控 linked worktree 中执行：
  当前 HEAD 加本轮变更，逐文件 SHA-256 校验，不修改/删除那个目录，不漏测 SDK 包。

本地完整证据：`bin/qa-artifacts/gl-dispatch-20261028/`。包括源文件 manifest、JSONL、
退出码、失败原始记录、native stdout/状态断言、Windows desktop task 与制品校验和。
这些是忽略产物；本报告保留可重放命令和关键结论，不把用户个人目录加入提交。

```sh
# 在可遍历的源码工作树；三平台均单独执行，而不是交叉编译冒充原生运行
go test -json -count=1 -timeout=30m -p 1 ./...
go test -json -race -count=1 -timeout=30m -p 1 ./...
(cd mod/map && go test -race -count=1 ./...)
SILK_TEST_SCENE_GPU=1 go test -race -count=1 -v ./scene3d/glrender
SILK_TEST_SCENE_GPU=1 go test -race -count=1 -v -run '^TestScene' ./gui
GOEXPERIMENT=cgocheck2 SILK_TEST_SCENE_GPU=1 go test -count=1 ./scene3d/glrender
go generate ./scene3d/glrender/internal/gl21
go test -tags silk_pure_go -count=1 -p 1 ./paint ./gui ./ged ./decl ./scene3d/...
go run ./cmd/silktools -out /path/to/stage/tools
bash scripts/test-bundled-gopls.sh /path/to/stage
# Linux native tests: prefix xvfb-run -a; explicitly identify llvmpipe as software.
# Windows native desktop: SILK_TEST_SCENE_WGL=1 + TestSceneNativeWindowsWGLCompositionAndFallback
```

未完成：Windows 真实硬件 GPU、Linux 物理桌面、EGL、零读回有序合成、硬件实例化/视锥/BVH、
glTF/PBR/纹理/抗锯齿、长期多视口压力与场景绑定编辑器。总体开发目标仍继续，不以本次提交
代替这些能力的验收。

# SDK UI 调度与线程边界迭代

基线：`0442eb2`。本轮不新增高频轮询，不把任意控件修改变成线程安全，也不将开发提交等同于发布认证。

## 功能与设计

- `IsUIThread`：公开后端真实线程身份查询。
- `Invoke` / `InvokeValue[T]`：工作线程同步等待短 UI 操作；UI 线程内联，支持嵌套调用、类型化结果、context 取消及 panic 错误反馈。执行队列复用既有公平分批的 `Post`，每次调用不新建 helper goroutine。
- `PostContext`：下一轮执行前检查取消，避免关闭页面后应用过期结果。
- `NewLatestUI[T]`：复用 `async.Latest` 合并可替换快照，最多一个待执行／执行中的投递；用于进度、预览，不用于必须逐条保留的事件。
- `Widget.TryAttachWindow`：错误线程、非法类型、无原生父级的 child 在读取／修改控件状态前或附加前拒绝；底层创建错误返回给宿主。已有 `AttachWindow` 保持签名。
- Win32 创建不再比较 `GetCurrentThread` 伪句柄；在读取 owner／焦点或调用原生创建前验证实际 UI 线程。
- 文本省略容忍 `1e-7` 逻辑像素的浮点往返误差，避免给恰好容纳的标签增加省略号；不是扩大可见布局范围，也不取消真实溢出时的裁剪。

提供 `examples/ui-dispatch`，演示同步／工作线程调用、取消、100,000 次进度发布、错误线程拒绝以及工作线程请求的原生模态对话框。API 说明见 `docs/SDK_FEATURES.md`。

## 回归与证据

冻结 1,791 个源码输入，本轮 9 个源码／测试文件发生变化。原始日志、平台产物身份、源码 overlay、截图与核验脚本保存在 `bin/qa-artifacts/sdk-dispatch-20261026/`。

- 红测：一个 ULP 的布局往返导致 `编辑器` 被误省略，修复后通过。
- 真实 Windows 截图发现示例的带图标按钮默认隐藏文字；新增可见性断言先复现，再显式启用文字，不改变工具栏已有的仅图标默认行为。示例布局测试同时检查按钮标题非空且可见。
- 单测覆盖：内联／嵌套调用、工作线程类型化返回、执行前／执行中取消、deadline、nil 参数、缺失后端、panic 错误链、过期 Post、100,000 次发布仅占一个队列槽、Close 后丢弃、附加失败不修改状态、无父级转换不销毁原窗口。
- Windows 原生测试覆盖：另一条锁定 OS 线程被拒绝、正常 UI 线程附加成功、child 的实际 HWND owner/thread 正确。原生测试 fixture 显式绑定自己的测试线程，不再依赖伪句柄绕过约束。
- macOS：9 包 `go test -race -short -count=1`，**6,288 pass / 49 skip**；专项 8 项各跑 3 次；pure-Go 路径、示例、构建、工具 bundle 校验和 `go vet -composites=false` 通过。
- Windows：同一 9 包 race 回归 **6,080 pass / 71 skip**，专项 10 项各跑 3 次；正常构建及内置工具校验通过。旧 Win32 线程检查的定向回放在工作线程进入窗口状态代码时失败，当前检查在该阶段之前返回 `ErrUIThread`。
- Windows 编辑器：独立 workspace、干净 PATH 和本轮二进制；F2 预填、关键字／空名称禁用确认、输入框撤销重做、跨两文件重命名、保存、逐文件撤销重做和取消均验证，共 **16 项精确字节回读**，最终两文件恢复基线。确认对话框的真实 owner、小／大图标像素一致且句柄独立。
- SDK 与 IDE 均正常关闭，远端最终回读无本轮应用或 gopls 残留进程。两者各采样约 20 秒空闲状态，保持 Responding；SDK 消耗 0.1875 CPU 秒，IDE 0.203125 CPU 秒（约 0.94%／1.01% 单核占用），不是整机负载、FPS 或输入延迟保证。
- SDK 原生窗口中 100,000 次发布最终送达 `99999`，本次合并为 9 次 UI 更新；取消后未执行过期回调。工作线程请求的模态窗口具有正确原生 owner，阻止 owner 输入，Esc 关闭后恢复焦点并返回结果。该采样不保证每次恰好 9 次投递。

### macOS 真实点击验收未完成

测试 app 已从本轮构建启动，启动快照确认主线程 `OnUI=true`。但 CUA 的 `getApp`、`getState` 以及重置连接后重试，均报 `Sky Computer Use native pipe startup failed`，无法取得窗口清单／截图或执行点击。**这不是 macOS 原生 UI 测试通过**，没有拿上一轮截图替代本轮证据。已仅终止本轮创建的测试进程，未改动系统权限或用户应用。

## 使用边界

- 工作线程调用 `Invoke` 应设置 deadline；停止的事件循环不会服务队列。
- 取消只能阻止尚未开始的回调；已开始的 UI 代码不能抢占，可能在调用者返回取消错误后结束。通过返回值传递结果，不要写调用者的局部输出变量；UI 不可等待正在等待 UI 的工作线程。
- `PostContext` 的闭包保留到队列执行该槽；`Invoke` 的取消会立即清掉尚未开始的回调引用，但队列槽仍待消费。
- 不允许回调使用 `runtime.Goexit`。同步 UI callback 应短小，HTTP／磁盘 IO／重计算用 `RunAsync`。
- `TryAttachWindow` 不为已有窗口被替换后的原生创建失败提供事务式回滚，也不让其余控件方法支持多线程修改。旧 `AttachWindow` 仍不向调用者返回错误。
- 自然标签宽度的数值缺陷已有回归，但 macOS 编辑器 dock 的完整视觉效果本轮因 CUA 故障未复验；不宣称所有截断问题已解决。
- 未验收 Linux 原生桌面、全 IME、高 DPI／跨显示器。空闲 CPU 采样不是输入延迟、整机负载或 FPS 保证。

## 复核

本地保留证据，可执行 `python3 bin/qa-artifacts/sdk-dispatch-20261026/verify.py` 回读冻结源、退出码、测试事件、原生动作断言和截图哈希；它不会自动重新点击 GUI。

两平台自动回归范围（使用各自原生依赖与构建环境）：

```sh
go test -race -short -count=1 -p 1 ./async ./examples/ui-dispatch ./paint ./graph ./decl ./ged ./gui ./prop ./cmd/silkide
```

Windows 实机截图：[SDK 调度示例](screenshots/sdk-dispatch-20261026-windows.png)。跳过项保留在原始 JSON 测试事件中；不将九包回归等同于整个仓库和所有桌面环境验收。

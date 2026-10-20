## SilkIDE 0.1.4 / Silk SDK 2.5.1

本轮重点是数据竞争、UI 调度与渲染可靠性。

> **已知限制：Windows 整窗的系统 DPI / 跨显示器缩放适配尚未完成，超过 100% 系统缩放时仍可能出现位图虚拟化模糊。** 本版修复图标设备分辨率、字体彩边和透明边缘合成，不宣称所有模糊场景已解决。

- 序列化子进程 stdout/stderr 进度交付，命令返回前等待心跳退出，避免旧进度覆盖完成状态。
- UI 队列采用 64 回调 / 4 ms 软预算；Windows 轮转处理输入、绘制、定时器及普通消息，静止界面仍阻塞等待事件。
- 在 GLFW 退出前安全注销并等待原生唤醒，避免后台任务访问已销毁运行时。
- Windows task-runner 按 PID 取消活跃进程树，并限制取消后的继承管道读取等待；不是 Job Object 隔离，已脱离进程树的后代不保证终止。
- 位图 / 自定义图标按实际设备变换选分辨率，保留小数逻辑尺寸；Windows 离屏文字使用灰度抗锯齿，避免 LCD 彩边。
- 修复 OpenGL 对 Cairo 预乘 alpha 再次相乘导致的暗边，新增最终 framebuffer 像素回读回归。
- 独立 purecairo 画布不再共享可变字体 face；修复颜色精度、路径状态、空文本、缩放描边、虚线、group 和 alpha mask。
- Windows 中文 / emoji / Ext-B 字体回退，以及原生文件对话框的 watchdog 误报修复。

### 下载与运行

提供 Windows amd64、Linux amd64、macOS arm64 / amd64 四种归档，SHA-256 见 `SHA256SUMS.txt`。

- Windows 包含 Cairo DLL；运行设计器不要求系统 PATH 安装 MSYS2，编译用户 Go 工程仍需 Go 工具链。
- macOS 包含 `SilkIDE.app` 和资源，但依赖外部 Homebrew Cairo，**不是免依赖或已签名公证的独立安装包**。
- Linux 需要 Cairo、X11/OpenGL 与 X11/XWayland 会话；具体依赖见 README 和包内说明。
- 不承诺早于各 CI runner 的 macOS / Linux glibc 兼容性。`gopls` / `dlv` 为可选外部工具。

桌面包使用原生 Cairo；可选 pure-Go 后端仍有裁剪、复杂文字 shaping 和部分合成算子限制。每个 UI 树及绘图 context 仍须单线程所有，后台变更使用 `gui.Post`。

仓库标签跟随 IDE 的 0.x 版本，保持现有 `github.com/uk0/silk` 导入路径；SDK 的 2.5.1 是框架显示版本，不是需要 `/v2` 的模块标签。

代码提交 `d6d18e3` 已通过[四平台 CI](https://github.com/uk0/silk/actions/runs/39428561489)。发布流程另外在 tag 源码上重新执行 native / race / pure-Go 门禁，先生成草稿，再校验四个归档和 Windows 制品本体 GUI，最后公开发布。托管 macOS 无 GL 能力时的显式 skip 不计作渲染通过；真实 GL 像素验收在本机 macOS 完成。

验收过程、跳过项与覆盖边界见 [race / 渲染 QA](https://github.com/uk0/silk/blob/v0.1.4/RACE_RENDER_QA_20261020.md)。

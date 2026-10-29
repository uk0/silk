# 有序 GPU 实例化 QA — 2026-10-28

> 阶段记录：以下保留本候选的实现、失败和测试边界；文中的待验收/未提交状态描述的是当时的候选。
> 最终整合版本使用独立的 2500 文件冻结清单重新验证，当前提交门禁与平台结果见
> [最终验收矩阵](SCENE_RELEASE_QA_20261028.md)。旧候选结果不替代最终版测试。

**隔离开发候选，尚未 commit / push。** 基于未发布的设备模板 + 视锥裁剪候选继续推进；
前两轮冻结源码没有被本次修改，Windows 连接阻塞不允许被旧的通过记录代替。

## 实现

- 58 个必需 + 2 个可选 per-context GL 入口；core / ARB / EXT 地址与驱动声明一起验证。
- `NewCurrentWithOptions` 的 Auto / Disabled / Required；缺少能力可保留原普通 GPU draw，
  Required 明确失败。真实硬件还是软件实现以运行证据区别，不以方法名字保证。
- 原顺序连续共享 mesh 批量 draw，1024 实例/批，原始节点 ID；不跨可见的不同 mesh 重排。
- 每实例 96 B，CPU / GPU staging 各最多 96 KiB；单实例路径不新建实例 buffer；缓存不重画。
- 颜色 / ID 用 flat varying，世界坐标仍平滑；窗口/context 归属、输入预验证、旧帧保留、
  网格预算、buffer 释放和关闭后的拒绝契约仍存在。
- `SceneView.GPUCapabilities()` 查询实际已建立后端；控件合成仍通过 Cairo 树，不创建
  可见的悬空 overlay。新增原生探针记录真正由 SceneView 发出的 1000 实例/1 draw。

## 已有证据与发现

- Apple M2 Max / OpenGL 2.1 Metal 实际支持 ARB instanced arrays / draw + EXT gpu shader4，
  不只是假定版本支持。严格 native race 通过。
- 90 个 GPU 完整 RGBA / ID 字节对照：0/1/2/1023/1024/1025/2051 实例、跨 mesh 等深度
  顺序、裁剪空洞与重新进入、80 个随机剪切/反射场景、1000 个圆柱。缓存、非法矩阵不覆盖
  旧帧、缓冲大小、Close 与原双 context/错误线程生命周期探针一起验证。
- 第一份 Linux llvmpipe 对照在随机场景 34 发现 **2 个颜色字节偏差 1 LSB**，ID 未偏移。
  原因是本应固定的实例颜色使用了平滑 varying；改 flat varying 后相同严格断言通过。
  原失败日志保留，不放宽比较阈值，不用 Apple 的通过覆盖 Linux 的失败。
- Linux llvmpipe 的 OpenGL 4.5 compatibility context 已额外编译 GLSL 1.30 core 变体，
  完成相同 90 组严格对照；Apple 2.1 则验证 EXT 变体，不混称相同路径。
- CPU 参考、SDK / GUI / 设计器定向 race、CGO=0 与 pure-Go 已运行；最终冻结后全量结果
  仍须另行收齐。最初 pure-Go 命令误写 `-p1` 导致测试命令失败，记录保留，正确 `-p 1`
  重跑通过；这不是源码修复，也不算第一次已通过。

## 基准边界

Apple 最新定向样本：1000 个共享圆柱（256000 三角形）、160×120、20 个改变相机的帧，
普通 draw **82.315 ms**，实例化 **36.962 ms**；draw 1000→1，三角数量不变，暖网格上传
均为 0，新增每帧实例上传 96000 B，RGBA / ID readback 均为 153600 B/帧。

Linux llvmpipe 是软件 GL：前一轮同夹具普通约 1.020 s、实例化约 1.155 s，**并未更快**。
这些是窄场景样本，不是全应用/全驱动提速保证，不消除三角光栅化或同步 readback，也不是
桌面端到端延迟或空闲 CPU 的验收。不能只引用减少 draw 的数字掩盖软件驱动成本。

## 未完成门禁

- 最终源码冻结后的主模块 full/race、子模块与原生回归、生成文件可重放校验。
- 本候选 Windows 部署哈希、构建/full/race、WGL 能力/回退与真实 UI；测试机 SSH 暂不可用。
- macOS 新候选真实 IDE 窗口交互；之前 CUA `cgWindowNotFound`，原生自动探针不替代它。
- Linux 当前是 Xvfb / Mesa 软件 GL，不是物理桌面或真实硬件 GPU 验收。
- 纹理/PBR、GPU 直接合成、设备库/gizmo 和完整工业组态 IDE 目标未完成。

本地日志/清单：`bin/qa-artifacts/scene-instancing-20261028/`。

# 2D 工业点位演示

```sh
go run ./examples/process2d
```

使用本地合成 HTTP 服务，不连接 PLC/现场设备；数据写入
`core.LocalDataDir()/process-points.db`。可通过 `SILK_DATA_DIR` 隔离。

按钮：正常 72.5、预警 92.8、报警 120.0、HTTP 503、非法数值、Snapshot。
一个 POST 返回 AI101 / DI11，三个数值卡片共享 AI101 的订阅；DI11 驱动三种设备符号。
数值按 ×0.1 转换后以绿/黄/红显示，异常变灰并显示 `—`，恢复后自动回到正常值。
Snapshot 在 stdout 输出 PROCESS_STATE（UI 线程、文本、颜色、质量和设备状态）。
不执行设备写控制，没有持续重绘定时器；HTTP 采集每秒一次，未变化样本只更新时间戳。
关闭时取消并等待采集，再关闭订阅、合成 HTTP 服务及 SQLite。

这是第一条可运行链路，不是图片所示完整工艺图编辑器。设备为基础矢量示意符号；
HTTP 配置仍在 SDK 代码中，ProcessValue 转换/颜色规则可在设计器属性中编辑 JSON。
完整契约与待办见 ../../point/README.md。

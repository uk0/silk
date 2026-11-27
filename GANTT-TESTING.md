# 甘特图回归测试

## 自动化

在安装了 Cairo/桌面依赖的本机，或 Linux Xvfb 环境运行：

```sh
GOMAXPROCS=2 go test -p 1 ./gui -run 'Test(Gantt|FocusedShortcut)' -count=1
GOMAXPROCS=2 go test -p 1 ./ged -run 'Test(Gantt|Workflow)' -count=1
GOMAXPROCS=2 go test -race -p 1 ./gui -run 'Test(Gantt|FocusedShortcut)' -count=1
```

- 模型：多层摘要、里程碑、闰日/DST、非法数据、依赖菱形与不同节点顺序、删除/升降级、原子 Undo/Redo。
- 交互：拖动父任务及后代、边缘缩放、进度、取消、只读/禁用、回调重入、无效坐标、快捷键与菜单状态。
- 绘图：明暗主题、1×/2× 像素颜色、行对齐、完整日期、微小控件裁剪、千行任务只绘制可见区域。
- SDK：真实 palette drop、嵌套容器、磁盘 `.silkui` 重载、两种代码生成方式在外部 Go module 内编译并执行，检查日期快照/属性/事件。

本轮回归还定位了基础绘图库的字号缓存问题：不同字号错误共用 scaled font，
CTM 哈希与比较也不一致。`paint/font_cache_test.go` 检查字号、样式、矩阵复制/变更，
对话框另用真实 14/20/28 字号与英中法标签验证布局。

`-short` 会跳过外部 SDK 编译，不能替代完整验收。

## 大项目拖动基准

```sh
GOMAXPROCS=2 go test -p 1 ./gui -run '^$' \
  -bench '^BenchmarkGanttDraggedSummaryAcrossSnappedDays$' \
  -benchmem -benchtime=500ms -count=3
```

测量拖动父任务跨日期时的排程验证与可见行更新，不包含原生事件和绘图。
样本为 1,000 / 5,000 个子任务，包含任务间依赖。拖动只在松手后生成一条撤销记录；
鼠标按下后若源计划已变更，旧预览不得覆盖它。

## 真实界面复现

生成隔离测试文件和渲染图片（不要指向需要保留的设计文件目录）：

```sh
SILK_GANTT_QA_DIR="$PWD/bin/qa-artifacts/gantt" \
  go test ./ged -run '^TestGanttDesignerWriteQAFixture$' -count=1
SILK_GANTT_QA_DIR="$PWD/bin/qa-artifacts/gantt" \
  go test ./gui -run '^TestGanttDrawThemeTokensRetinaAndAlignedRows$' -count=1
```

在设计器打开 `bin/qa-artifacts/gantt/workspace/gantt-qa.silkui`，点击预览：

1. 展开“设计阶段”，应出现子任务，表格与时间轴行对齐。
2. 把“跨平台发布项目”向右拖两天：全部后代平移两天；一次撤销整体恢复。
3. 将设计子任务右边缘延长一天：完成日期显示 `2028-03-01`，下游“实现与联调”推迟至 `2028-03-02` 开始，发布里程碑变为 `2028-03-06`。
4. 拖动任务底部进度条；右键编辑核对百分比。进度变化不应改变任务日期。
5. 编辑中文名称并保存；输入开始晚于完成的日期应提示错误且不关闭编辑器。
6. 测试升级/降级、折叠、菜单撤销/重做；关闭编辑器或菜单后应仍可直接操作甘特图。
7. 关闭预览后重新打开，原设计数据不应被临时预览编辑污染。

日期 SDK/JSON 的 `End` 为排他边界；表格与编辑器的 Finish 为包含当天。
默认设计画布可能小于甘特图的 180×95 mm 初始尺寸，应扩大表单后拖入，不应容许控件越界。

## 验证边界

2026-09-12 在 macOS 原生 Cairo/GLFW 预览中核验了父子拖动、依赖联动、进度、编辑与层级操作。
最终原生复验还确认：右键升级任务、编辑保存后，前台仍为预览窗口；无需额外点击即可立即撤销。
修改完成日期至 `2028-03-02` 后，下游开始日期更新至 `2028-03-03`，里程碑更新至 `2028-03-07`，一次撤销整体恢复。
合入品牌资源后的五包完整回归（`assets/branding`、`gui`、`ged`、`cmd/silkide`、`paint`）通过
（含子测试 3678 项通过，4 项平台或显式 opt-in 跳过），
最新焦点修复的甘特图/快捷键/临时窗口专项 `-race` 复测通过。
Windows 的 `silk_pure_go` 交叉编译只验证类型与构建约束，**不证明 Windows 原生 GUI、DPI、CPU 或输入延迟通过**。
真实打印、Windows/Linux 桌面交互和大项目长期性能仍需对应环境单独验收。

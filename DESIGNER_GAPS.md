# Go Designer：对照 Qt Creator / Qt Widgets Designer

本轮聚焦可视化控件设计器，不把 Go IDE 的所有面板等同于完整 Qt Creator。
比较基准为 Qt 官方的 [Widget Editing Mode](https://doc.qt.io/qt-6/designer-widget-mode.html)、[Using Layouts](https://doc.qt.io/qt-6/designer-layouts.html)、[Layout Management](https://doc.qt.io/qt-6/layout.html) 和 [Using Containers](https://doc.qt.io/qt-6/designer-using-containers.html)。

## 已实现的差距修补

| 范围 | 发现的问题 | 本轮实现 |
| --- | --- | --- |
| 控件投放 | 任意文本、表单外、尺规处也能进入投放路径；边缘控件裁切 | 校验控件类型、动作、表单和容器可用边界；自由容器显示真实尺寸预览并夹取位置；空间不足拒绝 |
| 投放反馈 | 目标和拒绝原因不明确，新增后属性仍是旧选区 | 蓝色＋／红色×与中文原因；新增即选中；一次撤销恢复原选区，重做重新选中 |
| 表单框选 | 表单根节点抢占内部空白拖动 | 根节点不参与平移手势；内部空白可框选，点击仍查看表单属性，尺寸手柄仍可缩放 |
| macOS 编辑键 | 设计器只认 Ctrl，Cmd+A/C/V/D 不生效 | 按平台接受左右 Cmd；Windows/Linux 仍用 Ctrl；普通字母不误触命令 |
| 自动布局 | H/V 可分组，Grid/Form 只是空容器：未登记 cells/rows | HBox、VBox、GridLayout、FormLayout 四种真实布局；画布／预览／声明式和命令式 Go 生成一致 |
| 布局编辑 | 关键操作隐藏，网格布局数据不完整 | 设计模式专属操作栏与命令面板入口；spacing、四边距，Grid 子控件行列、跨行跨列属性；保存与重载保留 |
| 子控件悬空 | 左上缩放未携带子树；嵌套自动布局遗留孙子；预览把共享坐标再次叠加父偏移 | 缩放命令携带子孙并统一撤销；父边界转换为局部坐标；自由布局明确禁用运行时重排 |
| 预览窗口 | 加入 Dock 后才读取原表单尺寸，尺寸已被覆盖为零，退回320×240并裁掉控件 | 提前保存设计尺寸，补足菜单和 Dock 占用，窗口完整容纳设计内容 |
| 属性输入 | macOS Cmd+A 在数值字段无效，10修改为25会追加成1025 | 基础 Edit 支持平台动作键 A/C/V/X，防止数值追加把控件送出画布 |
| 布局交互 | 布局子控件自由拖动后又被运行时排回，画布和预览不一致 | 自动布局内禁止自由移动／缩放，明确提示先解除布局；容器整体仍可移动／缩放；拖入自动布局明确为追加 |
| 布局历史 | 应用／解除／撤销后选区易停留在脱离场景的对象 | 布局与解除各为可撤销操作；撤销、重做同步容器与子控件选区 |
| 单容器布局 | 只选中已有自由布局容器时，必须重新选中全部子控件才能启用排列 | 选中 HBox/VBox/Grid/Form 自由容器后，点击同类型布局直接原位启用；保留容器身份、边界与层级，支持一个子控件和整棵子树几何撤销 |
| 图标 | 13 个静态提示／面板资源缺少内置实现 | 补齐矢量图标；新增六个布局图标，修正 maximize；静态引用与动态布局描述符均有覆盖测试 |
| 色彩 | 深色主题禁用按钮使用硬编码浅灰背景 | 使用当前主题背景／边框；精确 8-bit sRGB RGBA，语义图标颜色与禁用态一致 |
| 工具栏 | 隐藏文本仍占宽；SizeHints 低估文字按钮高度；溢出菜单空名称 | 共用尺寸计算；设计第二栏独立预留空间，不覆盖 Dock；溢出名称、启用／选中状态与原动作同步 |
| 文档状态 | 恢复后文件身份变为 `.autosave`；旧文件监视事件覆盖当前文档 | 原件身份与恢复来源分离、恢复保持 dirty；只重载当前干净文档；自身保存事件不重建控件、不清有效历史 |
| 命令一致性 | 命令面板重复 Undo/Redo，其中一组绕过可见文档分派 | 删除重复命令；注册幂等，设计动作有当前模式／合法选区保护 |
| 整数属性 | `prop.control.IntEdit` 未注册，退回普通文本；缺少整数范围与步进约束 | 注册精确有符号/无符号编辑器；非法中间输入不提交，Up/Down 有界步进；保留 uint64 精度、混合选区与 Undo |
| 保存与构建 | 生成 companion 固定 `package ui`，与同目录 `package main` 冲突 | 跟随相邻非测试 Go 文件的 package，忽略旧 companion；冲突或解析失败报告错误，无相邻代码仍默认 ui；尚不解析 build tags |

### 布局使用

1. 从控件箱拖入控件。自由摆放时蓝框是最终投放尺寸；拒绝位置显示红色说明。
2. 使用箭头工具在表单空白处框选；macOS 可用 Cmd+A 全选，其他平台用 Ctrl+A。
3. 同一容器内至少两个未锁定控件时，可使用“水平／垂直／网格／表单布局”。也可只选一个已有 HBox/VBox/Grid/Form 自由容器，点击**同类型**布局原位启用。容器及子孙必须未锁定；重复启用不增加历史。原位 H/V 保留文档子控件顺序，Grid/Form 按视觉顺序登记单元格，表单布局左右配对。
4. 选中布局容器设置边距、间距；选 Grid 子控件设置行列与跨度。
5. 自动布局内拖入控件是**追加**，不是插入任意槽位。需要自由摆放时选择布局容器并“解除布局”。
6. 自由容器通过属性修改 X/Y，或拖动左／上尺寸手柄时，子孙控件保持相对位置；预览和生成文件使用父局部坐标。
7. Cmd+Shift+R 预览；Cmd+S 保存设计及相邻 Go 文件。工具栏“适应视口”是缩放画布，不是 Qt 的 Adjust Size。

布局坐标在设计文件中为毫米，运行时按固定 96 DPI 逻辑像素转换。旧文件／旧 palette 容器的自由摆放不会被无声改成自动布局；显式应用布局才开启 `layout_managed`。四类 GUI 布局默认启用自动排列，手写 Go/decl 不变；旧设计中的自由容器导出 `layout_enabled=false`，后续 resize／Layout 也不会偷偷重排。

## 仍有差距，未宣称完成

- 顶层表单、普通 Card/GroupBox 直接应用布局，以及单容器切换为另一种布局类型仍未实现；已有同类型自由布局容器的原位启用已支持。
- 任意插槽插入指示线、布局内拖动重排；当前是带说明的追加。
- 通用 sizePolicy、min/max、stretch 可视化编辑；Splitter 应用／解除布局；Form 长行换行。
- 画布已有控件跨容器拖动的 move/reparent 仍是两个撤销步骤，本轮只统一了 palette 添加和布局操作。
- 提示文字、控件库和属性本地化尚未完全统一；复杂属性与所有控件组合未穷尽验收。
- gopls、Delve 的真实服务、跨平台字体／输入、signals/slots 级事件编辑器与自定义控件插件不是本轮完成项。
- Windows 已补齐 IME 光标定位和 UTF-16 字符输入；实际输入/保存闭环见 [输入 QA](WINDOWS_INPUT_QA_20261020.md)。后续已补齐原生文本 API 的中文/emoji/Ext-B 字体回退及测绘一致性，见 [文本 QA](WINDOWS_TEXT_QA_20261020.md)。跨焦点组合提交、复杂文字 shaping、低级富文本回退、TSF/其它输入法与混合 DPI 仍需专项处理；可选纯 Go paint 后端的既有测试失败单独保留。
- 色值精确不等于显示器校准、HDR 或 16-bit 色深。

## 验证层次

自动回归覆盖四类布局的画布、Undo/Redo、保存重载、两条声明式构造路径；Grid/Form 的生成 Go 程序还实际编译运行并验证 resize 后的控件几何。不是只做语法分析或编译。

桌面测试使用 `bin/qa-workspace` 与 `bin/qa-home`，原件、恢复副本和测试基线分别保留在 `bin/qa-artifacts/details-baseline`。屏幕验证与限制见 [WORKBENCH_QA.md](WORKBENCH_QA.md)。

主要回归：

```sh
go test ./cmd/silkide ./ged ./graph ./gui ./decl ./paint -count=1
go vet -composites=false ./cmd/silkide ./ged ./graph ./gui ./decl ./paint
git diff --check
```

精确图标图集：`bin/qa-artifacts/designer-panel-icons-2x.png`。

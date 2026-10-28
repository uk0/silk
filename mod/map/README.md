# 地图画布 SDK 子模块（实验性）

Go 模块与导入路径：`github.com/uk0/silk/mod/map`，包名为 `wmap`。
旧的 `module mod/map` 会被 Go 拒绝，不能作为可用导入路径；本次改为合法的完整模块身份。
源码目录仍是 `mod/map`，没有改变视图/场景类型名称。

```go
import wmap "github.com/uk0/silk/mod/map"

view := wmap.NewMapView()
view.SetParent(form)
```

这是基于 `graph.GraphView` / `MapScene` 的组装骨架，不是已经完成的地图引擎；
目前不提供地图瓦片、坐标投影或在线地图服务，也不是 Go 动态插件 ABI。

在当前仓库中独立测试：

```sh
cd mod/map
go test -race -count=1 ./...
```

子模块尚未单独发布版本标签。本地外部项目可以使用 Go workspace，或同时给根 SDK 与
本子模块配置 `replace` 指向对应检出目录。不要把文件列表测试当作模块可导入验证。

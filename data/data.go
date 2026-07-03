package data

import (
//"github.com/uk0/silk/core"
)

// 抽象的数据接口
// 此接口用来表示较大的数据, 不适合表示"曲线上的点"之类的琐碎数据
type IData interface {

	// 数据对象的标识
	// 在同一个父对象下, 对象标识应该唯一
	DataID() string

	// 显示的名称, 可随语种/用户习惯变化, 例如井在某些应用中应显示为"井筒"
	// 此名称的变化不应影响到软件的实际功能.
	DataName() string
}

type IWellID interface {
	// 井的标识，井号
	// 我们的软件以这个标识区分不同井, ID相同则井相同, ID不同则井不同
	// 如果数据不属于任何井(例如地震测网)则此标识为空
	WellID() string
}

type IFieldID interface {
	// Field identifier
	// Current version supports single-level fields only
	// If empty, the item will be categorized as "unspecified"
	FieldID() string
}

type ICategoryID interface {
	// 数据类型
	CategoryID() string
}

type IDataVersion interface {
	// 数据版本, 用一个字符串以区分同名数据的不同版本
	// 例如"张三4月修改", "来自于数模等"
	DataVersion() string
}

// 井(筒)数据
type IWellBasic interface {
	IData

	// 井口坐标, x为东西方向, y为南北方向
	WellLoc() (x, y float64)

	// 井型, 正钻井, 废弃井, 生产井
	WellStage() string

	// 井别, 油,气, 水...
	WellType() string

	// 是否直井
	IsVertWell() bool
}

// 测井数据
type IWellLog interface {
	IData
	// 数据名称, DEPTH, GR...
	WellLogName()

	// 起始深度
	Top() float32

	// 终止深度
	Bottom() float32

	// 数据
	WellLogData() FBlock32
}

// 层位数据
type IZone interface {
	IData
	// 起始深度
	Top() float32

	// 终止深度
	Bottom() float32
}

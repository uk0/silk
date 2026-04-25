package data

import (
	"silk/core"
	"io/ioutil"
	"runtime"
)

var dataMan IDataMan

// 因为数据管理器只能设置一次, 所以容易冲突, 我们记录下是谁设置的, 以便调试
var dataManInitFile string
var dataManInitLine int
var dataManIsDefault bool

// 数据管理器
// 数据管理器决定了软件可加载那些数据, 以什么方式加载, 又以什么方式呈现给用户
// 数据管理器的职责:
// 1 确保内存中的数据对象和数据库中的数据同步
// 2 决定数据对象之间的关系
// 3 自动管理对象占用的内存大小
// 4 对象的增删改查操作
type IDataMan interface {
	// 数据的根节点
	// 注: 如果有需要, 根节点可以不是项目
	Roots() []IData

	Children(d IData) []IData
}

// 设置数据管理器, 只能设置一次, 并且必须在使用任何数据之前设置
func SetDataMan(man IDataMan) {
	if dataMan != nil && man != dataMan {
		core.Error(`anthor data manager have been specified before this call, at "`+
			dataManInitFile+`" (`, dataManInitLine, `)`)
		panic("can not change data manager")
	}
	_, dataManInitFile, dataManInitLine, _ = runtime.Caller(1)
	dataMan = man
}

// 获取数据管理器
// 注: 如果未指定管理器, 则系统将使用默认的DefaultDataMan, 且不能再次更改
func DataMan() IDataMan {
	if dataMan == nil {
		// 使用默认的管理器
		dataManIsDefault = true
		_, dataManInitFile, dataManInitLine, _ = runtime.Caller(1)
		dataMan = new(DefaultDataMan)
	}
	return dataMan
}

// 默认的数据管理器, 可扩展
type DefaultDataMan struct {
	// 项目作为根节点
	projects []*Project
}

func (man *DefaultDataMan) Children(d IData) []IData {
	return nil
}

func (man *DefaultDataMan) Roots() (ret []IData) {
	man.RefreshProjects()
	for _, v := range man.projects {
		ret = append(ret, v)
	}
	return
}

// 列出所有项目
func (man *DefaultDataMan) Projects() (list []*Project) {
	for _, p := range man.projects {
		list = append(list, p)
	}
	return
}

func (man *DefaultDataMan) ProjectNames() (list []string) {
	for _, p := range man.projects {
		list = append(list, p.ProjectName())
	}
	return
}

// 刷新项目列表
func (man *DefaultDataMan) RefreshProjects() error {
	entries, err := ioutil.ReadDir(core.WorkspaceDir())
	if err != nil {
		return err
	}
	//sort.Sort(sortProjects()
	oldList := man.projects
	man.projects = nil
	var removedList []*Project
	var addedList []*Project
	i := 0
	for _, info := range entries {
		if !info.IsDir() {
			continue
		}
		if i >= len(oldList) || info.Name() < oldList[i].prjname {
			p := &Project{prjname: info.Name()}
			man.projects = append(man.projects, p)
			addedList = append(addedList, p)
		} else if info.Name() == oldList[i].prjname {
			man.projects = append(man.projects, oldList[i])
			i++
		} else {
			removedList = append(removedList, oldList[i])
			i++
		}
	}
	core.Trace(man.ProjectNames())
	return nil
}

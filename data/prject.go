package data

import (
	//	"database/sql"
	"github.com/uk0/silk/core"
	//	"github.com/uk0/silk/gui"
	//	"github.com/uk0/silk/gui"
	"errors"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"path"
	//	"sort"
)

/*
type sortProjects []*Project

func (f sortProjects) Len() int {
	return len(f)
}
func (f sortProjects) Less(i, j int) bool {
	return f[i].ProjectName() < f[j].ProjectName()
}
func (f sortProjects) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}
*/

type Project struct {
	prjname string

	db *DB

	ini core.IIni

	loadFailed bool
}

func (this *Project) DataID() string {
	return this.prjname
}

func (this *Project) DataName() string {
	return this.prjname
}

func (this *Project) DirtyList() []string {
	return nil
}

func (this *Project) Save() error {
	return nil
}

//
func (this *Project) Close() {
	if this.db != nil {
		this.db.Close()
		this.db = nil
	}
}

// 项目名称, 等同于目录名
func (this *Project) ProjectName() string {
	return this.prjname
}

// 项目是否已加载
// 未加载的项目只知道名称, 已加载的项目才能读取数据
func (this *Project) IsLoaded() bool {
	return this.db != nil
}

// 项目是否加载失败
func (this *Project) IsLoadFailed() bool {
	return this.loadFailed
}

// 项目的完整路径
func (this *Project) ProjectFullPath() string {
	return ProjectFullPath(this.prjname)
}

// 项目的完整路径
func ProjectFullPath(prjname string) string {
	dir := core.WorkspaceDir() + "/" + prjname
	return path.Clean(dir)
}

// 打开项目
func (this *Project) Load() error {
	if this.IsLoaded() {
		return nil
	}

	dbtype := this.ProjectType()
	if dbtype != "local" {
		return errors.New(`unsupported project type "` + dbtype + `"`)
	}

	var err error
	this.db, err = OpenDB("sqlite3", this.dbFullPath())
	return err
}

// 项目类型, 即后台数据类型
func (this *Project) ProjectType() string {
	var s string
	this.Ini().ReadAttr("type", &s)
	return s
}

// 数据库的位置
func (this *Project) DBLocation() string {
	var s string
	this.Ini().ReadAttr("data", &s)
	return s
}

func (this *Project) dbFullPath() string {
	return this.ProjectFullPath() + "/" + this.DBLocation()
}

func (this *Project) Ini() core.IIni {
	if this.ini == nil {
		this.ini = core.LoadTDocIni(this.ProjectFullPath()+"/project.cml", false)
	}
	return this.ini
}

//// 检测项目是否已经打开
//func IsProjectOpened(prjname string) bool {
//	//dir = path.Clean(dir)
//	dir := ProjectFullPath(prjname)
//	prjMutex.Lock()
//	defer prjMutex.Unlock()
//	_, ok := prjMap[dir]
//	return ok
//}

/*
// 删除项目
func DeleteProject(prjname string) error {

	//dir = path.Clean(dir)
	dir := ProjectFullPath(prjname)

	prjMutex.Lock()
	defer prjMutex.Unlock()

	_, ok := prjMap[dir]
	if ok {
		return errors.New(`project is in use: "` + dir + `"`)
	}

	if !IsProjectDir(dir) {
		return errors.New(`not a project: "` + dir + `"`)
	}

	err := os.RemoveAll(dir)
	if err != nil {
		return err
	}
	return nil
}

*/

// 创建项目
func CreateProject(prjname string) (err error) {

	dir := ProjectFullPath(prjname)

	// 如果文件已经存在则不能创建
	_, err = os.Stat(dir)
	if err == nil {
		core.Trace("project already exist: ", dir)
		return
	}

	// 创建目录
	err = os.MkdirAll(dir, 0770)
	if err != nil {
		core.Trace("failed to create dir: ", dir)
		core.Trace(err.Error())
		return
	}

	// 创建失败时清空目录
	defer func() {
		if err != nil {
			core.Trace(err.Error())
			core.Trace("create failed, clean up dir: ", dir)
			os.RemoveAll(dir)
		} else {
			core.Trace("create project succeeded: ", dir)
		}
	}()

	// 创建数据库
	dbPath := dir + "/data.dcp"
	db, err := OpenDB("sqlite3", dbPath)
	if err != nil {
		core.Trace("failed to create database: ", dbPath)
		return
	}
	defer db.Close()

	//	创建数据表
	err = db.BatchExec(create_db_sql, true, false)

	if err != nil {
		core.Trace("failed to initialze database")
		return
	}

	// 创建项目文件
	doc := core.NewTDoc()
	doc.WriteAttr("data", "data.dcp")
	doc.WriteAttr("type", "local")

	err = doc.SaveFile(dir + "/project.cml")
	if err != nil {
		core.Trace("failed to create ", dir+"/project.cml")
		return
	}

	e1 := core.SetDirIcon(dir, core.ResourceDir()+"/win32/project.ico", "Silk Project")
	if e1 != nil {
		core.Trace("failed to set folder icon: ", e1.Error())
	}

	//RefreshProjects()

	return
}

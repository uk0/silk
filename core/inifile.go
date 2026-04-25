package core

import (
	"errors"
	"os"
	"time"
)

type AttrWriter interface {
	// 写属性
	// key的要求同C语言变量命名, 可以有多级, 多级之间用'/'分隔
	WriteAttr(key string, data interface{}) error
}

type AttrReader interface {
	// 读属性
	// key的要求同C语言变量命名, 可以有多级, 多级之间用'/'分隔
	// ptr必须为非空指针, 并指向兼容的变量类型
	// 属性将被读到ptr指向的变量中
	ReadAttr(key string, ptr interface{}) error
}

// Ini接口
// Ini用来保存系列属性值, 平时驻留内存以保证读写效率
// 适当时候可调用Sync来和后台文件同步
type IIni interface {
	AttrWriter
	AttrReader

	// 同步, 较费时, 建议调用频率>1分钟
	Sync()

	// 上次同步时间
	SyncTime() time.Time

	// 上次修改时间
	ModifiedTime() time.Time
}

var userIni *TDocIni

var errNoSuchKey = errors.New("no such key")
var errIniReadOnly = errors.New("set value to read only ini")

// TDoc格式的ini文件
// 此为默认的ini文件格式
type TDocIni struct {
	doc      *TDoc
	fname    string
	stime    time.Time
	mtime    time.Time
	readonly bool
}

// 加载TDoc格式的ini文件
func LoadTDocIni(fname string, readonly bool) *TDocIni {
	ini := new(TDocIni)
	ini.fname = fname
	ini.readonly = readonly
	return ini
}

func (this *TDocIni) load() {
	if this.doc != nil {
		return
	}
	var err error
	this.doc, err = LoadTDocFile(this.fname)
	if err != nil {
		Trace(`failed to load ini file: "` + this.fname + `"`)
		this.doc = NewTDoc()
		this.doc.SaveFile(this.fname)
	} else {
		Trace(`ini file loaded: "` + this.fname + `"`)
	}
}

// 写属性
func (this *TDocIni) WriteAttr(path string, data interface{}) error {
	if this.readonly {
		return errIniReadOnly
	}
	if this.doc == nil {
		this.load()
	}

	err := this.doc.WriteAttr(path, data)
	if err == nil {
		this.mtime = time.Now()
	}
	return err
}

// 读属性
func (this *TDocIni) ReadAttr(path string, ptr interface{}) error {
	if this.doc == nil {
		this.load()
	}
	return this.doc.ReadAttr(path, ptr)
}

func (this *TDocIni) merge(src *TDoc) {
	if this.doc == nil {
		this.doc = src.Clone()
		return
	}
	mergeTDoc(this.doc, src)
}

// mergeTDoc merges src into dst using "defaults" strategy:
// only keys from src that are missing in dst get added.
// Local values always win; src fills in the gaps.
func mergeTDoc(dst, src *TDoc) {
	for _, srcChild := range src.Childdren() {
		k := srcChild.Key()
		if k == "" {
			// unnamed nodes: append clone
			dst.AddChild(srcChild)
			continue
		}
		dstChild := dst.ChildByKey(k, false)
		if dstChild == nil {
			// key missing locally, adopt from src
			dst.AddChild(srcChild)
		} else {
			// key exists locally, recurse to fill sub-keys
			mergeTDoc(dstChild, srcChild)
		}
	}
}

func (this *TDocIni) Sync() {
	now := time.Now()
	if this.doc == nil {
		this.load()
	} else {
		fi, err := os.Stat(this.fname)
		if err == nil && fi.ModTime().After(this.stime) {
			doc, err := LoadTDocFile(this.fname)
			if err == nil {
				this.merge(doc)
			} else {
				Warn(err)
			}
		}
	}
	if !this.readonly && this.stime.Before(this.mtime) {
		Trace(`save ini file: "` + this.fname + `"`)
		err := this.doc.SaveFile(this.fname)
		if err != nil {
			Warn(err, ObjInfo(err))
		}
	}
	this.stime = now
}

func (this *TDocIni) SyncTime() time.Time {
	return this.stime
}

func (this *TDocIni) ModifiedTime() time.Time {
	return this.mtime
}

// 用户的默认Ini文件
func UserIni() *TDocIni {
	if userIni == nil {
		fname := LocalDataDir() + "/ini.cml"
		_, err := os.Stat(fname)
		if err == nil {
			userIni = LoadTDocIni(fname, false)
		} else {
			userIni = LoadTDocIni(ResourceDir()+"/default/ini.cml", false)
			userIni.load()
			userIni.mtime = time.Now()
			userIni.fname = fname
		}
	}
	return userIni
}

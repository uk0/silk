package core

import (
	//	"errors"
	"fmt"
	"log"
	//	"os"
	"path"
	"reflect"
	"runtime"
	"strings"
	"sync"
)

var isDebugOn bool

// LogLevel 标识一条Log的级别, 供日志订阅者(sink)区分处理
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

// LogSink 是日志订阅回调, 每产生一条Log都会以对应级别和最终文本调用一次
type LogSink func(level LogLevel, message string)

var (
	logSinksLock sync.RWMutex
	logSinks     = make(map[int]LogSink)
	logSinkNext  int
)

// RegisterLogSink 注册一个日志订阅者, 返回的函数用于注销该订阅者
// 支持注册多个sink; 上层GUI(如LogPanel)可借此订阅日志而无需被core反向依赖
func RegisterLogSink(sink LogSink) (unregister func()) {
	logSinksLock.Lock()
	id := logSinkNext
	logSinkNext++
	logSinks[id] = sink
	logSinksLock.Unlock()

	return func() {
		logSinksLock.Lock()
		delete(logSinks, id)
		logSinksLock.Unlock()
	}
}

// dispatchLog 将一条Log派发给所有已注册的sink
// 先在读锁内复制sink列表, 释放锁后再调用, 以避免sink内部再次写日志时的重入死锁;
// 每个sink调用都用recover包裹, 单个sink的panic不会影响其它sink或日志本身
func dispatchLog(level LogLevel, message string) {
	logSinksLock.RLock()
	if len(logSinks) == 0 {
		logSinksLock.RUnlock()
		return
	}
	sinks := make([]LogSink, 0, len(logSinks))
	for _, s := range logSinks {
		sinks = append(sinks, s)
	}
	logSinksLock.RUnlock()

	for _, sink := range sinks {
		func() {
			defer func() { recover() }()
			sink(level, message)
		}()
	}
}

var logCategoryCache = make(map[string]string, 0)

var liveObjs = make(map[string]int)
var liveObjsLock sync.Mutex

func onObjFinalized(ps *string) {
	//	Trace("object finalized: ", *ps)
	if isDebugOn {
		log.Println("obj finalized:", *ps)
	}
	liveObjsLock.Lock()
	delete(liveObjs, *ps)
	liveObjsLock.Unlock()
}

// 跟踪对象的生存周期
// 此函数传入对象指针, 返回一个跟踪用的字符串对象
// 使用方法: 把返回的字符串挂回到对象上, 使得字符串的生存期和对象相同
func LiveCycleTrace(ptr interface{}) (ps *string) {
	ps = new(string)
	*ps = ObjInfo(ptr)
	runtime.SetFinalizer(ps, onObjFinalized)
	if isDebugOn {
		log.Println("obj created:", *ps)
	}
	liveObjsLock.Lock()
	liveObjs[*ps] = 1
	liveObjsLock.Unlock()
	return
}

func LiveObjects() (ret []string) {
	liveObjsLock.Lock()
	for s, _ := range liveObjs {
		ret = append(ret, s)
	}
	liveObjsLock.Unlock()
	return
}

func shortPath(s string) string {
	pos := strings.LastIndex(s, `/src/`)
	if pos == -1 {
		pos = strings.LastIndex(s, `\src\`)
	}
	var short string
	if pos == -1 {
		short = path.Base(s)
	} else {
		short = s[pos+5:]
	}
	return short
}

func categoryOf(file string) string {
	s, ok := logCategoryCache[file]
	if ok {
		return s
	}

	pos := strings.Index(file, `/src/`)
	if pos == -1 {
		pos = strings.Index(file, `\src\`)
	}
	if pos == -1 {
		s = path.Base(file)
		logCategoryCache[file] = s
		return s
	}

	s = file[pos+5:]
	pos = strings.LastIndex(s, `/`)
	if pos == -1 {
		pos = strings.LastIndex(s, `\`)
	}
	if pos == -1 {
		s = path.Base(file)
	} else {
		s = s[:pos]
	}
	logCategoryCache[file] = s
	return s
}

/*
func report(level int, a ...interface{}) {
	//	logMutex.Lock()
	//	defer logMutex.Unlock()
	if level > logLevel {
		return
	}

	var category string
	if level <= logStackTraceLevel && logStackTraceDepth > 0 {
		var fns []*runtime.Func
		var pcs []uintptr
		var maxFuncLen = 1
		for n := logStackTraceDepth - 1; n >= 0; n-- {
			pc, file, _, ok := runtime.Caller(n + 2)
			if ok && n <= logStackTraceDepth {

				fn := runtime.FuncForPC(pc)
				fns = append(fns, fn)
				pcs = append(pcs, pc)
				funcLen := len(fn.Name())
				if funcLen > maxFuncLen {
					maxFuncLen = funcLen
				}

				if n == 0 {
					category = categoryOf(file)
				}
			}
		}

		spaceString := "                                                    "
		for n := 0; n < len(fns); n++ {
			fn := fns[n]
			name := fn.Name()
			file, line := fn.FileLine(pcs[n])
			funcLen := len(name)
			padding := maxFuncLen - funcLen + 1
			if padding >= len(spaceString) {
				padding = len(spaceString)
			}
			defer log.Printf("%2d %s()%s %s:%d", len(fns)-n-1, name, spaceString[0:padding], shortPath(file), line)
		}

	} else {
		_, file, _, ok := runtime.Caller(2)
		if ok {
			category = categoryOf(file)
		}
	}

	var s string
	if level < 4 {
		ls := logLevelText[level]
		s = `[` + category + `] ` + ls + ` ` + fmt.Sprint(a...)
	} else {
		s = `[` + category + `] ` + fmt.Sprint(a...)

	}
	log.Println(s)
	if level == 0 {
		panic(s)
	}
}

*/
// 生成调试用的对象信息
func ObjInfo(ptr interface{}) string {
	v := reflect.ValueOf(ptr)
	if v.Type().Kind() == reflect.Ptr {
		ts := v.Type().Elem().String()
		if v.IsNil() {
			return "(" + ts + " nil)"
		}
		return "(" + ts + fmt.Sprintf(" @%X)", v.Pointer())
	}
	if v.CanAddr() {
		ts := v.Type().String()
		addr := v.Addr()
		return "(" + ts + fmt.Sprintf(" @%X)", addr)
	}
	return fmt.Sprint("(", ptr, ")")
}

// 生成调试用的类型信息
func TypeInfo(ptr interface{}) string {
	t := reflect.TypeOf(ptr)
	if t.Kind() == reflect.Ptr {
		return t.Elem().String()
	}
	return t.String()
}

// 生成"类型错误"
// 主要用来跟踪调试内部类型错误, 例如序列化不支持的类型, 对象不支持所需接口等
// 生成的错误中包含了对象的简要信息, 便于调试
func TypeErr(a interface{}) error {
	y := ObjInfo(a)
	return StrErr(fmt.Sprint("type error: ", y))
}

// 获取调试开关状态
// 底层和应用层可根据本函数的返回值来决定是否显示调试信息
func IsDebugOn() bool {
	return isDebugOn
}

// 输出Log, 相当于log.Print
// 注, 此函数仅为方便使用, 和log.Printf功能相同
func Log(a ...interface{}) {
	log.Print(a...)
	dispatchLog(LevelInfo, fmt.Sprint(a...))
}

// 输出Log, 相当于log.Printf
// 注, 此函数仅为方便使用, 和log.Printf功能相同
func Logf(format string, a ...interface{}) {
	log.Printf(format, a...)
	dispatchLog(LevelInfo, fmt.Sprintf(format, a...))
}

// 输出调试Log, 且在前面添加"debug: "字样
// 此函数只在打开Debug开关时生效, 输出的Log不带堆栈信息
func Debug(a ...interface{}) {
	if isDebugOn {
		s := "debug: " + fmt.Sprint(a...)
		log.Print(s)
		dispatchLog(LevelDebug, s)
	}
}

// 输出调试Log, 且在前面添加"trace: "字样
// 此函数只在打开Debug开关时生效, 输出的Log带有堆栈信息
func Trace(a ...interface{}) {
	if isDebugOn {
		s := "trace: " + fmt.Sprint(a...)
		log.Print(s)
		dispatchLog(LevelDebug, s)
	}
}

// 输出警告Log, 且在前面添加"warning: "字样
func Warn(a ...interface{}) {
	s := "warning: " + fmt.Sprint(a...)
	log.Print(s)
	dispatchLog(LevelWarn, s)
}

// 输出错误Log, 且在前面添加"error: "字样
// 此函数只输出Log, 不退出程序也不触发panic
func Error(a ...interface{}) {
	s := "error: " + fmt.Sprint(a...)
	log.Print(s)
	dispatchLog(LevelError, s)
}

package core

import (
	"reflect"
	//	"runtime"
	//	"strconv"
	"fmt"
	"os"
	"time"
	"unicode"
)

var startDate time.Time = time.Now()
var longAppName string

// 此函数检测接口是否良构, 如果接口非nil且含有空值则panic
// 此函数只在Debug版检测, Release版不检测
// 注: 在我们的架构里, 空值存放在接口里是非法的,
//     因为此时接口不为nil, 而指针却为nil, 容易引起混乱.
// 非法的例子:
//     func NewInt() interface{} {
//        var p *int = nil
//        return p // 危险! 把空指针赋值给接口
//     }
// 合法的例子:
//     func NewInt() interface{} {
//        var p *int = nil
//        if p == nil {
//            return nil // 正确
//        }
//        return p // 正确, 此处不会返回空指针
//     }
func CheckIface(pi *interface{}) {
	if isDebugOn && *pi != nil {
		v := reflect.ValueOf(*pi)
		k := v.Kind()
		if k == reflect.Chan || k == reflect.Func ||
			k == reflect.Interface || k == reflect.Map ||
			k == reflect.Ptr || k == reflect.Slice {
			if v.IsNil() {
				panic("ill-formed interface: nil " + v.Type().String() + " store in interface")
			}
		}
	}
}

// 判断接口本身或接口的值是否为nil
// 注: 在我们的架构里, 空值存放在接口里是非法的,
//     因为此时接口不为nil, 而指针却为nil, 容易引起混乱.
// 此函数和reflect.IsNil()的区别是:
//   1 此函数也判断接口是否为nil
//   2 在参数类型不可能为nil时, 此函数不paninc, 而是返回true
func IsNil(i interface{}) bool {
	if i == nil {
		return true
	}

	v := reflect.ValueOf(i)
	k := v.Kind()
	if k == reflect.Chan || k == reflect.Func ||
		k == reflect.Interface || k == reflect.Map ||
		k == reflect.Ptr || k == reflect.Slice {
		return v.IsNil()
	}
	return false
}

//// 程序启动日期
//func AppStartDate() time.Time {
//	return startDate
//}

//// 程序启动日期表示成"2006-01-02-15-04-05"格式字符串
//func AppStartDateString() string {
//	return startDate.Format("2006-01-02-15-04-05")
//}

// 程序启动日期表示成"20060102150405"形式的整数, 可用来识别程序版本
//func AppStartDateInt() int64 {
//	n, _ := strconv.ParseInt(startDate.Format("20060102150405"), 10, 64*8)
//	return n
//}

//func onIdle() {
//	// 以一定的间隔同步设置
//	//	dur := 10 * time.Minute
//	dur := 10 * time.Second
//	if time.Now().Sub(UserIni().SyncTime()) > dur {
//		UserIni().Sync()
//	}
//}

// 检测字符串是否可以直接用作文件基本名
// 允许含有'.', 不允许含有目录分隔符
// 不允许有控制字符
func IsValidFileName(s string) bool {
	a := []rune(s)
	if len(a) == 0 {
		return false
	}

	// 前后不允许有空格和控制字符
	if unicode.IsSpace(a[0]) ||
		!unicode.IsPrint(a[0]) ||
		unicode.IsSpace(a[len(a)-1]) ||
		!unicode.IsPrint(a[len(a)-1]) {
		return false
	}

	for _, r := range a {
		if !IsValidFileNameRune(r) {
			return false
		}
	}
	return true
}

// 把任意字符串转换为可以用作文件名的字符串
// 非法字符及前后空格将用'_'替换
func ToValidFileName(s string) string {

	a := []rune(s)
	if len(a) == 0 {
		return "_"
	}
	for i, r := range a {
		if !IsValidFileNameRune(r) {
			a[i] = '_'
		}
	}
	return string(a)
}

// 检测指定字符是否可以用在文件名里
func IsValidFileNameRune(r rune) bool {
	switch r {
	case '/':
		return false
	case ':':
		return false
	case '\\':
		return false
	case '"':
		return false
	case '\'':
		return false
	case '*':
		return false
	case '?':
		return false
	case '<':
		return false
	case '>':
		return false
	default:
		return unicode.IsPrint(r)
	}
}

// 设置应用程序的名字
func SetAppName(s string) {
	longAppName = s
}

// 获取应用程序的名字
func AppName() string {
	if longAppName == "" {
		return AppShortName()
	}
	return longAppName
}

func AppShortName() string {
	return ExeFileBaseName(false)
}

type StrErr string

func (s StrErr) Error() string {
	return string(s)
}

var exitRoutines []func()

var mainLoop func()
var quitLoop func()

var previousAbort bool

//var dispathers []func() bool
//var requestQuit bool
//var idleHandlers []func()
//var lastIdleHandlerId int
//var lastIdleTime time.Time

//var lastTimerTime = time.Now()

//var currentLoopLevel int
//var quitLoopLevel int

//var isQuit bool
//var isClosed bool
//var isClosing bool

func init() {
	fmt.Sscan(os.Getenv("SILK_DEBUG"), &isDebugOn)

	lockFilePath := LocalDataDir() + "/_" + ExeFileBaseName(true) + "_.runlock"
	_, err := os.Stat(lockFilePath)
	//Trace("err=", err)
	previousAbort = err == nil
	file, err := os.Create(lockFilePath)
	if err != nil {
		file.Close()
	}

	SetLogOutputFile(LocalDataDir()+"/"+ExeFileBaseName(false)+".log", true)

	if previousAbort {
		Log("Warning: 检测到程序上次运行时异常退出, 是否忘记core.Close()?")
	}

}

// 此函数供gui包调用, 应用层不需要调用此函数
func SetMainLoop(mainLoopFn, quitLoopFn func()) {
	mainLoop = mainLoopFn
	quitLoop = quitLoopFn
}

// 运行事件循环
func EventLoop() {
	if mainLoop == nil {
		panic("main loop mechanism unavailable, typically you need a gui package")
	}
	defer Close()
	mainLoop()
}

// 请求退出事件循环
func Quit() {
	if quitLoop == nil {
		return
	}
	quitLoop()
}

//// 请求退出主循环
//// 此函数立即返回. 主循环将收到退出请求, 随即退出
//// 此函数依赖于gui包, 非GUI程序不应该调用此函数
//func QuitLoop() {
//	if quitLoop == nil {
//		panic("main loop mechanism unavailable, typically you need a gui package")
//	}
//	quitLoop()
//}

//func needQuitLoop() bool {
//	return isQuit || currentLoopLevel == quitLoopLevel
//}

//// 事件循环
//// 循环调用事件分发例程, 并在空闲时调用空闲处理例程
//// 事件循环可以嵌套
//func EventLoop() {
//	mustNotClosed()
//	currentLoopLevel++
//	Trace("event loop level ", currentLoopLevel)
//	defer func() {
//		if e := recover(); e != nil {
//			Error("recover event loop level ", currentLoopLevel, ": ", e)
//		} else {
//			Trace("quit event loop level ", currentLoopLevel)
//		}
//		currentLoopLevel--
//		quitLoopLevel = 0
//		if currentLoopLevel == 0 {
//			doIdle() // 退出前触发一次空闲事件
//		}
//	}()

//	for !needQuitLoop() {
//		// 时间分发
//		var hasEvent bool
//		for i := 0; !needQuitLoop() && i < len(dispathers); i++ {
//			b := dispathers[i]()
//			hasEvent = hasEvent || b
//		}

//		if needQuitLoop() {
//			break
//		}

//		doTimer()

//		if time.Now().Sub(lastIdleTime) > time.Millisecond*100 {
//			// 强制空闲处理
//			doIdle()
//			//	Sleep(1)
//			continue
//		}

//		if hasEvent {
//			// 忙
//			continue
//		}

//		// 真正空闲
//		if time.Now().Sub(lastIdleTime) > time.Millisecond*20 {
//			doIdle()
//		}

//		//time.Sleep(10 * time.Millisecond)
//		Sleep(1)
//	}
//}

//// 退出当前事件循环
//// 注: 事件循环是可以嵌套的, 这个函数只请求退出当前循环, 即使连续调用多次也一样
//func EndEventLoop() {
//	if currentLoopLevel <= 0 {
//		panic("event loop not running")
//	}
//	quitLoopLevel = currentLoopLevel
//	Trace("request quit loop level ", quitLoopLevel)
//}

//// 退出所有事件循环, 并准备退出程序
//func Quit() {
//	Trace("request quit all loop levels")
//	isQuit = true
//}

//func doIdle() {
//	defer func() {
//		if e := recover(); e != nil {
//			Error("recover IDLE handling: ", e)
//		}
//		lastIdleTime = time.Now()
//	}()
//	return
//	for _, fn := range idleHandlers {
//		fn()
//	}
//	onIdle()
//}

// 注册事件分发例程
// 事件分发例程负责检测事件队列, 如果非空则分发1条然后返回true表示忙碌, 否则返回false表示空闲
// 注: 事件分发例程不支持注销
//func RegisterEventDispather(fn func() bool) {
//	dispathers = append(dispathers, fn)
//}

// 注册空闲处理例程
// 空闲处理例程会在事件队列为空时被调用, 如果事件队列一直非空也会不定期被调用
// 注: 空闲处理函数不支持注销, 如有需要请自行作二次分发
//func RegisterIdleRoutine(fn func()) {
//	idleHandlers = append(idleHandlers, fn)
//}

//var timerSet = make(map[*Timer]int)

//// 由主线程调度的低精度定时器
//type Timer struct {
//	count  time.Duration
//	period time.Duration

//	fn    func(param interface{})
//	param interface{}
//}

//// 停止定时器
//func (t *Timer) Stop() {
//	delete(timerSet, t)
//}

//// 用完以后必须调用Stop来停止
//func (t *Timer) Start(millisecond uint32, param interface{}, fn func(param interface{})) {
//	if millisecond == 0 {
//		millisecond = 1
//	}
//	t.count = 0
//	t.period = time.Duration(millisecond) * time.Millisecond
//	t.fn = fn
//	timerSet[t] = 1
//}

//// 定时器调度
//func doTimer() {
//	defer func() {
//		if e := recover(); e != nil {
//			Error("recover TIMER handling: ", e)
//		}
//	}()

//	t0 := time.Now()
//	d := t0.Sub(lastTimerTime)
//	if d >= time.Millisecond {
//		lastTimerTime = t0
//		for p, _ := range timerSet {
//			p.count += d
//			for p.count >= p.period {
//				p.count -= p.period
//				p.fn(p.param)
//			}
//		}
//	}
//}

//// 判断程序是否(将要)退出
//func IsQuited() bool {
//	return isQuit
//}

//func mustNotClosed() {
//	if isClosing {
//		panic("appliction is closing")
//	}
//	if isClosed {
//		panic("appliction already closed")
//	}
//}

//// 程序退出前应调用此函数, 调用以后应立即退出
//func Close() {
//	if currentLoopLevel > 0 {
//		panic("try close package when event loop is running")
//	}
//	if isClosing || isClosed {
//		return
//	}

//	isClosing = true
//	defer func() {
//		isClosed = true
//		isClosing = false
//	}()

//	waitAllWorkersDone()
//}

//// 检测上次运行程序时是否正确退出
//// 此函数的返回值不一定对, 但可以给应用层一些参考
//// 例如提示用户 "检测到程序上次运行时异常退出, 是否尝试恢复数据?"
//func IsPreviousRunClosedProperly() bool {
//	return !previousAbort
//}

func AtExit(fn func()) {
	exitRoutines = append(exitRoutines, fn)
}

func runExitRoutines() {
	for i := len(exitRoutines) - 1; i >= 0; i-- {
		fn := exitRoutines[i]
		if fn == nil {
			continue
		}
		exitRoutines[i] = nil
		fn()
	}
	exitRoutines = nil
}

func Close() {
	runExitRoutines()
	closeLogOut()
	lockFilePath := LocalDataDir() + "/_" + ExeFileBaseName(true) + "_.runlock"
	os.Remove(lockFilePath)
}

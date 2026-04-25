package core

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

// 连接信号槽, 信号和槽均在运行时查询
// Connect1(obj1, "SigSubmit", obj2, "OnObj1Submit")
// 或简写: Connect(obj1, "Submit", obj2, "OnObj1Submit")
// 注: 槽可以省略sender, 除此之外参数应匹配, 不匹配则输出警告, 且连接失败
func Connect1(sender interface{}, signal string, receiver interface{}, slotMethod string) {
	sig, ok := lookUpSigMethod(sender, signal)
	if !ok {
		return
	}
	slot, ok := lookUpSlotMethod(receiver, slotMethod)
	if !ok {
		return
	}
	connect(sig, slot)
}

// 连接信号槽, 信号已确定, 槽在运行时查询
// Connect2(obj1.SigSubmit, obj2, "OnObj1Submit")
// 注: 槽可以省略sender, 除此之外参数应匹配, 不匹配则输出警告, 且连接失败
func Connect2(sigFunc interface{}, receiver interface{}, slotMethod string) {
	sig, ok := sigFuncValue(sigFunc)
	if !ok {
		return
	}
	slot, ok := lookUpSlotMethod(receiver, slotMethod)
	if !ok {
		return
	}
	connect(sig, slot)
}

// 连接信号槽, 信号在运行时查询, 槽已确定
// Connect3(obj1, "SigSubmit", obj2.OnObj1Submit)
// 或简写: Connect2(obj1, "Submit", obj2.OnObj1Submit)
// 注: 槽可以省略sender, 除此之外参数应匹配, 不匹配则输出警告, 且连接失败
func Connect3(sender interface{}, signal string, slotFunc interface{}) {
	sig, ok := lookUpSigMethod(sender, signal)
	if !ok {
		return
	}

	slot, ok := slotFuncValue(slotFunc)
	if !ok {
		return
	}
	connect(sig, slot)
}

// 连接信号槽, 信号和槽均已确定
// Connect(obj1.SigSubmit, obj2.OnObj1Submit)
// 注: 槽可以省略sender, 除此之外参数应匹配, 不匹配则输出警告, 且连接失败
func Connect(sigFunc, slotFunc interface{}) {
	sig, ok := sigFuncValue(sigFunc)
	if !ok {
		return
	}

	slot, ok := slotFuncValue(slotFunc)
	if !ok {
		return
	}
	connect(sig, slot)
}

func connect(x reflect.Value, slot reflect.Value) (ret bool) {
	defer func() {
		if e := recover(); e != nil {
			Warn(fmt.Sprintf("painc when connect signal-slot: %s", e))
			ret = false
		}
	}()

	xt := x.Type()
	if xt.NumIn() != 1 {
		Warn(`irregal signal, wrong input parameter count: ` + x.Type().String())
		return
	}
	sigType := xt.In(0)
	if sigType.Kind() != reflect.Func {
		Warn(`irregal signal, input parameter is not func type : ` + x.Type().String())
		return
	}

	nSigIn := sigType.NumIn()
	nSlotIn := slot.Type().NumIn()

	if nSigIn == nSlotIn {
		x.Call([]reflect.Value{slot})
		ret = true
	} else if nSigIn == nSlotIn+1 {
		x.Call([]reflect.Value{slot_adaptor(sigType, slot)})
		ret = true
	} else {
		Warn(`signal-slot type missmatch : ` + x.Type().String() + " -> " + slot.Type().String())
		ret = false
	}
	return
}

func slot_adaptor(signalType reflect.Type, slotFn reflect.Value) reflect.Value {
	adaptor := func(in []reflect.Value) []reflect.Value {
		return slotFn.Call(in[1:])
	}
	return reflect.MakeFunc(signalType, adaptor)
}

func sigFuncValue(sigFunc interface{}) (ret reflect.Value, ok bool) {
	if sigFunc == nil {
		Warn(`nil signal`)
		return
	}
	sig := reflect.ValueOf(sigFunc)
	if sig.Type().Kind() != reflect.Func {
		Warn(`irregal signal, not a func: ` + sig.Type().String())
		return
	}
	return sig, true
}

func slotFuncValue(slotFunc interface{}) (ret reflect.Value, ok bool) {
	if slotFunc == nil {
		Warn(`nil slot`)
		return
	}
	slot := reflect.ValueOf(slotFunc)
	if slot.Type().Kind() != reflect.Func {
		Warn(`irregal slot, not a func: ` + slot.Type().String())
		return
	}
	return slot, true
}

func lookUpSigMethod(sender interface{}, signal string) (ret reflect.Value, ok bool) {
	if sender == nil {
		Warn(`nil sender`)
		return
	}
	a := reflect.ValueOf(sender)
	if strings.Index(signal, "Sig") != 0 {
		signal = "Sig" + signal
	}
	x := a.MethodByName(signal)
	if !x.IsValid() {
		Warn(`method not found: ` + ObjInfo(sender) + `."` + signal + `"`)
		return
	}
	return x, true
}

func lookUpSlotMethod(receiver interface{}, slotMethod string) (ret reflect.Value, ok bool) {
	if receiver == nil {
		Warn(`nil receiver`)
		return
	}

	if slotMethod == "" {
		Warn(`irregal slot method: "" `)
		return
	}

	if !unicode.IsUpper([]rune(slotMethod)[0]) {
		Warn(`failed to connect to unexported method : "` + slotMethod + `" `)
		return
	}

	b := reflect.ValueOf(receiver)
	slot := b.MethodByName(slotMethod)
	if !slot.IsValid() {
		Warn(`method not found: ` + ObjInfo(receiver) + `."` + slotMethod + `"`)
		return
	}
	return slot, true
}

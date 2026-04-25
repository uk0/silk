package core

import (
	"log"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	//	"strings"
)

func init() {
	loadFactoryAlias(ResourceDir() + "/sys/factory_alias.cml")
}

// 此函数功能同reflect.TypeOf, 放在这里是为了便于使用
func TypeOf(i interface{}) reflect.Type {
	return reflect.TypeOf(i)
}

// 用来关闭对象的接口
// 此接口应安静地释放资源并返回, Close()以后对象失效, 并可随意抛弃.
// 注: 如果是文档类对象(即可以选择是否保存的), Close() 里不应执行保存操作.
type IClose interface {
	Close()
}

// Factory
type Factory interface {
	// 对象的类名
	Name() string

	// 创建对象
	// p := c.New().(*MyStruct)
	// 注: 动态创建的图元有两种释放方式, 一种是自动销毁, 另一种要用Close()关闭
	// 在不知道应如何销毁时, 应尝试Close(), 以免出现资源泄露:
	//	if ia, ok := p.(core.IClose); ok {
	//		ia.Close()
	//	}
	New() interface{}

	// 工厂注册的位置
	Location() string
}

type _Factory struct {
	name string
	typ  reflect.Type
	init func(v reflect.Value)
	file string
	line int
}

var typeFactoryMap = make(map[reflect.Type]*_Factory)
var nameFactoryMap = make(map[string]*_Factory)
var factoryAlias = make(map[string]string)

func (c *_Factory) String() string {
	return `[ "` + c.name + `" @ ` + shortPath(c.file) + ":" + strconv.Itoa(c.line) + ` ]`
}

func (c *_Factory) Location() string {
	return shortPath(c.file) + ":" + strconv.Itoa(c.line)
}

func (c *_Factory) Name() string {
	return c.name
}

func (c *_Factory) New() interface{} {
	v := reflect.New(c.typ)
	if c.init != nil {
		c.init(v)
	}
	return v.Interface()
}

// 注册对象工厂
// RegisterFactory("my.StructA", gui.TypeOf(my.StructA{}))
func RegisterFactory(name string, typ reflect.Type) {

	if name == "" {
		log.Panic(`invalid factory name: ""`)
	}

	// 对指针和接口解引用
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Interface {
		typ = typ.Elem()
	}

	// Init函数receiver必须是指针类型
	ptrTyp := reflect.PtrTo(typ)

	_, file, line, _ := runtime.Caller(1)

	var fn func(v reflect.Value)
	initMethod, ok := ptrTyp.MethodByName("Init")
	//Trace(initMethod)
	if ok {
		t := initMethod.Func.Type()
		switch t.NumIn() {
		case 1:
			// X.Init()
			fn = func(v reflect.Value) {
				initMethod.Func.Call([]reflect.Value{v})
			}
		case 2:
			// X.Init(self interface{})
			fn = func(v reflect.Value) {
				initMethod.Func.Call([]reflect.Value{v, v})
			}
		default:
			var args []string
			for i := 1; i < initMethod.Type.NumIn(); i++ {
				args = append(args, initMethod.Type.In(i).String())
			}

			log.Panic(`ill-formed Init method: func (` + initMethod.Type.In(0).String() + `) Init(` + strings.Join(args, ", ") + `)`)
		}
	}

	c := &_Factory{name, typ, fn, file, line}

	if c0, ok := nameFactoryMap[name]; ok {
		log.Panic(`factory "` + name + `" already registered at ` + c0.Location())
	}
	if realFactory, ok := factoryAlias[name]; ok {
		Log(`warning: factory "` + name + `" already registered as alias of "` + realFactory + `"`)
		delete(factoryAlias, name)
	}
	nameFactoryMap[name] = c
	Log(`dbg: register factory: "` + name + `" at ` + c.Location())
	typeFactoryMap[typ] = c
}

// 用指定对象工厂创建对象
// p, ok := c.New("MyStruct").(*MyStruct)
// 参见 Factory.New()
func New(name string) interface{} {
	c := FindFactory(name)
	if c != nil {
		return c.New()
	}
	return nil
}

// 根据名字查找对象工厂
func FindFactory(name string) Factory {
	c, ok := nameFactoryMap[name]
	if ok {
		return c
	}

	realName, ok := factoryAlias[name]
	if ok {
		c, ok = nameFactoryMap[name]
		if ok {
			return c
		}
		Log(`warning: factory "` + name + `" is registered as alias of "` + realName +
			`", but "` + realName + `" is not registered`)
	} else {
		Log(`warning: factory "` + name + `" is not registered`)
	}
	return nil
}

// 获取全部对象工厂
func AllFactories() []Factory {
	var all []Factory
	for _, c := range nameFactoryMap {
		all = append(all, c)
	}
	return all
}

// 给软件工厂添加别名, 用aliasName也能生产realName对象的对象
// 别名本身不允许重复, 也不允许和实名重复
// 别名的别名不起作用
func AddFactoryAlias(aliasName, realName string) {
	Trace(`add factory alias: "` + aliasName + `" ==> "` + realName + `"`)

	if c0, ok := nameFactoryMap[aliasName]; ok {
		Warn(`factory "` + aliasName + `" already registered at ` + c0.Location())
	} else if realName1, ok := factoryAlias[aliasName]; ok {
		Warn(`factory "` + aliasName + `" already registered as alias of "` + realName1 + `"`)
	}
	factoryAlias[aliasName] = realName

}

// 从文件加载别名
func loadFactoryAlias(path string) {
	doc, err := LoadTDocFile(path)
	if err != nil {
		return
	}
	Trace(`factory alias loaded: `, path)
	for _, p := range doc.Childdren() {
		var s string
		p.Value(&s)
		AddFactoryAlias(p.Key(), s)
	}
}

func FactoryOf(i interface{}) Factory {

	typ, ok := i.(reflect.Type)
	if !ok {
		typ = reflect.TypeOf(i)
	}

	// 对指针和接口解引用
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Interface {
		typ = typ.Elem()
	}

	p, ok := typeFactoryMap[typ]
	if ok {
		return p
	}
	return nil
}

func FactoryNameOf(i interface{}) string {
	p := FactoryOf(i)
	if p != nil {
		return p.Name()
	}
	return ""
}

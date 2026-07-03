package prop

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	//	"github.com/uk0/silk/core"
	//	"encoding/json"
	"fmt"
	//	"io/ioutil"
	//	"os"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

func init() {
	core.RegisterFactory("prop.PropertySheet", core.TypeOf((*PropertySheet)(nil)))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "prop.PropertySheet",
		Name: "属性",
		Icon: "propsheet",
		Desc: "显示修改对象属性",
	})
}

// 属性表控件应实现的接口
type IPropertyControl interface {
	gui.IWidget

	// 绑定属性, 在创建之后调用
	BindProperty(item *PropertyItem1)

	// 更新属性值
	// 控件从item获取新值并显示
	UpdateValue()

	// 更新其他配置
	// 控件从item读取有用的配置并更新, 例如如下拉选项
	UpdateConfig()

	// 激活. 获取键盘焦点, 高亮显示
	Activate()

	// 取消激活, 如果属性值已被用户修改, 则控件应调用item的SetValueByControl
	Deactivate()
}

// 属性视图
type IPropertyView interface {
	// 清除属性表, 如果指定了owner, 则只在owner和Bind时候匹配时才清除
	// 此接口用来清理属性表中过时的条目
	Clear(owner interface{})

	// 绑定对象, 显示对象的属性
	// 属性视图先清除旧的条目, 然后逐个调用对象的IEnumProperties接口生成新条目
	// 同名的条目将合并成一条, 出现类型不匹配等异常时打出警告并跳过相应条目
	// 未实现IEnumProperties的对象将被忽略
	// owner只用在调用Clear()方法时判断是否需要清除, 没有特殊要求
	// 建议把owner设为相应视图, 以免Clear时误清其他视图的属性
	Bind(objs []interface{}, cfgName string, owner interface{})
}

// 属性条目列表
type IPropertyList interface {
	AddProperty(id string, get, set interface{}) (item *PropertyItem, first bool)
}

// 用来枚举属性的接口
// 对象实现这个接口后即可在属性表里显示
type IEnumProperties interface {
	EnumProperties(list IPropertyList)
}

// propertyListAdapter wraps PropertySheet to satisfy core.IPropertyList,
// which has AddProperty without return values.
type propertyListAdapter struct {
	sheet *PropertySheet
}

func (a *propertyListAdapter) AddProperty(id string, get, set interface{}) {
	a.sheet.AddProperty(id, get, set)
}

// 属性基本信息的实现
type Property struct {
	id  string
	typ reflect.Type

	get reflect.Value
	set []reflect.Value

	//cfgReadOnly bool
}

// 是否只读
// 未指定Set接口的属性为只读属性
func (prop Property) IsReadOnly() bool {
	return len(prop.set) == 0
}

func (prop Property) String() string {
	if prop.get.IsValid() {
		if len(prop.set) != 0 {
			return prop.id + " [get,set*" + strconv.Itoa(len(prop.set)) + "] " + prop.typ.Name()
		}
		return prop.id + " [get] " + prop.typ.Name()
	}

	return prop.id + " [n/a] " + prop.typ.Name()
}

// 属性的标识
// 具有相同标识的多条属性将被合并成一条
func (prop Property) Id() string {
	return prop.id
}

// 属性的参数类型
// 目前的版本要求参数严格匹配
func (prop Property) Type() reflect.Type {
	return prop.typ
}

// 设置属性的值
// 如果是合并后的属性, 此方法会为所有对象设置新值
func (prop Property) SetValue(a interface{}) {
	for _, v := range prop.set {
		v.Call([]reflect.Value{reflect.ValueOf(a)})
	}
}

// 获取属性的值
// 如果是合并后的属性, 此方法只取第一个对象的值
func (prop Property) GetValue() interface{} {
	return prop.get.Call([]reflect.Value{})[0].Interface()
}

// 设置字符串形式的属性的值, 如果设置失败, 此函数返回错误信息
// 如果是合并后的属性, 此方法会为所有对象设置新值
func (prop Property) SetValueStr(s string) error {
	v := reflect.New(prop.Type())
	_, err := core.PersistSscan(s, v.Interface())
	if err != nil {
		return err
	}
	prop.SetValue(v.Elem().Interface())
	return nil
}

// 获取字符串形式的属性值, 如果转换失败, 此函数返回空字符串
// 如果是合并后的属性, 此方法只取第一个对象的值
func (prop Property) GetValueStr() string {
	return core.VisualString(prop.GetValue())
}

// 修改属性的命令
type PropertyCommand struct {
	prop   *Property
	val    interface{}
	isUndo bool
}

//func NewPropertyCommand(prop *Property, newVal interface{}) *PropertyCommand {
//	p := new(PropertyCommand)
//	p.prop = prop
//	p.val = newVal
//	return p
//}

func (cmd *PropertyCommand) Redo() {
	if cmd.isUndo {
		panic("irregal Redo()")
	}
	oldVal := cmd.prop.GetValue()
	cmd.prop.SetValue(cmd.val)
	cmd.val = oldVal
	cmd.isUndo = true
}

func (cmd *PropertyCommand) Undo() {
	if !cmd.isUndo {
		panic("irregal Undo()")
	}
	oldVal := cmd.prop.GetValue()
	cmd.prop.SetValue(cmd.val)
	cmd.val = oldVal
	cmd.isUndo = false
}

func (cmd *PropertyCommand) Text() string {
	return fmt.Sprintf("Set Property %v", cmd.prop)
}

// 属性配置文件
type PropertyConfigFile struct {
	name string
	doc  *core.TDoc

	inheritLoaded bool
	inherit       *PropertyConfigFile
}

var configFileCache = make(map[string]*PropertyConfigFile)

func propertyConfigFilePath(name string) string {
	return core.ResourceDir() + "/property/" + name + ".cml"
}

func GetPropertyConfigFile(name string, fallbackToDefault bool) *PropertyConfigFile {
	name = strings.ToLower(name)
	pf, ok := configFileCache[name]
	if !ok {
		var doc *core.TDoc
		var err error
		if name == "" {
			err = errors.New(`property config "" is not valid `)
		} else {
			path := propertyConfigFilePath(name)
			doc, err = core.LoadTDocFile(path)
		}

		if err == nil {
			pf = new(PropertyConfigFile)
			pf.name = name
			pf.doc = doc
		} else if fallbackToDefault && name != "default" {
			if name == "" {
				core.Debug(err, `, fallback to "default"`)
			} else {
				core.Warn(err, ` (fallback to "default")`)
			}
			pf = GetPropertyConfigFile("default", false)
		} else if name == "default" {
			core.Warn(err, ` (empty file generated)`)
			pf = new(PropertyConfigFile)
			pf.name = "default"
			pf.doc = core.NewTDoc()
			pf.Save()
		} else {
			core.Warn(err)
		}
		configFileCache[name] = pf
	}
	return pf
}

func (this *PropertyConfigFile) Inherit() *PropertyConfigFile {
	if this.inheritLoaded {
		return this.inherit
	}
	this.inheritLoaded = true
	inheritName := this.InheritName()
	if inheritName == "" {
		if this.name == "default" {
			this.inherit = nil
			return nil // "default" is root
		}
		inheritName = "default"
	}
	this.inherit = GetPropertyConfigFile(inheritName, true)

	for p := this.inherit; p != nil; p = p.inherit {
		if p == this {
			core.Warn(`property config file cyclic inherit: "` + this.name + `"`)
			this.inherit = nil
			break
		}
	}
	if this.inherit == nil {
		core.Debug(`property config "` + this.name + `" is inherit from <nil>`)
	} else {
		core.Debug(`property config "` + this.name + `" is inherit from "` + this.inherit.name + `"`)
	}
	return this.inherit
}

func (this *PropertyConfigFile) Save() error {
	return this.doc.SaveFile(propertyConfigFilePath(this.name))
}

func (this *PropertyConfigFile) InheritName() (s string) {
	this.doc.ReadAttr("#inherit", &s)
	return
}

func (this *PropertyConfigFile) SetInheritName(s string) {
	this.inherit = nil
	this.doc.WriteAttr("#inherit", s)
	this.Inherit()
}

func (this *PropertyConfigFile) ItemConfig(id string) (*core.TDoc, *PropertyConfigFile) {
	sub := this.doc.ChildByKey(id, false)
	if sub != nil {
		return sub, this
	}
	p := this.Inherit()
	if p != nil {
		return p.ItemConfig(id)
	}
	return nil, nil
}

// 属性条目的实现
type PropertyItem struct {
	prop *Property

	// 对应的编辑控件
	control IPropertyControl

	// 父节点
	parent *PropertyItem

	// 子节点
	children []*PropertyItem

	// 是否展开, 在有子节点时起作用
	expand bool

	// 实际y坐标
	ypos float64

	// 实际高度
	//height float64

	// 缩进等级
	indent int

	// 当前所在行号
	vrow int

	// 当前所属的属性表, 使用时永不为空
	sheet *PropertySheet

	// 当前配置, 此配置初始值源于配置文件
	// 如果程序内部修改过, 则复制一份副本, 不再和原文件关联
	itemConfig *core.TDoc

	// 实际使用的配置文件
	// 在使用非"default"配置时, 如果条目实际继承自"default", 则这个变量指向的是"default"
	// 如需获取继承后的配置文件, 可以使用sheet.configFile
	// configFile可以为空, 表示配置是由程序内部指定的, 不是从配置文件读取
	configFile *PropertyConfigFile

	updatingControl bool
}

func (this *PropertyItem) GetValue() interface{} {
	return this.prop.GetValue()
}

func (this *PropertyItem) GetValueStr() string {
	return this.prop.GetValueStr()
}

func (this *PropertyItem) SetValue(a interface{}) {
	if this.IsReadOnly() {
		return
	}
	if ii, ok := this.sheet.objOwner.(interface {
		PushCommand(cmd gui.ICommand)
	}); ok {
		cmd := new(PropertyCommand)
		cmd.prop = this.prop
		cmd.val = a
		ii.PushCommand(cmd)
	} else {
		this.prop.SetValue(a)
	}
}

// 设置字符串形式的属性的值, 如果设置失败, 此函数返回错误信息
// 如果是合并后的属性, 此方法会为所有对象设置新值
func (this *PropertyItem) SetValueStr(s string) error {
	v := reflect.New(this.prop.Type())
	_, err := core.PersistSscan(s, v.Interface())
	if err != nil {
		return err
	}
	this.SetValue(v.Elem().Interface())
	return nil
}

func (this *PropertyItem) UpdateControlValue() {
	this.updatingControl = true
	defer func() {
		this.updatingControl = false
	}()

	this.Control().UpdateValue()
}

func (this *PropertyItem) UpdateControlConfig() {
	this.updatingControl = true
	defer func() {
		this.updatingControl = false
	}()

	this.Control().UpdateConfig()
}

func (this *PropertyItem) UpdateControl() {
	this.updatingControl = true
	defer func() {
		this.updatingControl = false
	}()

	this.Control().UpdateConfig()
	this.Control().UpdateValue()
}

func (this *PropertyItem) prepareModifyConfig() {
	if this.configFile != nil {
		this.configFile = nil
		this.itemConfig = this.itemConfig.Clone()
	}
}

// 只读, 属性没有set方法, 或者被配置为只读
func (this *PropertyItem) IsReadOnly() bool {
	if this.prop.IsReadOnly() {
		return true
	}
	return this.CfgReadOnly()
}

func (this *PropertyItem) ItemHeight() float64 {
	height := this.CfgItemHeight()
	if height <= 0 {
		height = this.sheet.DefaultItemHeight()
	}
	return height
}

func (this *PropertyItem) CfgItemHeight() (height float64) {
	this.itemConfig.ReadAttr("height", &height)
	return
}

func (this *PropertyItem) SetCfgItemHeight(height float64) {
	this.prepareModifyConfig()
	this.itemConfig.WriteAttr("height", height)
}

func (this *PropertyItem) CfgOrder() (ret int) {
	this.itemConfig.ReadAttr("order", &ret)
	return
}

func (this *PropertyItem) SetCfgOrder(order int) {
	this.prepareModifyConfig()
	this.itemConfig.WriteAttr("order", order)
}

func (this *PropertyItem) CfgReadOnly() (ret bool) {
	this.itemConfig.ReadAttr("readonly", &ret)
	return
}

func (this *PropertyItem) SetCfgReadOnly(b bool) {
	this.prepareModifyConfig()
	this.itemConfig.WriteAttr("readonly", b)
}

func (this *PropertyItem) CfgHidden() (ret bool) {
	this.itemConfig.ReadAttr("hidden", &ret)
	return
}

func (this *PropertyItem) SetCfgHidden(b bool) {
	this.prepareModifyConfig()
	this.itemConfig.WriteAttr("hidden", b)
}

func (this *PropertyItem) CfgWidgetType() (typ string) {
	this.itemConfig.ReadAttr("control", &typ)
	return
}

func (this *PropertyItem) SetCfgWidgetType(typ string) {
	this.prepareModifyConfig()
	if this.control != nil {
		this.control.SetParent(nil)
		this.control = nil
	}
	this.itemConfig.WriteAttr("control", typ)
}

func (this *PropertyItem) Id() string {
	return this.prop.Id()
}

func (this *PropertyItem) Type() reflect.Type {
	return this.prop.Type()
}

func (this *PropertyItem) Label() string {
	label := this.CfgLabel()
	if label == "" {
		label = this.Id()
	}
	return label
}

func (this *PropertyItem) CfgLabel() (label string) {
	this.itemConfig.ReadAttr("label", &label)
	return
}

func (this *PropertyItem) SetCfgLabel(label string) {
	this.prepareModifyConfig()
	this.itemConfig.WriteAttr("label", label)
}

func (this *PropertyItem) HasExpander() bool {
	return len(this.children) != 0
}

func (this *PropertyItem) IsVisible() bool {
	return !this.CfgHidden() && (this.parent == nil || this.parent.IsVisible())
}

func (this *PropertyItem) IsExpand() bool {
	return this.expand
}

func (this *PropertyItem) String() string {
	return this.prop.String()
}

func (this *PropertyItem) defaultControlType() string {
	switch this.Type().Kind() {
	case reflect.Bool:
		return "CheckBox"
	case reflect.Int:
		return "IntEdit"
	default:
		return "TextEdit"
	}
}

func (this *PropertyItem) createControl(typ string) IPropertyControl {
	tag := "prop.control." + typ
	obj := core.New(tag)
	if obj == nil {
		return nil
	}
	control, ok := obj.(IPropertyControl)
	if !ok {
		core.Warn(`object of type "` + tag + `" dosn't implement IPropertyControl`)
		return nil
	}
	return control
}

func (this *PropertyItem) createControl2() IPropertyControl {
	typ := this.CfgWidgetType()
	var fallbackAction string
	if typ != "" {
		control := this.createControl(typ)
		if control != nil {
			control.SetParent(this.sheet)
			return control
		}
		fallbackAction = "fallback"
	} else {
		fallbackAction = "default"
	}
	defaultTyp := this.defaultControlType()
	if defaultTyp != typ {
		control := this.createControl(defaultTyp)
		if control != nil {
			core.Debug(`property "` + this.Id() + `" ` + fallbackAction + ` to "` + defaultTyp + `" control`)
			control.SetParent(this.sheet)
			return control
		}
	}
	core.Debug(`property "` + this.Id() + `" ` + fallbackAction + ` to "Label" control`)
	control := NewTextEdit()
	control.SetParent(this.sheet)
	return control
}

func (this *PropertyItem) Control() (ret IPropertyControl) {
	if this.control != nil {
		return this.control
	}

	defer func() {
		if e := recover(); e != nil {
			core.Warn(fmt.Sprintf(`Recover PropertyItem.Control() (id = "%v") : %v `, this.Id(), e))
			this.control = NewTextEdit()
		}
		this.control.SetParent(this.sheet)
		this.control.BindProperty(&PropertyItem1{this})
		this.UpdateControl()
		ret = this.control
	}()
	this.control = this.createControl2()
	ret = this.control
	return
}

// 从配置文件里加载配置
func (this *PropertyItem) loadConfig() {
	this.itemConfig, this.configFile = this.sheet.configFile.ItemConfig(this.Id())
	if this.itemConfig == nil {
		core.Debug(`property config not found:"` + this.Id() + `"`)
		this.itemConfig = core.NewTDoc()
		this.configFile = nil
	}
}

// 除了重写部分函数以外, 其他和PropertyItem一致
// 用于和属性表控件交互
type PropertyItem1 struct {
	*PropertyItem
}

func (this *PropertyItem1) SetValue(a interface{}) {
	if this.updatingControl {
		return // 防止递归
	}
	this.PropertyItem.SetValue(a)
}

var errRecursSetValue = errors.New("recurs PropertyItem.SetValue() call")

func (this *PropertyItem1) SetValueStr(s string) error {
	if this.updatingControl {
		return errRecursSetValue
	}
	return this.PropertyItem.SetValueStr(s)
}

// propertyCategory represents a collapsible category group in the property sheet.
type propertyCategory struct {
	name     string // display name (Chinese)
	key      string // internal key
	expanded bool
	ypos     float64
}

// categoryHeaderHeight is the height of a category header row.
const categoryHeaderHeight = 24.0

// predefined category order
var categoryOrder = []string{"layout", "appearance", "behavior", "events", "general"}

// categoryNames maps internal keys to display names.
var categoryNames = map[string]string{
	"layout":     "\u5e03\u5c40", // 布局
	"appearance": "\u5916\u89c2", // 外观
	"behavior":   "\u884c\u4e3a", // 行为
	"events":     "\u4e8b\u4ef6", // 事件
	"general":    "\u5e38\u89c4", // 常规
}

// categoryOfPropID classifies a property by its lowercase ID.
func categoryOfPropID(id string) string {
	// Layout properties
	layoutIDs := []string{"x", "y", "width", "height", "w", "h", "pos", "size", "bounds", "left", "top", "right", "bottom", "margin", "padding"}
	for _, lid := range layoutIDs {
		if id == lid || strings.HasPrefix(id, lid+"_") || strings.HasSuffix(id, "_"+lid) {
			return "layout"
		}
	}

	// Appearance properties
	appearanceIDs := []string{"text", "color", "font", "icon", "image", "style", "bg", "background", "foreground", "visible", "opacity", "border"}
	for _, aid := range appearanceIDs {
		if id == aid || strings.HasPrefix(id, aid+"_") || strings.HasSuffix(id, "_"+aid) || strings.Contains(id, aid) {
			return "appearance"
		}
	}

	// Behavior properties
	behaviorIDs := []string{"enabled", "readonly", "checked", "value", "editable", "selected", "active", "focused", "clickable", "draggable"}
	for _, bid := range behaviorIDs {
		if id == bid || strings.HasPrefix(id, bid+"_") || strings.HasSuffix(id, "_"+bid) || strings.Contains(id, bid) {
			return "behavior"
		}
	}

	// Event properties
	eventIDs := []string{"on_", "event", "signal", "handler", "callback", "click", "changed"}
	for _, eid := range eventIDs {
		if strings.HasPrefix(id, eid) || strings.Contains(id, eid) {
			return "events"
		}
	}

	return "general"
}

// 属性表视图的实现
type PropertySheet struct {
	gui.ScrollArea
	objOwner   interface{}
	propMap    map[string]*PropertyItem
	rlist      []*PropertyItem
	vlist      []*PropertyItem
	configFile *PropertyConfigFile

	suspendLayout bool

	vertSplit float64

	title string
	icon  paint.Icon

	// Category support
	categories     map[string]*propertyCategory
	categoryLayout []categoryLayoutEntry // ordered layout entries for drawing
}

// categoryLayoutEntry is either a category header or a property item in the layout.
type categoryLayoutEntry struct {
	isHeader bool
	category string
	item     *PropertyItem
	ypos     float64
}

func (this *PropertySheet) Init(iw gui.IWidget) {
	this.ScrollArea.Init(iw)
	this.propMap = make(map[string]*PropertyItem)
	this.vertSplit = 0.5
	this.categories = make(map[string]*propertyCategory)
	for _, key := range categoryOrder {
		this.categories[key] = &propertyCategory{
			name:     categoryNames[key],
			key:      key,
			expanded: true,
		}
	}
}

func NewPropertySheet() *PropertySheet {
	p := new(PropertySheet)
	p.Init(p)
	return p
}

func (this *PropertySheet) DefaultItemHeight() float64 {
	return 26
}

func (this *PropertySheet) SetIcon(icon paint.Icon) {
	this.icon = icon
}

func (this *PropertySheet) Icon() paint.Icon {
	return this.icon
}

func (this *PropertySheet) SetTitle(s string) {
	this.title = s
}

func (this *PropertySheet) Title() string {
	return this.title
}

func (this *PropertySheet) Clear(owner interface{}) {
	if owner != nil && owner != this.objOwner {
		return
	}

	for _, p := range this.propMap {
		if p.control != nil {
			p.control.SetParent(nil)
		}
	}
	this.propMap = make(map[string]*PropertyItem)
	this.vlist = nil
	this.rlist = nil
	this.categoryLayout = nil
}

func IsValidPropId(s string) bool {
	n := len(s)
	if n == 0 {
		return false
	}
	if s[0] != '_' && (s[0] < 'a' || s[0] > 'z') {
		return false
	}

	for i := 1; i < n; i++ {
		if s[i] != '_' && (s[i] < 'a' || s[i] > 'z') && (s[i] < '0' || s[i] > '9') {
			return false
		}
	}
	return true
}

func (this *PropertySheet) AddProperty(id string, get, set interface{}) (item *PropertyItem, first bool) {
	id = strings.ToLower(id)
	if !IsValidPropId(id) {
		core.Warn(`Invalid prop id "` + id + `"`)
		return
	}

	if get == nil {
		core.Warn(`Prop "` + id + `" : irregal get-method: nil`)
		return
	}

	p, added := this.propMap[id]
	if !added {
		prop := new(Property)
		prop.id = id

		prop.get = reflect.ValueOf(get)
		if prop.get.Type().Kind() != reflect.Func || prop.get.Type().NumIn() != 0 || prop.get.Type().NumOut() != 1 {
			core.Warn(`Prop "`+id+`" : irregal get-method: `, prop.get.Type())
			return
		}
		prop.typ = prop.get.Type().Out(0)
		p = new(PropertyItem)
		p.prop = prop
		p.sheet = this
		p.loadConfig()
		this.propMap[id] = p
	}

	if set != nil {
		setMethod := reflect.ValueOf(set)
		if setMethod.Type().NumIn() != 1 || setMethod.Type().NumOut() != 0 {
			core.Warn(`Prop "`+id+`" : irregal set-method: `, setMethod.Type())
		} else if setMethod.Type().In(0) != p.prop.typ {
			core.Warn(`Prop "` + id + `" get-method and set-method type miss match: "` +
				p.prop.get.Type().String() + `" and "` + setMethod.Type().String() + `"`)
		} else {
			p.prop.set = append(p.prop.set, setMethod)
		}
	}
	return p, !added
}

type sortProps []*PropertyItem

func (s sortProps) Len() int {
	return len(s)
}

func (s sortProps) Less(i, j int) bool {
	a := s[i].CfgOrder()
	b := s[j].CfgOrder()
	if a == b {
		c := s[i].Id()
		d := s[j].Id()
		return c < d
	}
	return a < b
}

func (s sortProps) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (this *PropertySheet) Bind(objs []interface{}, configName string, owner interface{}) {
	core.Debug(`bind `, len(objs), ` object(s) to property sheet with config name = "`+configName+
		`", owner = `+reflect.TypeOf(owner).String())
	this.Clear(nil)

	this.configFile = GetPropertyConfigFile(configName, true)
	this.objOwner = owner
	if owner != nil {
		if _, ok := owner.(interface {
			PushCommand(cmd gui.ICommand)
		}); !ok {
			core.Debug(reflect.TypeOf(owner).String() + " has not PushCommand(gui.ICommand) method, undo/redo will not works for property change.")
		}
	}

	adapter := &propertyListAdapter{sheet: this}
	for _, obj := range objs {
		// Check prop.IEnumProperties first (original interface)
		if x, ok := obj.(IEnumProperties); ok {
			x.EnumProperties(this)
			continue
		}
		// Check core.IEnumProperties (gui widgets use this to avoid import cycle)
		if x, ok := obj.(core.IEnumProperties); ok {
			x.EnumProperties(adapter)
			continue
		}
	}

	for _, v := range this.propMap {
		this.rlist = append(this.rlist, v)
	}

	sort.Sort(sortProps(this.rlist))

	this.Layout()
}

func (this *PropertySheet) Layout() {
	if this.suspendLayout {
		return
	}
	this.vlist = nil
	this.categoryLayout = nil

	// Group properties by category
	catItems := make(map[string][]*PropertyItem)
	for _, p := range this.rlist {
		cat := categoryOfPropID(p.Id())
		catItems[cat] = append(catItems[cat], p)
	}

	vrow := 0
	ypos := 0.0

	for _, catKey := range categoryOrder {
		items := catItems[catKey]
		if len(items) == 0 {
			continue
		}

		cat := this.categories[catKey]
		if cat == nil {
			continue
		}

		// Category header
		cat.ypos = ypos
		this.categoryLayout = append(this.categoryLayout, categoryLayoutEntry{
			isHeader: true,
			category: catKey,
			ypos:     ypos,
		})
		ypos += categoryHeaderHeight

		if !cat.expanded {
			// Hide all items in collapsed category
			for _, p := range items {
				p.vrow = -1
				control := p.Control()
				if control != nil {
					control.SetVisible(false)
				}
			}
			continue
		}

		for _, p := range items {
			if p.IsVisible() {
				this.vlist = append(this.vlist, p)
				if p.parent != nil {
					p.indent = p.parent.indent + 1
				} else {
					p.indent = 0
				}
				p.vrow = vrow
				vrow++
				p.ypos = ypos
				this.categoryLayout = append(this.categoryLayout, categoryLayoutEntry{
					isHeader: false,
					item:     p,
					ypos:     ypos,
				})
				ypos += p.ItemHeight()
				// 显示控件
				control := p.Control()
				if control != nil {
					control.SetBounds1(this.getItemRect(p, pitemControl))
					control.SetVisible(true)
				}
			} else {
				p.vrow = -1
				// 隐藏控件
				control := p.Control()
				if control != nil {
					control.SetVisible(false)
				}
			}
		}
	}
	this.Update()
}

func (this *PropertySheet) vertSplitPx() float64 {
	return this.Width() * this.vertSplit
}

func (this *PropertySheet) headHight() float64 {
	return 22
}

func (this *PropertySheet) indentSize() float64 {
	return 18
}

func (this *PropertySheet) expanderSize() float64 {
	return 18
}

const (
	pitemWhole    = 0
	pitemExpander = 1
	pitemLabel    = 2
	pitemControl  = 3
)

func (this *PropertySheet) getItemRect(item *PropertyItem, part int) geom.Rect {
	width := this.Width()
	indent := float64(item.indent) * this.indentSize()
	height := item.ItemHeight()
	xedit := this.vertSplitPx()
	switch part {
	case pitemExpander:
		if item.HasExpander() {
			return geom.Rect{indent, item.ypos, this.expanderSize(), height}
		} else {
			return geom.Rect{indent, item.ypos, 0, 0}
		}
	case pitemLabel:
		if item.HasExpander() {
			return geom.Rect{indent + this.expanderSize(), item.ypos, xedit - indent - this.expanderSize(), height}
		} else {
			return geom.Rect{indent, item.ypos, xedit - indent, height}
		}
	case pitemControl:
		return geom.Rect{xedit, item.ypos, width - xedit, height}
	default:
		fallthrough
	case pitemWhole:
		return geom.Rect{indent, item.ypos, width - indent, height}
	}
}

// colorType is the reflect.Type of paint.Color for quick comparison.
var colorType = reflect.TypeOf(paint.Color{})

// isColorProperty reports whether the given property stores a paint.Color value.
func isColorProperty(p *PropertyItem) bool {
	if p == nil || p.prop == nil {
		return false
	}
	return p.prop.typ == colorType
}

// drawColorSwatch paints a 16x16 rounded swatch with a subtle border next to
// the label column, letting users visually identify color properties.
func (this *PropertySheet) drawColorSwatch(g paint.Painter, p *PropertyItem, itemH float64) {
	// Render in the gap to the left of the control (right end of the label column),
	// so the swatch is not covered by the child input control.
	xedit := this.vertSplitPx()
	const swatchSize = 14.0
	const swatchPad = 4.0

	sx := xedit - swatchSize - swatchPad
	sy := p.ypos + (itemH-swatchSize)*0.5
	if sx < 0 {
		return
	}

	// Try to fetch the current value; skip silently on panic (reflection edges).
	var cr paint.Color
	func() {
		defer func() { _ = recover() }()
		v := p.GetValue()
		if c, ok := v.(paint.Color); ok {
			cr = c
		}
	}()

	// Checker-style background hint for transparent colors.
	if cr.A < 255 {
		g.Rectangle(sx, sy, swatchSize, swatchSize)
		g.SetBrush1(paint.Color{R: 220, G: 220, B: 220, A: 255})
		g.Fill()
		g.Rectangle(sx+swatchSize*0.5, sy, swatchSize*0.5, swatchSize*0.5)
		g.SetBrush1(paint.Color{R: 170, G: 170, B: 170, A: 255})
		g.Fill()
		g.Rectangle(sx, sy+swatchSize*0.5, swatchSize*0.5, swatchSize*0.5)
		g.Fill()
	}

	// Solid swatch fill.
	g.Rectangle(sx, sy, swatchSize, swatchSize)
	g.SetBrush1(cr)
	g.Fill()

	// Outline for contrast in both themes.
	border := paint.Color{R: 90, G: 90, B: 100, A: 200}
	if gui.CurrentThemeMode() == gui.ThemeDark {
		border = paint.Color{R: 200, G: 200, B: 210, A: 180}
	}
	g.SetPen1(border, 1)
	g.Rectangle(sx+0.5, sy+0.5, swatchSize-1, swatchSize-1)
	g.Stroke()
}

func (this *PropertySheet) Draw(g paint.Painter) {
	w := this.Width()
	xedit := this.vertSplitPx()

	g.SetPen1(gui.Theme().BorderColor, 1)
	g.Line(xedit, 0, xedit, this.Height())
	g.Stroke()

	font := gui.Theme().Font
	fe := font.FontExtents()
	g.SetFont(font)

	// Draw using category layout
	for _, entry := range this.categoryLayout {
		if entry.isHeader {
			this.drawCategoryHeader(g, entry.category, entry.ypos, w, fe)
		} else if entry.item != nil {
			p := entry.item
			labelRect := this.getItemRect(p, pitemLabel)
			yt := labelRect.Y + fe.Ascent + (p.ItemHeight()-fe.Height)*0.5
			xt := labelRect.X
			g.SetBrush1(gui.Theme().TextColor)
			g.SetFont(font)
			g.DrawText1(xt, yt, p.Label())

			if isColorProperty(p) {
				this.drawColorSwatch(g, p, p.ItemHeight())
			}
		}
	}

	// If no category layout (empty), fall back to flat rendering
	if len(this.categoryLayout) == 0 {
		g.SetBrush1(gui.Theme().TextColor)
		for _, p := range this.rlist {
			labelRect := this.getItemRect(p, pitemLabel)
			yt := labelRect.Y + fe.Ascent + (p.ItemHeight()-fe.Height)*0.5
			xt := labelRect.X
			g.DrawText1(xt, yt, p.Label())
			if isColorProperty(p) {
				this.drawColorSwatch(g, p, p.ItemHeight())
			}
		}
	}
}

// drawCategoryHeader renders a collapsible category header row.
func (this *PropertySheet) drawCategoryHeader(g paint.Painter, catKey string, ypos, width float64, fe *paint.FontExtents) {
	cat := this.categories[catKey]
	if cat == nil {
		return
	}

	// Background
	headerBG := paint.Color{R: 70, G: 70, B: 85, A: 255}
	if gui.CurrentThemeMode() == gui.ThemeLight {
		headerBG = paint.Color{R: 210, G: 210, B: 220, A: 255}
	}
	g.SetBrush1(headerBG)
	g.Rectangle(0, ypos, width, categoryHeaderHeight)
	g.Fill()

	// Bottom border
	borderColor := paint.Color{R: 90, G: 90, B: 105, A: 255}
	if gui.CurrentThemeMode() == gui.ThemeLight {
		borderColor = paint.Color{R: 180, G: 180, B: 190, A: 255}
	}
	g.SetPen1(borderColor, 1)
	g.Line(0, ypos+categoryHeaderHeight, width, ypos+categoryHeaderHeight)
	g.Stroke()

	// Expander triangle
	triX := 6.0
	triY := ypos + categoryHeaderHeight/2
	triSize := 4.0
	textColor := paint.Color{R: 220, G: 220, B: 230, A: 255}
	if gui.CurrentThemeMode() == gui.ThemeLight {
		textColor = paint.Color{R: 40, G: 40, B: 50, A: 255}
	}
	g.SetBrush1(textColor)
	if cat.expanded {
		// Downward triangle
		g.MoveTo(triX, triY-triSize*0.5)
		g.LineTo(triX+triSize*2, triY-triSize*0.5)
		g.LineTo(triX+triSize, triY+triSize*0.5)
		g.LineTo(triX, triY-triSize*0.5)
	} else {
		// Rightward triangle
		g.MoveTo(triX, triY-triSize)
		g.LineTo(triX+triSize, triY)
		g.LineTo(triX, triY+triSize)
		g.LineTo(triX, triY-triSize)
	}
	g.Fill()

	// Category name (bold-style: draw twice with slight offset for fake bold)
	g.SetBrush1(textColor)
	yt := ypos + fe.Ascent + (categoryHeaderHeight-fe.Height)*0.5
	g.DrawText1(20, yt, cat.name)
	g.DrawText1(20.5, yt, cat.name) // fake bold offset
}

// OnLeftDown handles mouse clicks on category headers to toggle collapse.
func (this *PropertySheet) OnLeftDown(x, y float64) {
	// Check if click is on a category header
	for _, entry := range this.categoryLayout {
		if entry.isHeader {
			if y >= entry.ypos && y < entry.ypos+categoryHeaderHeight {
				cat := this.categories[entry.category]
				if cat != nil {
					cat.expanded = !cat.expanded
					this.Layout()
					return
				}
			}
		}
	}
}

func (this *PropertySheet) SaveSession() (doc *core.TDoc, err error) {
	doc = core.NewTDoc()
	return doc, nil
}

func (this *PropertySheet) LoadSession(doc *core.TDoc) error {
	return nil
}

func (this *PropertySheet) Class() string {
	return "prop.PropertySheet"
}

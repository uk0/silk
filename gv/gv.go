package gv

import (
	//	"reflect"
	"fmt"
	"github.com/uk0/silk/core"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"
	//"reflect"
)

type IExportGv interface {
	ExportGv(g *Graph)
}

type _Node struct {
	id        string
	Shape     string
	Color     string
	Text      string
	TextColor string

	obj interface{}
}

func (this *_Node) Id() string {
	return this.id
}

//func (this *_Node)

type _Edge0 struct {
	head, tail *_Node
}

type _Edge struct {
	_Edge0
	// 箭头风格, tee,vee,normal, ltee, lvee, lnormal, rtee, rvee, rnormal等
	// 注: 我们不支持设置箭尾风格, 如果需要双箭头请另加一个Edge
	ArrowHead string
	Color     string
	Text      string
	TextColor string

	// 重量表示在布局时的重要程度, 布局器倾向于缩短重的边
	// 默认值5 (Graph.DefaultEdge.Weight 不起作用)
	Weight uint8
}

func (this *_Edge) Head() *_Node {
	return this.head
}

func (this *_Edge) Tail() *_Node {
	return this.tail
}

type Graph struct {
	// 默认的边界属性
	DefaultEdge _Edge
	// 默认的结点属性
	DefaultNode _Node

	objmap map[interface{}]*_Node
	edges  map[_Edge0]*_Edge
	name   string
	lastId int

	AutoTypeInfo bool
	AutoObjInfo  bool
}

func (this *Graph) bind(obj interface{}, p *_Node) (ret bool) {
	defer func() {
		if e := recover(); e != nil {
			ret = false
		}
	}()

	ret = true
	a := obj
	if a != obj {
		return false
	}

	this.objmap[obj] = p
	return
}

func (this *Graph) getNode(obj interface{}) *_Node {
	if this.objmap == nil {
		this.objmap = make(map[interface{}]*_Node)
		return nil
	}
	p, _ := this.objmap[obj]
	return p
}

func (this *Graph) IsNodeAdded(obj interface{}) bool {
	if core.IsNil(obj) {
		return false
	}

	if _, ok := obj.(*_Node); ok {
		return true
	}

	p := this.getNode(obj)
	return p != nil
}

// 此函数返回obj对应的结点, 如果obj为nil则返回nil, 如果obj不为nil但结点不存在则创建
func (this *Graph) Node(obj interface{}) *_Node {
	if core.IsNil(obj) {
		return nil
	}

	if _, ok := obj.(*_Edge); ok {
		core.Warn("*_Edge could not use as gv node.")
		return nil
	}

	if node, ok := obj.(*_Node); ok {
		return node
	}

	p := this.getNode(obj)
	if p != nil {
		return p
	}
	p = new(_Node)
	this.lastId++
	p.id = fmt.Sprintf("_%d", this.lastId)
	if !this.bind(obj, p) {
		return nil
	}
	p.obj = obj
	if ia, ok := obj.(IExportGv); ok {
		ia.ExportGv(this)
	}
	return p
}

func (this *Graph) getEdge(key _Edge0) *_Edge {
	if this.edges == nil {
		this.edges = make(map[_Edge0]*_Edge)
		return nil
	}
	p, _ := this.edges[key]
	return p
}

func (this *Graph) IsEdgeAdded(tailObj, headObj interface{}) bool {
	if !this.IsNodeAdded(tailObj) || !this.IsNodeAdded(headObj) {
		return false
	}
	key := _Edge0{
		tail: this.Node(tailObj),
		head: this.Node(headObj),
	}
	p := this.getEdge(key)
	return p != nil
}

// 获取/添加边, 其中正方向(tail --> head) 默认有箭头
// 参数可以是 *_Node, 也可以是外部对象, 如果是外部对象则自动生成 *_Node
func (this *Graph) Edge(tailObj, headObj interface{}) *_Edge {
	return this.edge1(this.Node(tailObj), this.Node(headObj))
}

func (this *Graph) edge1(tail, head *_Node) *_Edge {
	key := _Edge0{
		tail: tail,
		head: head,
	}

	p := this.getEdge(key)
	if p != nil {
		return p
	}

	p = new(_Edge)

	if this.DefaultEdge.Weight == 0 {
		this.DefaultEdge.Weight = 5
	}
	p.Weight = this.DefaultEdge.Weight
	p._Edge0 = key
	this.edges[key] = p
	return p
}

func (this *Graph) Name() string {
	if this.name == "" {
		return "Graph"
	}
	return this.name
}

func (this *Graph) SetName(s string) {
	this.name = core.ToValidFileName(s)
}

func (this *_Node) write(w io.Writer, g *Graph) {
	fmt.Fprint(w, this.Id(), " [ ")
	this.writeAttr(w, g)
	fmt.Fprintln(w, " ]")
}

func (this *_Node) writeAttr(w io.Writer, g *Graph) {
	if this.Shape != "" {
		fmt.Fprint(w, " shape=", this.Shape)
	}
	if this.Color != "" {
		fmt.Fprint(w, " color=", this.Color)
	}
	if this.TextColor != "" {
		fmt.Fprint(w, " fontcolor=", this.TextColor)
	}

	s := this.Text
	if this.obj != nil {
		// 不是nil结点
		if g.AutoTypeInfo {
			if s != "" {
				s += "\n"
			}
			s += core.TypeInfo(this.obj)
		}

		if g.AutoObjInfo {
			if s != "" {
				s += "\n"
			}
			s += core.ObjInfo(this.obj)
		}
	}

	if s != "" {
		fmt.Fprint(w, " label=", strconv.Quote(s))
	}
}

func (this *_Edge) write(w io.Writer, g *Graph) {
	if this.head.obj == nil {
		this.head.write(w, g)
	}
	if this.tail.obj == nil {
		this.tail.write(w, g)
	}
	fmt.Fprint(w, this.tail.id, " -> ", this.head.id, " [")
	this.writeAttr(w, g)
	fmt.Fprintln(w, " ]")
}

func (this *_Edge) writeAttr(w io.Writer, g *Graph) {
	if this.ArrowHead != "" {
		fmt.Fprint(w, " arrowhead=", this.ArrowHead)
	}

	if this.Color != "" {
		fmt.Fprint(w, " color=", this.Color)
	}
	if this.TextColor != "" {
		fmt.Fprint(w, " fontcolor=", this.TextColor)
	}

	if this.ArrowHead != "" {
		fmt.Fprint(w, " arrowhead=", this.ArrowHead)
	}

	if this != &g.DefaultEdge {
		fmt.Fprint(w, " weight=", this.Weight)
	}

	s := this.Text

	if s != "" {
		fmt.Fprint(w, " label=", strconv.Quote(s))
	}

}

// 输出
func (this *Graph) Write(w io.Writer) {
	// digraph 是有向图
	fmt.Fprintln(w, "digraph "+this.Name()+" {")

	fmt.Fprint(w, "Node [")
	this.DefaultNode.writeAttr(w, this)
	fmt.Fprintln(w, " ]")

	fmt.Fprint(w, "Edge [")
	this.DefaultEdge.writeAttr(w, this)
	fmt.Fprintln(w, " ]")

	for _, node := range this.objmap {
		node.write(w, this)
	}

	for _, edge := range this.edges {
		if edge.head == nil && edge.tail == nil {
			continue
		}

		if edge.head == nil {
			edge.head = new(_Node)
			this.lastId++
			edge.head.id = fmt.Sprintf("_nil_%d", this.lastId)
			edge.head.Text = "nil"
			edge.head.Shape = "none"
		} else if edge.tail == nil {
			edge.tail = new(_Node)
			this.lastId++
			edge.tail.id = fmt.Sprintf("_nil_%d", this.lastId)
			edge.tail.Text = "nil"
			edge.tail.Shape = "none"
		}
		edge.write(w, this)
	}
	fmt.Fprintln(w, "}")
}

func (this *Graph) WriteFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	this.Write(file)
	return nil
}

// 运行dot工具, 把.gv转为图片
func runDot(gvPath, outPath, imgFormat string) error {
	if imgFormat == "" {
		imgFormat = "png"
	}
	dotPath, err := exec.LookPath("dot")
	if err != nil {
		dotPath0 := core.DbgToolDir() + "/dot.exe"
		dotPath, err = exec.LookPath(dotPath0)
		if err != nil {
			core.Debug(`"dot" not found: `, dotPath0)
			return err
		}
	}
	os.Remove(outPath)
	cmd := exec.Command(dotPath, "-T"+imgFormat, `-o`+outPath, gvPath)
	core.Debug(cmd.Args)
	msg, err := cmd.CombinedOutput()
	if err != nil {
		core.Debug(`failed to run "dot": `, err.Error())
		core.Debug(string(msg))
		return err
	}

	return nil
}

// 运行dot工具, 生成图片
func (this *Graph) GenDotOutput(outPath, imgFormat string) error {
	gvPath := core.TempDir() + "/gv_" + this.Name() + time.Now().Format("_01-02-15-04-05.gv")
	err := this.WriteFile(gvPath)
	if err != nil {
		return err
	}
	return runDot(gvPath, outPath, imgFormat)
}

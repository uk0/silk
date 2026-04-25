package data

type Category struct {
	cid       string
	cname     string
	underWell bool
}

func (this *Category) DataID() string {
	return this.cid
}

func (this *Category) DataName() string {
	return this.cname
}

// 井下的分类, 子节点应该是数据
// 不在井下的分类, 子节点应该是井, 再下一级才是数据
func (this *Category) IsUnderWell() bool {
	return this.underWell
}

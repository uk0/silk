package data

type Well struct {
	wid   string
	wname string

	underCategory bool
}

func (this *Well) DataID() string {
	return this.WellID()
}

func (this *Well) DataName() string {
	return this.WellName()
}

func (this *Well) WellID() string {
	return this.wid
}

func (this *Well) WellName() string {
	return this.wname
}

// 分类下的井, 子节点应该是数据
// 不在分类下的井, 子节点应该是分类, 再下一级才是数据
func (this *Well) IsUnderWell() bool {
	return this.underCategory
}

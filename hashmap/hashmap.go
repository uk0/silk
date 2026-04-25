//可以指定比较函数的HashMap.
//go语言内置的map要求key类型支持==和!=运算符, 有时候不能满足我们的要求.
package hashmap

type Node struct {
	first  interface{}
	second interface{}
	next   *Node
}

type HashMap struct {
	table      []*Node
	hash_mask  uint32
	grow_line  uint32
	count      uint32
	table_size uint32
	hashFunc   HashFunc
	equalFunc  EqualFunc
}

type HashFunc func(interface{}) uint32
type EqualFunc func(a, b interface{}) bool

func NewHashMap(hashFunc HashFunc, equalFunc EqualFunc) *HashMap {
	p := new(HashMap)
	p.Init(hashFunc, equalFunc)
	return p
}

func (this *HashMap) Init(hashFunc HashFunc, equalFunc EqualFunc) {
	this.count = 0
	this.table_size = 8
	this.grow_line = this.table_size * 3 / 4
	this.hash_mask = this.table_size - 1
	this.table = make([]*Node, this.table_size, this.table_size)
	this.hashFunc = hashFunc
	this.equalFunc = equalFunc
}

func (this *HashMap) Insert(k, v interface{}) (*Node, bool) {
	index := this.hashFunc(k) & this.hash_mask

	p := this.table[index]
	for p != nil {
		if this.equalFunc(p.first, k) {
			return p, false
		}
		p = p.next
	}

	p = &Node{k, v, this.table[index]}
	this.table[index] = p

	this.count++
	if this.count == this.grow_line {
		this.grow()
	}

	return p, true
}

func (this *HashMap) Len() int {
	return int(this.count)
}

func (this *HashMap) grow() {
	old_table := this.table
	old_table_size := this.table_size

	this.table_size *= 4
	this.grow_line = this.table_size * 3 / 4
	this.table = make([]*Node, this.table_size, this.table_size)
	this.hash_mask = this.table_size - 1

	for i := uint32(0); i < old_table_size; i++ {
		p := old_table[i]
		for p != nil {
			old := p
			p = p.next
			old.next = nil
			index := this.hashFunc(old.first) & this.hash_mask

			b := this.table[index]
			if b != nil {
				for b.next != nil {
					b = b.next
				}
				b.next = old
			} else {
				this.table[index] = old
			}
		}
	}
}

func (this *HashMap) Find(k interface{}) (interface{}, bool) {
	_, p := this.find(k)
	if p != nil {
		return p.second, true
	}
	return nil, false
}

func (this *HashMap) Delete(k interface{}) (interface{}, bool) {
	i, p := this.find(k)
	if p != nil {
		this.erase(i, p)
		return p.second, true
	}
	return nil, false
}

func (this *HashMap) find(k interface{}) (int, *Node) {
	index := this.hashFunc(k) & this.hash_mask
	p := this.table[index]
	for p != nil {
		if this.equalFunc(p.first, k) {
			return int(index), p
		}
		p = p.next
	}
	return 0, nil
}

func (this *HashMap) erase(i int, p *Node) {
	if this.table[i] == p {
		this.table[i] = p.next
	} else {
		p1 := this.table[i]
		for p1.next != p {
			p1 = p1.next
		}
		p1.next = p.next
	}
	this.count--
}

package core

import (
	//	"github.com/uk0/silk/core"
	//	"silk/factory"
	//	"io/ioutil"
	"os"
	"testing"
)

type Struct1 struct {
	field1 *Struct2
	field2 *Struct1
	field3 string
}

type Struct2 struct {
	field1 interface{}
}

func (a *Struct1) OnPersistSave(p IPersistSaver) {
	p.Write("f1", a.field1)
	p.Write("f2", a.field2)
	p.Write("f3", a.field3)
}

func (a *Struct1) OnPersistLoad(p IPersistLoader) {
	p.Read("f1", &a.field1)
	p.Read("f2", &a.field2)
	p.Read("f3", &a.field3)
}

func (a *Struct2) OnPersistSave(p IPersistSaver) {
	p.Write("f1", a.field1)
}

func (a *Struct2) OnPersistLoad(p IPersistLoader) {
	p.Read("f1", &a.field1)
}

func testAppend(t *testing.T, a interface{}) {
	buf, err := appendPersistVal(nil, a)
	if err != nil {
		t.Error(err)
	}
	t.Log(string(buf))
}

func TestConvert(t *testing.T) {
	testAppend(t, []string{"aaa", "bb\tb", "cc,c", "dd\r\nd", "ee]e", "ff\"f", "gg g", "中文"})
	testAppend(t, []int{1, -3, 5, -7, 9})
	testAppend(t, []byte{128, 127, 255, 96, 32})
	testAppend(t, []uint{1, 3, 5, 7, 9})
	testAppend(t, []float64{1.0 / 1.0, 1.0 / -3.0, 1.0 / 5.0, 1.0 / -7.0, 1.0 / 9.0})
	testAppend(t, []float32{1.0 / 1.0, 1.0 / -3.0, 1.0 / 5.0, 1.0 / -7.0, 1.0 / 9.0})
	testAppend(t, []bool{true, false, true})
	//testAppend(t, map[int]string{})
	testAppend(t, 2)
	testAppend(t, 2.1)
	var a int = 10
	testAppend(t, []interface{}{1, "aa", nil, &a})
}

func TestPersist(t *testing.T) {
	//DebugOn()
	a := &Struct1{}
	b := &Struct2{a}
	a.field1 = b
	a.field3 = "test\ttest\n\rtest test"
	//b.field1 = []string{"aa]bb[c]=d", "eee"}
	p := a
	for i := 0; i < 50000; i++ {
		p1 := &Struct1{}
		p.field2 = p1
		p = p1
	}

	tmpdir := os.TempDir() + "/silk_test_persist"
	os.RemoveAll(tmpdir)
	err := os.Mkdir(tmpdir, 0755)
	if err != nil {
		t.Fatal("Failed to create temp dir: ", err)
	}
	err = PersistSaveFile(a, false, tmpdir+"/test.cml")
	if err != nil {
		t.Fatal("SaveToTDocFile() failed: ", err)
	}
	o, err := PersistLoadFile(tmpdir + "/test.cml")
	if err != nil {
		t.Fatal("LoadFromTDocFile() failed: ", err)
	}
	a1, ok := o.(*Struct1)
	if !ok {
		t.Fatal("Loaded objects mismatch oringinal.")
	}

	err = PersistSaveFile(a1, false, tmpdir+"/test1.cml")
	if err != nil {
		t.Fatal("SaveToTDocFile() failed: ", err)
	}

}

func init() {
	RegisterFactory("pt.Struct1", TypeOf(Struct1{}))
	RegisterFactory("pt.Struct2", TypeOf(Struct2{}))
}

package core

import (
	//	"github.com/uk0/silk/core"
	//	"silk/factory"
	//	"io/ioutil"
	"os"
	"testing"
)

type S struct {
	A int
	B float64
	C string
	D []string
	E [3]*S
	F map[string]int
}

func TestTDocMarshal(t *testing.T) {
	p := &S{A: 123,
		B: 45.6,
		C: "test",
		D: []string{"aaa", "bbb", "ccc"},
		E: [3]*S{},
		F: map[string]int{"xxx": 12, "yyy": 34}}

	s := S{A: 123,
		B: 45.6,
		C: "test",
		D: []string{"aaa", "bbb", "ccc"},
		E: [3]*S{p, nil, p},
		F: map[string]int{"xxx": 12, "yyy": 34}}

	tmpdir := os.TempDir() + "/silk_test_tdoc"
	os.RemoveAll(tmpdir)
	err := os.Mkdir(tmpdir, 0755)
	if err != nil {
		t.Fatal("Failed to create temp dir: ", err)
	}
	doc, err := TDocMarshal(&s)
	if err != nil {
		t.Fatal("TDocMarshal() failed: ", err)
	}

	err = doc.SaveFile(tmpdir + "/test.cml")
	if err != nil {
		t.Fatal("failed to save file: ", err)
	}
	var x S
	err = doc.Unmarshal(&x)
	if err != nil {
		t.Fatal("failed to save file: ", err)
	}

	//	tmpdir := os.TempDir() + "/silk_test_tdoc"
	//os.RemoveAll(tmpdir)
	//err = os.Mkdir(tmpdir, os.ModeDir)
	//if err != nil {
	//	t.Fatal("Failed to create temp dir: ", err)
	//}
	doc1, err := TDocMarshal(&x)
	if err != nil {
		t.Fatal("TDocMarshal() failed: ", err)
	}

	err = doc1.SaveFile(tmpdir + "/test1.cml")
	if err != nil {
		t.Fatal("failed to save file: ", err)
	}

}

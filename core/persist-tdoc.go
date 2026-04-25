package core

import (
	//	"bytes"
	//	"silk/core"
	//	"fmt"
	//	"log"
	"os"
	//	"reflect"
	"reflect"
	"strconv"
)

// 持久化的数据, 用以和普通*TDoc区分
type PersistData *TDoc

type TDocPersist struct {
	isSave bool
	doc    *TDoc
	cur    *TDoc
	idmap  map[IPersist]int
	objs   []IPersist
}

func (t *TDocPersist) header() *TDoc {
	return t.doc.Child(0)
}

func (t *TDocPersist) addObject(v interface{}) objId {
	p, _ := v.(IPersist)
	//if IsNilRef(p) {
	//	Warn(reflect.TypeOf(v).String(), ` dosn't implement IPersist`)
	//	return objId(0)
	//}

	id, ok := t.idmap[p]
	if ok {
		return objId(id)
	}
	id = len(t.objs)
	//Info("p=", p)
	t.objs = append(t.objs, p)
	t.idmap[p] = id
	return objId(id)

}

func (t *TDocPersist) getId(p IPersist) int {
	return t.idmap[p]
}

func (t *TDocPersist) getObject(id objId) IPersist {
	if int(id) >= 0 && int(id) < len(t.objs) {
		return t.objs[id]
	}
	return nil
}

func (t *TDocPersist) Enter(key string) bool {
	sub := t.cur.ChildByKey(key, t.isSave)
	if sub != nil {
		t.cur = sub
		return true
	}
	return false
}

func (t *TDocPersist) Leave() {
	t.cur = t.cur.Parent()
}

func (t *TDocPersist) save(ro interface{}) (PersistData, error) {
	t.isSave = true
	t.doc = new(TDoc)
	t.cur = t.doc
	t.idmap = make(map[IPersist]int, 0)
	head := new(TDoc)
	head.SetKey("0")
	t.doc.AddChild(head)

	t.objs = append(t.objs, nil)
	t.idmap[nil] = 0

	t.addObject(ro)

	for i := 1; i < len(t.objs); i++ {
		o := t.objs[i]
		//Info(o)
		t.cur = new(TDoc)
		t.cur.SetKey(strconv.Itoa(i))
		factory := FactoryOf(o)
		if factory == nil {
			return nil, StrErr(`factory not found for GoLang type "` + TypeInfo(o) + `"`)
		}
		t.cur.SetValue(factory.Name())
		t.doc.AddChild(t.cur)
		o.OnPersistSave(t)
	}

	return t.doc, nil
}

func (t *TDocPersist) load(doc *TDoc) (IPersist, error) {

	if doc == nil {
		return nil, StrErr("try loading from empty doc")
	}

	t.isSave = false
	t.doc = doc
	t.objs = append(t.objs, nil)
	t.idmap = make(map[IPersist]int, 0)
	t.idmap[nil] = 0

	count := t.doc.Len()
	for i := 1; i < count; i++ {
		t.cur = t.doc.Child(i)
		var factoryName string
		err := t.cur.Value(&factoryName)
		if err != nil {
			return nil, err
		}
		o := New(factoryName)
		if o == nil {
			return nil, StrErr(`failed to create "` + factoryName + `" object`)
		}
		s, _ := o.(IPersist)
		t.objs = append(t.objs, s)
		t.idmap[s] = i
	}

	for id := 1; id < count; id++ {
		s := t.objs[id]
		t.cur = doc.Child(id)
		s.OnPersistLoad(t)
	}

	if len(t.objs) > 1 {
		return t.objs[1], nil
	}
	return nil, StrErr("try loading from empty doc")
}

func PersistSave(ro interface{}) (doc PersistData, err error) {
	t := new(TDocPersist)
	return t.save(ro)
}

func PersistSaveFile(ro interface{}, compress bool, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	t, err := PersistSave(ro)
	if err != nil {
		return err
	}
	return (*TDoc)(t).Save1(file, compress)
}

func PersistLoad(doc PersistData) (IPersist, error) {
	t := new(TDocPersist)
	return t.load(doc)
}

func PersistLoadFile(path string) (IPersist, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	t, err := LoadTDoc(file)
	if err != nil {
		return nil, err
	}
	return PersistLoad(t)
}

func (t *TDocPersist) Write(key string, v interface{}) error {
	switch x := v.(type) {
	case IPersist:
		if reflect.ValueOf(x).IsNil() {
			return t.cur.WriteAttr(key, nil)
		}
		id := t.addObject(x)
		return t.cur.WriteAttr(key, id)
	default:
		return t.cur.WriteAttr(key, v)
	}
}

func (t *TDocPersist) Read(key string, ptr interface{}) error {
	v := reflect.ValueOf(ptr) // **Obj, *IObj or *Data
	if v.Kind() != reflect.Ptr {
		panic("pointer expected")
	}
	if v.IsNil() {
		panic("unexpected nil pointer")
	}

	// *Obj, IObj or Data
	switch ev := v.Elem(); ev.Kind() {
	case reflect.Interface:
		fallthrough
	case reflect.Ptr:
		var id objId
		err := t.cur.ReadAttr(key, &id)
		if err != nil {
			ev.Set(reflect.New(ev.Type()).Elem())
			return StrErr(`value of field "` + t.cur.KeyPath(nil) + `/` + key +
				`" is not an object id`)
		}
		obj := t.getObject(id)
		if obj == nil {
			ev.Set(reflect.New(ev.Type()).Elem())
		} else {
			ev.Set(reflect.ValueOf(obj))
		}
		return nil
	default:
		err := t.cur.ReadAttr(key, ptr)
		if err != nil {
			return StrErr(`failed on field "` + t.cur.KeyPath(nil) + `/` + key +
				`": ` + err.Error())
		}
		return nil
	}

}

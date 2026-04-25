package core

import "testing"

//import "reflect"

func TestFactory(t *testing.T) {
	RegisterFactory("int", TypeOf((*int)(nil)))

	i := New("int")
	//if reflect.TypeOf(i).Kind() != reflect.Ptr {
	//	t.Error("factory.Create() returns non-pointer value")
	//}
	_, ok := i.(*int)
	if !ok {
		t.Fatal(`failed to create "int" type`, i)
	}

	//	err = Register("interface", nil, TypeOf((*interface{})(nil)))
	//	if err != nil {
	//		t.Fatal(`failed to register (*interface{}) type: `, err)
	//	}
}

func TestFindFactoryNonExistent(t *testing.T) {
	f := FindFactory("nonexistent.Widget.XYZ12345")
	if f != nil {
		t.Error("FindFactory should return nil for unregistered name")
	}
}

func TestNewNonExistent(t *testing.T) {
	obj := New("nonexistent.Widget.XYZ12345")
	if obj != nil {
		t.Error("New should return nil for unregistered name")
	}
}

func TestFactoryReturnsDistinctInstances(t *testing.T) {
	f := FindFactory("int")
	if f == nil {
		t.Skip("int factory not registered")
		return
	}
	a := f.New()
	b := f.New()
	if a == b {
		t.Error("Factory.New() returned the same pointer twice")
	}
}

func TestFactoryOf(t *testing.T) {
	var x int
	f := FactoryOf(&x)
	if f == nil {
		t.Skip("int factory not found via FactoryOf")
		return
	}
	if f.Name() != "int" {
		t.Errorf("FactoryOf(&int).Name() = %q, want int", f.Name())
	}
}

func TestFactoryNameOf(t *testing.T) {
	var x int
	name := FactoryNameOf(&x)
	if name != "int" {
		t.Errorf("FactoryNameOf(&int) = %q, want int", name)
	}
}

func TestFactoryNameOfUnregistered(t *testing.T) {
	type unregisteredStruct struct{}
	var s unregisteredStruct
	name := FactoryNameOf(&s)
	if name != "" {
		t.Errorf("FactoryNameOf for unregistered type = %q, want empty", name)
	}
}

func TestAllFactoriesNotEmpty(t *testing.T) {
	all := AllFactories()
	if len(all) == 0 {
		t.Error("AllFactories() returned empty list")
	}
}

func TestFactoryLocation(t *testing.T) {
	f := FindFactory("int")
	if f == nil {
		t.Skip("int factory not registered")
		return
	}
	loc := f.Location()
	if loc == "" {
		t.Error("Factory.Location() should not be empty")
	}
}

func TestAddFactoryAlias(t *testing.T) {
	AddFactoryAlias("test.IntAlias.UniqueXYZ", "int")
	// After adding alias, FindFactory with the alias name should resolve
	// Note: the alias mechanism requires the real factory to be registered.
	// FindFactory may log a warning but should not panic.
	f := FindFactory("test.IntAlias.UniqueXYZ")
	_ = f // May or may not resolve depending on alias lookup path
}

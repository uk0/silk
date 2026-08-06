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

// aliasTarget owns its own factory so this test does not disturb the "int"
// registration TestFactoryOf/TestFactoryNameOf assert on. The field only keeps
// the type non-empty, so two instances get distinct addresses.
type aliasTarget struct{ n int }

// TestAddFactoryAlias holds the alias mechanism to what it promises: a name
// registered as an alias produces the aliased factory's objects. The lookup
// used to re-read the alias name out of the factory map instead of the real
// name it maps to — a read that had already missed one line earlier — so every
// alias resolved to nil, New(alias) handed back nil, and the warning printed
// blamed the real factory for not being registered when it was. Documents
// renamed across versions load through this path, so the failure is a widget
// silently dropped from a form with a misleading line in the log.
func TestAddFactoryAlias(t *testing.T) {
	const real = "test.AliasTarget.UniqueXYZ"
	const alias = "test.AliasTargetAlias.UniqueXYZ"
	if FindFactory(real) == nil {
		RegisterFactory(real, TypeOf(aliasTarget{}))
	}
	AddFactoryAlias(alias, real)

	f := FindFactory(alias)
	if f == nil {
		t.Fatalf("FindFactory(%q) = nil; the alias of the registered factory %q did not resolve", alias, real)
	}
	if f.Name() != real {
		t.Errorf("FindFactory(%q).Name() = %q, want %q", alias, f.Name(), real)
	}
	if _, ok := New(alias).(*aliasTarget); !ok {
		t.Errorf("New(%q) = %T, want *aliasTarget", alias, New(alias))
	}
}

// TestFindFactoryAliasOfMissingTarget keeps the failure path a nil rather than
// a stale hit: an alias whose target was never registered must not resolve.
func TestFindFactoryAliasOfMissingTarget(t *testing.T) {
	const alias = "test.DanglingAlias.UniqueXYZ"
	AddFactoryAlias(alias, "test.NeverRegistered.UniqueXYZ")
	if f := FindFactory(alias); f != nil {
		t.Errorf("FindFactory(%q) = %v, want nil for an alias of an unregistered factory", alias, f)
	}
}

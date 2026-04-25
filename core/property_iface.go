package core

// IPropertyList is the minimal interface for property enumeration.
// Widgets call AddProperty to expose their configurable properties.
type IPropertyList interface {
	AddProperty(id string, get, set interface{})
}

// IEnumProperties is implemented by objects that expose configurable
// properties to a property sheet or inspector.
type IEnumProperties interface {
	EnumProperties(list IPropertyList)
}

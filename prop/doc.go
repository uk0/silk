// Package prop provides a reflection-based property editing system.
//
// Widgets implement IEnumProperties to expose their configurable properties.
// The PropertySheet widget displays these properties with auto-generated
// editors (text fields, checkboxes, etc.) for live editing.
//
// Key types:
//   - IEnumProperties: interface for exposing editable properties
//   - IPropertyList: interface for collecting property definitions
//   - PropertySheet: UI widget that displays and edits object properties
//   - IPropertyControl: interface for custom property editor widgets
package prop

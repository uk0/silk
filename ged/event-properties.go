package ged

import (
	"go/token"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/prop"
)

// enumEventProperties adds the property sheet's "事件" rows: one per event
// codegen can bind for this widget's type, whose value is the handler function
// name. The row set comes from codegen's own table (AvailableEvents), never
// from a list maintained beside it — an event the sheet accepts but the
// generator drops binds nothing at runtime while the sheet claims otherwise.
func (this *FakeWidget) enumEventProperties(list prop.IPropertyList) {
	for _, evt := range AvailableEvents(this.factoryName) {
		evt := evt
		list.AddProperty(evt,
			func() string { return this.eventHandlers[evt] },
			func(handler string) { this.SetEventHandlerChecked(evt, handler) })
	}
}

// SetEventHandlerChecked applies an edit of one event binding: a blank value
// unbinds the event, a Go identifier binds it, and anything else is rejected.
// The name is pasted verbatim into generated code as a func call, so a name
// that is not an identifier yields a design that cannot generate compilable
// output — and the failure would only surface at build time, far from the edit
// that caused it.
func (this *FakeWidget) SetEventHandlerChecked(event, handler string) {
	handler = strings.TrimSpace(handler)
	if handler == "" {
		this.RemoveEventHandler(event)
		return
	}
	if !token.IsIdentifier(handler) {
		core.Warn(`event handler "` + handler + `" for ` + event + ` is not a Go identifier; binding unchanged`)
		return
	}
	this.SetEventHandler(event, handler)
}

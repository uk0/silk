//go:build !windows

package gui

import (
	"testing"

	"github.com/uk0/silk/paint"
)

func testClientLayer(host, owner *Window, widget IWidget, x, y float64) *Window {
	widget.SetSize(120, 88)
	widget.NakedWidget().setVisible(true)
	w := &Window{
		wt: WtPopup, widget: widget, enabled: true, popupOwner: owner, clientPopupHost: host,
		clientPopupVisible: true, clientPopupX: x, clientPopupY: y,
	}
	widget.NakedWidget().wt = WtPopup
	widget.NakedWidget().win = w
	host.clientPopupLayers = append(host.clientPopupLayers, w)
	return w
}

func TestGLFWClientPopupUsesTopmostLocalCoordinates(t *testing.T) {
	host := &Window{wt: WtForm, enabled: true, widget: NewForm()}
	base := testClientLayer(host, host, NewPopupMenu(), 38, 24)
	nested := testClientLayer(host, base, NewPopupMenu(), 72, 53)
	tip := testClientLayer(host, base, newTooltipWindow("pass-through tooltip"), 72, 53)
	if got := host.clientPopupAt(80, 66); got != nested {
		t.Fatal("upper interactive menu was not the hit target")
	}
	if got := host.clientPopupAt(43, 26); got != base {
		t.Fatal("remaining area of the parent menu did not remain interactive")
	}
	x, y := nested.mapFromGlobal(89.5, 71)
	if x != 17.5 || y != 18 {
		t.Fatalf("translated coordinates %v, %v", x, y)
	}
	if gx, gy := nested.mapToGlobal(x, y); gx != 89.5 || gy != 71 {
		t.Fatalf("round trip %v, %v", gx, gy)
	}
	nested.unlinkClientPopup()
	if !host.fullDirty || len(host.clientPopupLayers) != 2 || host.clientPopupLayers[1] != tip ||
		host.clientPopupAt(80, 66) != base || nested.clientPopupVisible {
		t.Fatal("closing a layer did not repaint its old area or restore picking")
	}
	host.enabled = false
	if got := host.clientPopupAt(80, 66); got != nil {
		t.Fatal("transient layer was interactive over a disabled host")
	}
}

type clientPopupFill struct{ Widget }

func (w *clientPopupFill) Draw(g paint.Painter) {
	g.SetBrush1(paint.Color{R: 18, G: 140, B: 220, A: 255})
	g.Rectangle(0, 0, w.Width(), w.Height())
	g.Fill()
}

func TestGLFWClientPopupPaintClipsInHostCoordinates(t *testing.T) {
	host := &Window{wt: WtForm, widget: NewForm(), enabled: true}
	popup := new(clientPopupFill)
	popup.Init(popup)
	testClientLayer(host, host, popup, 289, 182)
	pix := paint.NewPixmap(450, 320)
	defer paint.CloseSurface(pix)
	g := pix.NewPainter()
	defer paint.ClosePainter(g)
	host.drawClientPopups(g, 390, 270) // the last 19px must be clipped
	pix.Flush()
	img, err := pix.Image()
	if err != nil {
		t.Fatal(err)
	}
	r, _, _, a := img.At(301, 198).RGBA()
	if r>>8 != 18 || a != 65535 {
		t.Fatalf("offset popup not painted inside its host: %d/%d", r, a)
	}
	for i, test := range [][2]int{{10, 10}, {395, 198}, {301, 275}} {
		if _, _, _, a := img.At(test[0], test[1]).RGBA(); a != 0 {
			t.Fatalf("outside pixel %d was painted: alpha %d", i, a)
		}
	}
}

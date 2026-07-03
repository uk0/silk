package main

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"strconv"
	"strings"
)

func main() {
	form := gui.NewForm()
	form.SetTitle("Silk Calculator")

	// Display
	display := gui.NewEdit()
	display.SetText("0")
	display.SetParent(form)
	display.SetBounds(10, 10, 220, 30)
	display.SetReadOnly(true)
	display.SetFont(paint.NewFont("Menlo", 18, false, false))

	// Calculator state
	var currentVal float64
	var pendingOp string
	var newInput bool = true

	appendDigit := func(d string) func() {
		return func() {
			if newInput {
				display.SetText(d)
				newInput = false
			} else {
				display.SetText(display.Text() + d)
			}
		}
	}

	doOp := func(op string) func() {
		return func() {
			v, _ := strconv.ParseFloat(display.Text(), 64)
			switch pendingOp {
			case "+":
				currentVal += v
			case "-":
				currentVal -= v
			case "*":
				currentVal *= v
			case "/":
				if v != 0 {
					currentVal /= v
				}
			default:
				currentVal = v
			}
			pendingOp = op
			newInput = true
			if op == "=" {
				display.SetText(fmt.Sprintf("%g", currentVal))
				pendingOp = ""
			}
		}
	}

	// Create button grid
	buttons := [][]string{
		{"7", "8", "9", "/"},
		{"4", "5", "6", "*"},
		{"1", "2", "3", "-"},
		{"0", ".", "=", "+"},
	}

	for row, btns := range buttons {
		for col, label := range btns {
			btn := gui.NewButton1(label, nil)
			btn.SetParent(form)
			btn.SetBounds(float64(10+col*57), float64(50+row*40), 52, 35)
			switch label {
			case "+", "-", "*", "/", "=":
				btn.Action().BindFunc0(doOp(label))
			case ".":
				btn.Action().BindFunc0(func() {
					if !strings.Contains(display.Text(), ".") {
						display.SetText(display.Text() + ".")
						newInput = false
					}
				})
			default:
				btn.Action().BindFunc0(appendDigit(label))
			}
		}
	}

	// Clear button
	btnC := gui.NewButton1("C", nil)
	btnC.SetParent(form)
	btnC.SetBounds(10, 210, 220, 35)
	btnC.Action().BindFunc0(func() {
		display.SetText("0")
		currentVal = 0
		pendingOp = ""
		newInput = true
	})

	form.AttachWindow(gui.WtForm)
	form.Window().SetSize(240, 260)
	form.Window().MoveToCenter()
	form.Show()
	core.EventLoop()
}

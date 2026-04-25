package gui

import "testing"

func BenchmarkButtonCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewButton1("Test", nil)
	}
}

func BenchmarkLabelCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewLabel("Test")
	}
}

func BenchmarkCheckBoxCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewCheckBox()
	}
}

func BenchmarkSliderCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewSlider(0, 100)
	}
}

func BenchmarkHBoxLayout(b *testing.B) {
	box := NewHBox()
	box.SetSize(800, 50)
	for i := 0; i < 20; i++ {
		box.AddWidget(NewButton1("Btn", nil))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		box.Layout()
	}
}

func BenchmarkVBoxLayout(b *testing.B) {
	box := NewVBox()
	box.SetSize(200, 800)
	for i := 0; i < 20; i++ {
		box.AddWidget(NewLabel("Label"))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		box.Layout()
	}
}

func BenchmarkHBoxSizeHints(b *testing.B) {
	box := NewHBox()
	for i := 0; i < 10; i++ {
		box.AddWidget(NewButton1("Btn", nil))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = box.SizeHints()
	}
}

func BenchmarkVBoxSizeHints(b *testing.B) {
	box := NewVBox()
	for i := 0; i < 10; i++ {
		box.AddWidget(NewLabel("Label"))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = box.SizeHints()
	}
}

func BenchmarkSliderSetValue(b *testing.B) {
	s := NewSlider(0, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.SetValue(float64(i % 1001))
	}
}

func BenchmarkProgressBarSetValue(b *testing.B) {
	p := NewProgressBar()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.SetValue(float64(i%101) / 100.0)
	}
}

func BenchmarkParseSymbols(b *testing.B) {
	e := NewCodeEditor()
	code := `package main

func hello() {}
func (s *Server) Start() {}
type Config struct {}
var version = "1.0"
const maxRetries = 3

func foo() {}
func bar() {}
type Handler struct {}
type Service interface {}
var counter int
const limit = 100
`
	e.SetText(code)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.ParseSymbols()
	}
}

func BenchmarkExpandSnippet(b *testing.B) {
	template := "if ${1:condition} {\n\t${0}\n}"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = expandSnippet(template)
	}
}

func BenchmarkRenameInFile(b *testing.B) {
	text := `func hello() {
	hello()
	helloWorld()
}

func main() {
	hello()
	hello()
}
`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenameInFile(text, "hello", "greet")
	}
}

func BenchmarkNestedLayout(b *testing.B) {
	// Nested VBox > HBox layout
	root := NewVBox()
	root.SetSize(600, 400)
	for row := 0; row < 5; row++ {
		hbox := NewHBox()
		for col := 0; col < 5; col++ {
			hbox.AddWidget(NewButton1("B", nil))
		}
		root.AddWidget(hbox)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		root.Layout()
	}
}

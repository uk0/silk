package data

type FBlock32 interface {
	Data() []float32
	RowCount() int
	ColCount() int
	Get(row, col int) float32
	Set(row, col int, a float32)
}

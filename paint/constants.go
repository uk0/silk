package paint

type Operator int

const (
	OpClear Operator = iota

	OpSource
	OpOver
	OpIn
	OpOut
	OpAtop

	OpDest
	OpDestOver
	OpDestIn
	OpDestOut
	OpDestAtop

	OpXor
	OpAdd
	OpSaturate

	OpMultiply
	OpScreen
	OpOverlay
	OpDarken
	OpLighten
	OpColorDodge
	OpColorBurn
	OpHardLigh
	OpSoftLigh
	OpDifference
	OpExclusion
	OpHslHue
	OpHslSaturate
	OpHslColor
	OpHslLuminosity
)

type Extend int

const (
	ExtNone Extend = iota
	ExtRepeat
	ExtReflect
	ExtPad
)

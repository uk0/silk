package geom

import "fmt"
import "errors"

type Vec2 struct {
	X, Y float64
}

func (a Vec2) String() string {
	return fmt.Sprint("[", a.X, a.Y, "]")
}

func (this *Vec2) Scan(state fmt.ScanState, verb rune) error {
	this.X, this.Y = 0, 0

	state.SkipSpace()
	r, _, _ := state.ReadRune()

	if r != '[' {
		return errors.New("Error in scaning geom.Rect: '[' expected.")
	}

	tok, err := state.Token(true, func(r rune) bool { return r != ']' })
	if err != nil {
		return errors.New("Error in scaning geom.Rect: " + err.Error())
	}

	_, err = fmt.Sscan(string(tok), &this.X, &this.Y)
	if err != nil {
		return errors.New("Error in scaning geom.Rect: " + err.Error())
	}

	state.SkipSpace()
	r, _, _ = state.ReadRune()

	if r != ']' {
		return errors.New("Error in scaning geom.Rect: ']' expected.")
	}

	return nil
}

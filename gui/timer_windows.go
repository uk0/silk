package gui

import (
	"github.com/uk0/silk/win32"
)

var timerMap = make(map[uintptr]func())

// Low precision timer in "UI thread"
type Timer uintptr

func (t *Timer) Stop() {
	if *t != 0 {
		win32.KillTimer(uintptr(*t))
		delete(timerMap, uintptr(*t))
		*t = 0
	}
}

func (t *Timer) Start(millisecond uint32, f func()) bool {
	t.Stop()
	id := win32.SetTimer(millisecond)
	if id != 0 {
		timerMap[id] = f
		*t = Timer(id)
		return true
	}
	return false
}

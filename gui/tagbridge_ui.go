package gui

import (
	"sync"

	"github.com/uk0/silk/core"
)

// BindTagUI delivers the latest sample on the UI thread, coalescing bursts to
// one queued callback. It is for visual state, NOT lossless alarm/event logging.
// Subscribe seeds the initial sample. Cancel is safe from any goroutine and
// invalidates queued work; an already executing setter is allowed to finish.
func BindTagUI(tag *core.Tag, set func(core.Value)) func() {
	var mu sync.Mutex
	var latest core.Value
	pending, stopped := false, false
	cancel := tag.Subscribe(func(v core.Value) {
		mu.Lock()
		if stopped {
			mu.Unlock()
			return
		}
		latest = v
		if pending {
			mu.Unlock()
			return
		}
		pending = true
		mu.Unlock()
		Post(func() {
			mu.Lock()
			v, dead := latest, stopped
			pending = false
			mu.Unlock()
			if !dead {
				set(v)
			}
		})
	})
	var once sync.Once
	return func() { once.Do(func() { mu.Lock(); stopped = true; mu.Unlock(); cancel() }) }
}

package gui

import (
	//	"github.com/uk0/silk/diag"
	//	"github.com/uk0/silk/factory"
	"github.com/uk0/silk/paint"
	"time"
)

var textStyleCache = make(map[string]*TextStyle)
var textStyleCachePurgeTime = time.Now()

// 文本格式, 支持高级文本编辑(未使用)
type TextStyle struct {
	font   paint.Font
	color  paint.Color
	access time.Time
}

func (p *TextStyle) Access() {
	p.access = time.Now()
}

func CachedTextStyle(font paint.Font, color paint.Color) *TextStyle {
	s := font.String() + color.String()
	p, ok := textStyleCache[s]
	if !ok {
		p = new(TextStyle)
		p.font = font
		p.color = color
		textStyleCache[s] = p
	}
	t := time.Now()
	p.access = t
	if t.Sub(textStyleCachePurgeTime) > 5*time.Minute {
		textStyleCachePurgeTime = t
		purgeLine := t.Add(-10 * time.Minute)
		for k, v := range textStyleCache {
			if v.access.Before(purgeLine) {
				delete(textStyleCache, k)
			}
		}
	}
	return p
}

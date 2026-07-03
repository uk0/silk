package gui

import (
	"math"
	"time"
)

// ─── Easing Functions ───

// EaseFunc 缓动函数类型
type EaseFunc func(t float64) float64

// 内置缓动函数

// EaseLinear 线性
func EaseLinear(t float64) float64 { return t }

// EaseInQuad 二次缓入
func EaseInQuad(t float64) float64 { return t * t }

// EaseOutQuad 二次缓出
func EaseOutQuad(t float64) float64 { return t * (2 - t) }

// EaseInOutQuad 二次缓入缓出
func EaseInOutQuad(t float64) float64 {
	if t < 0.5 {
		return 2 * t * t
	}
	return -1 + (4-2*t)*t
}

// EaseInCubic 三次缓入
func EaseInCubic(t float64) float64 { return t * t * t }

// EaseOutCubic 三次缓出
func EaseOutCubic(t float64) float64 {
	t--
	return t*t*t + 1
}

// EaseInOutCubic 三次缓入缓出
func EaseInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	return (t-1)*(2*t-2)*(2*t-2) + 1
}

// EaseInElastic 弹性缓入
func EaseInElastic(t float64) float64 {
	if t == 0 || t == 1 {
		return t
	}
	return -math.Pow(2, 10*(t-1)) * math.Sin((t-1.1)*5*math.Pi)
}

// EaseOutElastic 弹性缓出
func EaseOutElastic(t float64) float64 {
	if t == 0 || t == 1 {
		return t
	}
	return math.Pow(2, -10*t)*math.Sin((t-0.1)*5*math.Pi) + 1
}

// EaseOutBounce 弹跳缓出
func EaseOutBounce(t float64) float64 {
	if t < 1.0/2.75 {
		return 7.5625 * t * t
	} else if t < 2.0/2.75 {
		t -= 1.5 / 2.75
		return 7.5625*t*t + 0.75
	} else if t < 2.5/2.75 {
		t -= 2.25 / 2.75
		return 7.5625*t*t + 0.9375
	}
	t -= 2.625 / 2.75
	return 7.5625*t*t + 0.984375
}

// EaseInBounce 弹跳缓入
func EaseInBounce(t float64) float64 {
	return 1 - EaseOutBounce(1-t)
}

// EaseInBack 回退缓入
func EaseInBack(t float64) float64 {
	s := 1.70158
	return t * t * ((s+1)*t - s)
}

// EaseOutBack 回退缓出
func EaseOutBack(t float64) float64 {
	s := 1.70158
	t--
	return t*t*((s+1)*t+s) + 1
}

// EaseInOutBack 回退缓入缓出
func EaseInOutBack(t float64) float64 {
	s := 1.70158 * 1.525
	t *= 2
	if t < 1 {
		return 0.5 * (t * t * ((s+1)*t - s))
	}
	t -= 2
	return 0.5 * (t*t*((s+1)*t+s) + 2)
}

// ─── Animation ───

// AnimationState 动画状态
type AnimationState int

const (
	AnimIdle AnimationState = iota
	AnimRunning
	AnimPaused
	AnimDone
)

// Animation 属性动画，支持任意浮点值从 A 到 B 的过渡
type Animation struct {
	from     float64
	to       float64
	duration time.Duration
	ease     EaseFunc
	state    AnimationState
	start    time.Time
	current  float64
	loop     bool
	reverse  bool
	onUpdate func(float64)
	onDone   func()
}

// NewAnimation creates a property animation that transitions a float64 value
// from 'from' to 'to' over the given duration. Uses EaseOutCubic by default.
func NewAnimation(from, to float64, duration time.Duration) *Animation {
	return &Animation{
		from:     from,
		to:       to,
		duration: duration,
		ease:     EaseOutCubic, // 默认缓出
		state:    AnimIdle,
	}
}

// SetEase sets the easing function used to interpolate the animation progress.
func (a *Animation) SetEase(fn EaseFunc) *Animation {
	a.ease = fn
	return a
}

// SetLoop enables or disables continuous looping of the animation.
func (a *Animation) SetLoop(b bool) *Animation {
	a.loop = b
	return a
}

// SetReverse enables ping-pong mode, swapping from/to on each loop iteration.
func (a *Animation) SetReverse(b bool) *Animation {
	a.reverse = b
	return a
}

// OnUpdate registers a callback invoked on each animation tick with the current interpolated value.
func (a *Animation) OnUpdate(fn func(float64)) *Animation {
	a.onUpdate = fn
	return a
}

// OnDone registers a callback invoked when the animation completes.
func (a *Animation) OnDone(fn func()) *Animation {
	a.onDone = fn
	return a
}

// State returns the current animation state (idle, running, paused, or done).
func (a *Animation) State() AnimationState { return a.state }

// Value returns the current interpolated animation value.
func (a *Animation) Value() float64 { return a.current }

// Start begins the animation, registering it with the global animation manager.
func (a *Animation) Start() {
	a.state = AnimRunning
	a.start = time.Now()
	animManager.add(a)
}

// Stop terminates the animation immediately.
func (a *Animation) Stop() {
	a.state = AnimDone
}

// Pause suspends a running animation, preserving its current progress.
func (a *Animation) Pause() {
	if a.state == AnimRunning {
		a.state = AnimPaused
	}
}

// Resume continues a paused animation from where it left off.
func (a *Animation) Resume() {
	if a.state == AnimPaused {
		a.state = AnimRunning
	}
}

// tick 每帧调用，返回 true 表示动画完成
func (a *Animation) tick() bool {
	if a.state != AnimRunning {
		return a.state == AnimDone
	}

	elapsed := time.Since(a.start)
	progress := float64(elapsed) / float64(a.duration)

	if progress >= 1.0 {
		if a.loop {
			a.start = time.Now()
			if a.reverse {
				a.from, a.to = a.to, a.from
			}
			progress = 0
		} else {
			progress = 1.0
			a.state = AnimDone
		}
	}

	eased := a.ease(progress)
	a.current = a.from + (a.to-a.from)*eased

	if a.onUpdate != nil {
		a.onUpdate(a.current)
	}

	if a.state == AnimDone && a.onDone != nil {
		a.onDone()
	}

	return a.state == AnimDone
}

// ─── Animation Group ───

// AnimationGroup 动画组，可并行或串行运行多个动画
type AnimGroupMode int

const (
	AnimParallel   AnimGroupMode = iota // 并行
	AnimSequential                      // 串行
)

type AnimationGroup struct {
	anims   []*Animation
	mode    AnimGroupMode
	state   AnimationState
	current int
	onDone  func()
}

// NewAnimationGroup creates a group that runs multiple animations in parallel or sequentially.
func NewAnimationGroup(mode AnimGroupMode) *AnimationGroup {
	return &AnimationGroup{
		mode:  mode,
		state: AnimIdle,
	}
}

// Add appends an animation to the group.
func (g *AnimationGroup) Add(a *Animation) *AnimationGroup {
	g.anims = append(g.anims, a)
	return g
}

// OnDone registers a callback invoked when all animations in the group complete.
func (g *AnimationGroup) OnDone(fn func()) *AnimationGroup {
	g.onDone = fn
	return g
}

// Start begins all animations in the group according to its mode (parallel or sequential).
func (g *AnimationGroup) Start() {
	g.state = AnimRunning
	g.current = 0
	switch g.mode {
	case AnimParallel:
		for _, a := range g.anims {
			a.Start()
		}
	case AnimSequential:
		if len(g.anims) > 0 {
			g.anims[0].Start()
		}
	}
	animManager.addGroup(g)
}

func (g *AnimationGroup) tick() bool {
	if g.state != AnimRunning {
		return g.state == AnimDone
	}

	switch g.mode {
	case AnimParallel:
		allDone := true
		for _, a := range g.anims {
			if a.State() != AnimDone {
				allDone = false
			}
		}
		if allDone {
			g.state = AnimDone
		}

	case AnimSequential:
		if g.current < len(g.anims) {
			if g.anims[g.current].State() == AnimDone {
				g.current++
				if g.current < len(g.anims) {
					g.anims[g.current].Start()
				} else {
					g.state = AnimDone
				}
			}
		}
	}

	if g.state == AnimDone && g.onDone != nil {
		g.onDone()
	}
	return g.state == AnimDone
}

// ─── Transition Helpers ───

// FadeIn 创建淡入动画
func FadeIn(widget IWidget, duration time.Duration) *Animation {
	return NewAnimation(0, 1, duration).
		SetEase(EaseOutCubic).
		OnUpdate(func(v float64) {
			// widget opacity controlled via animation value
			widget.Update()
		})
}

// FadeOut 创建淡出动画
func FadeOut(widget IWidget, duration time.Duration) *Animation {
	return NewAnimation(1, 0, duration).
		SetEase(EaseInCubic).
		OnUpdate(func(v float64) {
			widget.Update()
		})
}

// SlideIn 创建滑入动画 (从 offset 滑到 target)
func SlideIn(widget IWidget, fromX, fromY, toX, toY float64, duration time.Duration) *AnimationGroup {
	ax := NewAnimation(fromX, toX, duration).
		SetEase(EaseOutCubic).
		OnUpdate(func(v float64) {
			_, y := widget.Pos()
			widget.SetPos(v, y)
		})
	ay := NewAnimation(fromY, toY, duration).
		SetEase(EaseOutCubic).
		OnUpdate(func(v float64) {
			x, _ := widget.Pos()
			widget.SetPos(x, v)
		})
	return NewAnimationGroup(AnimParallel).Add(ax).Add(ay)
}

// ScaleUp 创建缩放动画
func ScaleUp(widget IWidget, duration time.Duration) *Animation {
	origW, origH := widget.Size()
	return NewAnimation(0, 1, duration).
		SetEase(EaseOutBack).
		OnUpdate(func(v float64) {
			widget.SetSize(origW*v, origH*v)
		})
}

// Pulse 创建脉冲动画 (循环缩放)
func Pulse(widget IWidget, scale float64, duration time.Duration) *Animation {
	origW, origH := widget.Size()
	return NewAnimation(1, scale, duration).
		SetEase(EaseInOutQuad).
		SetLoop(true).
		SetReverse(true).
		OnUpdate(func(v float64) {
			widget.SetSize(origW*v, origH*v)
		})
}

// Shake 创建抖动动画
func Shake(widget IWidget, amplitude float64, duration time.Duration) *Animation {
	origX, _ := widget.Pos()
	return NewAnimation(0, 1, duration).
		SetEase(EaseLinear).
		OnUpdate(func(v float64) {
			offset := amplitude * math.Sin(v*math.Pi*6) * (1 - v)
			_, y := widget.Pos()
			widget.SetPos(origX+offset, y)
		})
}

// ─── Global Animation Manager ───

type animationManager struct {
	animations []*Animation
	groups     []*AnimationGroup
}

var animManager = &animationManager{}

func (m *animationManager) add(a *Animation) {
	m.animations = append(m.animations, a)
}

func (m *animationManager) addGroup(g *AnimationGroup) {
	m.groups = append(m.groups, g)
}

// Tick 应在每帧调用（由事件循环驱动）
func (m *animationManager) Tick() {
	// tick individual animations
	alive := m.animations[:0]
	for _, a := range m.animations {
		if !a.tick() {
			alive = append(alive, a)
		}
	}
	m.animations = alive

	// tick groups
	aliveGroups := m.groups[:0]
	for _, g := range m.groups {
		if !g.tick() {
			aliveGroups = append(aliveGroups, g)
		}
	}
	m.groups = aliveGroups
}

// AnimationTick advances all active animations by one frame. Call once per frame from the event loop.
func AnimationTick() {
	animManager.Tick()
}

// HasActiveAnimations returns true if any animations or groups are currently running.
func HasActiveAnimations() bool {
	return len(animManager.animations) > 0 || len(animManager.groups) > 0
}

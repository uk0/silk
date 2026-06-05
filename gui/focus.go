package gui

// 焦点策略与 Tab 焦点链.
//
// 大多数可交互控件已经实现了 IEventKeyDown (Button / Edit / CheckBox /
// ComboBox / Slider ...), 它们一旦取得焦点就会收到键盘事件. 因此这里用
// "是否实现 IEventKeyDown" 作为可 Tab 聚焦的默认启发式, 不必逐个控件去
// 标注. 纯展示型控件 (如 Label) 如需排除, 可显式设置 NoFocus.

// FocusPolicy 描述一个控件参与 Tab 焦点链的方式.
type FocusPolicy int

const (
	// AutoFocus 是零值: 按启发式判定 —— 控件可见、可用且实现了
	// IEventKeyDown 时即可被 Tab 聚焦. 未显式设置策略的控件都是此值.
	AutoFocus FocusPolicy = iota

	// NoFocus 显式把控件排除出 Tab 焦点链, 即便它实现了 IEventKeyDown.
	NoFocus

	// TabFocus 显式声明控件可被 Tab 聚焦, 即便它没有实现 IEventKeyDown.
	TabFocus
)

// isTabFocusable 判定 w 是否可作为 Tab 焦点目标.
//
// 规则 (与 Qt 的 focusPolicy 思路一致):
//   - 不可见或被禁用的控件一律跳过;
//   - NoFocus 显式排除;
//   - TabFocus 显式纳入;
//   - AutoFocus (零值) 时, 按 "实现了 IEventKeyDown" 的启发式判定.
func isTabFocusable(w IWidget) bool {
	if w == nil {
		return false
	}
	if !w.IsVisible() || !w.IsEnabled() {
		return false
	}
	switch w.NakedWidget().FocusPolicy() {
	case NoFocus:
		return false
	case TabFocus:
		return true
	}
	// AutoFocus: 实现了键盘事件接口即视为可聚焦.
	_, ok := w.(IEventKeyDown)
	return ok
}

// collectFocusable 以前序 (pre-order) 深度优先遍历可见子树, 按视觉顺序
// 把可 Tab 聚焦的控件收集到 out. 不可见的容器整棵跳过 (其子孙也不可见).
func collectFocusable(w IWidget, out *[]IWidget) {
	if w == nil || !w.IsVisible() {
		return
	}
	if isTabFocusable(w) {
		*out = append(*out, w)
	}
	for _, c := range w.Children() {
		collectFocusable(c, out)
	}
}

// nextFocusable 在以 root 为根的可见控件树里, 返回 current 之后 (forward 为
// true) 或之前 (forward 为 false) 的可 Tab 聚焦控件, 到达两端时环绕.
//
// 若 current 为 nil 或不在焦点链内, forward 时返回第一个、否则返回最后一个
// 可聚焦控件. 当树内没有任何可聚焦控件时返回 nil.
//
// 本函数只读地操作控件树 (不触碰全局 focusWidget), 因此可脱离 GLFW 窗口
// 直接做单元测试.
func nextFocusable(root IWidget, current IWidget, forward bool) IWidget {
	if root == nil {
		return nil
	}
	var list []IWidget
	collectFocusable(root, &list)
	if len(list) == 0 {
		return nil
	}

	// 找到 current 在焦点链中的位置.
	idx := -1
	if current != nil {
		cur := current.Self()
		for i, w := range list {
			if w.Self() == cur {
				idx = i
				break
			}
		}
	}

	if idx < 0 {
		// current 不在链内: 前进取首, 后退取尾.
		if forward {
			return list[0]
		}
		return list[len(list)-1]
	}

	n := len(list)
	if forward {
		return list[(idx+1)%n]
	}
	return list[(idx-1+n)%n]
}

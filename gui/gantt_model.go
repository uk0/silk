package gui

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
)

const (
	ganttMaxTasks     = 10000
	ganttMaxEdges     = 100000
	ganttHistoryLimit = 64
)

type GanttRow struct {
	Task        GanttTask
	Depth       int
	HasChildren bool
}

// GanttSnapshot is detached from the model. Modifying it cannot mutate live
// data or history. Restore validates it and records one atomic undoable edit.
type GanttSnapshot struct {
	Tasks []GanttTask `json:"tasks"`
}

// GanttModel is a UI-thread model. Successful edits are atomic and undoable;
// rejected/no-op edits do not discard redo. There is no fixed hierarchy depth.
type GanttModel struct {
	tasks      []GanttTask
	undo, redo [][]GanttTask
	revision   uint64
}

func NewGanttModel(tasks []GanttTask) (*GanttModel, error) {
	prepared, err := ganttPrepare(tasks, false)
	if err != nil {
		return nil, err
	}
	return &GanttModel{tasks: prepared}, nil
}

func cloneGanttTasks(tasks []GanttTask) []GanttTask {
	out := make([]GanttTask, len(tasks))
	copy(out, tasks)
	for i := range out {
		out[i].Dependencies = append([]string(nil), tasks[i].Dependencies...)
	}
	return out
}

func (m *GanttModel) Tasks() []GanttTask            { return cloneGanttTasks(m.tasks) }
func (m *GanttModel) Snapshot() GanttSnapshot       { return GanttSnapshot{m.Tasks()} }
func (m *GanttModel) Restore(s GanttSnapshot) error { return m.SetTasks(s.Tasks) }
func (m *GanttModel) Revision() uint64              { return m.revision }
func (m *GanttModel) Task(id string) (GanttTask, bool) {
	for _, task := range m.tasks {
		if task.ID == id {
			task.Dependencies = append([]string(nil), task.Dependencies...)
			return task, true
		}
	}
	return GanttTask{}, false
}
func (m *GanttModel) CanUndo() bool { return len(m.undo) > 0 }
func (m *GanttModel) CanRedo() bool { return len(m.redo) > 0 }
func (m *GanttModel) Undo() bool {
	if !m.CanUndo() {
		return false
	}
	m.redo = append(m.redo, m.tasks)
	m.tasks = m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	m.revision++
	return true
}
func (m *GanttModel) Redo() bool {
	if !m.CanRedo() {
		return false
	}
	m.undo = append(m.undo, m.tasks)
	m.tasks = m.redo[len(m.redo)-1]
	m.redo = m.redo[:len(m.redo)-1]
	m.revision++
	return true
}
func (m *GanttModel) commit(tasks []GanttTask, cascade bool) error {
	return m.commitOwned(cloneGanttTasks(tasks), cascade)
}

// commitOwned may normalize or schedule tasks in place, including on failure.
// Only pass a detached working slice (and detached dependency slices).
func (m *GanttModel) commitOwned(tasks []GanttTask, cascade bool) error {
	if err := ganttPrepareOwned(tasks, cascade); err != nil {
		return err
	}
	m.commitPrepared(tasks)
	return nil
}

// commitPrepared takes ownership of an already validated snapshot.
func (m *GanttModel) commitPrepared(prepared []GanttTask) bool {
	if (len(m.tasks) == 0 && len(prepared) == 0) || reflect.DeepEqual(m.tasks, prepared) {
		return false
	}
	m.undo = append(m.undo, m.tasks)
	if len(m.undo) > ganttHistoryLimit {
		m.undo[0] = nil
		m.undo = m.undo[1:]
	}
	m.tasks, m.redo = prepared, nil
	m.revision++
	return true
}
func (m *GanttModel) SetTasks(tasks []GanttTask) error { return m.commit(tasks, false) }
func (m *GanttModel) MarshalJSON() ([]byte, error)     { return json.Marshal(m.Tasks()) }
func (m *GanttModel) UnmarshalJSON(data []byte) error {
	if len(data) > ganttMaxJSONBytes {
		return fmt.Errorf("甘特 JSON 超过 4 MiB 限制")
	}
	if strings.TrimSpace(string(data)) == "null" {
		return fmt.Errorf("甘特 JSON 必须是任务数组，不能是 null")
	}
	var tasks []GanttTask
	if err := ganttDecodeJSON(data, &tasks); err != nil {
		return err
	}
	return m.SetTasks(tasks)
}
func (m *GanttModel) JSON() string { data, _ := m.MarshalJSON(); return string(data) }
func (m *GanttModel) SetJSON(data string) error {
	if len(data) > ganttMaxJSONBytes {
		return fmt.Errorf("甘特 JSON 超过 4 MiB 限制")
	}
	return m.UnmarshalJSON([]byte(data))
}

func (m *GanttModel) VisibleRows() []GanttRow {
	children := make(map[string][]int)
	for i, task := range m.tasks {
		children[task.ParentID] = append(children[task.ParentID], i)
	}
	type entry struct{ index, depth int }
	stack := []entry{}
	for i := len(children[""]) - 1; i >= 0; i-- {
		stack = append(stack, entry{children[""][i], 0})
	}
	rows := make([]GanttRow, 0, len(m.tasks))
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		task := m.tasks[current.index]
		task.Dependencies = append([]string(nil), task.Dependencies...)
		kids := children[task.ID]
		rows = append(rows, GanttRow{task, current.depth, len(kids) > 0})
		if !task.Collapsed {
			for i := len(kids) - 1; i >= 0; i-- {
				stack = append(stack, entry{kids[i], current.depth + 1})
			}
		}
	}
	return rows
}

func ganttTaskIndex(tasks []GanttTask, id string) (int, error) {
	for i := range tasks {
		if tasks[i].ID == id {
			return i, nil
		}
	}
	return -1, fmt.Errorf("甘特任务 %q 不存在", id)
}
func (m *GanttModel) MoveTask(id string, days int, cascade bool) error {
	tasks := m.Tasks()
	i, err := ganttTaskIndex(tasks, id)
	if err != nil {
		return err
	}
	if err := ganttShift(tasks, i, days); err != nil {
		return err
	}
	return m.commitOwned(tasks, cascade)
}
func (m *GanttModel) ResizeTask(id string, start, end time.Time, cascade bool) error {
	tasks := m.Tasks()
	i, err := ganttTaskIndex(tasks, id)
	if err != nil {
		return err
	}
	start, end = GanttDate(start), GanttDate(end)
	for _, task := range tasks {
		if task.ParentID == id {
			if GanttDaysBetween(tasks[i].Start, start) != GanttDaysBetween(tasks[i].End, end) {
				return fmt.Errorf("父任务范围由子任务汇总；请移动整个父任务或调整子任务")
			}
			return m.MoveTask(id, GanttDaysBetween(tasks[i].Start, start), cascade)
		}
	}
	tasks[i].Start, tasks[i].End = start, end
	return m.commitOwned(tasks, cascade)
}
func (m *GanttModel) SetProgress(id string, progress float64) error {
	tasks := m.Tasks()
	i, err := ganttTaskIndex(tasks, id)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.ParentID == id {
			return fmt.Errorf("父任务进度由子任务汇总")
		}
	}
	tasks[i].Progress = progress
	return m.commitOwned(tasks, false)
}
func (m *GanttModel) SetCollapsed(id string, collapsed bool) error {
	tasks := m.Tasks()
	i, err := ganttTaskIndex(tasks, id)
	if err != nil {
		return err
	}
	tasks[i].Collapsed = collapsed
	return m.commit(tasks, false)
}
func (m *GanttModel) AddTask(task GanttTask) error {
	return m.commit(append(m.Tasks(), task), false)
}
func (m *GanttModel) UpdateTask(task GanttTask, cascade bool) error {
	tasks := m.Tasks()
	i, err := ganttTaskIndex(tasks, task.ID)
	if err != nil {
		return err
	}
	for _, child := range tasks {
		if child.ParentID == task.ID && (!GanttDate(task.Start).Equal(tasks[i].Start) || !GanttDate(task.End).Equal(tasks[i].End)) {
			return fmt.Errorf("修改父任务日期请使用 MoveTask")
		}
	}
	tasks[i] = task
	if task.ParentID != m.tasks[i].ParentID {
		ganttPreserveEmptiedSummaries(m.tasks, tasks)
	}
	return m.commit(tasks, cascade)
}

// RemoveTask removes the whole subtree and its incoming dependency references.
func (m *GanttModel) RemoveTask(id string) error {
	if _, err := ganttTaskIndex(m.tasks, id); err != nil {
		return err
	}
	removed := ganttSubtree(m.tasks, id)
	tasks := m.Tasks()
	kept := tasks[:0]
	for _, task := range tasks {
		if !removed[task.ID] {
			deps := task.Dependencies[:0]
			for _, dep := range task.Dependencies {
				if !removed[dep] {
					deps = append(deps, dep)
				}
			}
			task.Dependencies = deps
			kept = append(kept, task)
		}
	}
	ganttPreserveEmptiedSummaries(m.tasks, kept)
	return m.commit(kept, false)
}

// ReparentTask moves a whole subtree without changing its calendar dates.
// An empty newParentID promotes it to a root. Both old and new ancestors are
// summarized; a former parent with no remaining children becomes an ordinary
// leaf retaining its last range (a milestone if that range has zero duration).
// Invalid FS/hierarchy changes are rejected.
func (m *GanttModel) ReparentTask(id, newParentID string) error {
	tasks := m.Tasks()
	i, err := ganttTaskIndex(tasks, id)
	if err != nil {
		return err
	}
	if newParentID != "" {
		if _, err := ganttTaskIndex(tasks, newParentID); err != nil {
			return err
		}
		if ganttSubtree(tasks, id)[newParentID] {
			return fmt.Errorf("任务不能移入自身或其后代任务")
		}
	}
	tasks[i].ParentID = newParentID
	ganttPreserveEmptiedSummaries(m.tasks, tasks)
	return m.commit(tasks, false)
}

// A summary containing only same-day milestones legitimately has Start==End.
// When a structural edit removes its last child, retain that date/progress as
// a milestone instead of rejecting the edit as an invalid zero-length leaf.
// Only emptied existing summaries are converted; imports and ordinary leaves
// still undergo strict date/type validation, and nonempty summaries stay groups.
func ganttPreserveEmptiedSummaries(before, after []GanttTask) {
	hadChildren := make(map[string]bool)
	for _, task := range before {
		if task.ParentID != "" {
			hadChildren[task.ParentID] = true
		}
	}
	for _, task := range after {
		delete(hadChildren, task.ParentID)
	}
	for i := range after {
		if hadChildren[after[i].ID] && after[i].Start.Equal(after[i].End) {
			after[i].Milestone = true
		}
	}
}

func ganttSubtree(tasks []GanttTask, id string) map[string]bool {
	children := make(map[string][]string)
	for _, task := range tasks {
		children[task.ParentID] = append(children[task.ParentID], task.ID)
	}
	found := map[string]bool{}
	stack := []string{id}
	for len(stack) > 0 {
		id = stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if found[id] {
			continue
		}
		found[id] = true
		stack = append(stack, children[id]...)
	}
	return found
}
func ganttShift(tasks []GanttTask, index, days int) error {
	if days < -3652058 || days > 3652058 {
		return fmt.Errorf("甘特日期移动超出支持范围")
	}
	found := ganttSubtree(tasks, tasks[index].ID)
	for i := range tasks {
		if found[tasks[i].ID] {
			tasks[i].Start = tasks[i].Start.AddDate(0, 0, days)
			tasks[i].End = tasks[i].End.AddDate(0, 0, days)
			if !ganttValidDate(tasks[i].Start) || !ganttValidDate(tasks[i].End) {
				return fmt.Errorf("甘特日期必须在 0001 至 9999 年范围内")
			}
		}
	}
	return nil
}
func ganttValidDate(t time.Time) bool { return !t.IsZero() && t.Year() >= 1 && t.Year() <= 9999 }

type ganttTree struct {
	index    map[string]int
	children [][]int
	order    []int // parent before children, input sibling order preserved
}

func ganttValidate(tasks []GanttTask) (*ganttTree, error) {
	if len(tasks) > ganttMaxTasks {
		return nil, fmt.Errorf("甘特任务数量超过 %d", ganttMaxTasks)
	}
	tree := &ganttTree{index: make(map[string]int), children: make([][]int, len(tasks))}
	for i, task := range tasks {
		if task.ID == "" || strings.TrimSpace(task.ID) != task.ID || len(task.ID) > 256 {
			return nil, fmt.Errorf("甘特任务 ID 无效")
		}
		if _, exists := tree.index[task.ID]; exists {
			return nil, fmt.Errorf("甘特任务 ID %q 重复", task.ID)
		}
		if math.IsNaN(task.Progress) || math.IsInf(task.Progress, 0) || task.Progress < 0 || task.Progress > 1 {
			return nil, fmt.Errorf("任务 %q 的进度必须在 0 到 1 之间", task.ID)
		}
		tree.index[task.ID] = i
	}
	for i, task := range tasks {
		if task.ParentID == "" {
			tree.order = append(tree.order, i)
			continue
		}
		p, exists := tree.index[task.ParentID]
		if !exists {
			return nil, fmt.Errorf("任务 %q 的父任务 %q 不存在", task.ID, task.ParentID)
		}
		tree.children[p] = append(tree.children[p], i)
	}
	for cursor := 0; cursor < len(tree.order); cursor++ {
		tree.order = append(tree.order, tree.children[tree.order[cursor]]...)
	}
	if len(tree.order) != len(tasks) {
		return nil, fmt.Errorf("甘特父子关系存在循环")
	}
	for i, task := range tasks {
		if len(tree.children[i]) > 0 {
			if task.Milestone {
				return nil, fmt.Errorf("里程碑 %q 不能包含子任务", task.ID)
			}
			continue
		}
		if !ganttValidDate(task.Start) || !ganttValidDate(task.End) {
			return nil, fmt.Errorf("任务 %q 需要有效开始及结束日期", task.ID)
		}
		if task.Milestone {
			if !task.Start.Equal(task.End) {
				return nil, fmt.Errorf("里程碑 %q 的开始和结束日期必须相同", task.ID)
			}
		} else if !task.End.After(task.Start) {
			return nil, fmt.Errorf("任务 %q 的排他结束日期必须晚于开始日期", task.ID)
		}
	}
	return tree, nil
}

func ganttSummarize(tasks []GanttTask, tree *ganttTree) {
	weight := make([]float64, len(tasks))
	for j := len(tree.order) - 1; j >= 0; j-- {
		i := tree.order[j]
		kids := tree.children[i]
		if len(kids) == 0 {
			weight[i] = float64(max(1, GanttDaysBetween(tasks[i].Start, tasks[i].End)))
			continue
		}
		start, end := tasks[kids[0]].Start, tasks[kids[0]].End
		progress := 0.0
		for _, child := range kids {
			if tasks[child].Start.Before(start) {
				start = tasks[child].Start
			}
			if tasks[child].End.After(end) {
				end = tasks[child].End
			}
			weight[i] += weight[child]
			progress += tasks[child].Progress * weight[child]
		}
		tasks[i].Start, tasks[i].End, tasks[i].Progress = start, end, progress/weight[i]
	}
}

// Expand group dependencies to leaf constraints solely for cycle validation.
// This detects otherwise-hidden parent/child + dependency feedback loops.
func ganttValidateDependencies(tasks []GanttTask, tree *ganttTree) error {
	leaves := func(index int) []int {
		out := []int{}
		stack := []int{index}
		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(tree.children[i]) == 0 {
				out = append(out, i)
			} else {
				stack = append(stack, tree.children[i]...)
			}
		}
		return out
	}
	adj := make([][]int, len(tasks))
	indegree := make([]int, len(tasks))
	edgeCount := 0
	for i, task := range tasks {
		seen := make(map[string]bool)
		for _, id := range task.Dependencies {
			pred, exists := tree.index[id]
			if !exists {
				return fmt.Errorf("任务 %q 的前置任务 %q 不存在", task.ID, id)
			}
			if seen[id] {
				return fmt.Errorf("任务 %q 的前置任务 %q 重复", task.ID, id)
			}
			seen[id] = true
			from, to := leaves(pred), leaves(i)
			if len(from) > 0 && len(to) > (ganttMaxEdges-edgeCount)/len(from) {
				return fmt.Errorf("甘特依赖展开超过 %d 条安全限制", ganttMaxEdges)
			}
			for _, a := range from {
				for _, b := range to {
					if a == b {
						return fmt.Errorf("甘特依赖不能连接自身或重叠父子任务")
					}
					adj[a] = append(adj[a], b)
					indegree[b]++
					edgeCount++
				}
			}
		}
	}
	queue := []int{}
	for i := range tasks {
		if indegree[i] == 0 {
			queue = append(queue, i)
		}
	}
	for cursor := 0; cursor < len(queue); cursor++ {
		for _, i := range adj[queue[cursor]] {
			indegree[i]--
			if indegree[i] == 0 {
				queue = append(queue, i)
			}
		}
	}
	if len(queue) != len(tasks) {
		return fmt.Errorf("甘特前置依赖存在循环")
	}
	return nil
}

func ganttPrepare(input []GanttTask, cascade bool) ([]GanttTask, error) {
	tasks := cloneGanttTasks(input)
	if err := ganttPrepareOwned(tasks, cascade); err != nil {
		return nil, err
	}
	return tasks, nil
}

func ganttPrepareOwned(tasks []GanttTask, cascade bool) error {
	for i := range tasks {
		tasks[i].Start = GanttDate(tasks[i].Start)
		tasks[i].End = GanttDate(tasks[i].End)
	}
	tree, err := ganttValidate(tasks)
	if err != nil {
		return err
	}
	ganttSummarize(tasks, tree)
	if err := ganttValidateDependencies(tasks, tree); err != nil {
		return err
	}
	for pass := 0; pass <= len(tasks); pass++ {
		changed := false
		for i := range tasks {
			for _, id := range tasks[i].Dependencies {
				pred := tree.index[id]
				days := GanttDaysBetween(tasks[i].Start, tasks[pred].End)
				if days <= 0 {
					continue
				}
				if !cascade {
					return fmt.Errorf("任务 %q 不能早于前置任务 %q 的结束日期", tasks[i].ID, id)
				}
				if err := ganttShift(tasks, i, days); err != nil {
					return err
				}
				ganttSummarize(tasks, tree)
				changed = true
			}
		}
		if !changed {
			return nil
		}
	}
	return fmt.Errorf("甘特依赖级联未能收敛；编辑已取消")
}

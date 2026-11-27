package gui

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkGanttDraggedSummaryAcrossSnappedDays includes validating the
// proposed edits and updating visible rows, but excludes rendering and setup.
func BenchmarkGanttDraggedSummaryAcrossSnappedDays(b *testing.B) {
	for _, children := range []int{1000, 5000} {
		b.Run(fmt.Sprint(children), func(b *testing.B) {
			start := time.Date(2028, time.February, 28, 0, 0, 0, 0, time.UTC)
			tasks := []GanttTask{{ID: "root", Name: "Project", Start: start, End: start.AddDate(0, 0, 1)}}
			for i := 0; i < children; i++ {
				s := start.AddDate(0, 0, i%2)
				t := GanttTask{ID: fmt.Sprintf("child-%d", i), ParentID: "root", Name: "Task", Start: s, End: s.AddDate(0, 0, 1)}
				if i%2 == 1 {
					t.Dependencies = []string{tasks[len(tasks)-1].ID}
				}
				tasks = append(tasks, t)
			}
			c := NewGanttChart()
			if err := c.SetTasks(tasks); err != nil {
				b.Fatal(err)
			}
			c.SetBounds(0, 0, 900, 420)
			d := &ganttGesture{kind: "move", id: "root", model: c.model, modelRevision: c.model.Revision(), before: c.Tasks()}
			c.drag = d
			w := c.DayWidth()
			c.previewGesture(4*w, 0)
			if c.lastError != nil || d.preview == nil {
				b.Fatalf("unable to preview before timing: %v", c.lastError)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.previewGesture(float64(1+i%3)*w, 0)
			}
			b.StopTimer()
			if c.lastError != nil || d.preview == nil {
				b.Fatalf("benchmark did not preview a valid edit: %v", c.lastError)
			}
			after, _ := d.preview.Task("child-0")
			if got := GanttDaysBetween(start, after.Start); got != 1+(b.N-1)%3 {
				b.Fatalf("wrong preview, shifted %d days", got)
			}
		})
	}
}

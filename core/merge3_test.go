package core

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// chunkKinds 抽出块列表的 Kind 序列, 便于一行断言合并结构
func chunkKinds(chunks []MergeChunk) []MergeKind {
	out := make([]MergeKind, len(chunks))
	for i, c := range chunks {
		out[i] = c.Kind
	}
	return out
}

// mergedLines 把一串块拼成最终文本行, 顺带断言里面没有 conflict
// (自动合并的用例都应该走到这里而不用人工选边)
func mergedLines(t *testing.T, chunks []MergeChunk) []string {
	t.Helper()
	var out []string
	for i, c := range chunks {
		if c.Kind == MergeConflict {
			t.Fatalf("chunk[%d] is a conflict, want auto-merged: %+v", i, c)
		}
		out = append(out, c.Resolved()...)
	}
	return out
}

// markerLines 把测试里的原始文本切成行, 去掉结尾换行带来的空行,
// 这样 render 的输出可以和它逐行比较
func markerLines(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// TestMerge3CleanDisjointEdits: ours 在头部插了一行, theirs 改了末行 ——
// 两处不重叠, 应当全自动合并, 结果同时包含两侧的改动.
func TestMerge3CleanDisjointEdits(t *testing.T) {
	base := []string{"A", "B", "C", "D", "E"}
	ours := []string{"A", "X", "B", "C", "D", "E"}
	theirs := []string{"A", "B", "C", "D", "E2"}

	chunks := Merge3(base, ours, theirs)
	wantKinds := []MergeKind{MergeStable, MergeOurs, MergeStable, MergeTheirs}
	if got := chunkKinds(chunks); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("kinds = %v, want %v (chunks: %+v)", got, wantKinds, chunks)
	}

	// ours 的插入块: base 一侧为空, theirs 一侧也为空
	ins := chunks[1]
	if !reflect.DeepEqual(ins.Ours, []string{"X"}) || ins.Base != nil || ins.Theirs != nil {
		t.Errorf("insert chunk = %+v, want Ours=[X] Base=nil Theirs=nil", ins)
	}
	// theirs 的改写块: ours 一侧仍等于 base
	mod := chunks[3]
	if !reflect.DeepEqual(mod.Base, []string{"E"}) || !reflect.DeepEqual(mod.Ours, []string{"E"}) ||
		!reflect.DeepEqual(mod.Theirs, []string{"E2"}) {
		t.Errorf("modify chunk = %+v, want Base=[E] Ours=[E] Theirs=[E2]", mod)
	}

	want := []string{"A", "X", "B", "C", "D", "E2"}
	if got := mergedLines(t, chunks); !reflect.DeepEqual(got, want) {
		t.Errorf("merged = %v, want %v", got, want)
	}
}

// TestMerge3DisjointInsertsBothSides: 双方在不同位置各插一行, 依然无冲突
func TestMerge3DisjointInsertsBothSides(t *testing.T) {
	base := []string{"A", "B", "C"}
	ours := []string{"A", "X", "B", "C"}
	theirs := []string{"A", "B", "Y", "C"}

	chunks := Merge3(base, ours, theirs)
	wantKinds := []MergeKind{MergeStable, MergeOurs, MergeStable, MergeTheirs, MergeStable}
	if got := chunkKinds(chunks); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("kinds = %v, want %v (chunks: %+v)", got, wantKinds, chunks)
	}
	want := []string{"A", "X", "B", "Y", "C"}
	if got := mergedLines(t, chunks); !reflect.DeepEqual(got, want) {
		t.Errorf("merged = %v, want %v", got, want)
	}
}

// TestMerge3IdenticalEditsCollapseToStable: 双方做了完全相同的改动 ——
// 整份文本折叠成一个 stable 块, Base 留旧行, Ours/Theirs 是那份改动.
func TestMerge3IdenticalEditsCollapseToStable(t *testing.T) {
	base := []string{"A", "B", "C"}
	edit := []string{"A", "X", "C"}

	chunks := Merge3(base, edit, edit)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1 (%+v)", len(chunks), chunks)
	}
	c := chunks[0]
	if c.Kind != MergeStable {
		t.Errorf("kind = %v, want stable", c.Kind)
	}
	if !reflect.DeepEqual(c.Base, base) {
		t.Errorf("Base = %v, want %v", c.Base, base)
	}
	if !reflect.DeepEqual(c.Ours, edit) || !reflect.DeepEqual(c.Theirs, edit) {
		t.Errorf("Ours/Theirs = %v/%v, want %v on both", c.Ours, c.Theirs, edit)
	}
	// stable 块的最终文本是那份改动, 不是 base
	if got := c.Resolved(); !reflect.DeepEqual(got, edit) {
		t.Errorf("Resolved() = %v, want %v", got, edit)
	}
}

// TestMerge3OverlappingEditsConflict: 双方改了同一行且改得不一样 → conflict
func TestMerge3OverlappingEditsConflict(t *testing.T) {
	base := []string{"A", "B", "C"}
	ours := []string{"A", "X", "C"}
	theirs := []string{"A", "Y", "C"}

	chunks := Merge3(base, ours, theirs)
	wantKinds := []MergeKind{MergeStable, MergeConflict, MergeStable}
	if got := chunkKinds(chunks); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("kinds = %v, want %v (chunks: %+v)", got, wantKinds, chunks)
	}
	c := chunks[1]
	if !reflect.DeepEqual(c.Base, []string{"B"}) || !reflect.DeepEqual(c.Ours, []string{"X"}) ||
		!reflect.DeepEqual(c.Theirs, []string{"Y"}) {
		t.Errorf("conflict chunk = %+v, want Base=[B] Ours=[X] Theirs=[Y]", c)
	}
	if got := c.Resolved(); got != nil {
		t.Errorf("Resolved() on a conflict = %v, want nil", got)
	}
}

// TestMerge3ConflictingInsertsAtSamePoint: 双方在同一位置插入不同内容 ——
// 冲突块的 base 一侧为空
func TestMerge3ConflictingInsertsAtSamePoint(t *testing.T) {
	chunks := Merge3([]string{"A", "B"}, []string{"A", "X", "B"}, []string{"A", "Y", "B"})
	wantKinds := []MergeKind{MergeStable, MergeConflict, MergeStable}
	if got := chunkKinds(chunks); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("kinds = %v, want %v (chunks: %+v)", got, wantKinds, chunks)
	}
	if c := chunks[1]; c.Base != nil || !reflect.DeepEqual(c.Ours, []string{"X"}) ||
		!reflect.DeepEqual(c.Theirs, []string{"Y"}) {
		t.Errorf("conflict chunk = %+v, want Base=nil Ours=[X] Theirs=[Y]", c)
	}
}

// TestMerge3OneSidedDeletion: ours 删了一行, theirs 没动 → 删除自动生效
func TestMerge3OneSidedDeletion(t *testing.T) {
	base := []string{"A", "B", "C"}
	chunks := Merge3(base, []string{"A", "C"}, base)
	wantKinds := []MergeKind{MergeStable, MergeOurs, MergeStable}
	if got := chunkKinds(chunks); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("kinds = %v, want %v (chunks: %+v)", got, wantKinds, chunks)
	}
	if c := chunks[1]; c.Ours != nil || !reflect.DeepEqual(c.Base, []string{"B"}) {
		t.Errorf("deletion chunk = %+v, want Ours=nil Base=[B]", c)
	}
	want := []string{"A", "C"}
	if got := mergedLines(t, chunks); !reflect.DeepEqual(got, want) {
		t.Errorf("merged = %v, want %v", got, want)
	}
}

// TestMerge3EmptyInputs: 三方全空返回 nil; ours 清空整个文件仍是 ours 块
func TestMerge3EmptyInputs(t *testing.T) {
	if got := Merge3(nil, nil, nil); len(got) != 0 {
		t.Errorf("Merge3(nil,nil,nil) = %+v, want no chunks", got)
	}
	base := []string{"A", "B"}
	chunks := Merge3(base, nil, base)
	if got := chunkKinds(chunks); !reflect.DeepEqual(got, []MergeKind{MergeOurs}) {
		t.Fatalf("kinds = %v, want [ours] (chunks: %+v)", got, chunks)
	}
	if got := mergedLines(t, chunks); len(got) != 0 {
		t.Errorf("merged = %v, want empty", got)
	}
}

// TestMerge3DoesNotMutateInputs: 块里的切片都是副本, 修改块不会回写入参
func TestMerge3DoesNotMutateInputs(t *testing.T) {
	base := []string{"A", "B", "C"}
	ours := []string{"A", "X", "C"}
	theirs := []string{"A", "B", "C2"}

	chunks := Merge3(base, ours, theirs)
	for i := range chunks {
		for j := range chunks[i].Base {
			chunks[i].Base[j] = "MUTATED"
		}
		for j := range chunks[i].Ours {
			chunks[i].Ours[j] = "MUTATED"
		}
		for j := range chunks[i].Theirs {
			chunks[i].Theirs[j] = "MUTATED"
		}
	}
	if !reflect.DeepEqual(base, []string{"A", "B", "C"}) ||
		!reflect.DeepEqual(ours, []string{"A", "X", "C"}) ||
		!reflect.DeepEqual(theirs, []string{"A", "B", "C2"}) {
		t.Errorf("inputs mutated: base=%v ours=%v theirs=%v", base, ours, theirs)
	}
}

// diff3StyleText 是 `git merge --conflict=diff3` 留下的样子: 带 "|||||||" base 段
const diff3StyleText = `package main

<<<<<<< ours
	fmt.Println("ours")
||||||| base
	fmt.Println("base")
=======
	fmt.Println("theirs")
>>>>>>> theirs

func main() {}
`

// TestParseConflictMarkersDiff3RoundTrip: diff3 风格的标记文本解析成
// stable/conflict/stable 三块, 再渲染回去要逐行一致
func TestParseConflictMarkersDiff3RoundTrip(t *testing.T) {
	lines := markerLines(diff3StyleText)
	chunks, err := ParseConflictMarkers(lines)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantKinds := []MergeKind{MergeStable, MergeConflict, MergeStable}
	if got := chunkKinds(chunks); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("kinds = %v, want %v (chunks: %+v)", got, wantKinds, chunks)
	}
	c := chunks[1]
	if !reflect.DeepEqual(c.Ours, []string{"\tfmt.Println(\"ours\")"}) {
		t.Errorf("conflict Ours = %q", c.Ours)
	}
	if !reflect.DeepEqual(c.Base, []string{"\tfmt.Println(\"base\")"}) {
		t.Errorf("conflict Base = %q", c.Base)
	}
	if !reflect.DeepEqual(c.Theirs, []string{"\tfmt.Println(\"theirs\")"}) {
		t.Errorf("conflict Theirs = %q", c.Theirs)
	}
	if !reflect.DeepEqual(chunks[0].Ours, []string{"package main", ""}) {
		t.Errorf("leading text = %q, want [\"package main\" \"\"]", chunks[0].Ours)
	}

	labels := MergeLabels{Ours: "ours", Base: "base", Theirs: "theirs"}
	if got := RenderConflictMarkers(chunks, labels); !reflect.DeepEqual(got, lines) {
		t.Errorf("render round-trip mismatch:\ngot  %q\nwant %q", got, lines)
	}
}

// mergeStyleText 是 git 默认(不带 base 段)的冲突样子
const mergeStyleText = `alpha
<<<<<<< HEAD
mine
=======
yours
>>>>>>> feature
omega
`

// TestParseConflictMarkersMergeStyleRoundTrip: 没有 "|||||||" 段时 Base 为 nil,
// 用 Base 标签为空的 labels 渲染回去同样逐行一致
func TestParseConflictMarkersMergeStyleRoundTrip(t *testing.T) {
	lines := markerLines(mergeStyleText)
	chunks, err := ParseConflictMarkers(lines)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantKinds := []MergeKind{MergeStable, MergeConflict, MergeStable}
	if got := chunkKinds(chunks); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("kinds = %v, want %v (chunks: %+v)", got, wantKinds, chunks)
	}
	if c := chunks[1]; c.Base != nil || !reflect.DeepEqual(c.Ours, []string{"mine"}) ||
		!reflect.DeepEqual(c.Theirs, []string{"yours"}) {
		t.Errorf("conflict chunk = %+v, want Base=nil Ours=[mine] Theirs=[yours]", c)
	}

	labels := MergeLabels{Ours: "HEAD", Theirs: "feature"}
	if got := RenderConflictMarkers(chunks, labels); !reflect.DeepEqual(got, lines) {
		t.Errorf("render round-trip mismatch:\ngot  %q\nwant %q", got, lines)
	}
}

// TestParseConflictMarkersLiteralSeparators: 冲突块之外的 "=======" 只是
// 普通文本(Markdown 下划线), "========" (8 个) 更是永远不算标记
func TestParseConflictMarkersLiteralSeparators(t *testing.T) {
	lines := []string{"Title", "=======", "========", "|||||||", "body"}
	chunks, err := ParseConflictMarkers(lines)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Kind != MergeStable {
		t.Fatalf("chunks = %+v, want a single stable chunk", chunks)
	}
	if !reflect.DeepEqual(chunks[0].Ours, lines) {
		t.Errorf("text = %q, want %q", chunks[0].Ours, lines)
	}
}

// TestParseConflictMarkersUnterminated: 文本结束时块还没闭合 —— 报错,
// 但已累积的两侧仍作为冲突块交出
func TestParseConflictMarkersUnterminated(t *testing.T) {
	chunks, err := ParseConflictMarkers([]string{"<<<<<<< ours", "mine", "=======", "yours"})
	if err == nil {
		t.Fatal("expected an error for the unterminated conflict")
	}
	if len(chunks) != 1 || chunks[0].Kind != MergeConflict {
		t.Fatalf("chunks = %+v, want a single conflict chunk", chunks)
	}
	if c := chunks[0]; !reflect.DeepEqual(c.Ours, []string{"mine"}) ||
		!reflect.DeepEqual(c.Theirs, []string{"yours"}) {
		t.Errorf("salvaged chunk = %+v, want Ours=[mine] Theirs=[yours]", c)
	}
}

// TestParseConflictMarkersMissingSeparator: 缺 "=======" 就直接 ">>>>>>>"
// —— 报错, theirs 侧当空
func TestParseConflictMarkersMissingSeparator(t *testing.T) {
	chunks, err := ParseConflictMarkers([]string{"<<<<<<< ours", "mine", ">>>>>>> theirs", "tail"})
	if err == nil {
		t.Fatal("expected an error for the missing separator")
	}
	wantKinds := []MergeKind{MergeConflict, MergeStable}
	if got := chunkKinds(chunks); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("kinds = %v, want %v (chunks: %+v)", got, wantKinds, chunks)
	}
	if c := chunks[0]; !reflect.DeepEqual(c.Ours, []string{"mine"}) || c.Theirs != nil {
		t.Errorf("chunk = %+v, want Ours=[mine] Theirs=nil", c)
	}
}

// TestParseConflictMarkersNested: 冲突块内又出现 "<<<<<<<" —— 报错,
// 半个块被丢掉, 后面那个完整的块正常产出
func TestParseConflictMarkersNested(t *testing.T) {
	chunks, err := ParseConflictMarkers([]string{
		"<<<<<<< a", "x", "<<<<<<< b", "y", "=======", "z", ">>>>>>> c",
	})
	if err == nil {
		t.Fatal("expected an error for the nested marker")
	}
	if len(chunks) != 1 || chunks[0].Kind != MergeConflict {
		t.Fatalf("chunks = %+v, want a single conflict chunk", chunks)
	}
	if c := chunks[0]; !reflect.DeepEqual(c.Ours, []string{"y"}) ||
		!reflect.DeepEqual(c.Theirs, []string{"z"}) {
		t.Errorf("chunk = %+v, want Ours=[y] Theirs=[z]", c)
	}
}

// TestRenderConflictMarkersNonConflict: stable/ours/theirs 块只输出各自的
// 最终行, 一个标记都不写
func TestRenderConflictMarkersNonConflict(t *testing.T) {
	chunks := []MergeChunk{
		{Kind: MergeStable, Base: []string{"A"}, Ours: []string{"A"}, Theirs: []string{"A"}},
		{Kind: MergeOurs, Ours: []string{"X"}},
		{Kind: MergeTheirs, Base: []string{"B"}, Ours: []string{"B"}, Theirs: []string{"Y"}},
	}
	want := []string{"A", "X", "Y"}
	got := RenderConflictMarkers(chunks, MergeLabels{Ours: "ours", Base: "base", Theirs: "theirs"})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("render = %q, want %q", got, want)
	}
}

// TestMerge3RenderParseIdempotent: Merge3 的结果渲染成带标记的文本后,
// 再解析 + 再渲染必须得到同一份文本(冲突块原样保留)
func TestMerge3RenderParseIdempotent(t *testing.T) {
	labels := MergeLabels{Ours: "ours", Base: "base", Theirs: "theirs"}
	chunks := Merge3(
		[]string{"A", "B", "C", "D"},
		[]string{"A", "X", "C", "D2"},
		[]string{"A", "Y", "C", "D"},
	)
	first := RenderConflictMarkers(chunks, labels)

	reparsed, err := ParseConflictMarkers(first)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := RenderConflictMarkers(reparsed, labels); !reflect.DeepEqual(got, first) {
		t.Errorf("second render differs:\ngot  %q\nwant %q", got, first)
	}
	// 重新解析后仍然只有一个冲突块, 且两侧内容没变
	var conflicts []MergeChunk
	for _, c := range reparsed {
		if c.Kind == MergeConflict {
			conflicts = append(conflicts, c)
		}
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflicts after reparse = %d, want 1 (%+v)", len(conflicts), reparsed)
	}
	if c := conflicts[0]; !reflect.DeepEqual(c.Ours, []string{"X"}) ||
		!reflect.DeepEqual(c.Theirs, []string{"Y"}) {
		t.Errorf("reparsed conflict = %+v, want Ours=[X] Theirs=[Y]", c)
	}
}

// TestMerge3OneSidedInvariants 是 diff3 的三条基本恒等式, 用固定种子的
// 随机小样本压一遍(顺带保证游标推进不会死循环):
//   - theirs 没动 → 结果必须逐行等于 ours;
//   - ours 没动   → 结果必须逐行等于 theirs;
//   - 双方改成同一份 → 结果就是那份改动, 且一个冲突都没有.
func TestMerge3OneSidedInvariants(t *testing.T) {
	rnd := rand.New(rand.NewSource(20260727))
	// 小字母表 + 短行数, 逼出大量重复行与歧义对齐 —— LCS 最容易出岔子的地方
	alphabet := []string{"a", "b", "c", "d"}
	randLines := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = alphabet[rnd.Intn(len(alphabet))]
		}
		return out
	}

	for i := 0; i < 300; i++ {
		base := randLines(rnd.Intn(7))
		side := randLines(rnd.Intn(7))

		if got := mergedLines(t, Merge3(base, side, base)); !reflect.DeepEqual(got, emptyToNil(side)) {
			t.Fatalf("case %d: Merge3(base, ours, base) = %v, want ours %v (base %v)", i, got, side, base)
		}
		if got := mergedLines(t, Merge3(base, base, side)); !reflect.DeepEqual(got, emptyToNil(side)) {
			t.Fatalf("case %d: Merge3(base, base, theirs) = %v, want theirs %v (base %v)", i, got, side, base)
		}
		if got := mergedLines(t, Merge3(base, side, side)); !reflect.DeepEqual(got, emptyToNil(side)) {
			t.Fatalf("case %d: Merge3(base, x, x) = %v, want x %v (base %v)", i, got, side, base)
		}
	}
}

// emptyToNil 把空切片规整为 nil, 好和 mergedLines 的累加结果直接比较
func emptyToNil(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

// TestMergeKindString 固定 Kind 的名字, 面板表头和测试都依赖它
func TestMergeKindString(t *testing.T) {
	cases := []struct {
		kind MergeKind
		want string
	}{
		{MergeStable, "stable"},
		{MergeOurs, "ours"},
		{MergeTheirs, "theirs"},
		{MergeConflict, "conflict"},
		{MergeKind(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.kind.String(); got != c.want {
			t.Errorf("MergeKind(%d).String() = %q, want %q", int(c.kind), got, c.want)
		}
	}
}

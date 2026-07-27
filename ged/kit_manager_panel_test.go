package ged

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ide/kits"
)

// writeFakeGoMod drops a go.mod into dir and returns dir, so a test can
// point the panel at a project root that is not the process working dir.
func writeFakeGoMod(t *testing.T, dir, body string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(body), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	return dir
}

// rowValue returns the displayed value of the row with the given key.
func rowValue(t *testing.T, p *ProjectSettingsPanel, key string) string {
	t.Helper()
	idx := p.rowIndexByKey(key)
	if idx < 0 {
		t.Fatalf("no row with key %q", key)
	}
	return p.rows[idx].Value
}

// editRow applies val to the row with the given key, the way a click on
// that row would.
func editRow(t *testing.T, p *ProjectSettingsPanel, key, val string) {
	t.Helper()
	idx := p.rowIndexByKey(key)
	if idx < 0 {
		t.Fatalf("no row with key %q", key)
	}
	p.applyEdit(idx, val)
}

// TestProjectSettings_SetProjectRootSelectsWhatIsRead proves the panel reads
// the *selected* project root: pointing it at two different temp roots yields
// each root's own go.mod facts, independent of the process working directory.
func TestProjectSettings_SetProjectRootSelectsWhatIsRead(t *testing.T) {
	rootA := writeFakeGoMod(t, t.TempDir(),
		"module demo/app\n\ngo 1.22\n\ntoolchain go1.24.3\n\nrequire foo v1.0.0\nrequire bar v2.0.0\n")
	rootB := writeFakeGoMod(t, t.TempDir(), "module other/tool\n\ngo 1.21\n")

	p := NewProjectSettingsPanel()

	p.SetProjectRoot(rootA)
	if got := p.ProjectRoot(); got != rootA {
		t.Fatalf("ProjectRoot = %q, want %q", got, rootA)
	}
	if got := rowValue(t, p, psKeyModule); got != "demo/app" {
		t.Errorf("module row = %q, want demo/app", got)
	}
	// toolchain 比 go 指令更强, 展示的必须是它
	if got := rowValue(t, p, psKeyGoVersion); got != "1.24.3" {
		t.Errorf("Go version row = %q, want 1.24.3 (the toolchain wins)", got)
	}
	if got := rowValue(t, p, psKeyToolchain); got != "go1.24.3" {
		t.Errorf("toolchain row = %q, want go1.24.3", got)
	}
	if got := rowValue(t, p, psKeyRequires); got != "2" {
		t.Errorf("requires row = %q, want 2", got)
	}
	if got := rowValue(t, p, psKeyDir); got != rootA {
		t.Errorf("project dir row = %q, want %q", got, rootA)
	}

	p.SetProjectRoot(rootB)
	if got := rowValue(t, p, psKeyModule); got != "other/tool" {
		t.Errorf("module row after switching root = %q, want other/tool", got)
	}
	if got := rowValue(t, p, psKeyGoVersion); got != "1.21" {
		t.Errorf("Go version row = %q, want 1.21", got)
	}
	if got := rowValue(t, p, psKeyToolchain); got != "(默认)" {
		t.Errorf("toolchain row without a directive = %q, want (默认)", got)
	}
}

// TestProjectSettings_RefreshKeepsSelectedRoot verifies the refresh button
// re-reads the selected root, not the process working directory.
func TestProjectSettings_RefreshKeepsSelectedRoot(t *testing.T) {
	root := writeFakeGoMod(t, t.TempDir(), "module demo/app\n\ngo 1.22\n")
	p := NewProjectSettingsPanel()
	p.SetProjectRoot(root)

	writeFakeGoMod(t, root, "module demo/app2\n\ngo 1.22\n\ntoolchain go1.25.1\n")
	p.Refresh()

	if got := p.ProjectRoot(); got != root {
		t.Fatalf("ProjectRoot after Refresh = %q, want %q", got, root)
	}
	if got := rowValue(t, p, psKeyModule); got != "demo/app2" {
		t.Errorf("module row after Refresh = %q, want demo/app2", got)
	}
	if got := rowValue(t, p, psKeyGoVersion); got != "1.25.1" {
		t.Errorf("Go version row after Refresh = %q, want 1.25.1", got)
	}
}

// TestProjectSettings_DefaultKitRows verifies a panel with no kits file still
// shows a usable host kit, and that the legacy accessors keep their meaning.
func TestProjectSettings_DefaultKitRows(t *testing.T) {
	root := writeFakeGoMod(t, t.TempDir(), "module demo/app\n\ngo 1.22\n")
	p := NewProjectSettingsPanel()
	p.SetProjectRoot(root)

	k := p.Kit()
	if k.GOOS == "" || k.GOARCH == "" {
		t.Errorf("default kit target = %s/%s, want the host platform", k.GOOS, k.GOARCH)
	}
	if err := k.Validate(); err != nil {
		t.Errorf("default kit does not validate: %v", err)
	}
	if got := p.OutputDir(); got != "." {
		t.Errorf("OutputDir = %q, want %q", got, ".")
	}
	if got := p.BuildTags(); got != "" {
		t.Errorf("BuildTags = %q, want empty", got)
	}
	if got := p.VariantName(); got != kits.VariantDebug {
		t.Errorf("VariantName = %q, want %q", got, kits.VariantDebug)
	}
	// go.mod 的有效版本要灌进Kit, 免得面板显示的和构建用的不是一个版本
	if k.GoVersion != "1.22" {
		t.Errorf("kit GoVersion = %q, want 1.22 (from the project go.mod)", k.GoVersion)
	}
}

// TestProjectSettings_SigApplyFiresAndPersists is the core of the fix: an
// edit reaches the kit, is written to the project's kits file, and the host
// is told to re-run the build.
func TestProjectSettings_SigApplyFiresAndPersists(t *testing.T) {
	root := writeFakeGoMod(t, t.TempDir(), "module demo/app\n\ngo 1.22\n")
	p := NewProjectSettingsPanel()
	p.SetProjectRoot(root)

	fired := 0
	p.SigApply(func() { fired++ })

	editRow(t, p, psKeyTags, "prod, netgo")
	if fired != 1 {
		t.Fatalf("SigApply fired %d times, want 1", fired)
	}
	if err := p.SaveError(); err != nil {
		t.Fatalf("SaveError = %v, want nil", err)
	}
	if got := p.BuildTags(); got != "prod,netgo" {
		t.Errorf("BuildTags = %q, want prod,netgo", got)
	}
	if got := rowValue(t, p, psKeyTags); got != "prod,netgo" {
		t.Errorf("tags row = %q, want prod,netgo", got)
	}

	// 落盘了吗
	st := kits.New(root)
	if !st.Exists() {
		t.Fatalf("no kits file at %s", st.Path())
	}
	if err := st.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	k, ok := st.ActiveKit()
	if !ok {
		t.Fatal("no active kit in the saved file")
	}
	if want := []string{"prod", "netgo"}; !reflect.DeepEqual(k.Tags, want) {
		t.Errorf("saved Tags = %#v, want %#v", k.Tags, want)
	}

	// 同值再编辑一次不算改动, 不应再通知宿主
	editRow(t, p, psKeyTags, "prod,netgo")
	if fired != 1 {
		t.Errorf("SigApply fired %d times after a no-op edit, want 1", fired)
	}
	// 只读行编辑不动模型
	editRow(t, p, psKeyModule, "hacked")
	if fired != 1 {
		t.Errorf("SigApply fired %d times after editing a read-only row, want 1", fired)
	}
	if got := rowValue(t, p, psKeyModule); got != "demo/app" {
		t.Errorf("module row = %q, want demo/app (read-only)", got)
	}
}

// TestProjectSettings_EditsSurviveANewPanel verifies the edits are still
// there when the panel is rebuilt against the same project root.
func TestProjectSettings_EditsSurviveANewPanel(t *testing.T) {
	root := writeFakeGoMod(t, t.TempDir(), "module demo/app\n\ngo 1.22\n")

	p1 := NewProjectSettingsPanel()
	p1.SetProjectRoot(root)
	editRow(t, p1, psKeyKit, "linux-server")
	editRow(t, p1, psKeyGOOS, "linux")
	editRow(t, p1, psKeyGOARCH, "arm64")
	editRow(t, p1, psKeyOutputDir, "dist")
	editRow(t, p1, psKeyBuildMode, "pie")
	editRow(t, p1, psKeyRace, "on")
	editRow(t, p1, psKeyCoverage, "on")
	if err := p1.SaveError(); err != nil {
		t.Fatalf("SaveError = %v, want nil", err)
	}

	p2 := NewProjectSettingsPanel()
	p2.SetProjectRoot(root)
	k := p2.Kit()
	if k.Name != "linux-server" || k.GOOS != "linux" || k.GOARCH != "arm64" {
		t.Errorf("reloaded kit = %+v, want linux-server linux/arm64", k)
	}
	if k.OutputDir != "dist" || k.BuildMode != "pie" || !k.Race || !k.Coverage {
		t.Errorf("reloaded build settings = %+v", k)
	}
	if got := rowValue(t, p2, psKeyBuildMode); got != "pie" {
		t.Errorf("build mode row = %q, want pie", got)
	}
	if got := rowValue(t, p2, psKeyRace); got != "on" {
		t.Errorf("race row = %q, want on", got)
	}
}

// TestProjectSettings_NoRootNoFile verifies the panel refuses to drop a kits
// file into the process working directory when nobody told it which project
// it belongs to.
func TestProjectSettings_NoRootNoFile(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Skipf("cannot determine the working directory: %v", err)
	}
	stray := filepath.Join(wd, kits.FileName)
	if _, err := os.Stat(stray); err == nil {
		t.Skipf("%s already exists; cannot tell a stray write apart", stray)
	}
	// 万一门禁失效, 别把文件留在工作树里
	t.Cleanup(func() { _ = os.Remove(stray) })

	p := NewProjectSettingsPanel()
	fired := 0
	p.SigApply(func() { fired++ })
	editRow(t, p, psKeyOutputDir, "dist")

	if fired != 1 {
		t.Errorf("SigApply fired %d times, want 1 (the in-memory kit did change)", fired)
	}
	if got := p.OutputDir(); got != "dist" {
		t.Errorf("OutputDir = %q, want dist (edit kept in memory)", got)
	}
	if _, err := os.Stat(stray); err == nil {
		t.Fatalf("panel without an explicit project root wrote %s", stray)
	}
}

// TestProjectSettings_DeployProfile walks the ssh deploy profile: a
// half-configured profile is reported and never written, a complete one is.
func TestProjectSettings_DeployProfile(t *testing.T) {
	root := writeFakeGoMod(t, t.TempDir(), "module demo/app\n\ngo 1.22\n")
	p := NewProjectSettingsPanel()
	p.SetProjectRoot(root)

	if got := rowValue(t, p, psKeyDeployKind); got != core.DeployLocal {
		t.Errorf("deploy kind row = %q, want %q", got, core.DeployLocal)
	}

	// 未知方式: 拒绝, 模型不动
	editRow(t, p, psKeyDeployKind, "ftp")
	if got := p.Kit().Deploy.Kind; got != core.DeployLocal {
		t.Errorf("deploy kind = %q after an unknown value, want %q", got, core.DeployLocal)
	}
	if p.SaveError() == nil {
		t.Error("SaveError = nil after an unknown deploy kind, want an error")
	}

	// ssh 但还没填主机: 编辑留在内存, 不落盘
	editRow(t, p, psKeyDeployKind, core.DeploySSH)
	if got := p.Kit().Deploy.Kind; got != core.DeploySSH {
		t.Errorf("deploy kind = %q, want ssh", got)
	}
	if p.SaveError() == nil {
		t.Error("SaveError = nil for a half-configured ssh deploy, want an error")
	}
	if st := kits.New(root); st.Exists() {
		t.Errorf("an invalid kit was written to %s", st.Path())
	}

	// 补齐主机和远程目录后才允许落盘
	editRow(t, p, psKeyDeployHost, "10.0.0.9")
	editRow(t, p, psKeyDeployUser, "deploy")
	editRow(t, p, psKeyRemoteDir, "/opt/app")
	if err := p.SaveError(); err != nil {
		t.Fatalf("SaveError = %v after completing the profile, want nil", err)
	}

	st := kits.New(root)
	if err := st.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	k, ok := st.ActiveKit()
	if !ok {
		t.Fatal("no active kit in the saved file")
	}
	want := core.DeployProfile{Kind: core.DeploySSH, Host: "10.0.0.9", User: "deploy", RemoteDir: "/opt/app"}
	if k.Deploy != want {
		t.Errorf("saved deploy profile = %+v, want %+v", k.Deploy, want)
	}
}

// TestProjectSettings_VariantSwitch verifies the build variant switch, its
// persistence, and that ResolvedKit is the kit the build should actually use.
func TestProjectSettings_VariantSwitch(t *testing.T) {
	root := writeFakeGoMod(t, t.TempDir(), "module demo/app\n\ngo 1.22\n")
	p := NewProjectSettingsPanel()
	p.SetProjectRoot(root)
	fired := 0
	p.SigApply(func() { fired++ })

	// Debug 变体开竞态检测, 产物落 build/debug
	res := p.ResolvedKit()
	if !res.Race || res.OutputDir != "build/debug" {
		t.Errorf("resolved Debug kit = %+v, want race on and build/debug", res)
	}
	if p.Kit().Race {
		t.Error("the variant overlay leaked into the kit itself")
	}

	if err := p.SetVariantName("release"); err != nil {
		t.Fatalf("SetVariantName: %v", err)
	}
	if got := p.VariantName(); got != kits.VariantRelease {
		t.Errorf("VariantName = %q, want %q", got, kits.VariantRelease)
	}
	if fired != 1 {
		t.Errorf("SigApply fired %d times, want 1", fired)
	}
	res = p.ResolvedKit()
	if res.Race || res.OutputDir != "build/release" {
		t.Errorf("resolved Release kit = %+v, want race off and build/release", res)
	}

	// 未知变体: 报错且不改动
	if err := p.SetVariantName("Profiling"); err == nil {
		t.Error("SetVariantName(Profiling) = nil, want an error")
	}
	if got := p.VariantName(); got != kits.VariantRelease {
		t.Errorf("VariantName after a failed switch = %q, want %q", got, kits.VariantRelease)
	}

	// 持久化: 新面板要恢复到 Release
	st := kits.New(root)
	if err := st.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := st.ActiveVariantName(); got != kits.VariantRelease {
		t.Errorf("saved active variant = %q, want %q", got, kits.VariantRelease)
	}
	p2 := NewProjectSettingsPanel()
	p2.SetProjectRoot(root)
	if got := p2.VariantName(); got != kits.VariantRelease {
		t.Errorf("reloaded VariantName = %q, want %q", got, kits.VariantRelease)
	}

	// 通过行编辑切回 Debug
	editRow(t, p2, psKeyVariant, "Debug")
	if got := p2.VariantName(); got != kits.VariantDebug {
		t.Errorf("VariantName after the row edit = %q, want %q", got, kits.VariantDebug)
	}
	editRow(t, p2, psKeyVariant, "Nope")
	if got := p2.VariantName(); got != kits.VariantDebug {
		t.Errorf("VariantName after an invalid row edit = %q, want %q", got, kits.VariantDebug)
	}
}

// TestProjectSettings_SetKitAndLoadKits covers the programmatic host path.
func TestProjectSettings_SetKitAndLoadKits(t *testing.T) {
	root := writeFakeGoMod(t, t.TempDir(), "module demo/app\n\ngo 1.22\n")
	p := NewProjectSettingsPanel()
	p.SetProjectRoot(root)
	fired := 0
	p.SigApply(func() { fired++ })

	k := core.DefaultKit()
	k.Name = "cross-linux"
	k.GOOS = "linux"
	k.Tags = []string{"prod"}
	p.SetKit(k)
	if fired != 1 {
		t.Fatalf("SigApply fired %d times, want 1", fired)
	}
	if err := p.SaveError(); err != nil {
		t.Fatalf("SaveError = %v, want nil", err)
	}
	if got := rowValue(t, p, psKeyKit); got != "cross-linux" {
		t.Errorf("kit row = %q, want cross-linux", got)
	}

	// 未保存的内存改动被 LoadKits 丢弃
	p.kit.Tags = []string{"scratch"}
	if err := p.LoadKits(); err != nil {
		t.Fatalf("LoadKits: %v", err)
	}
	if got := p.BuildTags(); got != "prod" {
		t.Errorf("BuildTags after LoadKits = %q, want prod", got)
	}
}

// TestProjectSettings_RejectedEditsResyncRows verifies a rejected edit is not
// left sitting on screen.
func TestProjectSettings_RejectedEditsResyncRows(t *testing.T) {
	root := writeFakeGoMod(t, t.TempDir(), "module demo/app\n\ngo 1.22\n")
	p := NewProjectSettingsPanel()
	p.SetProjectRoot(root)

	editRow(t, p, psKeyRace, "maybe")
	if p.Kit().Race {
		t.Error("a non-boolean turned the race switch on")
	}
	if got := rowValue(t, p, psKeyRace); got != "off" {
		t.Errorf("race row = %q, want off", got)
	}

	editRow(t, p, psKeyBuildMode, "turbo")
	if got := p.Kit().BuildMode; got != "" {
		t.Errorf("build mode = %q after an unknown mode, want empty", got)
	}
	if got := rowValue(t, p, psKeyBuildMode); got != "default" {
		t.Errorf("build mode row = %q, want default", got)
	}

	editRow(t, p, psKeyKit, "   ")
	if got := p.Kit().Name; got == "" {
		t.Error("a blank kit name was accepted")
	}
}

func TestParseSettingsBool(t *testing.T) {
	trues := []string{"on", "ON", "true", "True", "yes", "y", "1", " on ", "开"}
	for _, s := range trues {
		b, ok := parseSettingsBool(s)
		if !ok || !b {
			t.Errorf("parseSettingsBool(%q) = (%v, %v), want (true, true)", s, b, ok)
		}
	}
	falses := []string{"off", "OFF", "false", "no", "n", "0", " off ", "关"}
	for _, s := range falses {
		b, ok := parseSettingsBool(s)
		if !ok || b {
			t.Errorf("parseSettingsBool(%q) = (%v, %v), want (false, true)", s, b, ok)
		}
	}
	for _, s := range []string{"", "maybe", "2", "on/off"} {
		if _, ok := parseSettingsBool(s); ok {
			t.Errorf("parseSettingsBool(%q) reported ok, want rejected", s)
		}
	}
	if got := formatSettingsBool(true); got != "on" {
		t.Errorf("formatSettingsBool(true) = %q, want on", got)
	}
	if got := formatSettingsBool(false); got != "off" {
		t.Errorf("formatSettingsBool(false) = %q, want off", got)
	}
}

// TestProjectSettings_RowKeysUnique guards the key-addressed rows: a
// duplicate key would make an edit land on the wrong field.
func TestProjectSettings_RowKeysUnique(t *testing.T) {
	root := writeFakeGoMod(t, t.TempDir(), "module demo/app\n\ngo 1.22\n")
	p := NewProjectSettingsPanel()
	p.SetProjectRoot(root)

	seen := map[string]bool{}
	for i, r := range p.rows {
		if r.Key == "" {
			t.Errorf("rows[%d] (%q) has no key", i, r.Label)
			continue
		}
		if seen[r.Key] {
			t.Errorf("duplicate row key %q", r.Key)
		}
		seen[r.Key] = true
	}
	for _, key := range []string{
		psKeyDir, psKeyModule, psKeyGoVersion, psKeyToolchain, psKeyRequires,
		psKeyKit, psKeyGOOS, psKeyGOARCH, psKeyTags, psKeyBuildMode,
		psKeyRace, psKeyCoverage, psKeyOutputDir, psKeyVariant,
		psKeyDeployKind, psKeyDeployHost, psKeyDeployUser, psKeyRemoteDir,
	} {
		if !seen[key] {
			t.Errorf("row key %q is missing from the panel", key)
		}
	}
	// 只读的是 go.mod 事实行, 其余都可编辑
	for _, r := range p.rows {
		wantEditable := true
		switch r.Key {
		case psKeyDir, psKeyModule, psKeyGoVersion, psKeyToolchain, psKeyRequires:
			wantEditable = false
		}
		if r.Editable != wantEditable {
			t.Errorf("row %q editable = %v, want %v", r.Key, r.Editable, wantEditable)
		}
	}
	if got, want := p.contentHeight(), psHeaderH+float64(len(p.rows))*psRowH; got <= want {
		t.Errorf("contentHeight = %v, want more than the rows themselves (%v)", got, want)
	}
}

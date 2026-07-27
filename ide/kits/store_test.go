package kits

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/uk0/silk/core"
)

func hostKit(name string) core.Kit {
	k := core.DefaultKit()
	k.Name = name
	return k
}

func sshKit(name string) core.Kit {
	k := hostKit(name)
	k.GOOS = "linux"
	k.GOARCH = "amd64"
	k.Tags = []string{"prod"}
	k.Env = map[string]string{"CGO_ENABLED": "0"}
	k.OutputDir = "build/release"
	k.Deploy = core.DeployProfile{
		Kind: core.DeploySSH, Host: "10.0.0.9", User: "deploy", RemoteDir: "/opt/app",
	}
	return k
}

func TestStore_NewDefaults(t *testing.T) {
	s := New(t.TempDir())
	if got := s.List(); len(got) != 0 {
		t.Errorf("List on a fresh store = %#v, want empty", got)
	}
	vs := s.Variants()
	if len(vs) != 2 || vs[0].Name != VariantDebug || vs[1].Name != VariantRelease {
		t.Fatalf("Variants = %#v, want Debug + Release", vs)
	}
	if got := s.ActiveVariantName(); got != VariantDebug {
		t.Errorf("ActiveVariantName = %q, want %q", got, VariantDebug)
	}
	if got := s.ActiveKitName(); got != "" {
		t.Errorf("ActiveKitName = %q, want empty", got)
	}
	if _, ok := s.Resolved(); ok {
		t.Error("Resolved() with no kits should report false")
	}
}

func TestStore_Path(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if got, want := s.Path(), filepath.Join(dir, FileName); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
	if s.Dir() != dir {
		t.Errorf("Dir = %q, want %q", s.Dir(), dir)
	}
	if s.Exists() {
		t.Error("Exists on a fresh dir = true, want false")
	}
}

// TestStore_LoadMissingFileKeepsDefaults: 工程还没配过Kit不是错误
func TestStore_LoadMissingFileKeepsDefaults(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Load(); err != nil {
		t.Fatalf("Load on a missing file = %v, want nil", err)
	}
	if len(s.List()) != 0 || len(s.Variants()) != 2 {
		t.Errorf("defaults lost: kits=%#v variants=%#v", s.List(), s.Variants())
	}
}

func TestStore_LoadMalformedKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(dir)
	if err := s.Load(); err == nil {
		t.Fatal("Load on a corrupt file = nil, want error")
	}
	if len(s.List()) != 0 || len(s.Variants()) != 2 {
		t.Errorf("corrupt file clobbered defaults: kits=%#v variants=%#v", s.List(), s.Variants())
	}
}

func TestStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	local := hostKit("desktop")
	remote := sshKit("linux-server")
	if err := s.Set(local); err != nil {
		t.Fatalf("Set(local): %v", err)
	}
	if err := s.Set(remote); err != nil {
		t.Fatalf("Set(remote): %v", err)
	}
	// 第一个加入的Kit自动激活
	if got := s.ActiveKitName(); got != "desktop" {
		t.Fatalf("ActiveKitName = %q, want %q", got, "desktop")
	}
	if err := s.SetActiveKit("linux-server"); err != nil {
		t.Fatalf("SetActiveKit: %v", err)
	}
	if err := s.SetActiveVariant(VariantRelease); err != nil {
		t.Fatalf("SetActiveVariant: %v", err)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !s.Exists() {
		t.Fatalf("Exists after Save = false, want true (%s)", s.Path())
	}

	s2 := New(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := s2.List(), []core.Kit{local, remote}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reloaded kits = %#v\nwant %#v", got, want)
	}
	if got := s2.ActiveKitName(); got != "linux-server" {
		t.Errorf("reloaded ActiveKitName = %q, want %q", got, "linux-server")
	}
	if got := s2.ActiveVariantName(); got != VariantRelease {
		t.Errorf("reloaded ActiveVariantName = %q, want %q", got, VariantRelease)
	}
	k, ok := s2.ActiveKit()
	if !ok {
		t.Fatal("reloaded ActiveKit not found")
	}
	if !reflect.DeepEqual(k, remote) {
		t.Errorf("reloaded ActiveKit = %#v\nwant %#v", k, remote)
	}
}

func TestStore_GetSetRemove(t *testing.T) {
	s := New(t.TempDir())
	if _, ok := s.Get("nope"); ok {
		t.Error("Get on an empty store reported ok")
	}
	if s.Remove("nope") {
		t.Error("Remove of an unknown kit reported true")
	}

	if err := s.Set(hostKit("a")); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(hostKit("b")); err != nil {
		t.Fatal(err)
	}

	// 同名 Set 是覆盖, 不是追加
	upd := hostKit("a")
	upd.OutputDir = "dist"
	if err := s.Set(upd); err != nil {
		t.Fatal(err)
	}
	if got := len(s.List()); got != 2 {
		t.Fatalf("len(List) = %d, want 2 after same-name Set", got)
	}
	got, ok := s.Get("a")
	if !ok || got.OutputDir != "dist" {
		t.Fatalf("Get(a) = %#v ok=%v, want OutputDir dist", got, ok)
	}

	// 删掉激活Kit时激活项顺延
	if s.ActiveKitName() != "a" {
		t.Fatalf("ActiveKitName = %q, want a", s.ActiveKitName())
	}
	if !s.Remove("a") {
		t.Fatal("Remove(a) = false")
	}
	if got := s.ActiveKitName(); got != "b" {
		t.Errorf("ActiveKitName after removing the active kit = %q, want b", got)
	}
	if !s.Remove("b") {
		t.Fatal("Remove(b) = false")
	}
	if got := s.ActiveKitName(); got != "" {
		t.Errorf("ActiveKitName after removing everything = %q, want empty", got)
	}
}

func TestStore_SetRejectsInvalidKit(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Set(core.Kit{}); err == nil {
		t.Fatal("Set of a nameless kit = nil, want error")
	}
	half := sshKit("broken")
	half.Deploy.Host = ""
	if err := s.Set(half); err == nil {
		t.Fatal("Set of a half-configured ssh kit = nil, want error")
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("rejected kits reached the store: %#v", got)
	}
}

func TestStore_SetActiveKitUnknown(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Set(hostKit("a")); err != nil {
		t.Fatal(err)
	}
	if err := s.SetActiveKit("ghost"); err == nil {
		t.Fatal("SetActiveKit(ghost) = nil, want error")
	}
	if got := s.ActiveKitName(); got != "a" {
		t.Errorf("ActiveKitName = %q, want it unchanged (a)", got)
	}
}

func TestStore_VariantSwitchAndResolve(t *testing.T) {
	s := New(t.TempDir())
	k := hostKit("desktop")
	k.OutputDir = "."
	if err := s.Set(k); err != nil {
		t.Fatal(err)
	}

	// Debug: 变体开竞态检测, 产物落 build/debug
	res, ok := s.Resolved()
	if !ok {
		t.Fatal("Resolved = false, want the active kit")
	}
	if !res.Race {
		t.Error("Debug variant did not turn Race on")
	}
	if res.OutputDir != "build/debug" {
		t.Errorf("Debug OutputDir = %q, want build/debug", res.OutputDir)
	}

	// Release: 不开竞态, 产物落 build/release
	if err := s.SetActiveVariant(VariantRelease); err != nil {
		t.Fatal(err)
	}
	res, _ = s.Resolved()
	if res.Race {
		t.Error("Release variant turned Race on")
	}
	if res.OutputDir != "build/release" {
		t.Errorf("Release OutputDir = %q, want build/release", res.OutputDir)
	}

	// 叠加不能改到存储里的Kit本身
	stored, _ := s.Get("desktop")
	if stored.Race || stored.OutputDir != "." {
		t.Errorf("stored kit was mutated by the overlay: %#v", stored)
	}

	// 大小写不敏感, 但写回磁盘的是规范名
	if err := s.SetActiveVariant("debug"); err != nil {
		t.Fatalf("SetActiveVariant(debug): %v", err)
	}
	if got := s.ActiveVariantName(); got != VariantDebug {
		t.Errorf("ActiveVariantName = %q, want %q", got, VariantDebug)
	}
	if err := s.SetActiveVariant("Profiling"); err == nil {
		t.Fatal("SetActiveVariant of an unknown variant = nil, want error")
	}
	if got := s.ActiveVariantName(); got != VariantDebug {
		t.Errorf("ActiveVariantName after a failed switch = %q, want %q", got, VariantDebug)
	}
}

func TestStore_SetVariant(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SetVariant(Variant{Name: "  "}); err == nil {
		t.Fatal("SetVariant with a blank name = nil, want error")
	}
	if err := s.SetVariant(Variant{Name: "Profiling", Tags: []string{"pprof"}, OutputDir: "build/prof"}); err != nil {
		t.Fatal(err)
	}
	if got := len(s.Variants()); got != 3 {
		t.Fatalf("len(Variants) = %d, want 3", got)
	}
	// 覆盖已有变体
	if err := s.SetVariant(Variant{Name: VariantRelease, OutputDir: "out"}); err != nil {
		t.Fatal(err)
	}
	if got := len(s.Variants()); got != 3 {
		t.Fatalf("len(Variants) = %d, want 3 after an overwrite", got)
	}
	v, ok := s.Variant(VariantRelease)
	if !ok || v.OutputDir != "out" {
		t.Fatalf("Variant(Release) = %#v ok=%v", v, ok)
	}
}

func TestVariantApply(t *testing.T) {
	kit := core.Kit{Name: "k", GOOS: "linux", GOARCH: "amd64", Tags: []string{"prod"}, OutputDir: "bin"}
	v := Variant{Name: "Debug", Tags: []string{"prod", "debugtrace"}, Race: true, Coverage: true, OutputDir: "build/debug"}
	got := v.Apply(kit)

	if want := []string{"prod", "debugtrace"}; !reflect.DeepEqual(got.Tags, want) {
		t.Errorf("merged Tags = %#v, want %#v (no duplicates)", got.Tags, want)
	}
	if !got.Race || !got.Coverage {
		t.Errorf("switches not applied: %#v", got)
	}
	if got.OutputDir != "build/debug" {
		t.Errorf("OutputDir = %q, want build/debug", got.OutputDir)
	}
	// 入参不能被改
	if !reflect.DeepEqual(kit.Tags, []string{"prod"}) || kit.Race || kit.OutputDir != "bin" {
		t.Errorf("Apply mutated its input: %#v", kit)
	}
	// 空变体是恒等变换
	if idn := (Variant{Name: "x"}).Apply(kit); !reflect.DeepEqual(idn, kit) {
		t.Errorf("empty variant changed the kit: %#v", idn)
	}
}

func TestStore_ListReturnsCopies(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Set(sshKit("linux-server")); err != nil {
		t.Fatal(err)
	}
	got := s.List()
	got[0].Tags[0] = "mutated"
	got[0].Env["CGO_ENABLED"] = "1"

	again, _ := s.Get("linux-server")
	if again.Tags[0] != "prod" || again.Env["CGO_ENABLED"] != "0" {
		t.Fatalf("List did not return a deep copy: %#v", again)
	}
}

func TestStore_SaveWithoutDir(t *testing.T) {
	s := New("")
	if err := s.Save(); err == nil {
		t.Fatal("Save without a project dir = nil, want error")
	}
}

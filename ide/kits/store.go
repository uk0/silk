// Package kits 存放"某个工程用哪些Kit构建"的持久化状态:
// Kit列表, 叠在Kit上的构建变体(Debug/Release), 以及当前激活的那一组.
//
// Store 绑定一个工程根目录, 只读写该目录下的一个文件 <root>/.silk-kits.json,
// 自己不探测机器 —— 需要填充新工程时由调用方跑 core.DetectKits 再 Set 进来.
package kits

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/uk0/silk/core"
)

// FileName 是工程级Kit文件名, 放在工程根目录下
const FileName = ".silk-kits.json"

// fileVersion 在磁盘结构不兼容变更时递增
const fileVersion = 1

// 默认构建变体名
const (
	VariantDebug   = "Debug"
	VariantRelease = "Release"
)

// Variant 是叠在Kit之上的具名构建变体, 对应 Qt Creator 的 Debug/Release
// 构建配置. 零值字段表示"沿用Kit本身的设置", 只有非零字段才覆盖
type Variant struct {
	Name      string   `json:"name"`
	Tags      []string `json:"tags,omitempty"`       // 追加到Kit的tag后面
	Race      bool     `json:"race,omitempty"`       // 与Kit的Race取或
	Coverage  bool     `json:"coverage,omitempty"`   // 与Kit的Coverage取或
	OutputDir string   `json:"output_dir,omitempty"` // 非空则覆盖Kit的输出目录
}

// DefaultVariants 是新工程的两个标准变体: Debug 开竞态检测, Release 不开,
// 产物分别落在 build/debug 与 build/release, 避免两种构建互相覆盖
func DefaultVariants() []Variant {
	return []Variant{
		{Name: VariantDebug, Race: true, OutputDir: "build/debug"},
		{Name: VariantRelease, OutputDir: "build/release"},
	}
}

// Apply 把变体叠到kit上, 返回合并后的新Kit(不改动入参)
func (v Variant) Apply(kit core.Kit) core.Kit {
	out := kit.Clone()
	for _, t := range v.Tags {
		if t == "" || containsString(out.Tags, t) {
			continue
		}
		out.Tags = append(out.Tags, t)
	}
	if v.Race {
		out.Race = true
	}
	if v.Coverage {
		out.Coverage = true
	}
	if v.OutputDir != "" {
		out.OutputDir = v.OutputDir
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// fileData 是 .silk-kits.json 的磁盘结构
type fileData struct {
	Version       int        `json:"version"`
	ActiveKit     string     `json:"active_kit"`
	ActiveVariant string     `json:"active_variant"`
	Kits          []core.Kit `json:"kits"`
	Variants      []Variant  `json:"variants"`
}

// Store 是一个工程的Kit配置
// New 之后是"默认状态"(无Kit, Debug/Release两个变体, 激活Debug),
// Load 把磁盘上的内容读进来, Save 写回去
type Store struct {
	dir  string
	data fileData
}

// New 返回绑定到工程根目录dir的Store, 内容是默认状态
func New(dir string) *Store {
	return &Store{
		dir: dir,
		data: fileData{
			Version:       fileVersion,
			ActiveVariant: VariantDebug,
			Variants:      DefaultVariants(),
		},
	}
}

// Dir 返回Store绑定的工程根目录
func (s *Store) Dir() string { return s.dir }

// Path 返回Kit文件的路径
func (s *Store) Path() string { return filepath.Join(s.dir, FileName) }

// Exists 报告Kit文件是否已经存在
func (s *Store) Exists() bool {
	fi, err := os.Stat(s.Path())
	return err == nil && !fi.IsDir()
}

// Load 读取Kit文件
// 文件不存在不算错误: 这是"工程还没配过Kit", Store保持默认状态并返回nil.
// 文件损坏时返回包装后的error, 同样保持默认状态, 绝不让脏文件把面板带崩
func (s *Store) Load() error {
	raw, err := os.ReadFile(s.Path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", s.Path(), err)
	}
	var d fileData
	if err := json.Unmarshal(raw, &d); err != nil {
		return fmt.Errorf("parse %s: %w", s.Path(), err)
	}
	if d.Version == 0 {
		d.Version = fileVersion
	}
	if len(d.Variants) == 0 {
		d.Variants = DefaultVariants()
	}
	if d.ActiveVariant == "" {
		d.ActiveVariant = d.Variants[0].Name
	}
	s.data = d
	return nil
}

// Save 把当前状态写回Kit文件
func (s *Store) Save() error {
	if s.dir == "" {
		return errors.New("kits: store has no project dir")
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", s.dir, err)
	}
	s.data.Version = fileVersion
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.Path(), append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", s.Path(), err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Kits
// ---------------------------------------------------------------------------

// List 返回所有Kit的副本, 顺序即写入顺序
func (s *Store) List() []core.Kit {
	out := make([]core.Kit, 0, len(s.data.Kits))
	for _, k := range s.data.Kits {
		out = append(out, k.Clone())
	}
	return out
}

// Get 按名字取Kit(副本)
func (s *Store) Get(name string) (core.Kit, bool) {
	if i := s.indexOf(name); i >= 0 {
		return s.data.Kits[i].Clone(), true
	}
	return core.Kit{}, false
}

// Set 新增或按名字覆盖一个Kit
// 校验不过的Kit一律拒绝, 免得把跑不起来的配置写进工程文件.
// 第一个被加进来的Kit自动成为激活Kit
func (s *Store) Set(k core.Kit) error {
	if err := k.Validate(); err != nil {
		return err
	}
	k = k.Clone()
	if i := s.indexOf(k.Name); i >= 0 {
		s.data.Kits[i] = k
	} else {
		s.data.Kits = append(s.data.Kits, k)
	}
	if s.data.ActiveKit == "" {
		s.data.ActiveKit = k.Name
	}
	return nil
}

// Remove 按名字删除Kit, 返回是否真的删掉了
// 删掉的正好是激活Kit时, 激活项顺延到剩下的第一个(没有则清空)
func (s *Store) Remove(name string) bool {
	i := s.indexOf(name)
	if i < 0 {
		return false
	}
	s.data.Kits = append(s.data.Kits[:i], s.data.Kits[i+1:]...)
	if s.data.ActiveKit == name {
		s.data.ActiveKit = ""
		if len(s.data.Kits) > 0 {
			s.data.ActiveKit = s.data.Kits[0].Name
		}
	}
	return true
}

// ActiveKitName 返回激活Kit的名字, 没有则为 ""
func (s *Store) ActiveKitName() string { return s.data.ActiveKit }

// ActiveKit 返回激活Kit(副本)
func (s *Store) ActiveKit() (core.Kit, bool) {
	if s.data.ActiveKit == "" {
		return core.Kit{}, false
	}
	return s.Get(s.data.ActiveKit)
}

// SetActiveKit 切换激活Kit, 名字不存在时报错且不改动状态
func (s *Store) SetActiveKit(name string) error {
	if s.indexOf(name) < 0 {
		return fmt.Errorf("kits: unknown kit %q", name)
	}
	s.data.ActiveKit = name
	return nil
}

func (s *Store) indexOf(name string) int {
	for i := range s.data.Kits {
		if s.data.Kits[i].Name == name {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Variants
// ---------------------------------------------------------------------------

// Variants 返回所有构建变体的副本
func (s *Store) Variants() []Variant {
	out := make([]Variant, 0, len(s.data.Variants))
	for _, v := range s.data.Variants {
		cp := v
		if len(v.Tags) > 0 {
			cp.Tags = make([]string, len(v.Tags))
			copy(cp.Tags, v.Tags)
		}
		out = append(out, cp)
	}
	return out
}

// Variant 按名字取变体, 名字大小写不敏感(用户手输 "release" 也能命中)
func (s *Store) Variant(name string) (Variant, bool) {
	for _, v := range s.Variants() {
		if strings.EqualFold(v.Name, name) {
			return v, true
		}
	}
	return Variant{}, false
}

// SetVariant 新增或按名字覆盖一个变体
func (s *Store) SetVariant(v Variant) error {
	if strings.TrimSpace(v.Name) == "" {
		return errors.New("kits: variant name is empty")
	}
	for i := range s.data.Variants {
		if strings.EqualFold(s.data.Variants[i].Name, v.Name) {
			s.data.Variants[i] = v
			return nil
		}
	}
	s.data.Variants = append(s.data.Variants, v)
	return nil
}

// ActiveVariantName 返回激活变体的名字
func (s *Store) ActiveVariantName() string { return s.data.ActiveVariant }

// ActiveVariant 返回激活变体
func (s *Store) ActiveVariant() (Variant, bool) {
	return s.Variant(s.data.ActiveVariant)
}

// SetActiveVariant 切换激活变体, 名字不存在时报错且不改动状态
// 命中后写回变体自身的规范名字, 磁盘上不会留下 "release" 这类随手输入的大小写
func (s *Store) SetActiveVariant(name string) error {
	v, ok := s.Variant(name)
	if !ok {
		return fmt.Errorf("kits: unknown variant %q", name)
	}
	s.data.ActiveVariant = v.Name
	return nil
}

// Resolved 返回"激活Kit叠上激活变体"的最终构建配置
// 没有激活Kit时返回false; 有Kit但变体缺失时返回未叠加的Kit
func (s *Store) Resolved() (core.Kit, bool) {
	k, ok := s.ActiveKit()
	if !ok {
		return core.Kit{}, false
	}
	if v, ok := s.ActiveVariant(); ok {
		return v.Apply(k), true
	}
	return k, true
}

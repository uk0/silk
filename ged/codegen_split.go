package ged

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"

	"github.com/uk0/silk/graph"
)

// One design, two files, one owner each — the split that makes regeneration
// safe. <base>.silk.go is the machine's: it is rewritten wholesale every time,
// so nothing a developer types may live there. <base>.go is the developer's: it
// is written once and from then on the only write codegen makes to it is
// appending a stub for a handler the design has gained.

// SplitResult reports what one generation did. Only the user side is worth
// reporting — the machine file is rewritten unconditionally.
type SplitResult struct {
	MachineFile string   // <base>.silk.go, rewritten wholesale
	UserFile    string   // <base>.go, created once and thereafter only appended to
	UserCreated bool     // the user file was absent and written from scratch
	AddedStubs  []string // handler methods appended to an already existing user file
}

// handlerStub is one handler method the user file owns. The machine file calls
// ui.<name>, so the user file has to declare it or the pair does not compile.
type handlerStub struct {
	name   string
	params string // parameter list for a synthesized stub, from widgetEvents
	imp    string // import the parameter list references, "" for none
	body   string // design-time source seeding the declaration, "" for an empty stub
}

// GenerateCodeFile writes the design as the machine/user pair described above.
// filename names either half (or neither): its extension is dropped and both
// files are derived from the stem, so exporting over an earlier export lands on
// the same pair.
func (scene *GedScene) GenerateCodeFile(filename string, opts CodeGenOptions) (SplitResult, error) {
	opts = defaultCodeGenOptions(scene, opts)
	machine, user := splitCodePaths(filename)
	res := SplitResult{MachineFile: machine, UserFile: user}

	machineOpts := opts
	machineOpts.SplitHandlers = true
	if err := os.WriteFile(machine, []byte(scene.GenerateCode(machineOpts)), 0644); err != nil {
		return res, err
	}

	stubs := scene.collectHandlerStubs()
	existing, err := os.ReadFile(user)
	if os.IsNotExist(err) {
		res.UserCreated = true
		return res, os.WriteFile(user, []byte(renderUserCode(opts, scene.needsServices(), stubs)), 0644)
	}
	if err != nil {
		return res, err
	}

	updated, added, err := appendMissingStubs(string(existing), opts.TypeName, stubs)
	if err != nil {
		return res, fmt.Errorf("%s: %w", user, err)
	}
	res.AddedStubs = added
	if len(added) == 0 {
		return res, nil
	}
	return res, os.WriteFile(user, []byte(updated), 0644)
}

// GenerateUserCode renders the user-owned half from scratch, as it is written
// the first time a design is generated.
func (scene *GedScene) GenerateUserCode(opts CodeGenOptions) string {
	opts = defaultCodeGenOptions(scene, opts)
	return renderUserCode(opts, scene.needsServices(), scene.collectHandlerStubs())
}

// splitCodePaths derives the machine/user pair from the path the user picked.
// Naming either half names the same pair.
func splitCodePaths(filename string) (machine, user string) {
	base := filename
	switch {
	case strings.HasSuffix(base, ".silk.go"):
		base = strings.TrimSuffix(base, ".silk.go")
	case strings.HasSuffix(base, ".go"):
		base = strings.TrimSuffix(base, ".go")
	}
	return base + ".silk.go", base + ".go"
}

// collectHandlerStubs returns one declaration per distinct handler the user file
// owes the machine file, in scene order. Two widgets sharing a handler name
// produce one method: they call the same one.
//
// A widget's design-time code attr comes first and keeps its own signature —
// it is the developer's source, and after this generation the user file is
// where it lives. A handler that is only wired (bound in the property sheet,
// never written) is synthesized empty from widgetEvents, which is what lets a
// design that has never been generated still produce a pair that builds.
func (scene *GedScene) collectHandlerStubs() []handlerStub {
	var out []handlerStub
	seen := map[string]bool{}
	walkFakeWidgets(scene.Children(), func(fake *FakeWidget) {
		code := strings.TrimSpace(fake.GetCode())
		if name := extractHandlerName(code); name != "" && !seen[name] {
			seen[name] = true
			out = append(out, handlerStub{name: name, body: code})
		}
		for _, b := range handlerBindingsFor(fake.WidgetFactoryName(), code, fake.EventHandlers()) {
			if !b.known || b.handler == "" || seen[b.handler] {
				continue
			}
			seen[b.handler] = true
			out = append(out, handlerStub{name: b.handler, params: b.params, imp: b.imp})
		}
	})
	return out
}

// needsServices reports whether the generated app owns a shared scada.Services,
// so the user file's main() builds one exactly when the machine file declares a
// BindServices to take it. It answers through sceneNeedsServices rather than
// re-deciding, so the two files cannot disagree.
func (scene *GedScene) needsServices() bool {
	var fields []fieldInfo
	walkFakeWidgets(scene.Children(), func(fake *FakeWidget) {
		f := fieldInfo{factoryName: fake.WidgetFactoryName()}
		if tw, ok := fake.Widget().(interface{ TagName() string }); ok {
			f.tagName = tw.TagName()
		}
		fields = append(fields, f)
	})
	return sceneNeedsServices(fields, scene.TagDict())
}

// walkFakeWidgets visits every FakeWidget in the tree, parents before children —
// the order GenerateCode collects fields in, so both halves of a generation see
// the same handlers in the same order.
func walkFakeWidgets(items []graph.IItem, visit func(*FakeWidget)) {
	for _, item := range items {
		fake, ok := item.(*FakeWidget)
		if !ok {
			continue
		}
		visit(fake)
		if fake.HasChildren() {
			walkFakeWidgets(fake.Children(), visit)
		}
	}
}

// renderUserCode builds the user-owned file: the entry point plus one method per
// handler. Its header deliberately omits the "DO NOT EDIT" marker — editing it
// is the point.
func renderUserCode(opts CodeGenOptions, needServices bool, stubs []handlerStub) string {
	imports := map[string]bool{}
	var body strings.Builder
	if opts.PackageName == "main" {
		emitMain(&body, imports, "New"+opts.TypeName, needServices)
	}
	for _, s := range stubs {
		if s.imp != "" {
			imports[s.imp] = true
		}
		body.WriteString("\n")
		body.WriteString(renderStub(s, opts.TypeName))
	}
	scanStdlibImports(body.String(), imports)

	var out strings.Builder
	out.WriteString("// Written once by the Silk Designer Editor and owned by you from here on.\n")
	out.WriteString("// The widget tree lives in the generated .silk.go file next to this one and is\n")
	out.WriteString("// rewritten on every export; nothing here is ever overwritten or reordered —\n")
	out.WriteString("// a later export only appends a method for an event the design has gained.\n")
	fmt.Fprintf(&out, "package %s\n\n", opts.PackageName)

	src := body.String()
	if used := sortedUsedImports(imports, src); len(used) > 0 {
		out.WriteString("import (\n")
		for _, imp := range used {
			fmt.Fprintf(&out, "\t%q\n", imp)
		}
		out.WriteString(")\n")
	}
	out.WriteString(src)

	full := out.String()
	formatted, err := format.Source([]byte(full))
	if err != nil {
		// Only the seeded handler bodies can be malformed here. Hand back the
		// unformatted source so the developer sees and fixes their own code.
		return full
	}
	return string(formatted)
}

// renderStub writes one handler declaration for the user file. A stub seeded
// from a design-time code attr keeps that source byte for byte apart from its
// header, which gains the receiver: the body is the developer's, and all
// ownership moves is what the func hangs off.
func renderStub(s handlerStub, typeName string) string {
	recv := fmt.Sprintf("func (ui *%s) ", typeName)
	if s.body == "" {
		return fmt.Sprintf("%s%s(%s) {\n}\n", recv, s.name, s.params)
	}
	return methodize(s.body, recv) + "\n"
}

// methodize rewrites the first "func <name>(" header in code into a method,
// leaving every other byte — leading comments, body, spacing — alone. Source
// that already declares a receiver is returned untouched.
func methodize(code, recv string) string {
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(trimmed, "func ") {
			continue
		}
		rest := trimmed[len("func "):]
		if strings.HasPrefix(strings.TrimLeft(rest, " \t"), "(") {
			return code
		}
		lines[i] = line[:len(line)-len(trimmed)] + recv + rest
		return strings.Join(lines, "\n")
	}
	return code
}

// appendMissingStubs returns src with a declaration appended for every handler
// it does not already declare as a method on typeName, plus the names appended.
//
// Appending is the ONLY write codegen may make to the user side. Nothing
// already in the file is rewritten, reordered or even reformatted, which is
// what lets a developer keep their own layout and their own edits across every
// regeneration. The cost of that rule: a stub seeded from a design-time body
// may reference a package the file does not import yet — adding the import is
// the developer's call, because their import block is theirs.
func appendMissingStubs(src, typeName string, stubs []handlerStub) (string, []string, error) {
	declared, err := declaredMethods(src, typeName)
	if err != nil {
		return src, nil, err
	}
	var added []string
	var buf strings.Builder
	buf.WriteString(src)
	if !strings.HasSuffix(src, "\n") {
		buf.WriteString("\n")
	}
	for _, s := range stubs {
		if declared[s.name] {
			continue
		}
		declared[s.name] = true
		buf.WriteString("\n")
		buf.WriteString(renderStub(s, typeName))
		added = append(added, s.name)
	}
	if len(added) == 0 {
		return src, nil, nil
	}
	return buf.String(), added, nil
}

// declaredMethods names the methods src declares on typeName. It parses the
// user file instead of grepping it because the price of a wrong answer is a
// lost handler or a duplicate one, and a method name is free to appear in a
// comment or a string. This reads the user's file only — the design itself is
// still generated one way.
func declaredMethods(src, typeName string) (map[string]bool, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "user.go", src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		if receiverTypeName(fn.Recv.List[0].Type) == typeName {
			out[fn.Name.Name] = true
		}
	}
	return out, nil
}

// receiverTypeName reads the type name out of a method receiver, pointer or not.
func receiverTypeName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// sortedUsedImports keeps only the imports src actually references, sorted. The
// running set is optimistic — main() seeds gui and core before the stubs are
// known — and an import Go cannot see used is a compile error.
func sortedUsedImports(imports map[string]bool, src string) []string {
	var out []string
	for imp := range imports {
		if referencesPackage(src, imp[strings.LastIndex(imp, "/")+1:]) {
			out = append(out, imp)
		}
	}
	sort.Strings(out)
	return out
}

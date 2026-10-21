package macro

import (
	"errors"
	"github.com/uk0/silk/diagnostic"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// WriteFile replaces only this generator's own output using a synced temporary
// file and rename (rename atomicity is OS-dependent). Never replace a handwritten
// Go file or follow a symlink. The caller chooses the output path;
// generation does not discover files, execute tools or write outside that path.
func WriteFile(path string, data []byte) (err error) {
	defer func() {
		var known *diagnostic.Error
		if err != nil && !errors.As(err, &known) {
			err = diagnostic.New(diagnostic.MacroOutputWrite, "cannot install macro output", err).At(diagnostic.Location{File: path})
		}
	}()
	if !strings.HasSuffix(strings.ToLower(path), ".go") || !isGenerated(data) {
		return diagnostic.New(diagnostic.MacroOutputProtected, "macro output must be a generated .go file", nil).At(diagnostic.Location{File: path})
	}
	if st, err := os.Lstat(path); err == nil {
		if !st.Mode().IsRegular() {
			return diagnostic.New(diagnostic.MacroOutputProtected, "refusing non-regular output", nil).At(diagnostic.Location{File: path})
		}
		old, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !isGenerated(old) {
			return diagnostic.New(diagnostic.MacroOutputProtected, "refusing to overwrite handwritten file", nil).At(diagnostic.Location{File: path})
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".silk-macro-*.go")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// gofmt moves build constraints before the generated marker. Recognize the
// exact standalone marker before the package clause, never inside a literal
// or a block comment. Parse the whole file so malformed output is not installed.
func isGenerated(data []byte) bool {
	f, err := parser.ParseFile(token.NewFileSet(), "generated.go", data, parser.ParseComments)
	if err != nil {
		return false
	}
	for _, group := range f.Comments {
		for _, comment := range group.List {
			if comment.Pos() < f.Package && comment.Text == strings.TrimSuffix(Header, "\n") {
				return true
			}
		}
	}
	return false
}

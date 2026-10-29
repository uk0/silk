package scene3d

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadedModel is transferred to its caller as a single-owner Scene. Meshes
// remain immutable and shareable. Failed loads never publish a partial scene.
type LoadedModel struct {
	Scene    *Scene
	Warnings []string
	// Bindings are inert document metadata; the host chooses a tag registry.
	Bindings []Binding
}

// LoadModelFile explicitly opens an OBJ, glTF, GLB or .silkscene regular file. Referenced
// buffers are restricted to that file's directory using os.Root (including
// symlink containment); no network, absolute paths, images or MTL are opened.
// A hostile local process can still replace a regular file with a special file
// between Stat/Open; callers controlling the filesystem must prevent that race.
func LoadModelFile(ctx context.Context, path string, limits LoadLimits) (*LoadedModel, error) {
	if ctx == nil {
		return nil, ErrGeometry
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	extension := strings.ToLower(filepath.Ext(path))
	if extension != ".obj" && extension != ".gltf" && extension != ".glb" && extension != ".silkscene" {
		return nil, ErrUnsupported
	}
	if _, err := limits.normalized(); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: model must be a regular file", ErrUnsupported)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: model must be a regular file", ErrUnsupported)
	}
	stop := context.AfterFunc(ctx, func() { _ = f.Close() })
	defer stop()
	if extension == ".silkscene" {
		d, e := loadDocumentWithLimits(ctx, f, limits)
		if e != nil {
			return nil, e
		}
		s, e := d.Build(ctx)
		if e != nil {
			return nil, e
		}
		return &LoadedModel{Scene: s, Warnings: append([]string(nil), d.Warnings...), Bindings: append([]Binding(nil), d.Bindings...)}, nil
	}
	if extension == ".obj" {
		model, e := LoadOBJ(ctx, f, limits)
		if e != nil {
			return nil, e
		}
		s := NewScene()
		if e = s.Add(Node{ID: "model", Name: filepath.Base(path), Mesh: model.Mesh}); e != nil {
			return nil, e
		}
		return &LoadedModel{Scene: s, Warnings: model.Warnings}, nil
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	model, err := LoadGLTF(ctx, f, GLTFOptions{Limits: limits, Buffers: root.FS()})
	if err != nil {
		return nil, err
	}
	s, err := model.NewScene("model")
	if err != nil {
		return nil, err
	}
	return &LoadedModel{Scene: s, Warnings: model.Warnings()}, nil
}

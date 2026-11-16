package ged

import (
	"errors"
	"fmt"
	"strings"
)

// CodeSnapshot contains only immutable captured values/source fragments, never
// widgets, scenes, callbacks or UI line-map keys. Capture it on UI after draft
// commit, then use Generate/GenerateCodeFile on a bounded worker. Package/type
// names remain parameters, so existing machine-file ownership can be inspected
// off UI without retargeting the snapshot or constructing a second UI scene.
// Do not copy or mutate internal slices/maps through unsafe/reflection.
type CodeSnapshot struct {
	fields        []fieldInfo
	imports       map[string]bool
	decls         []TagDecl
	title         string
	width, height float64
	errText       string
	missing       []string
}

// CapturedCode deliberately carries no map keyed by live UI widgets.
type CapturedCode struct {
	Code string
	Err  error
}

// CaptureCodeSnapshot reads committed model state without document-file I/O.
// It does not commit drafts itself, preserving GenerateCodeIndexed semantics.
// Save hosts should PrepareSave first and verify the receipt is still current
// after capture. Widget property getters/emission hooks run here, on UI only.
func (s *GedScene) CaptureCodeSnapshot() *CodeSnapshot {
	snapshot, _ := s.captureCodeSnapshot()
	return snapshot
}

func (s *CodeSnapshot) defaultOptions(opts CodeGenOptions) CodeGenOptions {
	if opts.PackageName == "" {
		opts.PackageName = "main"
	}
	if opts.TypeName == "" {
		opts.TypeName = sanitizeIdentifier(s.title) + "UI"
	}
	return opts
}
func (s *CodeSnapshot) capturedError() error {
	if s.errText != "" {
		return errors.New(s.errText)
	}
	return s.handlerError()
}

// Handler AST validation is deliberately deferred until rendering/writing on
// the worker. UI capture copies source and bindings only, not parsed handlers.
func (s *CodeSnapshot) handlerError() error {
	var problems []string
	for _, field := range s.fields {
		if reason := handlerSourceProblem(field.code); reason != "" {
			name := field.diagnosticName
			if name == "" {
				name = "未命名"
			}
			problems = append(problems, fmt.Sprintf("%s (%s): %s", name, field.factoryName, reason))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("无法生成代码: %s", strings.Join(problems, "; "))
}

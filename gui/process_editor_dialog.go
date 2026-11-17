package gui

import (
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
)

var showProcessDocumentDialog = (*Dialog).ShowModal

type processDocumentEditor struct{ ProcessEditor }

func (e *processDocumentEditor) OnResize() {
	e.ProcessEditor.OnResize()
	e.Diagram.FitToView()
}

func init() {
	i18n.AddMany("zh", map[string]string{
		"2D Process Editor":                           "2D 工业组态编辑器",
		"Edit 2D Process...":                          "编辑 2D 工业组态...",
		"Document changed; reopen the process editor": "文档已更改，请重新打开组态编辑器",
		"Go to source":                                "转到源代码",
		"Go to source needs one device or pipe and no unapplied changes": "请选择一个设备或一条管线，并先处理未应用的更改",
	})
}

// ShowProcessDocumentEditor stages an inert document in an owned modal editor.
// Only Use changes submits a changed, validated independent project to commit.
// Cancel has no side effects; a failed commit keeps the dialog open. The host
// owns revision checks, its undo transaction and eventual explicit file save.
// Call on the UI thread. No runtime, store or network source is started here.
func ShowProcessDocumentEditor(parent IWidget, project hmi.Project, commit func(hmi.Project) error) error {
	_, err := showProcessDocumentEditor(parent, project, commit, ProcessDocumentNavigation{})
	return err
}

// ProcessDocumentNavigation links an inert visual editor to one host snapshot.
// HasTarget is called on the UI thread and must be read-only. Nil disables the
// navigation controls. Initial is ignored if it is not present in the project.
type ProcessDocumentNavigation struct {
	Initial   hmi.SourceTarget
	HasTarget func(hmi.SourceTarget) bool
}

// ShowNavigableProcessDocumentEditor returns a target only after an explicit
// Go to source action on an unchanged draft. The caller owns source identity
// and must revalidate it after the modal returns, before moving the caret.
func ShowNavigableProcessDocumentEditor(parent IWidget, project hmi.Project, commit func(hmi.Project) error, nav ProcessDocumentNavigation) (hmi.SourceTarget, error) {
	return showProcessDocumentEditor(parent, project, commit, nav)
}

func showProcessDocumentEditor(parent IWidget, project hmi.Project, commit func(hmi.Project) error, nav ProcessDocumentNavigation) (hmi.SourceTarget, error) {
	var target hmi.SourceTarget
	if commit == nil {
		return target, ErrUIInvalid
	}
	data, err := project.JSON()
	if err != nil {
		return target, err
	}
	e := new(processDocumentEditor)
	e.ProcessEditor.Init(e)
	defer e.Close()
	e.SetProjectJSON(string(data))
	if err = e.LastError(); err != nil {
		return target, err
	}
	e.Diagram.ClearHistory()
	before := e.ProjectJSON()
	dlg := NewDialog(i18n.T("2D Process Editor"), parent)
	dlg.SetContent(e)
	dlg.AddButton(i18n.T("Cancel"), DialogCancel)
	if nav.HasTarget != nil {
		goTo := dlg.AddButton(i18n.T("Go to source"), DialogNo)
		message := i18n.T("Go to source needs one device or pipe and no unapplied changes")
		SetToolTip(goTo, message)
		defer SetToolTip(goTo, "")
		selected := func() hmi.SourceTarget {
			if id := e.Diagram.SelectedLink(); id != "" {
				return hmi.SourceTarget{Kind: hmi.SourceLink, ID: id}
			}
			if ids := e.Diagram.SelectedNodes(); len(ids) == 1 {
				return hmi.SourceTarget{Kind: hmi.SourceNode, ID: ids[0]}
			}
			return hmi.SourceTarget{}
		}
		available := func(s hmi.SourceTarget) bool {
			return !dlg.closed && !e.closed && s.ID != "" && e.ProjectJSON() == before && nav.HasTarget(s)
		}
		refresh := func() { goTo.SetEnabled(available(selected())) }
		onSelection := e.Diagram.selectionChanged
		e.Diagram.SigSelectionChanged(func() { onSelection(); refresh() })
		e.SigChanged(refresh)
		goTo.Action().BindFunc0(func() {
			s := selected()
			if !available(s) {
				return
			}
			e.Diagram.OnPointerCancel()
			if selected() != s || !available(s) {
				return
			}
			target = s
			dlg.onBtnClick(dialogResultKey(DialogNo))
		})
		if nav.Initial.ID != "" && nav.HasTarget(nav.Initial) {
			switch nav.Initial.Kind {
			case hmi.SourceNode:
				e.Diagram.SelectNodes([]string{nav.Initial.ID})
			case hmi.SourceLink:
				for _, link := range project.Links {
					if link.ID == nav.Initial.ID {
						e.Diagram.selectPipe(link.ID)
						break
					}
				}
			}
		}
		refresh()
	}
	use := dlg.AddButton(i18n.T("Use changes"), DialogOK)
	use.Action().BindFunc0(func() {
		// An unfinished drag is not authored state. Never commit its preview.
		e.Diagram.OnPointerCancel()
		if e.ProjectJSON() != before {
			if err := commit(e.Diagram.Project()); err != nil {
				e.setError(err)
				return
			}
		}
		dlg.onBtnClick(dialogResultKey(DialogOK))
	})
	dlg.SetSize(1060, 760)
	dlg.SetInitialFocus(e.Diagram)
	_ = showProcessDocumentDialog(dlg)
	if err := dlg.LastError(); err != nil {
		return hmi.SourceTarget{}, err
	}
	return target, nil
}

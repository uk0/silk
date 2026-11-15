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
	})
}

// ShowProcessDocumentEditor stages an inert document in an owned modal editor.
// Only Use changes submits a changed, validated independent project to commit.
// Cancel has no side effects; a failed commit keeps the dialog open. The host
// owns revision checks, its undo transaction and eventual explicit file save.
// Call on the UI thread. No runtime, store or network source is started here.
func ShowProcessDocumentEditor(parent IWidget, project hmi.Project, commit func(hmi.Project) error) error {
	if commit == nil {
		return ErrUIInvalid
	}
	data, err := project.JSON()
	if err != nil {
		return err
	}
	e := new(processDocumentEditor)
	e.ProcessEditor.Init(e)
	defer e.Close()
	e.SetProjectJSON(string(data))
	if err = e.LastError(); err != nil {
		return err
	}
	e.Diagram.ClearHistory()
	before := e.ProjectJSON()
	dlg := NewDialog(i18n.T("2D Process Editor"), parent)
	dlg.SetContent(e)
	dlg.AddButton(i18n.T("Cancel"), DialogCancel)
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
	return dlg.LastError()
}

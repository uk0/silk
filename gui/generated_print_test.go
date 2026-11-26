package gui

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/printing"
)

func TestGeneratedPrintSkipsUnconfirmedCapture(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result DialogResult
		found  bool
		err    error
	}{
		{"cancel", DialogCancel, true, printing.ErrCanceled},
		{"no destination", DialogOK, false, printing.ErrInvalidOptions},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &printTestBackend{}
			if tc.found {
				b.printers = []printing.Printer{{Name: "Office", Default: true}}
			}
			deps := printTestDeps(b)
			deps.modal = func(*Dialog) DialogResult { return tc.result }
			deps.capture = func() (*widgetPDFSnapshot, error) {
				t.Fatal("generated print captured without a confirmed printer")
				return nil, nil
			}
			var got error
			calls := 0
			showPrintDialogWith(NewForm(), "", printing.Options{}, []func(printing.Job, error){func(_ printing.Job, e error) {
				calls++
				got = e
			}}, deps)
			if calls != 1 || b.jobs != 0 || !errors.Is(got, tc.err) {
				t.Fatalf("done=%d jobs=%d err=%v", calls, b.jobs, got)
			}
		})
	}
}

func TestGeneratedPrintCaptureIsDetachedAndSubmittedByWorker(t *testing.T) {
	form := NewForm()
	form.SetSize(160, 80)
	b := &printTestBackend{printers: []printing.Printer{{Name: "Office", Default: true}}}
	deps := printTestDeps(b)
	var work, onUI func()
	var confirmed bool
	spawns, captures, completions := 0, 0, 0
	deps.spawn = func(fn func()) {
		spawns++
		switch spawns {
		case 1:
			fn() // printer discovery has already finished
		case 2:
			work = fn
		default:
			t.Fatal("unexpected extra work scheduled")
		}
	}
	deps.post = func(fn func()) {
		if !confirmed {
			fn()
		} else {
			onUI = fn
		}
	}
	deps.choose = func() string {
		t.Fatal("a generated document was replaceable with a file")
		return ""
	}
	deps.modal = func(d *Dialog) DialogResult {
		body := d.Content().(*printDialogBody)
		if captures != 0 || body.path.Text() == "" || body.path.IsEnabled() || body.choose.IsEnabled() {
			t.Fatal("generated document prepared too early or editable")
		}
		body.choose.Action().Trigger(body.choose)
		body.copies.SetValue(3)
		body.title.SetText("Captured print")
		if !d.buttons[len(d.buttons)-1].IsEnabled() {
			t.Fatal("confirmed destination did not enable Print")
		}
		confirmed = true
		return DialogOK
	}
	deps.capture = func() (*widgetPDFSnapshot, error) {
		if !confirmed || work != nil {
			t.Fatal("capture did not run synchronously after confirmation")
		}
		captures++
		return captureWidgetPDF(form)
	}
	var staged string
	b.submit = func(path string, opts printing.Options) (printing.Job, error) {
		staged = path
		if filepath.Base(staged) != "document.pdf" || opts.Copies != 3 || opts.PrinterName != "Office" || opts.Title != "Captured print" {
			t.Fatalf("staged target=%q options=%+v", path, opts)
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.HasPrefix(got, []byte("%PDF-")) || !bytes.Contains(got, []byte("/MediaBox [0 0 120 60]")) ||
			!bytes.Contains(got, []byte("/Width 480 /Height 240")) {
			t.Fatalf("snapshot was not the confirmed viewport: %v", err)
		}
		return printing.Job{ID: "Office-1", PrinterName: "Office"}, nil
	}
	showPrintDialogWith(form, "", printing.Options{}, []func(printing.Job, error){func(job printing.Job, err error) {
		completions++
		if job.ID != "Office-1" || err != nil {
			t.Errorf("job=%+v err=%v", job, err)
		}
	}}, deps)
	if work == nil || captures != 1 || b.jobs != 0 || completions != 0 {
		t.Fatalf("capture=%d jobs=%d done=%d", captures, b.jobs, completions)
	}
	form.SetSize(700, 400) // worker must not inspect or lay out this widget again
	work()
	if b.jobs != 1 || completions != 0 || onUI == nil || staged == "" {
		t.Fatal("submit or UI completion did not follow the worker")
	}
	if _, err := os.Stat(filepath.Dir(staged)); !os.IsNotExist(err) {
		t.Fatalf("private stage survived completed submit: %v", err)
	}
	onUI()
	if completions != 1 {
		t.Fatal("expected exactly one UI completion")
	}
}

func TestGeneratedPrintCaptureFailureNeverQueuesSubmission(t *testing.T) {
	b := &printTestBackend{printers: []printing.Printer{{Name: "Office", Default: true}}}
	d := printTestDeps(b)
	d.modal = func(*Dialog) DialogResult { return DialogOK }
	want := errors.New("source retired")
	d.capture = func() (*widgetPDFSnapshot, error) { return nil, want }
	spawns := 0
	d.spawn = func(fn func()) { spawns++; fn() }
	var got error
	showPrintDialogWith(NewForm(), "", printing.Options{}, []func(printing.Job, error){func(_ printing.Job, e error) { got = e }}, d)
	if b.jobs != 0 || spawns != 1 || !errors.Is(got, want) {
		t.Fatalf("jobs=%d spawns=%d err=%v", b.jobs, spawns, got)
	}
}

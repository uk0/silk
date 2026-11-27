//go:build !windows

package fileportal

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestFilePortalResponse(t *testing.T) {
	for _, tt := range []struct{ uri, want string }{
		{"file:///tmp/design%20%E7%A7%91.svg", "/tmp/design 科.svg"},
		{"file://localhost/tmp/%23input.pdf", "/tmp/#input.pdf"},
		{"file://network/share/file", ""},
		{"https://example.com/tmp/file", ""},
		{"file:///tmp/%00abc", ""},
		{"file:relative", ""},
		{"file:///tmp/document.pdf?changed", ""},
	} {
		got, handled, err := responsePath([]any{uint32(0), map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{tt.uri})}})
		if !handled || got != tt.want || (err != nil) != (tt.want == "") {
			t.Errorf("%q: path=%q handled=%t err=%v", tt.uri, got, handled, err)
		}
	}
	if got, handled, err := responsePath([]any{uint32(1), map[string]dbus.Variant{}}); got != "" || !handled || err != nil {
		t.Fatalf("cancel: path=%q handled=%t err=%v", got, handled, err)
	}
}

type recordedCall struct {
	sender        dbus.Sender
	parent, title string
	opts          map[string]dbus.Variant
}

type fakePortal struct {
	conn     *dbus.Conn
	calls    chan recordedCall
	closed   chan dbus.Sender
	response int // -1 leaves a request pending
}

func (p *fakePortal) SaveFile(sender dbus.Sender, msg dbus.Message, parent, title string, _ map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	// Check the wire signature before Godbus Store rewraps the variant's
	// decoded generic tuple values and loses their original type shape.
	opts := msg.Body[2].(map[string]dbus.Variant)
	// A different handle returned by an older backend, delivered before
	// the method reply, must still reach the client.
	handle := dbus.ObjectPath(requestPath + "/legacy_response")
	if p.response < 0 {
		name := strings.ReplaceAll(strings.TrimPrefix(string(sender), ":"), ".", "_")
		token, _ := opts["handle_token"].Value().(string)
		handle = dbus.ObjectPath(requestPath + "/" + name + "/" + token)
		if err := p.conn.ExportMethodTable(map[string]any{"Close": func(s dbus.Sender) *dbus.Error {
			p.closed <- s
			return nil
		}}, handle, strings.TrimSuffix(request, ".")); err != nil {
			return "", dbus.MakeFailedError(err)
		}
	} else if err := p.conn.Emit(handle, request+"Response", uint32(p.response), map[string]dbus.Variant{
		"uris": dbus.MakeVariant([]string{"file:///tmp/result%20plant.pdf"}),
	}); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	p.calls <- recordedCall{sender, parent, title, opts}
	return handle, nil
}

func privateBus(t *testing.T) string {
	t.Helper()
	exe, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("private D-Bus daemon is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	config := filepath.Join(t.TempDir(), "dbus.conf")
	if err := os.WriteFile(config, []byte(`<busconfig>
		<type>session</type><listen>unix:tmpdir=/tmp</listen><auth>EXTERNAL</auth>
		<policy context="default"><allow send_destination="*"/><allow own="*"/><allow receive_sender="*"/></policy>
	</busconfig>`), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, exe, "--config-file="+config, "--nofork", "--print-address=1")
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })
	address, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(address)
}

func TestFilePortalPrivateBus(t *testing.T) {
	for _, mode := range []string{"unavailable", "save", "cancel", "retire-owner"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("DBUS_SESSION_BUS_ADDRESS", privateBus(t))
			ctx, stop := context.WithTimeout(context.Background(), 7*time.Second)
			defer stop()
			p := &fakePortal{calls: make(chan recordedCall, 1), closed: make(chan dbus.Sender, 1)}
			switch mode {
			case "cancel":
				p.response = 1
			case "retire-owner":
				p.response = -1
			}
			if mode != "unavailable" {
				conn, err := dbus.ConnectSessionBus()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = conn.Close() })
				p.conn = conn
				r, err := conn.RequestName(service, dbus.NameFlagDoNotQueue)
				if err != nil || r != dbus.RequestNameReplyPrimaryOwner {
					t.Fatalf("own private test portal: reply=%v err=%v", r, err)
				}
				if err := conn.Export(p, dbus.ObjectPath(desktopPath), strings.TrimSuffix(chooser, ".")); err != nil {
					t.Fatal(err)
				}
			}
			type result struct {
				path    string
				handled bool
				err     error
			}
			done := make(chan result, 1)
			go func() {
				path, handled, err := Choose(ctx, "x11:2b00007", Options{Save: true, Title: "Save PDF File", SuggestName: "document.pdf",
					Filters: []Filter{{Name: "PDF", Patterns: []Pattern{{Value: "*.pdf"}}}}})
				done <- result{path, handled, err}
			}()
			if mode == "unavailable" {
				r := <-done
				if r.handled || r.path != "" {
					t.Fatalf("unavailable service should allow fallback: %+v", r)
				}
				return
			}
			var c recordedCall
			select {
			case c = <-p.calls:
			case <-ctx.Done():
				t.Fatal("portal did not receive SaveFile")
			}
			if c.parent != "x11:2b00007" || c.title != "Save PDF File" || c.opts["current_name"].Value() != "document.pdf" ||
				c.opts["modal"].Value() != true || c.opts["filters"].Signature().String() != "a(sa(us))" {
				t.Fatalf("incorrect chooser payload: %+v", c)
			}
			if mode == "retire-owner" {
				stop()
			}
			r := <-done
			want := ""
			if mode == "save" {
				want = "/tmp/result plant.pdf"
			}
			if !r.handled || r.path != want || (mode == "retire-owner" && !errors.Is(r.err, context.Canceled)) ||
				(mode != "retire-owner" && r.err != nil) {
				t.Fatalf("result: %+v", r)
			}
			if mode == "retire-owner" {
				select {
				case sender := <-p.closed:
					if sender != c.sender {
						t.Fatal("Close came from a different D-Bus client")
					}
				case <-time.After(time.Second):
					t.Fatal("retiring the owner did not close the portal request")
				}
			}
		})
	}
}

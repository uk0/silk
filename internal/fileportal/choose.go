//go:build !windows

// Package fileportal calls the Linux desktop's XDG FileChooser portal over the
// session bus. It never opens a second chooser after a request may be visible.
package fileportal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	service     = "org.freedesktop.portal.Desktop"
	desktopPath = "/org/freedesktop/portal/desktop"
	requestPath = desktopPath + "/request"
	chooser     = "org.freedesktop.portal.FileChooser."
	request     = "org.freedesktop.portal.Request."
)

type Pattern struct {
	Type  uint32
	Value string
}

type Filter struct {
	Name     string
	Patterns []Pattern
}

type Options struct {
	Save        bool
	Title       string
	SuggestName string
	Filters     []Filter
}

// Choose returns handled=false only when it is safe to try another
// implementation. A handled result with no path never implies fallback.
func Choose(ctx context.Context, parent string, options Options) (path string, handled bool, err error) {
	if err := ctx.Err(); err != nil {
		return "", true, err
	}
	address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if address == "" {
		return "", false, nil // do not autolaunch a bus on minimal desktops
	}
	// Keep the bus connection alive through explicit Request.Close on owner
	// cancellation; tying its lifetime to ctx would close the socket first.
	conn, err := dbus.Connect(address)
	if err != nil {
		return "", ctx.Err() != nil, err
	}
	defer conn.Close()
	return choose(ctx, conn, parent, options)
}

func choose(ctx context.Context, conn *dbus.Conn, parent string, options Options) (string, bool, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", false, err
	}
	token := "silk_" + hex.EncodeToString(random[:])
	names := conn.Names()
	if len(names) == 0 {
		return "", false, errors.New("file chooser session bus did not assign a caller name")
	}
	name := strings.TrimPrefix(names[0], ":")
	predicted := dbus.ObjectPath(requestPath + "/" + strings.ReplaceAll(name, ".", "_") + "/" + token)
	ch := make(chan *dbus.Signal, 16)
	conn.Signal(ch)
	defer conn.RemoveSignal(ch)
	start, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// A private bus connection can subscribe before making the request. The
	// namespace also covers a legacy portal that returns a different handle.
	if err := conn.AddMatchSignalContext(start, dbus.WithMatchSender(service),
		dbus.WithMatchPathNamespace(dbus.ObjectPath(requestPath)),
		dbus.WithMatchInterface(strings.TrimSuffix(request, ".")),
		dbus.WithMatchMember("Response")); err != nil {
		return "", ctx.Err() != nil, err
	}
	params := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(token),
		"modal":        dbus.MakeVariant(true),
		"filters":      dbus.MakeVariant(options.Filters),
	}
	method := "OpenFile"
	if options.Save {
		method = "SaveFile"
		if options.SuggestName != "" {
			params["current_name"] = dbus.MakeVariant(options.SuggestName)
		}
	}

	obj := conn.Object(service, dbus.ObjectPath(desktopPath))
	var handle dbus.ObjectPath
	err := obj.CallWithContext(start, chooser+method, 0, parent, options.Title, params).Store(&handle)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			closeRequest(conn, predicted)
		}
		return "", !unavailable(err), err
	}
	cancel() // no user-interaction timeout once the request is registered
	if !handle.IsValid() || !strings.HasPrefix(string(handle), requestPath+"/") {
		return "", true, fmt.Errorf("unexpected file chooser handle")
	}

	for {
		select {
		case <-ctx.Done():
			closeRequest(conn, handle)
			return "", true, ctx.Err()
		case <-conn.Context().Done():
			return "", true, errors.New("file chooser session bus disconnected")
		case signal, ok := <-ch:
			if !ok {
				return "", true, errors.New("file chooser response channel closed")
			}
			if signal != nil && signal.Path == handle && signal.Name == request+"Response" {
				return responsePath(signal.Body)
			}
		}
	}
}

func closeRequest(conn *dbus.Conn, handle dbus.ObjectPath) {
	ctx, stop := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer stop()
	_ = conn.Object(service, handle).CallWithContext(ctx, request+"Close", 0).Err
}

func unavailable(err error) bool {
	var value dbus.Error
	if !errors.As(err, &value) {
		return false
	}
	switch value.Name {
	case "org.freedesktop.DBus.Error.ServiceUnknown", "org.freedesktop.DBus.Error.NameHasNoOwner",
		"org.freedesktop.DBus.Error.UnknownInterface", "org.freedesktop.DBus.Error.UnknownMethod":
		return true
	default:
		return false
	}
}

func responsePath(body []any) (string, bool, error) {
	if len(body) != 2 {
		return "", true, errors.New("malformed file chooser response")
	}
	code, ok := body[0].(uint32)
	if !ok {
		return "", true, errors.New("malformed file chooser result code")
	}
	if code != 0 {
		return "", true, nil
	}
	results, ok := body[1].(map[string]dbus.Variant)
	if !ok {
		return "", true, errors.New("malformed file chooser results")
	}
	uris, ok := results["uris"].Value().([]string)
	if !ok || len(uris) != 1 {
		return "", true, errors.New("file chooser did not return exactly one path")
	}
	u, err := url.Parse(uris[0])
	if err != nil || u.Scheme != "file" || u.User != nil ||
		(u.Host != "" && !strings.EqualFold(u.Host, "localhost")) ||
		u.RawQuery != "" || u.Fragment != "" || !filepath.IsAbs(u.Path) || strings.IndexByte(u.Path, 0) >= 0 {
		return "", true, errors.New("file chooser did not return a local file URI")
	}
	return u.Path, true, nil
}

package driver

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

// startBroker stands up an in-process mochi MQTT broker on a loopback port with
// an allow-all auth hook and an inline client (so the test can inject/observe
// messages directly). It returns the running server and the "tcp://..." URL.
func startBroker(t *testing.T) (*mqtt.Server, string) {
	t.Helper()
	srv := mqtt.New(&mqtt.Options{
		InlineClient: true,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)), // keep test output clean
	})
	if err := srv.AddHook(new(auth.AllowHook), nil); err != nil {
		t.Fatalf("AddHook: %v", err)
	}
	var addr string
	for _, port := range []int{41883, 42883, 43883} {
		a := fmt.Sprintf("127.0.0.1:%d", port)
		l := listeners.NewTCP(listeners.Config{ID: fmt.Sprintf("t%d", port), Address: a})
		if err := srv.AddListener(l); err != nil {
			continue // port busy, try the next candidate
		}
		addr = a
		break
	}
	if addr == "" {
		t.Fatal("no free port for the test mqtt broker")
	}
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() { _ = srv.Close() })
	return srv, "tcp://" + addr
}

// connectMQTT stands up a broker and returns a connected driver against it.
func connectMQTT(t *testing.T, clientID string) (*mqtt.Server, *MQTT) {
	t.Helper()
	srv, url := startBroker(t)
	d := NewMQTT(url, clientID)
	if err := d.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return srv, d
}

// waitFor polls cond up to ~2s. cond may re-drive the action each iteration so
// the test is robust to MQTT's async subscribe/deliver timing.
func waitFor(t *testing.T, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(msg)
}

// TestMQTTReadPoint subscribes on first read (nil until a message lands), then
// publishes a numeric message from the broker and asserts ReadPoint surfaces it.
func TestMQTTReadPoint(t *testing.T) {
	srv, d := connectMQTT(t, "silk-test-reader")
	p := TagPoint{Address: "silk/sensor/temp", Type: TypeFloat64}

	// First read primes the subscription; no value has arrived yet.
	if v, err := d.ReadPoint(p); err != nil || v != nil {
		t.Fatalf("first ReadPoint = %v, %v; want nil, nil", v, err)
	}

	// Publish a numeric payload; poll ReadPoint until the cache reflects it.
	// Re-publishing each iteration removes any residual subscribe-commit race.
	waitFor(t, "ReadPoint did not surface the published value", func() bool {
		if err := srv.Publish(p.Address, []byte("42.5"), false, 0); err != nil {
			t.Fatalf("broker Publish: %v", err)
		}
		v, err := d.ReadPoint(p)
		return err == nil && v == float64(42.5)
	})
}

// TestMQTTWritePoint publishes through the driver and asserts an independent
// broker subscription receives the value on the topic.
func TestMQTTWritePoint(t *testing.T) {
	srv, d := connectMQTT(t, "silk-test-writer")

	var mu sync.Mutex
	var got string
	err := srv.Subscribe("silk/cmd/setpoint", 1, func(_ *mqtt.Client, _ packets.Subscription, pk packets.Packet) {
		mu.Lock()
		got = string(pk.Payload)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("broker Subscribe: %v", err)
	}

	p := TagPoint{Address: "silk/cmd/setpoint", Type: TypeFloat64, Access: ReadWrite}
	waitFor(t, "broker subscription never received the written value", func() bool {
		if err := d.WritePoint(p, 123.0); err != nil { // tags hand numerics over as float64
			t.Fatalf("WritePoint: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		return got == "123"
	})
}

// TestParsePayload covers the payload parse order: JSON number, plain float,
// bool string, else raw string.
func TestParsePayload(t *testing.T) {
	cases := []struct {
		in   string
		want interface{}
	}{
		{"42", float64(42)},
		{"42.5", float64(42.5)},
		{" -3.5 ", float64(-3.5)}, // trimmed
		{"1", float64(1)},         // number wins over bool for "1"
		{"true", true},
		{"false", false},
		{"on", "on"}, // not a Go bool literal -> raw string
		{"hello", "hello"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := parsePayload([]byte(tc.in)); got != tc.want {
			t.Errorf("parsePayload(%q) = %v (%T), want %v (%T)", tc.in, got, got, tc.want, tc.want)
		}
	}
}

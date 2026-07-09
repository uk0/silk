package driver

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MQTT is a Driver for MQTT brokers. Unlike Modbus/S7 it is push, not poll: the
// broker delivers messages when they change, so the driver keeps the latest
// value per topic in a cache and the Poller surfaces that cache on each tick.
//
// A point's Address is an MQTT topic (e.g. "silk/plant/line1/temp"). ReadPoint
// subscribes to the topic the first time the point is asked for and thereafter
// returns the cached value — nil until the first message arrives, then whatever
// the subscribe callback last parsed. WritePoint publishes the value to the
// topic at QoS 0. Payloads are parsed as a JSON number first, then a plain
// float, then a bool string, else kept as the raw string.
type MQTT struct {
	broker   string // e.g. "tcp://127.0.0.1:1883"
	clientID string

	client mqtt.Client
	mu     sync.Mutex
	cache  map[string]interface{} // topic -> latest parsed value
	subs   map[string]bool        // topics already subscribed (on-demand set)
}

var _ Driver = (*MQTT)(nil)

// NewMQTT builds a driver for the broker at broker (form "tcp://host:port")
// using clientID as the MQTT client identifier.
func NewMQTT(broker, clientID string) *MQTT {
	return &MQTT{
		broker:   broker,
		clientID: clientID,
		cache:    map[string]interface{}{},
		subs:     map[string]bool{},
	}
}

// Connect dials the broker. Auto-reconnect is on; each (re)connect re-subscribes
// the tracked topics so the cache stays live across drops.
func (m *MQTT) Connect() error {
	opts := mqtt.NewClientOptions().
		AddBroker(m.broker).
		SetClientID(m.clientID).
		SetConnectTimeout(5 * time.Second).
		SetAutoReconnect(true).
		SetOnConnectHandler(m.resubscribe)
	c := mqtt.NewClient(opts)
	tok := c.Connect()
	if !tok.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("driver: mqtt connect %s: timeout", m.broker)
	}
	if err := tok.Error(); err != nil {
		return fmt.Errorf("driver: mqtt connect %s: %w", m.broker, err)
	}
	m.client = c
	return nil
}

// Close disconnects from the broker. Safe on a never-connected driver.
func (m *MQTT) Close() error {
	if m.client == nil {
		return nil
	}
	m.client.Disconnect(250)
	m.client = nil
	return nil
}

// ReadPoint returns the latest cached value for the point's topic, subscribing
// to it on first use. The value is nil until the first message is received.
func (m *MQTT) ReadPoint(p TagPoint) (interface{}, error) {
	if m.client == nil {
		return nil, fmt.Errorf("driver: mqtt %s: not connected", m.broker)
	}
	topic := p.Address
	m.mu.Lock()
	first := !m.subs[topic]
	if first {
		m.subs[topic] = true // claim it before subscribing so we subscribe once
	}
	m.mu.Unlock()
	if first {
		tok := m.client.Subscribe(topic, 0, m.onMessage)
		if !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
			m.mu.Lock()
			delete(m.subs, topic) // roll back so a later poll retries the subscribe
			m.mu.Unlock()
			if tok.Error() != nil {
				return nil, fmt.Errorf("driver: mqtt subscribe %s: %w", topic, tok.Error())
			}
			return nil, fmt.Errorf("driver: mqtt subscribe %s: timeout", topic)
		}
	}
	m.mu.Lock()
	v := m.cache[topic]
	m.mu.Unlock()
	return v, nil
}

// WritePoint publishes the value to the point's topic at QoS 0.
func (m *MQTT) WritePoint(p TagPoint, v interface{}) error {
	if m.client == nil {
		return fmt.Errorf("driver: mqtt %s: not connected", m.broker)
	}
	tok := m.client.Publish(p.Address, 0, false, fmt.Sprint(v))
	if !tok.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("driver: mqtt publish %s: timeout", p.Address)
	}
	return tok.Error()
}

// onMessage parses an incoming payload and stores it under its topic.
func (m *MQTT) onMessage(_ mqtt.Client, msg mqtt.Message) {
	v := parsePayload(msg.Payload())
	m.mu.Lock()
	m.cache[msg.Topic()] = v
	m.mu.Unlock()
}

// resubscribe re-establishes every tracked subscription. paho drops
// subscriptions on reconnect, so this runs on each (re)connect; it is a no-op
// on the initial connect when no topic has been asked for yet.
func (m *MQTT) resubscribe(c mqtt.Client) {
	m.mu.Lock()
	topics := make([]string, 0, len(m.subs))
	for t := range m.subs {
		topics = append(topics, t)
	}
	m.mu.Unlock()
	for _, t := range topics {
		c.Subscribe(t, 0, m.onMessage)
	}
}

// parsePayload turns a raw MQTT payload into a Go value: a JSON number first,
// then a plain float, then a bool string, else the raw (trimmed) string.
func parsePayload(b []byte) interface{} {
	s := strings.TrimSpace(string(b))
	var f float64
	if json.Unmarshal([]byte(s), &f) == nil {
		return f
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	if v, err := strconv.ParseBool(s); err == nil {
		return v
	}
	return s
}

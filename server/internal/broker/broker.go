// Package broker embeds a mochi-mqtt broker (TCP + WebSocket listeners) with an
// auth/ACL hook implementing SPEC.md §6. The Go server publishes inline via
// Publish() (bypasses the ACL — only external connections are gated).
package broker

import (
	"crypto/subtle"
	"log/slog"
	"strings"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

// Creds holds the two broker roles (SPEC §6).
type Creds struct {
	PublisherUser, PublisherPass string // the Go server / trusted publishers
	ClientUser, ClientPass       string // every receiving device
}

// authHook enforces connect auth + topic ACL.
type authHook struct {
	mqtt.HookBase
	creds Creds
}

func (h *authHook) ID() string { return "alerthub-auth" }

func (h *authHook) Provides(b byte) bool {
	return b == mqtt.OnConnectAuthenticate || b == mqtt.OnACLCheck
}

func (h *authHook) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
	u := string(pk.Connect.Username)
	p := pk.Connect.Password
	switch u {
	case h.creds.PublisherUser:
		return subtle.ConstantTimeCompare(p, []byte(h.creds.PublisherPass)) == 1
	case h.creds.ClientUser:
		return subtle.ConstantTimeCompare(p, []byte(h.creds.ClientPass)) == 1
	}
	return false
}

// OnACLCheck: write==true is publish, write==false is subscribe (SPEC §6).
func (h *authHook) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
	switch string(cl.Properties.Username) {
	case h.creds.PublisherUser:
		return true // publisher (the signer) may read/write everything
	case h.creds.ClientUser:
		if !write {
			// clients may subscribe to the alert, presence, and system trees
			// (system/ carries the fail-loud heartbeat — SPEC-SAFETY §3.1)
			return strings.HasPrefix(topic, "alerts/") ||
				strings.HasPrefix(topic, "status/") ||
				strings.HasPrefix(topic, "system/")
		}
		// clients may ONLY publish acks + their own presence — never the alert
		// channels. (Per-device %u binding is a production hardening; MVP shares
		// one client credential and enforces topic shape only. See SPEC §6.)
		if strings.HasPrefix(topic, "alerts/") && strings.Contains(topic, "/ack/") {
			return true
		}
		return strings.HasPrefix(topic, "status/")
	}
	return false
}

// Broker wraps the embedded mochi server.
type Broker struct{ Server *mqtt.Server }

// New builds the broker with both listeners and the auth hook installed.
func New(tcpAddr, wsAddr string, creds Creds, logger *slog.Logger) (*Broker, error) {
	s := mqtt.New(&mqtt.Options{InlineClient: true, Logger: logger})
	if err := s.AddHook(&authHook{creds: creds}, nil); err != nil {
		return nil, err
	}
	if err := s.AddListener(listeners.NewTCP(listeners.Config{ID: "tcp", Address: tcpAddr})); err != nil {
		return nil, err
	}
	if err := s.AddListener(listeners.NewWebsocket(listeners.Config{ID: "ws", Address: wsAddr})); err != nil {
		return nil, err
	}
	return &Broker{Server: s}, nil
}

// Start blocks-less: mochi's Serve returns after listeners are up.
func (b *Broker) Start() error { return b.Server.Serve() }

func (b *Broker) Stop() error { return b.Server.Close() }

// Publish sends inline (as the server), bypassing the ACL. SPEC §5/§7.
func (b *Broker) Publish(topic string, payload []byte, retain bool, qos byte) error {
	return b.Server.Publish(topic, payload, retain, qos)
}

// Subscribe registers an inline subscription (server-side consumer), e.g. to
// track device presence on status/#. Requires InlineClient (enabled in New).
func (b *Broker) Subscribe(filter string, handler func(topic string, payload []byte)) error {
	return b.Server.Subscribe(filter, 1, func(_ *mqtt.Client, _ packets.Subscription, pk packets.Packet) {
		handler(pk.TopicName, pk.Payload)
	})
}

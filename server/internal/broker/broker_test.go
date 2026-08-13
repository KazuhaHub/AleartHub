package broker

import (
	"testing"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"
)

func testHook() *authHook {
	return &authHook{creds: Creds{
		PublisherUser: "pub", PublisherPass: "pubpw",
		ClientUser: "cli", ClientPass: "clipw",
	}}
}

func connectPacket(user, pass string) packets.Packet {
	return packets.Packet{Connect: packets.ConnectParams{
		Username: []byte(user), Password: []byte(pass),
	}}
}

func clientAs(user string) *mqtt.Client {
	cl := &mqtt.Client{}
	cl.Properties.Username = []byte(user)
	return cl
}

// TestOnConnectAuthenticate covers SPEC §6: anonymous off, both roles authenticate
// only with the exact password; any other user is rejected.
func TestOnConnectAuthenticate(t *testing.T) {
	h := testHook()
	tests := []struct {
		name       string
		user, pass string
		want       bool
	}{
		{"publisher ok", "pub", "pubpw", true},
		{"publisher wrong pw", "pub", "nope", false},
		{"client ok", "cli", "clipw", true},
		{"client wrong pw", "cli", "nope", false},
		{"unknown user", "mallory", "whatever", false},
		{"empty user", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.OnConnectAuthenticate(nil, connectPacket(tt.user, tt.pass)); got != tt.want {
				t.Fatalf("auth(%q,%q) = %v, want %v", tt.user, tt.pass, got, tt.want)
			}
		})
	}
}

// TestACL_Publisher: the signer role may read and write everything.
func TestACL_Publisher(t *testing.T) {
	h := testHook()
	cl := clientAs("pub")
	for _, topic := range []string{"alerts/active", "alerts/events", "system/heartbeat", "status/dev1", "alerts/id/ack/dev1"} {
		if !h.OnACLCheck(cl, topic, true) {
			t.Errorf("publisher must be allowed to WRITE %q", topic)
		}
		if !h.OnACLCheck(cl, topic, false) {
			t.Errorf("publisher must be allowed to READ %q", topic)
		}
	}
}

// TestACL_ClientSubscribe: clients may read the alert/presence/system trees only.
func TestACL_ClientSubscribe(t *testing.T) {
	h := testHook()
	cl := clientAs("cli")
	allowed := []string{"alerts/active", "alerts/events", "status/dev1", "status/#", "system/heartbeat"}
	for _, topic := range allowed {
		if !h.OnACLCheck(cl, topic, false) {
			t.Errorf("client must be allowed to SUBSCRIBE %q", topic)
		}
	}
	denied := []string{"secret/stuff", "$SYS/broker/load", "random/topic"}
	for _, topic := range denied {
		if h.OnACLCheck(cl, topic, false) {
			t.Errorf("client must NOT be allowed to SUBSCRIBE %q", topic)
		}
	}
}

// TestACL_ClientPublish is the core defense-in-depth check (SPEC §6): a compromised
// receiver must be unable to inject alerts at the broker layer. Clients may publish
// ONLY their own acks and presence — never the alert/heartbeat channels.
func TestACL_ClientPublish(t *testing.T) {
	h := testHook()
	cl := clientAs("cli")

	allowed := []string{"alerts/01ABC/ack/dev1", "status/dev1"}
	for _, topic := range allowed {
		if !h.OnACLCheck(cl, topic, true) {
			t.Errorf("client must be allowed to PUBLISH %q (ack/presence)", topic)
		}
	}
	// The critical negatives: a client forging alerts must be denied at the ACL.
	forbidden := []string{"alerts/active", "alerts/events", "system/heartbeat", "alerts/whatever"}
	for _, topic := range forbidden {
		if h.OnACLCheck(cl, topic, true) {
			t.Errorf("SECURITY: client must NOT be allowed to PUBLISH %q — alert injection path", topic)
		}
	}
}

func TestACL_UnknownUserDenied(t *testing.T) {
	h := testHook()
	cl := clientAs("mallory")
	if h.OnACLCheck(cl, "alerts/events", false) || h.OnACLCheck(cl, "status/x", true) {
		t.Fatal("unknown user must be denied all ACL checks")
	}
}

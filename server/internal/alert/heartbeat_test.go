package alert

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
)

func sampleHeartbeat() *Heartbeat {
	return &Heartbeat{
		SchemaVersion: SchemaVersion,
		Type:          "heartbeat",
		Seq:           7,
		IssuedAt:      1765238400,
		Interval:      10,
		ActiveCount:   1,
		Health:        HealthOK,
	}
}

// TestCanonicalHeartbeat_ExactBytes locks the wire format. It MUST stay
// byte-identical with web/verify.js canonicalHeartbeatBytes() — the conformance
// script checks the pair, this pins the Go side on its own.
func TestCanonicalHeartbeat_ExactBytes(t *testing.T) {
	want := strings.Join([]string{"hb2", "7", "1765238400", "10", "1", "ok"}, "\n")
	if got := string(CanonicalHeartbeat(sampleHeartbeat())); got != want {
		t.Fatalf("canonical mismatch:\n got=%q\nwant=%q", got, want)
	}
}

// TestCanonicalHeartbeat_DomainSeparated is the anti-confusion property: a
// heartbeat canonical must never collide with an alert canonical, so an alert
// signature can't be replayed as a heartbeat (or vice versa).
func TestCanonicalHeartbeat_DomainSeparated(t *testing.T) {
	hb := string(CanonicalHeartbeat(sampleHeartbeat()))
	if !strings.HasPrefix(hb, "hb2\n") {
		t.Fatalf("heartbeat canonical must lead with the hb2 domain tag, got %q", hb)
	}
	a := string(Canonical(&Alert{SchemaVersion: SchemaVersion}))
	if strings.HasPrefix(a, "hb") {
		t.Fatal("alert canonical must not share the heartbeat domain tag")
	}
}

func TestHeartbeat_SignVerifyRoundTrip(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	hb := sampleHeartbeat()
	SignHeartbeat(hb, priv)
	if hb.Sig == "" {
		t.Fatal("SignHeartbeat left Sig empty")
	}
	if !VerifyHeartbeat(hb, pub) {
		t.Fatal("VerifyHeartbeat failed on a freshly signed beat")
	}
}

// TestVerifyHeartbeat_RejectsTamper checks every signed field, and especially
// `health`: if that were forgeable, a compromised LAN device could downgrade a
// degraded server to "ok" and silence the warning — the exact attack the beat is
// signed to prevent.
func TestVerifyHeartbeat_RejectsTamper(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	base := sampleHeartbeat()
	SignHeartbeat(base, priv)

	for field, mutate := range map[string]func(*Heartbeat){
		"seq":          func(h *Heartbeat) { h.Seq++ },
		"issued_at":    func(h *Heartbeat) { h.IssuedAt++ },
		"interval":     func(h *Heartbeat) { h.Interval++ },
		"active_count": func(h *Heartbeat) { h.ActiveCount++ },
		"health":       func(h *Heartbeat) { h.Health = HealthDegraded },
	} {
		h := *base
		mutate(&h)
		if VerifyHeartbeat(&h, pub) {
			t.Errorf("accepted a heartbeat with tampered %q — field not covered by the signature", field)
		}
	}
}

// TestVerifyHeartbeat_DegradedCannotBeUpgraded is the same property stated the way
// it matters operationally: a signed "degraded" beat must not verify once someone
// rewrites it to "ok".
func TestVerifyHeartbeat_DegradedCannotBeUpgraded(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	hb := sampleHeartbeat()
	hb.Health = HealthDegraded
	SignHeartbeat(hb, priv)
	if !VerifyHeartbeat(hb, pub) {
		t.Fatal("a degraded beat must verify as signed")
	}
	forged := *hb
	forged.Health = HealthOK
	if VerifyHeartbeat(&forged, pub) {
		t.Fatal("SECURITY: a degraded beat was silently upgraded to ok and still verified")
	}
}

func TestVerifyHeartbeat_RejectsBadSigAndWrongKey(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	hb := sampleHeartbeat()
	SignHeartbeat(hb, priv)

	if VerifyHeartbeat(hb, otherPub) {
		t.Error("verified against the wrong public key")
	}
	bad := *hb
	bad.Sig = "not-base64url-!!!"
	if VerifyHeartbeat(&bad, pub) {
		t.Error("accepted a malformed signature")
	}
	empty := *hb
	empty.Sig = ""
	if VerifyHeartbeat(&empty, pub) {
		t.Error("accepted an empty signature")
	}
}

func TestHealthConstantsAreFramingSafe(t *testing.T) {
	for _, v := range []string{HealthOK, HealthDegraded} {
		if v == "" || strings.ContainsAny(v, "\n\r") {
			t.Errorf("health constant %q must be non-empty and newline-free (it is a canonical field)", v)
		}
	}
	if HealthOK == HealthDegraded {
		t.Fatal("health constants must be distinct")
	}
}

package alert

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
)

// sampleAlert is a fully-populated envelope used across the canonical/sign tests.
func sampleAlert() *Alert {
	return &Alert{
		SchemaVersion: SchemaVersion,
		ID:            "01J9Z3M8XK000000000000000",
		Type:          "alert",
		Category:      "earthquake",
		Severity:      "emergency",
		Title:         "正在发生地震",
		Body:          "震中距你约 42 公里，预计 15 秒后到达。",
		Action:        "趴下，掩护，抓牢",
		Source:        "panel",
		IssuedAt:      1765238400,
		TTL:           60,
		Nonce:         "9f86d081884c7d659a2feaa0c55ad015",
		Cancels:       "",
	}
}

// TestCanonical_ExactBytes locks the wire format (SPEC §3.2): 13 fields, '\n'
// joined, no trailing newline, base-10 integers, empty fields still occupy a slot.
func TestCanonical_ExactBytes(t *testing.T) {
	a := sampleAlert()
	want := strings.Join([]string{
		"1",
		"01J9Z3M8XK000000000000000",
		"alert",
		"earthquake",
		"emergency",
		"正在发生地震",
		"震中距你约 42 公里，预计 15 秒后到达。",
		"趴下，掩护，抓牢",
		"panel",
		"1765238400",
		"60",
		"9f86d081884c7d659a2feaa0c55ad015",
		"", // empty cancels still produces its slot (SPEC §3.2 rule 6)
	}, "\n")
	if got := string(Canonical(a)); got != want {
		t.Fatalf("canonical mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestCanonical_FieldCountAndNoTrailingNewline(t *testing.T) {
	c := string(Canonical(sampleAlert()))
	// 13 fields => exactly 12 separators.
	if n := strings.Count(c, "\n"); n != 12 {
		t.Fatalf("want 12 '\\n' separators (13 fields), got %d", n)
	}
	// "No trailing newline" means Canonical never *appends* a newline after the
	// last field. With a non-empty final field (cancels), the bytes must not end
	// in '\n'. (An empty final field legitimately ends the string at its own — the
	// 12th — separator; that is still exactly 12 separators, checked above.)
	cancel := sampleAlert()
	cancel.Type = "cancel"
	cancel.Cancels = "01J9Z3M8XK000000000000000"
	if cc := string(Canonical(cancel)); strings.HasSuffix(cc, "\n") {
		t.Fatalf("canonical must not append a trailing newline after the last field")
	}
}

// TestCanonical_NFC verifies that a decomposed and a precomposed string produce
// byte-identical canonical output (SPEC §3.2 rule 3 — NFC before concatenation).
func TestCanonical_NFC(t *testing.T) {
	decomposed := sampleAlert()
	decomposed.Title = "café" // 'e' + U+0301 combining acute
	precomposed := sampleAlert()
	precomposed.Title = "café" // 'é' U+00E9

	if string(Canonical(decomposed)) != string(Canonical(precomposed)) {
		t.Fatalf("NFC normalization failed: decomposed and precomposed titles differ after canonicalization")
	}
}

func TestSignVerify_RoundTrip(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	a := sampleAlert()
	Sign(a, priv)
	if a.Sig == "" {
		t.Fatal("Sign left Sig empty")
	}
	if !Verify(a, pub) {
		t.Fatal("Verify failed on a freshly signed alert")
	}
}

// TestVerify_RejectsTamper ensures every signed field is actually covered: mutate
// one field at a time and the signature must no longer verify (SPEC §3 threat: an
// attacker must not be able to forge or flip alert/cancel).
func TestVerify_RejectsTamper(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	base := sampleAlert()
	Sign(base, priv)

	mutate := map[string]func(*Alert){
		"type":           func(a *Alert) { a.Type = "cancel" },
		"category":       func(a *Alert) { a.Category = "fire" },
		"severity":       func(a *Alert) { a.Severity = "notice" },
		"title":          func(a *Alert) { a.Title = "假警报" },
		"body":           func(a *Alert) { a.Body = "tampered" },
		"action":         func(a *Alert) { a.Action = "ignore" },
		"source":         func(a *Alert) { a.Source = "attacker" },
		"issued_at":      func(a *Alert) { a.IssuedAt++ },
		"ttl":            func(a *Alert) { a.TTL++ },
		"nonce":          func(a *Alert) { a.Nonce = "00000000000000000000000000000000" },
		"cancels":        func(a *Alert) { a.Cancels = "some-other-id" },
		"schema_version": func(a *Alert) { a.SchemaVersion = 2 },
		"id":             func(a *Alert) { a.ID = "different-id" },
	}
	for field, f := range mutate {
		a := *base // copy, keep the original signature
		f(&a)
		if Verify(&a, pub) {
			t.Errorf("Verify accepted a message with tampered %q — field not covered by signature", field)
		}
	}
}

func TestVerify_RejectsBadSignatureEncoding(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	a := sampleAlert()
	a.Sig = "not-valid-base64url-!!!"
	if Verify(a, pub) {
		t.Fatal("Verify accepted a non-base64url signature")
	}
	a.Sig = "" // empty
	if Verify(a, pub) {
		t.Fatal("Verify accepted an empty signature")
	}
}

func TestVerify_RejectsWrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	a := sampleAlert()
	Sign(a, priv)
	if Verify(a, otherPub) {
		t.Fatal("Verify accepted a signature made by a different key")
	}
}

func TestValidateInput(t *testing.T) {
	tests := []struct {
		name                        string
		severity, category          string
		title, body, action, source string
		want                        error
	}{
		{"ok", "emergency", "earthquake", "t", "b", "a", "panel", nil},
		{"ok empty optional", "notice", "system", "t", "", "", "s", nil},
		{"bad severity", "boom", "earthquake", "t", "b", "a", "s", ErrBadSeverity},
		{"bad category", "warning", "meteor", "t", "b", "a", "s", ErrBadCategory},
		{"newline in title", "warning", "fire", "a\nb", "b", "a", "s", ErrNewline},
		{"carriage return in body", "warning", "fire", "t", "a\rb", "a", "s", ErrNewline},
		{"newline in action", "warning", "fire", "t", "b", "a\nb", "s", ErrNewline},
		{"newline in source", "warning", "fire", "t", "b", "a", "s\ns", ErrNewline},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateInput(tt.severity, tt.category, tt.title, tt.body, tt.action, tt.source)
			if got != tt.want {
				t.Fatalf("ValidateInput = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultTTL(t *testing.T) {
	cases := map[string]int64{
		"emergency": 120, "critical": 120,
		"warning": 600, "notice": 600, "unknown": 600,
	}
	for sev, want := range cases {
		if got := DefaultTTL(sev); got != want {
			t.Errorf("DefaultTTL(%q) = %d, want %d", sev, got, want)
		}
	}
}

func TestDefaultAction(t *testing.T) {
	if DefaultAction("earthquake") == "" || DefaultAction("fire") == "" {
		t.Error("earthquake/fire should have a default action")
	}
	if DefaultAction("weather") != "" || DefaultAction("custom") != "" {
		t.Error("weather/custom should have no default action")
	}
}

func TestSeverityRank_Ordering(t *testing.T) {
	if !(SeverityRank("notice") < SeverityRank("warning") &&
		SeverityRank("warning") < SeverityRank("critical") &&
		SeverityRank("critical") < SeverityRank("emergency")) {
		t.Fatal("severity ranks must be strictly increasing notice<warning<critical<emergency")
	}
}

func TestNewNonce(t *testing.T) {
	n := NewNonce()
	if len(n) != 32 {
		t.Fatalf("nonce length = %d, want 32 hex chars", len(n))
	}
	if strings.ToLower(n) != n {
		t.Fatalf("nonce must be lowercase hex, got %q", n)
	}
	for _, r := range n {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("nonce has non-hex char %q", r)
		}
	}
	if NewNonce() == n {
		t.Fatal("two nonces collided — not random")
	}
}

func TestNewID_UniqueNonEmpty(t *testing.T) {
	a, b := NewID(), NewID()
	if a == "" || b == "" {
		t.Fatal("NewID returned empty")
	}
	if a == b {
		t.Fatal("NewID returned duplicate ids")
	}
}

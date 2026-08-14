package alert

import (
	"crypto/ed25519"
	"encoding/base64"
	"strconv"
	"strings"
)

// Heartbeat is the FAIL-LOUD liveness beacon (SPEC-SAFETY §3.1). The server
// publishes it (signed) to system/heartbeat every Interval seconds; each client
// runs a LOCAL watchdog and alarms if it stops arriving. Signed so a compromised
// LAN device can't forge an "all healthy" beat to suppress the offline alarm.
type Heartbeat struct {
	SchemaVersion int    `json:"schema_version"`
	Type          string `json:"type"` // "heartbeat"
	Seq           int64  `json:"seq"`
	IssuedAt      int64  `json:"issued_at"` // unix seconds — also the client clock-drift reference
	Interval      int64  `json:"interval"`  // seconds; clients derive their timeout from this
	ActiveCount   int    `json:"active_count"`
	// Health is the server's own verdict on itself. Before this existed the beat
	// was unconditionally "green": a server whose store was unreachable still told
	// every client it was fine, which defeats the point of a fail-loud channel.
	// It is SIGNED, so a compromised LAN device cannot downgrade or forge it.
	Health string `json:"health"` // HealthOK | HealthDegraded
	Sig    string `json:"sig"`
}

// Heartbeat health values. Kept to a closed set of newline-free constants so they
// can never break the canonical field framing.
const (
	HealthOK       = "ok"
	HealthDegraded = "degraded"
)

// CanonicalHeartbeat builds the signed bytes. The leading domain tag "hb2" is
// DISTINCT from the alert canonical (which leads with schema_version "1"), so an
// alert signature can never be replayed as a heartbeat or vice versa. The tag was
// bumped hb1 -> hb2 when the health field joined the signed set: an old 5-field
// beat and a new 6-field one must never cross-verify.
// MUST stay byte-identical with web/verify.js canonicalHeartbeatBytes().
func CanonicalHeartbeat(h *Heartbeat) []byte {
	parts := []string{
		"hb2",
		strconv.FormatInt(h.Seq, 10),
		strconv.FormatInt(h.IssuedAt, 10),
		strconv.FormatInt(h.Interval, 10),
		strconv.Itoa(h.ActiveCount),
		h.Health,
	}
	return []byte(strings.Join(parts, "\n"))
}

func SignHeartbeat(h *Heartbeat, priv ed25519.PrivateKey) {
	h.Sig = base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, CanonicalHeartbeat(h)))
}

func VerifyHeartbeat(h *Heartbeat, pub ed25519.PublicKey) bool {
	sig, err := base64.RawURLEncoding.DecodeString(h.Sig)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pub, CanonicalHeartbeat(h), sig)
}

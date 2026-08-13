// Package alert defines the AlertHub signed message envelope and the LOCKED
// canonicalization + Ed25519 signing scheme. See SPEC.md §2–§3.
//
// The Canonical() function MUST stay byte-identical with web/verify.js. Any
// change here is a wire-protocol change: update SPEC.md and verify.js together.
package alert

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

// SchemaVersion is the canonical-layout version (SPEC.md §3.2). Bump only when
// the signed field set changes.
const SchemaVersion = 1

// severityRank ranks severities for the alerts/active replacement policy (SPEC §5).
var severityRank = map[string]int{"notice": 0, "warning": 1, "critical": 2, "emergency": 3}

var validCategory = map[string]bool{
	"earthquake": true, "fire": true, "weather": true,
	"system": true, "security": true, "custom": true,
}

// SeverityRank returns the ordering rank; callers must validate severity first.
func SeverityRank(s string) int { return severityRank[s] }

// Alert is the wire envelope. JSON serialization is for transport/debugging
// only — signing covers Canonical(), never the JSON bytes.
type Alert struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	Type          string `json:"type"` // "alert" | "cancel"
	Category      string `json:"category"`
	Severity      string `json:"severity"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	Action        string `json:"action"`
	Source        string `json:"source"`
	IssuedAt      int64  `json:"issued_at"` // unix seconds
	TTL           int64  `json:"ttl"`       // seconds
	Nonce         string `json:"nonce"`     // 32 lowercase hex chars
	Cancels       string `json:"cancels"`   // original id for type=="cancel", else ""
	Sig           string `json:"sig"`       // base64url(64-byte sig), no padding
}

func nfc(s string) string { return norm.NFC.String(s) }

// Canonical builds the exact bytes that get signed (SPEC §3.2):
// 13 fields, NFC-normalized strings, base-10 integers, '\n'-joined, no trailing newline.
func Canonical(a *Alert) []byte {
	parts := []string{
		strconv.Itoa(a.SchemaVersion),
		nfc(a.ID),
		nfc(a.Type),
		nfc(a.Category),
		nfc(a.Severity),
		nfc(a.Title),
		nfc(a.Body),
		nfc(a.Action),
		nfc(a.Source),
		strconv.FormatInt(a.IssuedAt, 10),
		strconv.FormatInt(a.TTL, 10),
		nfc(a.Nonce),
		nfc(a.Cancels),
	}
	return []byte(strings.Join(parts, "\n"))
}

// Sign fills a.Sig with base64url(no-pad) Ed25519 over Canonical(a).
func Sign(a *Alert, priv ed25519.PrivateKey) {
	a.Sig = base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, Canonical(a)))
}

// Verify checks a.Sig against pub over Canonical(a).
func Verify(a *Alert, pub ed25519.PublicKey) bool {
	sig, err := base64.RawURLEncoding.DecodeString(a.Sig)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pub, Canonical(a), sig)
}

// NewID returns a time-ordered UUIDv7 (SPEC §2).
func NewID() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return uuid.NewString()
}

// NewNonce returns 16 random bytes as 32 lowercase hex chars (SPEC §3.2 rule 5).
func NewNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

var (
	ErrBadSeverity = errors.New("invalid severity")
	ErrBadCategory = errors.New("invalid category")
	ErrNewline     = errors.New("title/body/action/source must not contain newline (SPEC §3.2 rule 7)")
)

// ValidateInput enforces the publish-time rules from SPEC §3.2/§7.
func ValidateInput(severity, category, title, body, action, source string) error {
	if _, ok := severityRank[severity]; !ok {
		return ErrBadSeverity
	}
	if !validCategory[category] {
		return ErrBadCategory
	}
	for _, s := range []string{title, body, action, source} {
		if strings.ContainsAny(s, "\n\r") {
			return ErrNewline
		}
	}
	return nil
}

// DefaultTTL returns the per-severity default validity window (SPEC §7).
func DefaultTTL(severity string) int64 {
	switch severity {
	case "emergency", "critical":
		return 120
	default:
		return 600
	}
}

// DefaultAction suggests a disposition line from the category (SPEC §1/§2).
func DefaultAction(category string) string {
	switch category {
	case "earthquake":
		return "趴下，掩护，抓牢"
	case "fire":
		return "立即撤离，不要乘电梯"
	case "system":
		return "确认并检查节点"
	case "security":
		return "核实并确认"
	default:
		return ""
	}
}

package store

// Append-only audit log with a hash chain (ARCHITECTURE §8, SOC2 CC7).
//
// For a mass-notification platform the compliance-critical question is "who fired
// that alert, and who could have?". Plain rows answer it only if you trust that
// nobody edited the table. Each entry therefore carries
//
//	hash = SHA-256( prev_hash || canonical(entry) )
//
// so removing or editing any row breaks every hash after it and VerifyAuditChain
// can prove it. The chain is GLOBAL (not per-org): it protects platform
// integrity, so verification is a platform-level operation, while ordinary
// per-org reads are just a filtered view. There are deliberately no update or
// delete methods.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Actor kinds. Who acted matters as much as what happened.
const (
	ActorUser           = "user"            // a human session (JWT)
	ActorServiceAccount = "service_account" // an ahk_ API key
	ActorAdminToken     = "admin_token"     // the static script token
	ActorSystem         = "system"          // internal producers: EEW, sweeper, workers
)

// AuditEntry is one recorded action.
type AuditEntry struct {
	ID         int64  `json:"id"`
	OrgID      int64  `json:"org_id"`
	At         int64  `json:"at"`
	ActorType  string `json:"actor_type"`
	ActorID    int64  `json:"actor_id"`   // user id / service-account id; 0 when N/A
	ActorName  string `json:"actor_name"` // upn / key name / "admin-token" / source
	Action     string `json:"action"`     // e.g. alert.publish, auth.login
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Detail     string `json:"detail"` // short, non-secret context
	IP         string `json:"ip"`
	PrevHash   string `json:"prev_hash"`
	Hash       string `json:"hash"`
}

// canonicalAudit is the byte string the chain hashes. Field order is fixed and
// values are length-prefixed so no combination of contents can be rearranged to
// produce the same bytes as a different entry.
func canonicalAudit(e *AuditEntry, prevHash string) []byte {
	var b strings.Builder
	put := func(s string) {
		b.WriteString(strconv.Itoa(len(s)))
		b.WriteByte(':')
		b.WriteString(s)
		b.WriteByte('|')
	}
	put(prevHash)
	put(strconv.FormatInt(e.OrgID, 10))
	put(strconv.FormatInt(e.At, 10))
	put(e.ActorType)
	put(strconv.FormatInt(e.ActorID, 10))
	put(e.ActorName)
	put(e.Action)
	put(e.TargetType)
	put(e.TargetID)
	put(e.Detail)
	put(e.IP)
	return []byte(b.String())
}

func auditHash(e *AuditEntry, prevHash string) string {
	sum := sha256.Sum256(canonicalAudit(e, prevHash))
	return hex.EncodeToString(sum[:])
}

// AppendAudit records an action, linking it to the current chain head.
func (s *Store) AppendAudit(e *AuditEntry) error {
	if e.Action == "" {
		return errors.New("audit: action required")
	}
	if e.At == 0 {
		e.At = time.Now().Unix()
	}
	if e.ActorType == "" {
		e.ActorType = ActorSystem
	}
	// Read the head and insert under one lock so concurrent writers cannot both
	// chain onto the same predecessor and fork the chain.
	s.auditMu.Lock()
	defer s.auditMu.Unlock()

	var prev string
	err := s.queryRow(`SELECT hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&prev)
	if err != nil && !isNoRows(err) {
		return err
	}
	e.PrevHash = prev
	e.Hash = auditHash(e, prev)

	_, err = s.exec(
		`INSERT INTO audit_log
		 (org_id, at, actor_type, actor_id, actor_name, action, target_type, target_id, detail, ip, prev_hash, hash)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.OrgID, e.At, e.ActorType, e.ActorID, e.ActorName, e.Action,
		e.TargetType, e.TargetID, e.Detail, e.IP, e.PrevHash, e.Hash)
	return err
}

const auditCols = `id, org_id, at, actor_type, actor_id, actor_name, action, target_type, target_id, detail, ip, prev_hash, hash`

func scanAudit(rows interface{ Scan(...any) error }) (AuditEntry, error) {
	var e AuditEntry
	err := rows.Scan(&e.ID, &e.OrgID, &e.At, &e.ActorType, &e.ActorID, &e.ActorName,
		&e.Action, &e.TargetType, &e.TargetID, &e.Detail, &e.IP, &e.PrevHash, &e.Hash)
	return e, err
}

// ListAudit returns an org's most recent entries, newest first.
func (s *Store) ListAudit(orgID int64, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.query(
		`SELECT `+auditCols+` FROM audit_log WHERE org_id = ? ORDER BY id DESC LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		e, err := scanAudit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AuditChainResult reports whether the global chain is intact.
type AuditChainResult struct {
	OK      bool   `json:"ok"`
	Entries int    `json:"entries"`
	BadID   int64  `json:"bad_id,omitempty"` // first entry that fails
	Reason  string `json:"reason,omitempty"`
}

// VerifyAuditChain walks the whole log in insertion order and recomputes every
// link. A single edited, inserted or removed row makes this fail and names the
// first entry that does not check out.
func (s *Store) VerifyAuditChain() (AuditChainResult, error) {
	rows, err := s.query(`SELECT ` + auditCols + ` FROM audit_log ORDER BY id`)
	if err != nil {
		return AuditChainResult{}, err
	}
	defer rows.Close()

	prev := ""
	n := 0
	for rows.Next() {
		e, err := scanAudit(rows)
		if err != nil {
			return AuditChainResult{}, err
		}
		n++
		if e.PrevHash != prev {
			return AuditChainResult{OK: false, Entries: n, BadID: e.ID,
				Reason: "prev_hash does not match the preceding entry (a row was edited, inserted or removed)"}, nil
		}
		if want := auditHash(&e, prev); want != e.Hash {
			return AuditChainResult{OK: false, Entries: n, BadID: e.ID,
				Reason: "hash does not match the entry contents (the row was edited)"}, nil
		}
		prev = e.Hash
	}
	if err := rows.Err(); err != nil {
		return AuditChainResult{}, err
	}
	return AuditChainResult{OK: true, Entries: n}, nil
}

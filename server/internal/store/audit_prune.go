package store

// Retention for the audit log.
//
// The log is append-only and hash-chained, which is exactly what makes pruning
// awkward: deleting the oldest entries leaves the first survivor pointing at a
// predecessor that no longer exists, and naive verification would then report
// "chain broken" forever — training the operator to ignore the one alarm that
// means someone tampered with the record.
//
// So a prune is not a silent delete. It:
//   1. records the prune itself as an audit entry (who, when, how many), so the
//      act of shortening the record is part of the record;
//   2. stores the hash of the last removed entry as the chain ANCHOR;
//   3. deletes.
//
// VerifyAuditChain then starts from the anchor instead of from genesis, so a
// pruned log still verifies end-to-end and a real tampering still fails.

import "time"

const auditAnchorKey = "audit_chain_anchor"

// PruneAudit removes entries older than `before` (unix seconds) and returns how
// many were removed. actor describes who triggered it, for the record it leaves.
func (s *Store) PruneAudit(before int64, actor string) (int, error) {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()

	// The last entry that will be removed becomes the anchor the survivors chain from.
	var anchor string
	var lastID int64
	err := s.queryRow(
		`SELECT hash, id FROM audit_log WHERE at < ? ORDER BY id DESC LIMIT 1`, before).Scan(&anchor, &lastID)
	if isNoRows(err) {
		return 0, nil // nothing old enough
	}
	if err != nil {
		return 0, err
	}

	var n int
	if err := s.queryRow(`SELECT COUNT(*) FROM audit_log WHERE at < ?`, before).Scan(&n); err != nil {
		return 0, err
	}

	// Record the prune BEFORE performing it, so the entry lands in the chain that
	// is about to be shortened and cannot itself be lost by the same operation.
	if err := s.appendAuditLocked(&AuditEntry{
		ActorType: ActorSystem, ActorName: actor, Action: "audit.prune",
		TargetType: "audit_log", TargetID: anchor[:12],
		Detail: "pruned entries older than " + time.Unix(before, 0).UTC().Format(time.RFC3339),
	}); err != nil {
		return 0, err
	}
	if _, err := s.exec(`DELETE FROM audit_log WHERE at < ?`, before); err != nil {
		return 0, err
	}
	if err := s.setSetting(auditAnchorKey, anchor); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) setSetting(k, v string) error {
	_, err := s.exec(
		`INSERT INTO settings (k, v) VALUES (?,?) ON CONFLICT(k) DO UPDATE SET v = excluded.v`, k, v)
	return err
}

func (s *Store) getSetting(k string) (string, error) {
	var v string
	err := s.queryRow(`SELECT v FROM settings WHERE k = ?`, k).Scan(&v)
	if isNoRows(err) {
		return "", nil
	}
	return v, err
}

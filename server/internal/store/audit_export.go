package store

// SIEM export cursor. Shipping the audit trail to the customer's SIEM is a
// compliance requirement (ARCHITECTURE §8), and the only safe way to do it is
// at-least-once from a durable cursor: an exporter that streams "whatever is new
// since I last looked" loses entries across a restart, and an audit trail with
// holes is worse than none because the holes are invisible.
//
// The cursor is the id of the last entry successfully shipped. Entries are
// immutable and ids are monotonic, so resuming is just "everything above N".

// AuditSince returns entries with id > afterID in insertion order — the shape an
// exporter needs, as opposed to ListAudit's newest-first per-org view.
func (s *Store) AuditSince(afterID int64, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.query(
		`SELECT `+auditCols+` FROM audit_log WHERE id > ? ORDER BY id LIMIT ?`, afterID, limit)
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

// GetCursor reads a named durable cursor (0 when unset).
func (s *Store) GetCursor(name string) (int64, error) {
	var v int64
	err := s.queryRow(`SELECT position FROM cursors WHERE name = ?`, name).Scan(&v)
	if isNoRows(err) {
		return 0, nil
	}
	return v, err
}

// SetCursor advances a named cursor.
func (s *Store) SetCursor(name string, pos int64) error {
	_, err := s.exec(
		`INSERT INTO cursors (name, position) VALUES (?,?)
		 ON CONFLICT(name) DO UPDATE SET position = excluded.position`, name, pos)
	return err
}

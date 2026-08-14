package store

// Acknowledgement roster (SPEC §5.3). Clients publish an ack to
// alerts/<id>/ack/<deviceId>; the server collects them here.
//
// Persisted rather than kept in memory like presence, because "who acknowledged
// the evacuation order" is an incident-review question and must survive a
// restart. Note what an ack does and does not prove: the DEVICE confirmed, not
// that a person read and understood it (SPEC-SAFETY §11).

import "time"

type Ack struct {
	AlertID  string `json:"alert_id"`
	DeviceID string `json:"device_id"`
	AckAt    int64  `json:"ack_at"`
	By       string `json:"by,omitempty"`
}

// RecordAck stores one acknowledgement, idempotent per (alert, device): a
// retained ack is re-delivered on every reconnect, and the roster must not grow
// a duplicate row each time.
func (s *Store) RecordAck(orgID int64, a Ack) error {
	if a.AlertID == "" || a.DeviceID == "" {
		return nil // nothing addressable; ignore rather than store junk
	}
	if a.AckAt == 0 {
		a.AckAt = time.Now().Unix()
	}
	_, err := s.exec(
		`INSERT INTO alert_acks (org_id, alert_id, device_id, ack_at, by_who)
		 VALUES (?,?,?,?,?)
		 ON CONFLICT(alert_id, device_id) DO UPDATE SET ack_at = excluded.ack_at, by_who = excluded.by_who`,
		orgID, a.AlertID, a.DeviceID, a.AckAt, a.By)
	return err
}

// ListAcks returns who acknowledged one alert, earliest first.
func (s *Store) ListAcks(orgID int64, alertID string) ([]Ack, error) {
	rows, err := s.query(
		`SELECT alert_id, device_id, ack_at, by_who FROM alert_acks
		 WHERE org_id = ? AND alert_id = ? ORDER BY ack_at`, orgID, alertID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ack{}
	for rows.Next() {
		var a Ack
		if err := rows.Scan(&a.AlertID, &a.DeviceID, &a.AckAt, &a.By); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CountAcks returns how many distinct devices acknowledged an alert.
func (s *Store) CountAcks(orgID int64, alertID string) (int, error) {
	var n int
	err := s.queryRow(
		`SELECT COUNT(*) FROM alert_acks WHERE org_id = ? AND alert_id = ?`, orgID, alertID).Scan(&n)
	return n, err
}

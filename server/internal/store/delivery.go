package store

// Transactional outbox for at-least-once alert delivery. PublishAlert enqueues one
// delivery_jobs row per (alert, channel, target); a worker claims due jobs, attempts
// delivery, and either marks them sent or reschedules with backoff until max_attempts,
// after which they go to the dead-letter state (fail-loud). The claim is a single
// atomic UPDATE…RETURNING that leases rows (bumps attempts + pushes next_attempt_at
// forward), so a crashed worker's in-flight jobs are retried once the lease expires —
// no separate "claimed" state to get stuck in. Works on both SQLite and PostgreSQL;
// Postgres additionally uses FOR UPDATE SKIP LOCKED for safe multi-worker concurrency.

// DeliveryJob is one queued send attempt to a single target on one channel.
type DeliveryJob struct {
	ID          int64
	OrgID       int64
	AlertID     string
	Channel     string // "webhook" | "email" | …
	Target      string // url / email address / topic
	Payload     string // signed alert envelope JSON
	Severity    string
	Attempts    int
	MaxAttempts int
}

// EnqueueDelivery inserts a job; idempotent on (alert_id, channel, target).
// Returns inserted=false if a job for that tuple already existed.
func (s *Store) EnqueueDelivery(j DeliveryJob, now int64) (bool, error) {
	res, err := s.exec(
		`INSERT INTO delivery_jobs
		 (org_id, alert_id, channel, target, payload, severity, status, attempts, max_attempts, next_attempt_at, created_at, updated_at)
		 VALUES (?,?,?,?,?,?, 'pending', 0, ?, ?, ?, ?)
		 ON CONFLICT (alert_id, channel, target) DO NOTHING`,
		j.OrgID, j.AlertID, j.Channel, j.Target, j.Payload, j.Severity, j.MaxAttempts, now, now, now)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ClaimDueDeliveries atomically leases up to limit pending jobs whose
// next_attempt_at <= now: it bumps attempts and pushes next_attempt_at to
// now+lease (the crash-safety lease), returning the leased rows to process.
func (s *Store) ClaimDueDeliveries(now, lease int64, limit int) ([]DeliveryJob, error) {
	inner := `SELECT id FROM delivery_jobs
	          WHERE status='pending' AND next_attempt_at <= ?
	          ORDER BY next_attempt_at`
	if s.driver == "postgres" {
		inner += ` FOR UPDATE SKIP LOCKED`
	}
	inner += ` LIMIT ?`
	q := `UPDATE delivery_jobs SET attempts = attempts + 1, next_attempt_at = ?, updated_at = ?
	      WHERE id IN (` + inner + `)
	      RETURNING id, org_id, alert_id, channel, target, payload, severity, attempts, max_attempts`
	rows, err := s.query(q, now+lease, now, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeliveryJob
	for rows.Next() {
		var j DeliveryJob
		if err := rows.Scan(&j.ID, &j.OrgID, &j.AlertID, &j.Channel, &j.Target,
			&j.Payload, &j.Severity, &j.Attempts, &j.MaxAttempts); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// MarkDeliverySent finalizes a job as delivered.
func (s *Store) MarkDeliverySent(id, now int64) error {
	_, err := s.exec(`UPDATE delivery_jobs SET status='sent', last_error='', updated_at=? WHERE id=?`, now, id)
	return err
}

// RescheduleDelivery sets the next retry time after a transient failure.
func (s *Store) RescheduleDelivery(id, nextAt int64, lastErr string, now int64) error {
	_, err := s.exec(
		`UPDATE delivery_jobs SET next_attempt_at=?, last_error=?, updated_at=? WHERE id=?`,
		nextAt, lastErr, now, id)
	return err
}

// MarkDeliveryDead moves a job to the dead-letter state after exhausting retries.
func (s *Store) MarkDeliveryDead(id int64, lastErr string, now int64) error {
	_, err := s.exec(`UPDATE delivery_jobs SET status='dead', last_error=?, updated_at=? WHERE id=?`, lastErr, now, id)
	return err
}

// CountDeliveriesByStatus returns a global status→count map (worker/tests).
func (s *Store) CountDeliveriesByStatus() (map[string]int, error) {
	return s.deliveryCounts(`SELECT status, COUNT(*) FROM delivery_jobs GROUP BY status`)
}

// DeliveryStatusCounts returns an org-scoped status→count map (admin dashboard).
func (s *Store) DeliveryStatusCounts(orgID int64) (map[string]int, error) {
	return s.deliveryCounts(`SELECT status, COUNT(*) FROM delivery_jobs WHERE org_id = ? GROUP BY status`, orgID)
}

func (s *Store) deliveryCounts(q string, args ...any) (map[string]int, error) {
	rows, err := s.query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

// DeadDelivery is a failed (dead-lettered) job, surfaced for fail-loud visibility.
type DeadDelivery struct {
	AlertID   string `json:"alert_id"`
	Channel   string `json:"channel"`
	Target    string `json:"target"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"last_error"`
	UpdatedAt int64  `json:"updated_at"`
}

// RecentDeadDeliveries lists an org's most recent dead-lettered jobs, newest first.
func (s *Store) RecentDeadDeliveries(orgID int64, limit int) ([]DeadDelivery, error) {
	rows, err := s.query(
		`SELECT alert_id, channel, target, attempts, last_error, updated_at
		 FROM delivery_jobs WHERE org_id = ? AND status = 'dead'
		 ORDER BY updated_at DESC LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeadDelivery{}
	for rows.Next() {
		var d DeadDelivery
		if err := rows.Scan(&d.AlertID, &d.Channel, &d.Target, &d.Attempts, &d.LastError, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

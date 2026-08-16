package store

// Drill results (SPEC-SAFETY §3.4). Kept over time because the question a drill
// answers is not "did it work today" but "is it decaying" — a device that has
// missed the last three drills is telling you something a single run cannot.

import "strings"

type DrillResult struct {
	ID       int64  `json:"id"`
	OrgID    int64  `json:"org_id"`
	At       int64  `json:"at"`
	AlertID  string `json:"alert_id"`
	Severity string `json:"severity"`
	Expected int    `json:"expected"` // devices online when the drill fired
	Acked    int    `json:"acked"`
	Missed   string `json:"missed"` // comma-joined device ids that never answered
	Passed   bool   `json:"passed"`
}

func (s *Store) RecordDrill(d DrillResult) error {
	_, err := s.exec(
		`INSERT INTO drills (org_id, at, alert_id, severity, expected, acked, missed, passed)
		 VALUES (?,?,?,?,?,?,?,?)`,
		d.OrgID, d.At, d.AlertID, d.Severity, d.Expected, d.Acked, d.Missed, boolToInt(d.Passed))
	return err
}

// ListDrills returns recent drills newest first, so a dashboard can show the
// trend rather than only the latest verdict.
func (s *Store) ListDrills(orgID int64, limit int) ([]DrillResult, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.query(
		`SELECT id, org_id, at, alert_id, severity, expected, acked, missed, passed
		 FROM drills WHERE org_id = ? ORDER BY id DESC LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DrillResult{}
	for rows.Next() {
		var d DrillResult
		var passed int
		if err := rows.Scan(&d.ID, &d.OrgID, &d.At, &d.AlertID, &d.Severity,
			&d.Expected, &d.Acked, &d.Missed, &passed); err != nil {
			return nil, err
		}
		d.Passed = passed != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

// MissedList splits the stored comma-joined ids.
func (d DrillResult) MissedList() []string {
	if d.Missed == "" {
		return nil
	}
	return strings.Split(d.Missed, ",")
}

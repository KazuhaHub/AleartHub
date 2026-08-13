package store

import (
	"database/sql"
	"errors"
	"time"
)

type Org struct {
	ID        int64
	Slug      string
	Name      string
	CreatedAt int64
}

func (s *Store) CreateOrg(slug, name string) (int64, error) {
	return s.insertID(`INSERT INTO orgs (slug, name, created_at) VALUES (?,?,?)`, slug, name, time.Now().Unix())
}

func (s *Store) GetOrgBySlug(slug string) (*Org, error) {
	var o Org
	err := s.queryRow(`SELECT id, slug, name, created_at FROM orgs WHERE slug = ?`, slug).
		Scan(&o.ID, &o.Slug, &o.Name, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &o, err
}

func (s *Store) ListOrgs() ([]Org, error) {
	rows, err := s.query(`SELECT id, slug, name, created_at FROM orgs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Org{}
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Slug, &o.Name, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// EnsureDefaultOrg returns the "default" org id, creating it on first run.
func (s *Store) EnsureDefaultOrg() (int64, error) {
	if o, err := s.GetOrgBySlug("default"); err == nil {
		return o.ID, nil
	}
	return s.CreateOrg("default", "AlertHub")
}

func (s *Store) AddMembership(orgID, userID int64, baseRole string) error {
	_, err := s.exec(
		`INSERT INTO memberships (org_id, user_id, base_role, created_at) VALUES (?,?,?,?)
		 ON CONFLICT DO NOTHING`,
		orgID, userID, baseRole, time.Now().Unix())
	return err
}

// GetMembershipRole returns a user's base role within an org.
func (s *Store) GetMembershipRole(orgID, userID int64) (string, bool) {
	var role string
	err := s.queryRow(`SELECT base_role FROM memberships WHERE org_id = ? AND user_id = ?`, orgID, userID).Scan(&role)
	if err != nil {
		return "", false
	}
	return role, true
}

// OrgsForUser lists the orgs a user is a member of.
func (s *Store) OrgsForUser(userID int64) ([]Org, error) {
	rows, err := s.query(
		`SELECT o.id, o.slug, o.name, o.created_at FROM orgs o
		 JOIN memberships m ON m.org_id = o.id WHERE m.user_id = ? ORDER BY o.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Org{}
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Slug, &o.Name, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// BackfillMemberships gives every membership-less user a membership in orgID with
// their current users.role (one-time migration to multi-tenancy).
func (s *Store) BackfillMemberships(orgID int64) error {
	_, err := s.exec(
		`INSERT INTO memberships (org_id, user_id, base_role, created_at)
		 SELECT ?, id, role, ? FROM users WHERE id NOT IN (SELECT user_id FROM memberships)
		 ON CONFLICT DO NOTHING`,
		orgID, time.Now().Unix())
	return err
}

// BackfillOrgID assigns pre-tenancy rows to the default org.
func (s *Store) BackfillOrgID(orgID int64) error {
	if _, err := s.exec(`UPDATE alerts SET org_id = ? WHERE org_id = 0`, orgID); err != nil {
		return err
	}
	_, err := s.exec(`UPDATE service_accounts SET org_id = ? WHERE org_id = 0`, orgID)
	return err
}

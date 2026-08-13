package store

import (
	"database/sql"
	"errors"
	"time"
)

// ServiceAccount is a machine identity (CAP ingest, webhooks, integrations).
// The plaintext token is shown once at creation; only its SHA-256 hash is stored.
type ServiceAccount struct {
	ID         int64
	OrgID      int64
	Name       string
	Scopes     string // comma-joined, e.g. "alerts:ingest"
	Disabled   bool
	CreatedAt  int64
	LastUsedAt int64
}

func (s *Store) CreateServiceAccount(name, tokenHash, scopes string, orgID int64) (int64, error) {
	return s.insertID(
		`INSERT INTO service_accounts (name, token_hash, scopes, created_at, org_id) VALUES (?,?,?,?,?)`,
		name, tokenHash, scopes, time.Now().Unix(), orgID)
}

func (s *Store) ListServiceAccounts(orgID int64) ([]ServiceAccount, error) {
	rows, err := s.query(
		`SELECT id, org_id, name, scopes, disabled, created_at, COALESCE(last_used_at,0)
		 FROM service_accounts WHERE org_id = ? ORDER BY id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ServiceAccount{}
	for rows.Next() {
		var a ServiceAccount
		var disabled int
		if err := rows.Scan(&a.ID, &a.OrgID, &a.Name, &a.Scopes, &disabled, &a.CreatedAt, &a.LastUsedAt); err != nil {
			return nil, err
		}
		a.Disabled = disabled != 0
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetServiceAccountByTokenHash resolves an API key (by hash) to its account
// (carries OrgID → the tenant CAP ingest is scoped to).
func (s *Store) GetServiceAccountByTokenHash(hash string) (*ServiceAccount, error) {
	var a ServiceAccount
	var disabled int
	err := s.queryRow(
		`SELECT id, org_id, name, scopes, disabled, created_at, COALESCE(last_used_at,0)
		 FROM service_accounts WHERE token_hash = ?`, hash,
	).Scan(&a.ID, &a.OrgID, &a.Name, &a.Scopes, &disabled, &a.CreatedAt, &a.LastUsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.Disabled = disabled != 0
	return &a, nil
}

func (s *Store) TouchServiceAccount(id int64) error {
	_, err := s.exec(`UPDATE service_accounts SET last_used_at = ? WHERE id = ?`, time.Now().Unix(), id)
	return err
}

// DeleteServiceAccount removes a key, scoped to its org (tenant isolation: an
// admin in org A cannot delete org B's key by guessing its id).
func (s *Store) DeleteServiceAccount(id, orgID int64) error {
	_, err := s.exec(`DELETE FROM service_accounts WHERE id = ? AND org_id = ?`, id, orgID)
	return err
}

package store

import (
	"database/sql"
	"errors"
	"time"
)

type User struct {
	ID           int64
	UPN          string
	Email        string
	PasswordHash string
	Role         string
	Enabled      bool
	TokenVersion int
	CreatedAt    int64
	IsSuperadmin bool
}

var ErrNotFound = errors.New("not found")

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	var enabled, super int
	var email, hash sql.NullString
	if err := row.Scan(&u.ID, &u.UPN, &email, &hash, &u.Role, &enabled, &u.TokenVersion, &u.CreatedAt, &super); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.Email = email.String
	u.PasswordHash = hash.String
	u.Enabled = enabled != 0
	u.IsSuperadmin = super != 0
	return &u, nil
}

const userCols = `id, upn, email, password_hash, role, enabled, token_version, created_at, COALESCE(is_superadmin,0)`

func (s *Store) GetUserByUPN(upn string) (*User, error) {
	return scanUser(s.queryRow(`SELECT `+userCols+` FROM users WHERE upn = ?`, upn))
}

func (s *Store) GetUserByID(id int64) (*User, error) {
	return scanUser(s.queryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

// GetUserBySSO resolves an SSO identity (provider, subject) to a user.
func (s *Store) GetUserBySSO(provider, subject string) (*User, error) {
	return scanUser(s.queryRow(
		`SELECT `+userCols+` FROM users WHERE sso_provider = ? AND sso_subject = ?`, provider, subject))
}

// CreateSSOUser JIT-provisions an SSO user (no password).
func (s *Store) CreateSSOUser(upn, email, provider, subject, role string) (int64, error) {
	if role == "" {
		role = "user"
	}
	if upn == "" {
		upn = subject
	}
	return s.insertID(
		`INSERT INTO users (upn, email, password_hash, role, enabled, token_version, created_at, sso_provider, sso_subject)
		 VALUES (?,?,'',?,1,0,?,?,?)`,
		upn, email, role, time.Now().Unix(), provider, subject)
}

func (s *Store) CreateUser(u *User) (int64, error) {
	if u.CreatedAt == 0 {
		u.CreatedAt = time.Now().Unix()
	}
	if u.Role == "" {
		u.Role = "user"
	}
	return s.insertID(
		`INSERT INTO users (upn, email, password_hash, role, enabled, token_version, created_at)
		 VALUES (?,?,?,?,?,?,?)`,
		u.UPN, u.Email, u.PasswordHash, u.Role, boolToInt(u.Enabled), u.TokenVersion, u.CreatedAt)
}

// BumpTokenVersion revokes all of a user's existing JWTs (logout-everywhere /
// password change / disable).
func (s *Store) BumpTokenVersion(id int64) error {
	_, err := s.exec(`UPDATE users SET token_version = token_version + 1 WHERE id = ?`, id)
	return err
}

func (s *Store) MakeSuperadmin(id int64) error {
	_, err := s.exec(`UPDATE users SET is_superadmin = 1 WHERE id = ?`, id)
	return err
}

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.queryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

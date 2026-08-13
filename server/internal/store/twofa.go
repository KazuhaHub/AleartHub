package store

import (
	"database/sql"
	"errors"
)

type TOTP struct {
	SecretEnc []byte
	Recovery  string // newline-joined sha256 hex
	Enabled   bool
}

// GetTOTP returns the user's TOTP row; ErrNotFound if none.
func (s *Store) GetTOTP(userID int64) (*TOTP, error) {
	var t TOTP
	var enabled int
	err := s.queryRow(
		`SELECT secret_enc, recovery, enabled FROM user_totp WHERE user_id = ?`, userID,
	).Scan(&t.SecretEnc, &t.Recovery, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled != 0
	return &t, nil
}

// UpsertTOTPPending stores a not-yet-enabled secret (enrollment begin).
func (s *Store) UpsertTOTPPending(userID int64, secretEnc []byte) error {
	_, err := s.exec(
		`INSERT INTO user_totp (user_id, secret_enc, recovery, enabled) VALUES (?,?, '', 0)
		 ON CONFLICT(user_id) DO UPDATE SET secret_enc = excluded.secret_enc, recovery='', enabled=0`,
		userID, secretEnc)
	return err
}

func (s *Store) EnableTOTP(userID int64, recovery string) error {
	_, err := s.exec(`UPDATE user_totp SET enabled = 1, recovery = ? WHERE user_id = ?`, recovery, userID)
	return err
}

func (s *Store) SetTOTPRecovery(userID int64, recovery string) error {
	_, err := s.exec(`UPDATE user_totp SET recovery = ? WHERE user_id = ?`, recovery, userID)
	return err
}

func (s *Store) DeleteTOTP(userID int64) error {
	_, err := s.exec(`DELETE FROM user_totp WHERE user_id = ?`, userID)
	return err
}

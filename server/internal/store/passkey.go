package store

import "time"

// Passkey is a stored WebAuthn credential. `Credential` is the JSON-marshalled
// webauthn.Credential (public key, aaguid, transports…); the passkey service
// owns that encoding so the store stays crypto-library-agnostic.
type Passkey struct {
	ID           int64
	UserID       int64
	CredentialID string // base64url(raw credential id)
	Credential   []byte // JSON(webauthn.Credential)
	SignCount    int64
	Name         string
	CreatedAt    int64
	LastUsedAt   int64
}

func (s *Store) AddPasskey(p *Passkey) (int64, error) {
	if p.CreatedAt == 0 {
		p.CreatedAt = time.Now().Unix()
	}
	return s.insertID(
		`INSERT INTO passkey_credentials (user_id, credential_id, credential, sign_count, name, created_at)
		 VALUES (?,?,?,?,?,?)`,
		p.UserID, p.CredentialID, p.Credential, p.SignCount, p.Name, p.CreatedAt)
}

func (s *Store) ListPasskeys(userID int64) ([]Passkey, error) {
	rows, err := s.query(
		`SELECT id, user_id, credential_id, credential, sign_count, name, created_at, last_used_at
		 FROM passkey_credentials WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Passkey
	for rows.Next() {
		var p Passkey
		var lastUsed *int64
		if err := rows.Scan(&p.ID, &p.UserID, &p.CredentialID, &p.Credential, &p.SignCount, &p.Name, &p.CreatedAt, &lastUsed); err != nil {
			return nil, err
		}
		if lastUsed != nil {
			p.LastUsedAt = *lastUsed
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPasskeyByCredID resolves a credential (for discoverable login) → its owner.
func (s *Store) GetPasskeyByCredID(credID string) (*Passkey, error) {
	row := s.queryRow(
		`SELECT id, user_id, credential_id, credential, sign_count, name, created_at, COALESCE(last_used_at,0)
		 FROM passkey_credentials WHERE credential_id = ?`, credID)
	var p Passkey
	if err := row.Scan(&p.ID, &p.UserID, &p.CredentialID, &p.Credential, &p.SignCount, &p.Name, &p.CreatedAt, &p.LastUsedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) UpdatePasskeyUsage(credID string, signCount int64) error {
	_, err := s.exec(
		`UPDATE passkey_credentials SET sign_count = ?, last_used_at = ? WHERE credential_id = ?`,
		signCount, time.Now().Unix(), credID)
	return err
}

func (s *Store) DeletePasskey(userID, id int64) error {
	_, err := s.exec(`DELETE FROM passkey_credentials WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

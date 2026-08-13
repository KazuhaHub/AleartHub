// Package auth implements admin/human authentication, modeled on the Passwall
// panel: JWT access/refresh tokens with a per-user TokenVersion revocation
// counter, bcrypt passwords. Passkey/TOTP/SSO are layered on in later phases.
// (Device/receiver connection auth is SEPARATE — see SPEC-SAFETY §6 / Devices.)
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Token subjects.
const (
	SubjectAccess  = "access"
	SubjectRefresh = "refresh"
	SubjectPending = "2fa_pending" // short-lived: password OK, awaiting 2FA
)

// Roles (Passwall parity).
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleUser     = "user"
)

type Claims struct {
	UserID       int64  `json:"uid"`
	UPN          string `json:"upn"`
	Role         string `json:"role"`
	TokenVersion int    `json:"tv"`
	jwt.RegisteredClaims
}

type Service struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func New(secret []byte) *Service {
	return &Service{secret: secret, accessTTL: 2 * time.Hour, refreshTTL: 7 * 24 * time.Hour}
}

func (s *Service) sign(subject string, uid int64, upn, role string, tv int, ttl time.Duration) (string, error) {
	now := time.Now()
	c := Claims{
		UserID: uid, UPN: upn, Role: role, TokenVersion: tv,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(s.secret)
}

// IssuePending returns a short-lived (5 min) token proving the password factor
// passed; it is exchanged at /api/auth/2fa/verify for a full session.
func (s *Service) IssuePending(uid int64, upn, role string, tv int) (string, error) {
	return s.sign(SubjectPending, uid, upn, role, tv, 5*time.Minute)
}

// IssueTokens returns an (access, refresh) pair for a user.
func (s *Service) IssueTokens(uid int64, upn, role string, tv int) (access, refresh string, err error) {
	access, err = s.sign(SubjectAccess, uid, upn, role, tv, s.accessTTL)
	if err != nil {
		return "", "", err
	}
	refresh, err = s.sign(SubjectRefresh, uid, upn, role, tv, s.refreshTTL)
	return access, refresh, err
}

var ErrInvalidToken = errors.New("invalid token")

// Verify parses+validates a token and checks it has the expected subject.
func (s *Service) Verify(raw, expectedSubject string) (*Claims, error) {
	var c Claims
	tok, err := jwt.ParseWithClaims(raw, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	})
	if err != nil || !tok.Valid || c.Subject != expectedSubject {
		return nil, ErrInvalidToken
	}
	return &c, nil
}

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

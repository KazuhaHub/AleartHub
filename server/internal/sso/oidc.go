// Package sso implements OIDC single sign-on for admin login (Passwall pattern,
// coreos/go-oidc). Single-tenant for now; per-tenant SSO is a multi-tenancy-gated
// follow-up. Uses PKCE + nonce; RP-ID/redirect from config, never the Host header.
package sso

import (
	"context"
	"errors"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDCConfig struct {
	Enabled      bool
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

// Claims is the normalized identity from a verified ID token.
type Claims struct {
	Subject  string
	Email    string
	Name     string
	Username string
	Groups   []string
}

type OIDC struct {
	cfg      OIDCConfig
	oauth2   *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

// NewOIDC discovers the provider. If cfg.Enabled is false it returns a disabled
// instance (no network). On discovery failure (enabled) it returns an error so
// the caller can log + run disabled rather than crash.
func NewOIDC(ctx context.Context, cfg OIDCConfig) (*OIDC, error) {
	if !cfg.Enabled {
		return &OIDC{cfg: cfg}, nil
	}
	p, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, err
	}
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	return &OIDC{
		cfg: cfg,
		oauth2: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     p.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       scopes,
		},
		verifier: p.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
	}, nil
}

func Disabled() *OIDC { return &OIDC{} }

func (o *OIDC) Enabled() bool { return o.oauth2 != nil && o.verifier != nil }

// AuthCodeURL builds the IdP authorize URL with nonce + PKCE S256 challenge.
func (o *OIDC) AuthCodeURL(state, nonce, verifier string) string {
	return o.oauth2.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
}

// Exchange swaps the code for tokens, verifies the ID token signature/aud/iss,
// checks the nonce, and returns normalized claims.
func (o *OIDC) Exchange(ctx context.Context, code, expectedNonce, verifier string) (*Claims, error) {
	tok, err := o.oauth2.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, err
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok || raw == "" {
		return nil, errors.New("oidc: no id_token in token response")
	}
	idt, err := o.verifier.Verify(ctx, raw)
	if err != nil {
		return nil, err
	}
	if idt.Nonce != expectedNonce {
		return nil, errors.New("oidc: nonce mismatch")
	}
	var c struct {
		Sub               string   `json:"sub"`
		Email             string   `json:"email"`
		Name              string   `json:"name"`
		PreferredUsername string   `json:"preferred_username"`
		Groups            []string `json:"groups"`
	}
	if err := idt.Claims(&c); err != nil {
		return nil, err
	}
	username := c.PreferredUsername
	if username == "" {
		username = c.Email
	}
	if username == "" {
		username = c.Sub
	}
	return &Claims{Subject: c.Sub, Email: c.Email, Name: c.Name, Username: username, Groups: c.Groups}, nil
}

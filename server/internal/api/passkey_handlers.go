package api

import (
	"encoding/json"
	"net/http"
)

// POST /api/auth/passkey/register/begin  (session required)
func (s *Server) handlePasskeyRegBegin(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r)
	if c == nil {
		http.Error(w, "passkey registration requires a session login (not the admin token)", http.StatusBadRequest)
		return
	}
	u, err := s.Store.GetUserByID(c.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	creation, sid, err := s.Passkey.BeginRegistration(u.ID, u.UPN)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"options": creation, "session": sid})
}

// POST /api/auth/passkey/register/finish?session=..&name=..  body = attestation
func (s *Server) handlePasskeyRegFinish(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r)
	if c == nil {
		http.Error(w, "session required", http.StatusBadRequest)
		return
	}
	u, err := s.Store.GetUserByID(c.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if err := s.Passkey.FinishRegistration(u.ID, u.UPN, r.URL.Query().Get("session"), r.URL.Query().Get("name"), r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// A new credential that can log in as this user — the classic takeover
	// footprint, so it is recorded with the name the user gave it.
	s.audit(r, s.orgFor(r), AuditPasskeyAdd, "user", u.UPN, "passkey added: "+r.URL.Query().Get("name"))
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/auth/passkey/login/begin  (public — usernameless)
func (s *Server) handlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	assertion, sid, err := s.Passkey.BeginLogin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"options": assertion, "session": sid})
}

// POST /api/auth/passkey/login/finish?session=..  body = assertion
func (s *Server) handlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	u, err := s.Passkey.FinishLogin(r.URL.Query().Get("session"), r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	access, refresh, err := s.Auth.IssueTokens(u.ID, u.UPN, u.Role, u.TokenVersion)
	if err != nil {
		http.Error(w, "token error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, tokenResp{access, refresh, toDTO(u)})
}

type passkeyDTO struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CreatedAt  int64  `json:"created_at"`
	LastUsedAt int64  `json:"last_used_at"`
}

// GET /api/auth/passkey/list  (session required)
func (s *Server) handlePasskeyList(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r)
	if c == nil {
		writeJSON(w, []passkeyDTO{})
		return
	}
	pks, err := s.Passkey.List(c.UserID)
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	out := make([]passkeyDTO, 0, len(pks))
	for _, p := range pks {
		out = append(out, passkeyDTO{ID: p.ID, Name: p.Name, CreatedAt: p.CreatedAt, LastUsedAt: p.LastUsedAt})
	}
	writeJSON(w, out)
}

// POST /api/auth/passkey/delete  body {id}  (session required)
func (s *Server) handlePasskeyDelete(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r)
	if c == nil {
		http.Error(w, "session required", http.StatusBadRequest)
		return
	}
	var req struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if err := s.Passkey.Delete(c.UserID, req.ID); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	s.audit(r, s.orgFor(r), AuditPasskeyDel, "user", c.UPN, "passkey removed")
	w.WriteHeader(http.StatusNoContent)
}

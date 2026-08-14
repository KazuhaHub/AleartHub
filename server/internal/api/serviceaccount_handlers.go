package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type saDTO struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Scopes     []string `json:"scopes"`
	Disabled   bool     `json:"disabled"`
	CreatedAt  int64    `json:"created_at"`
	LastUsedAt int64    `json:"last_used_at"`
}

func splitScopes(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// /api/admin/service-accounts — GET list, POST create (admin only).
func (s *Server) handleServiceAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.Store.ListServiceAccounts(s.orgFor(r))
		if err != nil {
			http.Error(w, "list failed", http.StatusInternalServerError)
			return
		}
		out := make([]saDTO, 0, len(list))
		for _, a := range list {
			out = append(out, saDTO{a.ID, a.Name, splitScopes(a.Scopes), a.Disabled, a.CreatedAt, a.LastUsedAt})
		}
		writeJSON(w, out)
	case http.MethodPost:
		var req struct {
			Name   string   `json:"name"`
			Scopes []string `json:"scopes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if len(req.Scopes) == 0 {
			req.Scopes = []string{"alerts:ingest"}
		}
		token := apiKeyPrefix + randToken()
		id, err := s.Store.CreateServiceAccount(req.Name, sha256hex(token), strings.Join(req.Scopes, ","), s.orgFor(r))
		if err != nil {
			http.Error(w, "create failed", http.StatusInternalServerError)
			return
		}
		s.audit(r, s.orgFor(r), AuditSACreate, "service_account", strconv.FormatInt(id, 10),
			req.Name+" scopes="+strings.Join(req.Scopes, ","))
		// token shown ONCE
		writeJSON(w, map[string]any{"id": id, "name": req.Name, "scopes": req.Scopes, "token": token})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// POST /api/admin/service-accounts/delete {id} (admin only).
func (s *Server) handleServiceAccountDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if err := s.Store.DeleteServiceAccount(req.ID, s.orgFor(r)); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func randToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

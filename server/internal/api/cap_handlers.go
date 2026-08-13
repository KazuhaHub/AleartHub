package api

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kazuha/alerthub/server/internal/alert"
	"github.com/kazuha/alerthub/server/internal/cap"
	"github.com/kazuha/alerthub/server/internal/metrics"
)

// oneline strips CR/LF so CAP-sourced text is safe for the signed canonical form
// (SPEC §3.2 rule 7 forbids newlines in title/body/action).
func oneline(s string) string {
	return strings.TrimSpace(strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(s))
}

// POST /api/cap — ingest a CAP 1.2 XML alert (scope alerts:ingest). The standard
// interop entry point for "other programs / emergency systems can call".
func (s *Server) handleCAPIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	doc, err := cap.Parse(body)
	if err != nil {
		metrics.CapIngest.WithLabelValues("error").Inc()
		http.Error(w, "invalid CAP XML: "+err.Error(), http.StatusBadRequest)
		return
	}
	m := doc.MapToAlert(time.Now())
	if m.MsgType == "Cancel" {
		s.handleCAPCancel(w, r, doc)
		return
	}
	m.Title, m.Body, m.Action = oneline(m.Title), oneline(m.Body), oneline(m.Action)
	if err := alert.ValidateInput(m.Severity, m.Category, m.Title, m.Body, m.Action, "cap"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a := &alert.Alert{
		SchemaVersion: alert.SchemaVersion,
		// Deterministic id from (sender, identifier) so a later CAP Cancel can recall
		// it (see cap.AlertID). An Update re-using the same identifier maps to the same
		// id → clients dedup/renew rather than double-alarm.
		ID:       cap.AlertID(doc.Sender, doc.Identifier),
		Type:     "alert",
		Category: m.Category,
		Severity: m.Severity,
		Title:    m.Title,
		Body:     m.Body,
		Action:   m.Action,
		Source:   "cap",
		IssuedAt: time.Now().Unix(),
		TTL:      m.TTL,
		Nonce:    alert.NewNonce(),
	}
	if err := s.PublishAlert(a, s.orgFor(r)); err != nil {
		http.Error(w, "publish failed", http.StatusBadGateway)
		return
	}
	metrics.CapIngest.WithLabelValues("ok").Inc()
	writeJSON(w, map[string]any{
		"id": a.ID, "severity": a.Severity, "category": a.Category, "test": m.IsTest,
	})
}

// handleCAPCancel recalls the alert(s) a CAP Cancel references. Each CAP
// <references> entry is a (sender, identifier) that we map back — via the same
// deterministic cap.AlertID — to the id we published, then recall with the existing
// signed-cancel path (SPEC §5.1).
func (s *Server) handleCAPCancel(w http.ResponseWriter, r *http.Request, doc *cap.Document) {
	refs := cap.ParseReferences(doc.References)
	if len(refs) == 0 {
		metrics.CapIngest.WithLabelValues("error").Inc()
		http.Error(w, "CAP Cancel requires <references> to the alert(s) being cancelled", http.StatusBadRequest)
		return
	}
	org := s.orgFor(r)
	cancelled := make([]string, 0, len(refs))
	for _, ref := range refs {
		id := cap.AlertID(ref.Sender, ref.Identifier)
		if _, err := s.CancelByID(id, "cap", org); err != nil {
			http.Error(w, "cancel failed", http.StatusBadGateway)
			return
		}
		cancelled = append(cancelled, id)
	}
	metrics.CapIngest.WithLabelValues("cancel").Inc()
	writeJSON(w, map[string]any{"cancelled": cancelled})
}

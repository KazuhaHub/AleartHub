package api

// Read-only view of what can put an alert into this system, and which backup
// channels it fans out to.
//
// Source configuration is environment-driven and deliberately stays that way:
// making feeds runtime-editable would turn the admin console into another path
// for injecting alerts, and the console is the surface most likely to be
// phished. So this endpoint REPORTS configuration, it never changes it — and it
// reports whether a thing is configured, never its secrets.

import "net/http"

// SourceInfo describes one ingress or egress channel.
type SourceInfo struct {
	Key       string `json:"key"`
	Kind      string `json:"kind"` // "ingress" | "egress"
	Enabled   bool   `json:"enabled"`
	Detail    string `json:"detail"`     // how it is reached / what it covers
	ConfigVar string `json:"config_var"` // the env var that controls it
}

// SourcesConfig is what /api/sources returns.
type SourcesConfig struct {
	Sources []SourceInfo `json:"sources"`
}

// handleSources reports the configured ingress/egress channels. No secret value
// is ever included — only whether something is switched on.
func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	out := []SourceInfo{
		{
			Key: "panel", Kind: "ingress", Enabled: true,
			Detail:    "POST /api/publish — the console and any script holding a session or the admin token",
			ConfigVar: "ALERTHUB_ADMIN_TOKEN",
		},
		{
			Key: "cap", Kind: "ingress", Enabled: true,
			Detail:    "POST /api/cap — CAP 1.2 XML, service-account key with scope alerts:ingest",
			ConfigVar: "(service account)",
		},
		{
			Key: "eew_wolfx", Kind: "ingress", Enabled: s.EEWEnabled,
			Detail:    "Japan EEW via the Wolfx WebSocket feed (single source; SPEC-SAFETY §6.1 asks for two)",
			ConfigVar: "ALERTHUB_EEW",
		},
		{
			Key: "ntfy", Kind: "egress", Enabled: s.Ntfy != nil && s.Ntfy.Enabled(),
			Detail:    "Independent backup channel — self-hosted for all severities, ntfy.sh for critical/emergency only",
			ConfigVar: "ALERTHUB_NTFY_URL / ALERTHUB_NTFY_SH_TOPIC",
		},
		{
			Key: "delivery", Kind: "egress", Enabled: s.Delivery != nil && s.Delivery.Enabled(),
			Detail:    "Durable outbox: webhook (all severities) + SMTP email (critical/emergency)",
			ConfigVar: "ALERTHUB_WEBHOOK_URLS / ALERTHUB_SMTP_HOST",
		},
		{
			Key: "watchdog", Kind: "egress", Enabled: s.WatchdogConfigured,
			Detail:    "External dead-man switch (SPEC-SAFETY §3.3) — silence at the far end is the alarm",
			ConfigVar: "ALERTHUB_WATCHDOG_URL",
		},
		{
			Key: "siem", Kind: "egress", Enabled: s.SIEMConfigured,
			Detail:    "Audit-trail export to an external collector, at-least-once from a durable cursor",
			ConfigVar: "ALERTHUB_SIEM_URL",
		},
	}
	writeJSON(w, SourcesConfig{Sources: out})
}

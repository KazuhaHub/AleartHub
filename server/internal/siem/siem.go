// Package siem ships the audit trail to an external collector.
//
// Compliance buyers do not accept "the log is in our database" — the trail has
// to reach their SIEM, where it is outside the reach of anyone who compromises
// this host. That is also why the exporter is at-least-once from a DURABLE
// cursor rather than a best-effort tail: an audit trail with invisible holes is
// worse than no trail, because you cannot tell a quiet period from a lost one.
package siem

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/kazuha/alerthub/server/internal/store"
)

// cursorName is the durable position of the exporter in the audit log.
const cursorName = "siem_export"

type Config struct {
	URL      string        // collector endpoint; empty disables the exporter
	Token    string        // optional bearer token
	Interval time.Duration // poll cadence
	Batch    int           // max entries per POST
}

type Exporter struct {
	cfg   Config
	store *store.Store
	http  *http.Client
}

func New(st *store.Store, cfg Config) *Exporter {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.Batch <= 0 {
		cfg.Batch = 200
	}
	return &Exporter{cfg: cfg, store: st, http: &http.Client{Timeout: 15 * time.Second}}
}

func (e *Exporter) Enabled() bool { return e != nil && e.cfg.URL != "" }

// Run polls until ctx is done. It advances the cursor ONLY after the collector
// has accepted a batch, so a crash or a collector outage replays rather than
// skips — the receiving end must dedupe on entry id.
func (e *Exporter) Run(ctx context.Context) {
	if !e.Enabled() {
		return
	}
	slog.Info("siem exporter started", "url", e.cfg.URL, "interval", e.cfg.Interval.String())
	t := time.NewTicker(e.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := e.exportOnce(ctx); err != nil {
				// Loud: a silently broken export is exactly the failure mode that
				// makes an audit trail useless when it is finally needed.
				slog.Error("siem export failed", "err", err)
			}
		}
	}
}

// ExportOnce is one drain pass, exposed for tests.
func (e *Exporter) ExportOnce(ctx context.Context) error { return e.exportOnce(ctx) }

func (e *Exporter) exportOnce(ctx context.Context) error {
	for {
		pos, err := e.store.GetCursor(cursorName)
		if err != nil {
			return err
		}
		batch, err := e.store.AuditSince(pos, e.cfg.Batch)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		if err := e.post(ctx, batch); err != nil {
			return err // cursor stays put: the batch will be retried
		}
		last := batch[len(batch)-1].ID
		if err := e.store.SetCursor(cursorName, last); err != nil {
			return err
		}
		slog.Debug("siem export batch shipped", "entries", len(batch), "cursor", last)
		if len(batch) < e.cfg.Batch {
			return nil // drained
		}
	}
}

func (e *Exporter) post(ctx context.Context, entries []store.AuditEntry) error {
	// Entries are sent with their prev_hash/hash so the collector can verify the
	// chain independently of this server — the point of shipping them off-host.
	body, err := json.Marshal(map[string]any{
		"source":  "alerthub",
		"kind":    "audit",
		"entries": entries,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+e.cfg.Token)
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &httpError{Status: resp.StatusCode}
	}
	return nil
}

type httpError struct{ Status int }

func (e *httpError) Error() string { return "collector returned HTTP " + http.StatusText(e.Status) }

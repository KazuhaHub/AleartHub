package siem

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/kazuha/alerthub/server/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func seed(t *testing.T, st *store.Store, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := st.AppendAudit(&store.AuditEntry{
			OrgID: 1, Action: "alert.publish", ActorType: store.ActorUser, ActorName: "alice",
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func TestExport_ShipsEntriesAndAdvancesCursor(t *testing.T) {
	st := newStore(t)
	seed(t, st, 3)

	var got atomic.Int64
	var lastBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		lastBody = b
		var payload struct {
			Entries []store.AuditEntry `json:"entries"`
		}
		_ = json.Unmarshal(b, &payload)
		got.Add(int64(len(payload.Entries)))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := New(st, Config{URL: srv.URL})
	if err := e.ExportOnce(context.Background()); err != nil {
		t.Fatalf("export: %v", err)
	}
	if got.Load() != 3 {
		t.Fatalf("collector received %d entries, want 3", got.Load())
	}
	// The chain fields must travel, so the collector can verify independently of
	// this server — the whole point of shipping them off-host.
	if !json.Valid(lastBody) {
		t.Fatal("payload is not valid JSON")
	}
	var payload struct {
		Kind    string             `json:"kind"`
		Entries []store.AuditEntry `json:"entries"`
	}
	_ = json.Unmarshal(lastBody, &payload)
	if payload.Kind != "audit" || payload.Entries[0].Hash == "" || payload.Entries[1].PrevHash == "" {
		t.Fatalf("payload must carry kind and chain hashes: %+v", payload)
	}

	// A second pass must ship nothing: the cursor advanced.
	if err := e.ExportOnce(context.Background()); err != nil {
		t.Fatalf("second export: %v", err)
	}
	if got.Load() != 3 {
		t.Fatalf("re-shipped entries; collector saw %d, want 3", got.Load())
	}
}

// TestExport_CollectorDownDoesNotLoseEntries is the property that makes this an
// audit-grade exporter rather than a best-effort tail: while the collector is
// failing the cursor must NOT advance, so nothing is silently dropped.
func TestExport_CollectorDownDoesNotLoseEntries(t *testing.T) {
	st := newStore(t)
	seed(t, st, 2)

	var fail atomic.Bool
	fail.Store(true)
	var received atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var payload struct {
			Entries []store.AuditEntry `json:"entries"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &payload)
		received.Add(int64(len(payload.Entries)))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := New(st, Config{URL: srv.URL})
	if err := e.ExportOnce(context.Background()); err == nil {
		t.Fatal("a failing collector must surface an error")
	}
	if received.Load() != 0 {
		t.Fatal("collector should not have accepted anything")
	}
	// More activity happens during the outage.
	seed(t, st, 2)

	fail.Store(false)
	if err := e.ExportOnce(context.Background()); err != nil {
		t.Fatalf("export after recovery: %v", err)
	}
	if received.Load() != 4 {
		t.Fatalf("after recovery the collector saw %d entries, want all 4 — entries were LOST", received.Load())
	}
}

func TestExport_DisabledWithoutURL(t *testing.T) {
	e := New(newStore(t), Config{})
	if e.Enabled() {
		t.Fatal("exporter must be disabled without a collector URL")
	}
	// Run must return immediately rather than spin.
	e.Run(context.Background())
}

func TestExport_PaginatesLargeBacklog(t *testing.T) {
	st := newStore(t)
	seed(t, st, 25)
	var total atomic.Int64
	var batches atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Entries []store.AuditEntry `json:"entries"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &payload)
		total.Add(int64(len(payload.Entries)))
		batches.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := New(st, Config{URL: srv.URL, Batch: 10})
	if err := e.ExportOnce(context.Background()); err != nil {
		t.Fatalf("export: %v", err)
	}
	if total.Load() != 25 {
		t.Fatalf("shipped %d of 25 entries", total.Load())
	}
	if batches.Load() < 3 {
		t.Fatalf("expected the backlog to be paginated, got %d batch(es)", batches.Load())
	}
}

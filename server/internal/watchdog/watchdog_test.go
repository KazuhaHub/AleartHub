package watchdog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu    sync.Mutex
	paths []string
}

func (r *recorder) add(p string) { r.mu.Lock(); r.paths = append(r.paths, p); r.mu.Unlock() }
func (r *recorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

func TestPing_HealthyHitsTheBaseURL(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := New(Config{URL: srv.URL + "/hc/abc"}, func() (bool, string) { return true, "" })
	w.Ping(context.Background())

	got := rec.all()
	if len(got) != 1 || got[0] != "/hc/abc" {
		t.Fatalf("healthy ping hit %v, want [/hc/abc]", got)
	}
}

// TestPing_DegradedHitsFail is the difference between "I am dead" and "I am
// alive but broken": a degraded server signals /fail explicitly so the far end
// alerts immediately rather than waiting out its grace period.
func TestPing_DegradedHitsFail(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := New(Config{URL: srv.URL + "/hc/abc"}, func() (bool, string) { return false, "store unreachable" })
	w.Ping(context.Background())

	got := rec.all()
	if len(got) != 1 || !strings.HasSuffix(got[0], "/fail") {
		t.Fatalf("degraded ping hit %v, want a /fail suffix", got)
	}
}

// The whole design rests on this: if the process is gone, nothing pings, and the
// far end's grace period is what raises the alarm. Nothing here should ever try
// to compensate for a dead process — there is nothing left to compensate with.
func TestRun_StopsPingingWhenCancelled(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	w := New(Config{URL: srv.URL, Interval: 20 * time.Millisecond}, func() (bool, string) { return true, "" })
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	time.Sleep(90 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
	before := len(rec.all())
	if before < 2 {
		t.Fatalf("expected repeated pings while running, got %d", before)
	}
	time.Sleep(80 * time.Millisecond)
	if after := len(rec.all()); after != before {
		t.Fatalf("kept pinging after shutdown: %d -> %d", before, after)
	}
}

func TestDisabledWithoutURL(t *testing.T) {
	w := New(Config{}, func() (bool, string) { return true, "" })
	if w.Enabled() {
		t.Fatal("watchdog must be disabled without a URL")
	}
	w.Run(context.Background()) // must return immediately, not spin
}

// A collector that is down must not take the server with it.
func TestPing_SurvivesCollectorFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	w := New(Config{URL: srv.URL}, func() (bool, string) { return true, "" })
	w.Ping(context.Background()) // must not panic
}

func TestPing_UnreachableCollectorDoesNotPanic(t *testing.T) {
	w := New(Config{URL: "http://127.0.0.1:1"}, func() (bool, string) { return true, "" })
	w.Ping(context.Background())
}

// Package watchdog is the B-layer of fail-loud (SPEC-SAFETY §3.3): a dead-man
// switch held by somebody else.
//
// The A-layer heartbeat is published BY this server, so it can only report
// problems this server is alive to notice. If the process dies, the machine
// loses power, or the house loses its uplink, nothing is left to raise a hand —
// the alert clients simply go quiet, and quiet is indistinguishable from "no
// emergencies today". That is the failure this layer exists to catch.
//
// The mechanism is deliberately inverted: instead of asking an external service
// to poll us (which would need an inbound path into the home), we ping it while
// we are healthy. Silence is the alarm. A monitor that shares fate with the
// thing it monitors is not a monitor, so this ping must go to a third party
// (healthchecks.io, a Cloudflare Worker, an uptime service) — see docs.
package watchdog

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// HealthFunc reports the server's own verdict on itself; it is the same
// self-check that feeds the signed heartbeat.
type HealthFunc func() (healthy bool, reason string)

type Config struct {
	// URL is the dead-man switch endpoint. Empty disables the watchdog.
	URL string
	// Interval must be comfortably shorter than the grace period configured at
	// the far end, so one lost ping does not raise a false alarm.
	Interval time.Duration
}

type Watchdog struct {
	cfg    Config
	health HealthFunc
	http   *http.Client
}

func New(cfg Config, health HealthFunc) *Watchdog {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	return &Watchdog{
		cfg:    cfg,
		health: health,
		// Shorter than the interval: a hung request must never stall the next ping.
		http: &http.Client{Timeout: 20 * time.Second},
	}
}

func (w *Watchdog) Enabled() bool { return w != nil && w.cfg.URL != "" }

// Run pings until ctx is done. It pings immediately on start so a restart is
// visible at the far end without waiting a whole interval.
func (w *Watchdog) Run(ctx context.Context) {
	if !w.Enabled() {
		// Say so plainly: a self-hoster who believes they have an external
		// watchdog and does not is worse off than one who knows they do not.
		slog.Warn("external watchdog NOT configured — nothing outside this host will notice if it dies (SPEC-SAFETY §3.3)")
		return
	}
	slog.Info("external watchdog started", "url", w.cfg.URL, "interval", w.cfg.Interval.String())
	w.ping(ctx)
	t := time.NewTicker(w.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.ping(ctx)
		}
	}
}

// Ping performs one cycle, exposed for tests.
func (w *Watchdog) Ping(ctx context.Context) { w.ping(ctx) }

func (w *Watchdog) ping(ctx context.Context) {
	healthy, reason := true, ""
	if w.health != nil {
		healthy, reason = w.health()
	}
	url := w.cfg.URL
	if !healthy {
		// healthchecks.io convention: /fail signals a known-bad state explicitly,
		// which alerts immediately instead of waiting out the grace period.
		url += "/fail"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Error("watchdog ping build failed", "err", err)
		return
	}
	resp, err := w.http.Do(req)
	if err != nil {
		// Losing the ping is itself meaningful: from the far end this is
		// indistinguishable from the host being dead, which is the correct
		// outcome — but it should be visible locally too.
		slog.Error("watchdog ping failed", "err", err, "healthy", healthy)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		slog.Error("watchdog ping rejected", "status", resp.StatusCode)
		return
	}
	if !healthy {
		slog.Warn("watchdog signalled FAIL to the external switch", "reason", reason)
	}
}

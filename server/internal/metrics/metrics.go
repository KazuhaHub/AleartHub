// Package metrics exposes Prometheus instrumentation for the observability story
// (ARCHITECTURE §6). Served at /metrics; paired with /healthz + /readyz.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	AlertsPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "alerthub_alerts_published_total",
		Help: "Alerts published, by severity/category/source.",
	}, []string{"severity", "category", "source"})

	CapIngest = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "alerthub_cap_ingest_total",
		Help: "CAP ingest requests, by result (ok|error).",
	}, []string{"result"})

	Logins = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "alerthub_auth_logins_total",
		Help: "Admin logins, by method (password|passkey|2fa) and result.",
	}, []string{"method", "result"})

	// Labelled by the server's self-reported health so a degraded run is visible
	// in Prometheus, not just to the browser clients watching the beat.
	Heartbeats = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "alerthub_heartbeats_total",
		Help: "FAIL-LOUD heartbeats published, by self-reported health.",
	}, []string{"health"})

	// Durable delivery pipeline (transactional outbox + worker).
	DeliveryEnqueued = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "alerthub_delivery_enqueued_total",
		Help: "Delivery jobs enqueued, by channel.",
	}, []string{"channel"})

	DeliveryAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "alerthub_delivery_attempts_total",
		Help: "Delivery attempts, by channel and result (sent|retry|dead).",
	}, []string{"channel", "result"})
)

func Handler() http.Handler { return promhttp.Handler() }

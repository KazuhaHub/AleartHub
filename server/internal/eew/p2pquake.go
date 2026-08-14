package eew

// Second EEW source: P2P地震情報 (p2pquake.net). SPEC-SAFETY §6.1 asks for two
// independent relays because both are free third-party services fronting JMA,
// and a large quake is exactly when a free relay gets overwhelmed — i.e. the
// moment it is least allowed to fail. Two relays do not make this authoritative
// (carrier cell broadcast still is), they make it less likely to be silent.

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

const P2PQuakeURL = "wss://api.p2pquake.net/v2/ws"

// p2pMsg is the subset of the P2PQuake v2 JMA-EEW payload we use (code 556).
type p2pMsg struct {
	Code  int `json:"code"`
	Issue struct {
		EventID string `json:"eventId"`
		Serial  string `json:"serial"`
		Type    string `json:"type"`
	} `json:"issue"`
	Earthquake struct {
		Hypocenter struct {
			Name      string  `json:"name"`
			Magnitude float64 `json:"magnitude"`
		} `json:"hypocenter"`
		// maxScale is JMA intensity ×10 (45 = 5弱, 50 = 5強, 55 = 6弱 …).
		MaxScale int `json:"maxScale"`
	} `json:"earthquake"`
	Cancelled bool `json:"cancelled"`
	Test      bool `json:"test"`
}

// scaleToIntensity converts P2PQuake's ×10 scale to the JMA notation the shared
// severity mapper already understands, so both sources collapse identically.
func scaleToIntensity(scale int) string {
	switch {
	case scale >= 70:
		return "7"
	case scale >= 60:
		return "6+"
	case scale >= 55:
		return "6-"
	case scale >= 50:
		return "5+"
	case scale >= 45:
		return "5-"
	case scale >= 40:
		return "4"
	case scale >= 30:
		return "3"
	case scale >= 20:
		return "2"
	case scale >= 10:
		return "1"
	}
	return ""
}

// MapP2PQuake normalizes one P2PQuake frame. ok=false for frames that are not
// an EEW we can act on.
func MapP2PQuake(data []byte) (Event, bool) {
	var m p2pMsg
	if err := json.Unmarshal(data, &m); err != nil {
		return Event{}, false
	}
	// 556 is the JMA EEW channel; everything else on this socket is quake
	// reports, tsunami info, or peer chatter.
	if m.Code != 556 || m.Issue.EventID == "" {
		return Event{}, false
	}
	serial, _ := strconv.Atoi(m.Issue.Serial)
	e := Event{
		EventID:  m.Issue.EventID,
		Serial:   serial,
		IsCancel: m.Cancelled,
		IsTest:   m.Test,
	}
	intensity := scaleToIntensity(m.Earthquake.MaxScale)
	e.Severity = severityForIntensity(intensity)
	if e.IsTest && rank(e.Severity) > rank("warning") {
		e.Severity = "warning" // a drill must never fire a real fullscreen alarm
	}
	place := oneline(m.Earthquake.Hypocenter.Name)
	if place == "" {
		place = "震源不明"
	}
	e.Title = "緊急地震速報（" + place + "）"
	body := "最大震度 " + intensity
	if m.Earthquake.Hypocenter.Magnitude > 0 {
		body += " · M" + strconv.FormatFloat(m.Earthquake.Hypocenter.Magnitude, 'f', 1, 64)
	}
	e.Body = oneline(body)
	e.Action = "身の安全を確保 / 趴下，掩护，抓牢"
	return e, true
}

// RunP2PQuake maintains the P2PQuake WS connection and emits deduped events.
// It shares the Deduper with the other source so one earthquake reported by both
// relays produces exactly one alert.
func RunP2PQuake(ctx context.Context, d *Deduper, emit func(Event)) {
	runFeed(ctx, "p2pquake", P2PQuakeURL, MapP2PQuake, d, emit)
}

// runFeed is the shared reconnect/read/dedup loop behind both sources.
func runFeed(ctx context.Context, name, url string, mapFn func([]byte) (Event, bool),
	d *Deduper, emit func(Event)) {
	backoff := time.Second
	for ctx.Err() == nil {
		conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
		if err != nil {
			slog.Error("eew dial failed", "source", name, "err", err)
			sleepBackoff(ctx, &backoff)
			continue
		}
		slog.Info("eew source connected", "source", name, "url", url)
		backoff = time.Second
		for ctx.Err() == nil {
			_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
			_, data, err := conn.ReadMessage()
			if err != nil {
				break
			}
			ev, ok := mapFn(data)
			if !ok {
				continue
			}
			if ev.IsCancel {
				d.Forget(ev.EventID)
				emit(ev)
				continue
			}
			// The dedup is SHARED across sources: whichever relay reports first
			// wins, and the slower one is suppressed. Without this, dual-source
			// would double-alarm on every quake.
			if !d.FirstSeen(ev.EventID) {
				continue
			}
			slog.Info("eew event", "source", name, "event_id", ev.EventID,
				"severity", ev.Severity, "serial", ev.Serial)
			emit(ev)
		}
		conn.Close()
		sleepBackoff(ctx, &backoff)
	}
}

// Package eew ingests Japan earthquake early warning from third-party relays and
// maps it to AlertHub alerts (SPEC-SAFETY §6). COMPLEMENT only — carrier cell
// broadcast (緊急地震速報) is faster and authoritative; this covers desktops and
// unifies sources. Primary feed: Wolfx WS (relays JMA 予報 + 警報).
package eew

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"
)

const WolfxURL = "wss://ws-api.wolfx.jp/jma_eew"

// wolfxMsg is the subset of the Wolfx jma_eew payload we use. Note Wolfx spells
// magnitude "Magunitude".
type wolfxMsg struct {
	Type         string `json:"type"`
	Title        string `json:"Title"`
	EventID      string `json:"EventID"`
	Serial       int    `json:"Serial"`
	Hypocenter   string `json:"Hypocenter"`
	Magnitude    string `json:"Magunitude"`
	MaxIntensity string `json:"MaxIntensity"`
	OriginTime   string `json:"OriginTime"`
	IsWarn       bool   `json:"isWarn"`
	IsFinal      bool   `json:"isFinal"`
	IsCancel     bool   `json:"isCancel"`
	IsTraining   bool   `json:"isTraining"`
}

// Event is the normalized EEW the publisher turns into an alert.Alert.
type Event struct {
	EventID  string
	Serial   int
	Severity string
	Title    string
	Body     string
	Action   string
	IsCancel bool
	IsTest   bool
	// IsUpgrade marks a report whose intensity was revised upward for an event
	// already published. It is re-issued under the same alert id, so SPEC §5.2
	// makes the client present it again rather than extend it quietly.
	IsUpgrade bool
}

func rank(s string) int {
	return map[string]int{"notice": 0, "warning": 1, "critical": 2, "emergency": 3}[s]
}

// severityForIntensity maps JMA max intensity to AlertHub severity (SPEC-SAFETY §6):
// ≤3 → warning, 4/5弱/5強 → critical, 6弱+ → emergency.
func severityForIntensity(maxInt string) string {
	switch maxInt {
	case "6-", "6+", "7":
		return "emergency"
	case "5-", "5+", "4":
		return "critical"
	default:
		return "warning"
	}
}

// MapWolfx normalizes a Wolfx message; ok=false for heartbeats/non-EEW.
func MapWolfx(data []byte) (Event, bool) {
	var m wolfxMsg
	if json.Unmarshal(data, &m) != nil {
		return Event{}, false
	}
	if m.EventID == "" || (m.Type != "" && m.Type != "jma_eew") {
		return Event{}, false // heartbeat / ping / unrelated
	}
	e := Event{EventID: m.EventID, Serial: m.Serial, IsCancel: m.IsCancel, IsTest: m.IsTraining}
	e.Severity = severityForIntensity(m.MaxIntensity)
	if e.IsTest && rank(e.Severity) > rank("warning") {
		e.Severity = "warning" // training must never fire a real alarm
	}
	title := m.Title
	if title == "" {
		title = "緊急地震速報"
	}
	if m.MaxIntensity != "" {
		title += " 最大震度" + m.MaxIntensity
	}
	e.Title = oneline(title)
	body := m.Hypocenter
	if m.Magnitude != "" {
		body += " M" + m.Magnitude
	}
	e.Body = oneline(body)
	e.Action = "頭を守り、低く・動かず・身を守る"
	return e, true
}

func oneline(s string) string {
	return strings.TrimSpace(strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(s))
}

// RunWolfx maintains the Wolfx WS connection (reconnect+backoff) and emits
// deduped events, sharing the Deduper with any other source.
func RunWolfx(ctx context.Context, d *Deduper, emit func(Event)) {
	runFeed(ctx, "wolfx", WolfxURL, MapWolfx, d, emit)
}

// Source names accepted by Run.
const (
	SourceWolfx    = "wolfx"
	SourceP2PQuake = "p2pquake"
)

// Run starts every named source against ONE shared deduper, which is what makes
// dual-source redundancy rather than double-alarming: either relay can be down
// or slow and the other still delivers, but a quake both of them report still
// produces a single alert (SPEC-SAFETY §6.1).
func Run(ctx context.Context, sources []string, emit func(Event)) {
	d := NewDeduper()
	started := 0
	for _, name := range sources {
		switch name {
		case SourceWolfx:
			go RunWolfx(ctx, d, emit)
			started++
		case SourceP2PQuake:
			go RunP2PQuake(ctx, d, emit)
			started++
		default:
			slog.Error("unknown EEW source, ignoring", "source", name)
		}
	}
	if started == 1 {
		// Say it out loud: one relay is a single point of failure on the path that
		// matters most, and §6.1 asks for two.
		slog.Warn("only ONE EEW source configured — §6.1 asks for two independent relays")
	}
}

func sleepBackoff(ctx context.Context, b *time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(*b):
	}
	if *b *= 2; *b > 30*time.Second {
		*b = 30 * time.Second
	}
}

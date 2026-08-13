// Package eew ingests Japan earthquake early warning from third-party relays and
// maps it to AlertHub alerts (SPEC-SAFETY §6). COMPLEMENT only — carrier cell
// broadcast (緊急地震速報) is faster and authoritative; this covers desktops and
// unifies sources. Primary feed: Wolfx WS (relays JMA 予報 + 警報).
package eew

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
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

// RunWolfx maintains the Wolfx WS connection (reconnect+backoff) and calls emit
// for each new event (deduped once per EventID; cancels always emitted).
func RunWolfx(ctx context.Context, emit func(Event)) {
	seen := map[string]bool{}
	backoff := time.Second
	for ctx.Err() == nil {
		conn, _, err := websocket.DefaultDialer.DialContext(ctx, WolfxURL, nil)
		if err != nil {
			log.Printf("eew: Wolfx dial: %v", err)
			sleepBackoff(ctx, &backoff)
			continue
		}
		log.Printf("eew: connected to Wolfx (%s)", WolfxURL)
		backoff = time.Second
		for ctx.Err() == nil {
			_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
			_, data, err := conn.ReadMessage()
			if err != nil {
				break
			}
			ev, ok := MapWolfx(data)
			if !ok {
				continue
			}
			if ev.IsCancel {
				delete(seen, ev.EventID)
				emit(ev)
				continue
			}
			if seen[ev.EventID] {
				continue // once per event (MVP; escalation/renewal is a later refinement)
			}
			seen[ev.EventID] = true
			emit(ev)
		}
		conn.Close()
		sleepBackoff(ctx, &backoff)
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

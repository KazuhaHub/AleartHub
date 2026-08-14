package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Presence is a device's last-known online state, tracked from status/<deviceId>
// (LWT + birth, SPEC §5.4). Drives the Dashboard "device roster".
type Presence struct {
	DeviceID string `json:"device_id"`
	State    string `json:"state"` // online | offline
	At       int64  `json:"at"`
	Client   string `json:"client"`
	LastSeen int64  `json:"last_seen"`
}

// OnPresence is the inline-subscription handler for status/#.
func (s *Server) OnPresence(topic string, payload []byte) {
	deviceID := strings.TrimPrefix(topic, "status/")
	if deviceID == "" || deviceID == topic {
		return
	}
	var p Presence
	if len(payload) > 0 {
		// A payload we cannot parse must NOT mutate the roster. Discarding the
		// error here used to store a zero-value Presence with an empty state,
		// which OVERWROTE the device's real status — and since the broker ACL
		// lets a device write its own status/<id>, a buggy or compromised client
		// could blank itself out of the "who can receive an alert" roster.
		if err := json.Unmarshal(payload, &p); err != nil {
			return
		}
		if p.State != "online" && p.State != "offline" {
			return // unrecognised state is as untrustworthy as unparseable bytes
		}
	} else {
		p.State = "offline" // empty retained = cleared
	}
	p.DeviceID = deviceID
	p.LastSeen = time.Now().Unix()

	s.presenceMu.Lock()
	if s.presence == nil {
		s.presence = map[string]Presence{}
	}
	s.presence[deviceID] = p
	s.presenceMu.Unlock()
}

// GET /api/devices — the device roster (online/offline + last seen).
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	s.presenceMu.Lock()
	out := make([]Presence, 0, len(s.presence))
	for _, p := range s.presence {
		out = append(out, p)
	}
	s.presenceMu.Unlock()
	writeJSON(w, out)
}

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
		_ = json.Unmarshal(payload, &p)
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

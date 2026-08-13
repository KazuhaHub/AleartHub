package passkey

import (
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// TestPutSession_SweepsExpired verifies the leak fix: abandoned begin-sessions
// (never finished) are swept on the next put, so the map can't grow unbounded.
func TestPutSession_SweepsExpired(t *testing.T) {
	s := &Service{sessions: map[string]sessionEntry{}}
	s.sessions["stale"] = sessionEntry{expires: time.Now().Add(-time.Minute)}

	id := s.putSession(webauthn.SessionData{}, 0) // triggers the sweep

	s.mu.Lock()
	_, stale := s.sessions["stale"]
	_, fresh := s.sessions[id]
	s.mu.Unlock()
	if stale {
		t.Fatal("putSession must sweep expired sessions to avoid a memory leak")
	}
	if !fresh {
		t.Fatal("putSession must keep the freshly-created session")
	}
}

// TestTakeSession_RejectsExpired confirms an expired session is not accepted even
// if it is still in the map (defense against a slow sweep).
func TestTakeSession_RejectsExpired(t *testing.T) {
	s := &Service{sessions: map[string]sessionEntry{}}
	s.sessions["e"] = sessionEntry{expires: time.Now().Add(-time.Second)}
	if _, ok := s.takeSession("e"); ok {
		t.Fatal("takeSession must reject an expired session")
	}
}

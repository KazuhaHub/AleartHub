package api

import (
	"net/http"
	"testing"
	"time"
)

func TestRateLimiter_AllowsUpToMaxThenBlocks(t *testing.T) {
	rl := newRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.allow("1.2.3.4") {
			t.Fatalf("hit %d should be allowed", i+1)
		}
	}
	if rl.allow("1.2.3.4") {
		t.Fatal("4th hit must be blocked")
	}
	if !rl.allow("5.6.7.8") {
		t.Fatal("a different key must have its own budget")
	}
}

func TestRateLimiter_WindowReset(t *testing.T) {
	now := time.Unix(1000, 0)
	rl := newRateLimiter(1, time.Minute)
	rl.now = func() time.Time { return now }
	if !rl.allow("k") {
		t.Fatal("first hit allowed")
	}
	if rl.allow("k") {
		t.Fatal("second hit blocked in same window")
	}
	now = now.Add(61 * time.Second)
	if !rl.allow("k") {
		t.Fatal("allowed again after the window elapsed")
	}
}

func TestRateLimiter_SweepsExpiredWindows(t *testing.T) {
	now := time.Unix(1000, 0)
	rl := newRateLimiter(5, time.Minute)
	rl.now = func() time.Time { return now }
	rl.allow("old")
	now = now.Add(2 * time.Minute) // "old" window is now expired
	rl.allow("new")                // any allow() sweeps expired windows
	rl.mu.Lock()
	_, oldPresent := rl.hits["old"]
	rl.mu.Unlock()
	if oldPresent {
		t.Fatal("expired window must be swept so the map stays bounded")
	}
}

// TestRateLimit_LoginReturns429 drives the real endpoint: httptest requests share
// a fixed RemoteAddr, so after 10 attempts the same IP is throttled.
func TestRateLimit_LoginReturns429(t *testing.T) {
	ts := newTestServer(t)
	body := loginReq{UPN: "nobody", Password: "wrong"}
	got429 := false
	for i := 0; i < 12; i++ {
		w := ts.req(t, http.MethodPost, "/api/auth/login", body, nil)
		if w.Code == http.StatusTooManyRequests {
			got429 = true
			if w.Header().Get("Retry-After") == "" {
				t.Error("429 response should carry a Retry-After header")
			}
			break
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: want 401 (bad creds) before throttling, got %d", i+1, w.Code)
		}
	}
	if !got429 {
		t.Fatal("login must return 429 after exceeding the per-IP attempt limit")
	}
}

func TestPutSSOCode_SweepsExpired(t *testing.T) {
	ssoMu.Lock()
	ssoCodes["stale"] = ssoPending{exp: time.Now().Add(-time.Minute)}
	ssoMu.Unlock()

	code := putSSOCode("a", "r", userDTO{}) // triggers the sweep

	ssoMu.Lock()
	_, stale := ssoCodes["stale"]
	_, fresh := ssoCodes[code]
	ssoMu.Unlock()
	if stale {
		t.Fatal("putSSOCode must sweep expired (never-exchanged) codes")
	}
	if !fresh {
		t.Fatal("putSSOCode must keep the freshly-issued code")
	}
}

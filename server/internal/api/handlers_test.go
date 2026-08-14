package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kazuha/alerthub/server/internal/alert"
	"github.com/kazuha/alerthub/server/internal/auth"
	"github.com/kazuha/alerthub/server/internal/store"
)

// Route-level coverage for the handlers that carry security or tenancy meaning.
// api_test.go covers the alert plane; this covers auth, orgs, service accounts,
// the API-key scope gate, presence and the TTL sweeper.

// --- auth surface ----------------------------------------------------------

func TestAuthMethods_ReportsWhatIsWired(t *testing.T) {
	ts := newTestServer(t)
	w := ts.req(t, http.MethodGet, "/api/auth/methods", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("methods = %d, want 200 (public)", w.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m["local"] != true {
		t.Error("local login should always be advertised")
	}
	// SSO is nil in the test server, so it must not be advertised as available.
	for _, k := range []string{"sso", "oidc", "saml"} {
		if m[k] != false {
			t.Errorf("%s = %v, want false when not configured", k, m[k])
		}
	}
}

func TestMe_AdminTokenVsSession(t *testing.T) {
	ts := newTestServer(t)
	// admin token has no JWT claims — the handler reports the synthetic identity.
	w := ts.req(t, http.MethodGet, "/api/auth/me", nil, adminHdr())
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "admin-token") {
		t.Fatalf("me(admin token) = %d %s", w.Code, w.Body.String())
	}
	// a real session resolves the actual user
	access := ts.seedUser(t, "me@x", "viewer")
	w = ts.req(t, http.MethodGet, "/api/auth/me", nil, userHdr(access))
	if w.Code != http.StatusOK {
		t.Fatalf("me(session) = %d", w.Code)
	}
	var u userDTO
	_ = json.Unmarshal(w.Body.Bytes(), &u)
	if u.UPN != "me@x" {
		t.Fatalf("me = %+v, want upn me@x", u)
	}
	if w := ts.req(t, http.MethodGet, "/api/auth/me", nil, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth me = %d, want 401", w.Code)
	}
}

func TestLogout_ClearsCookie(t *testing.T) {
	ts := newTestServer(t)
	w := ts.req(t, http.MethodPost, "/api/auth/logout", nil, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", w.Code)
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), "ah_access=") {
		t.Error("logout must clear the ah_access cookie")
	}
}

func TestLogin_Succeeds(t *testing.T) {
	ts := newTestServer(t)
	hash, _ := auth.HashPassword("s3cret")
	uid, err := ts.srv.Store.CreateUser(&store.User{UPN: "login@x", PasswordHash: hash, Role: auth.RoleAdmin, Enabled: true})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	_ = ts.srv.Store.AddMembership(ts.srv.DefaultOrgID, uid, "owner")

	w := ts.req(t, http.MethodPost, "/api/auth/login", loginReq{UPN: "login@x", Password: "s3cret"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login = %d; body=%s", w.Code, w.Body.String())
	}
	var got tokenResp
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AccessToken == "" || got.RefreshToken == "" || got.User.UPN != "login@x" {
		t.Fatalf("login response incomplete: %+v", got)
	}
	if w := ts.req(t, http.MethodPost, "/api/auth/login",
		loginReq{UPN: "login@x", Password: "wrong"}, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", w.Code)
	}
}

// TestRefresh_HonoursRevocation is the important half of refresh: bumping a
// user's TokenVersion must invalidate refresh tokens too, not just access ones.
func TestRefresh_HonoursRevocation(t *testing.T) {
	ts := newTestServer(t)
	uid, _ := ts.srv.Store.CreateUser(&store.User{UPN: "r@x", Role: auth.RoleUser, Enabled: true})
	_, refresh, _ := ts.srv.Auth.IssueTokens(uid, "r@x", auth.RoleUser, 0)

	w := ts.req(t, http.MethodPost, "/api/auth/refresh", refreshReq{RefreshToken: refresh}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("refresh = %d; body=%s", w.Code, w.Body.String())
	}
	if err := ts.srv.Store.BumpTokenVersion(uid); err != nil {
		t.Fatalf("bump: %v", err)
	}
	if w := ts.req(t, http.MethodPost, "/api/auth/refresh",
		refreshReq{RefreshToken: refresh}, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked refresh = %d, want 401", w.Code)
	}
	// An access token must not be usable as a refresh token (subject is checked).
	access, _, _ := ts.srv.Auth.IssueTokens(uid, "r@x", auth.RoleUser, 1)
	if w := ts.req(t, http.MethodPost, "/api/auth/refresh",
		refreshReq{RefreshToken: access}, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("access-as-refresh = %d, want 401", w.Code)
	}
}

func TestRefresh_RejectsDisabledUser(t *testing.T) {
	ts := newTestServer(t)
	uid, _ := ts.srv.Store.CreateUser(&store.User{UPN: "d@x", Role: auth.RoleUser, Enabled: false})
	_, refresh, _ := ts.srv.Auth.IssueTokens(uid, "d@x", auth.RoleUser, 0)
	if w := ts.req(t, http.MethodPost, "/api/auth/refresh",
		refreshReq{RefreshToken: refresh}, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user refresh = %d, want 401", w.Code)
	}
}

func Test2FAStatus_DefaultsOff(t *testing.T) {
	ts := newTestServer(t)
	access := ts.seedUser(t, "tf@x", "viewer")
	w := ts.req(t, http.MethodGet, "/api/auth/2fa/status", nil, userHdr(access))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "false") {
		t.Fatalf("2fa status = %d %s, want enabled:false", w.Code, w.Body.String())
	}
	if w := ts.req(t, http.MethodGet, "/api/auth/2fa/status", nil, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth 2fa status = %d, want 401", w.Code)
	}
}

func Test2FAVerify_RejectsBadPendingToken(t *testing.T) {
	ts := newTestServer(t)
	w := ts.req(t, http.MethodPost, "/api/auth/2fa/verify",
		verify2FAReq{PendingToken: "garbage", Code: "000000"}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad pending token = %d, want 401", w.Code)
	}
}

// --- SSO endpoints are 404 while disabled ----------------------------------

func TestSSOEndpoints_404WhenDisabled(t *testing.T) {
	ts := newTestServer(t)
	for _, p := range []string{
		"/api/auth/oidc/login", "/api/auth/oidc/callback",
		"/api/auth/saml/login", "/api/auth/saml/metadata",
	} {
		if w := ts.req(t, http.MethodGet, p, nil, nil); w.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404 while SSO is unconfigured", p, w.Code)
		}
	}
}

func TestOIDCExchange_RejectsUnknownCode(t *testing.T) {
	ts := newTestServer(t)
	w := ts.req(t, http.MethodPost, "/api/auth/oidc/exchange", map[string]string{"code": "nope"}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown sso code = %d, want 401", w.Code)
	}
}

// --- orgs ------------------------------------------------------------------

func TestOrgs_ListAndCreate(t *testing.T) {
	ts := newTestServer(t)
	// admin token counts as super-admin → sees all orgs and may create.
	w := ts.req(t, http.MethodGet, "/api/orgs", nil, adminHdr())
	if w.Code != http.StatusOK {
		t.Fatalf("list orgs = %d", w.Code)
	}
	var orgs []orgDTO
	_ = json.Unmarshal(w.Body.Bytes(), &orgs)
	if len(orgs) != 1 || orgs[0].Slug != "default" {
		t.Fatalf("orgs = %+v, want just default", orgs)
	}

	w = ts.req(t, http.MethodPost, "/api/orgs", map[string]string{"slug": "acme", "name": "Acme"}, adminHdr())
	if w.Code != http.StatusOK {
		t.Fatalf("create org = %d; body=%s", w.Code, w.Body.String())
	}
	// slug is required, and duplicates are rejected.
	if w := ts.req(t, http.MethodPost, "/api/orgs", map[string]string{"name": "x"}, adminHdr()); w.Code != http.StatusBadRequest {
		t.Errorf("missing slug = %d, want 400", w.Code)
	}
	if w := ts.req(t, http.MethodPost, "/api/orgs", map[string]string{"slug": "acme"}, adminHdr()); w.Code != http.StatusBadRequest {
		t.Errorf("duplicate slug = %d, want 400", w.Code)
	}
}

// TestOrgs_NonSuperSeesOnlyItsOwn is the tenancy property: a plain member must
// not be able to enumerate other tenants, nor create new ones.
func TestOrgs_NonSuperSeesOnlyItsOwn(t *testing.T) {
	ts := newTestServer(t)
	other, err := ts.srv.Store.CreateOrg("other", "Other")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	access := ts.seedUser(t, "member@x", "viewer") // member of the default org only

	w := ts.req(t, http.MethodGet, "/api/orgs", nil, userHdr(access))
	if w.Code != http.StatusOK {
		t.Fatalf("list = %d", w.Code)
	}
	var orgs []orgDTO
	_ = json.Unmarshal(w.Body.Bytes(), &orgs)
	if len(orgs) != 1 || orgs[0].ID == other {
		t.Fatalf("SECURITY: non-super saw %+v; must see only its own memberships", orgs)
	}
	if w := ts.req(t, http.MethodPost, "/api/orgs", map[string]string{"slug": "sneaky"}, userHdr(access)); w.Code != http.StatusForbidden {
		t.Fatalf("non-super create = %d, want 403", w.Code)
	}
}

// --- service accounts + the API-key scope gate ------------------------------

func TestServiceAccounts_CreateListDelete(t *testing.T) {
	ts := newTestServer(t)
	w := ts.req(t, http.MethodPost, "/api/admin/service-accounts",
		map[string]any{"name": "eew-feed"}, adminHdr())
	if w.Code != http.StatusOK {
		t.Fatalf("create sa = %d; body=%s", w.Code, w.Body.String())
	}
	var created struct {
		ID     int64    `json:"id"`
		Token  string   `json:"token"`
		Scopes []string `json:"scopes"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if !strings.HasPrefix(created.Token, apiKeyPrefix) {
		t.Fatalf("token %q must carry the %q prefix", created.Token, apiKeyPrefix)
	}
	if len(created.Scopes) != 1 || created.Scopes[0] != "alerts:ingest" {
		t.Errorf("default scope = %v, want [alerts:ingest]", created.Scopes)
	}

	// The raw token is shown once and never again.
	w = ts.req(t, http.MethodGet, "/api/admin/service-accounts", nil, adminHdr())
	if w.Code != http.StatusOK {
		t.Fatalf("list sa = %d", w.Code)
	}
	if strings.Contains(w.Body.String(), created.Token) {
		t.Fatal("SECURITY: the raw API key must never be returned by the list endpoint")
	}

	if w := ts.req(t, http.MethodPost, "/api/admin/service-accounts",
		map[string]any{"name": ""}, adminHdr()); w.Code != http.StatusBadRequest {
		t.Errorf("empty name = %d, want 400", w.Code)
	}

	w = ts.req(t, http.MethodPost, "/api/admin/service-accounts/delete",
		map[string]int64{"id": created.ID}, adminHdr())
	if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
		t.Fatalf("delete sa = %d", w.Code)
	}
}

func TestServiceAccounts_RequiresPermission(t *testing.T) {
	ts := newTestServer(t)
	viewer := ts.seedUser(t, "v@x", "viewer") // viewer lacks sa:manage
	if w := ts.req(t, http.MethodGet, "/api/admin/service-accounts", nil, userHdr(viewer)); w.Code != http.StatusForbidden {
		t.Fatalf("viewer list sa = %d, want 403", w.Code)
	}
	if w := ts.req(t, http.MethodGet, "/api/admin/service-accounts", nil, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth list sa = %d, want 401", w.Code)
	}
}

// TestAPIKeyScopeGate drives /api/cap with a real service-account key: the right
// scope is accepted, a key without it is rejected, and a bogus key is rejected.
func TestAPIKeyScopeGate(t *testing.T) {
	ts := newTestServer(t)
	mkKey := func(scopes []string) string {
		w := ts.req(t, http.MethodPost, "/api/admin/service-accounts",
			map[string]any{"name": "k", "scopes": scopes}, adminHdr())
		if w.Code != http.StatusOK {
			t.Fatalf("create sa = %d; body=%s", w.Code, w.Body.String())
		}
		var got struct {
			Token string `json:"token"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		return got.Token
	}
	good := mkKey([]string{"alerts:ingest"})
	wrong := mkKey([]string{"something:else"})

	post := func(tok string) int {
		rr := httptest.NewRequest(http.MethodPost, "/api/cap", strings.NewReader(capAlertXML))
		rr.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		ts.handler.ServeHTTP(w, rr)
		return w.Code
	}
	if code := post(good); code != http.StatusOK {
		t.Errorf("key with alerts:ingest = %d, want 200", code)
	}
	if code := post(wrong); code != http.StatusForbidden {
		t.Errorf("key without the scope = %d, want 403", code)
	}
	if code := post(apiKeyPrefix + "totally-bogus"); code != http.StatusForbidden {
		t.Errorf("unknown key = %d, want 403", code)
	}
}

func TestScopeAllowed(t *testing.T) {
	cases := []struct {
		scopes, want string
		ok           bool
	}{
		{"alerts:ingest", "alerts:ingest", true},
		{"a,alerts:ingest,b", "alerts:ingest", true},
		{" alerts:ingest ", "alerts:ingest", true},
		{"*", "anything", true},
		{"other", "alerts:ingest", false},
		{"", "alerts:ingest", false},
	}
	for _, c := range cases {
		if got := scopeAllowed(c.scopes, c.want); got != c.ok {
			t.Errorf("scopeAllowed(%q,%q) = %v, want %v", c.scopes, c.want, got, c.ok)
		}
	}
}

func TestSplitScopes(t *testing.T) {
	got := splitScopes(" a , b ,, c ")
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("splitScopes = %#v", got)
	}
	if len(splitScopes("")) != 0 {
		t.Error("empty scopes must yield an empty slice, not a nil-ish surprise")
	}
}

// --- delivery stats + presence + sweeper ------------------------------------

func TestDeliveryStats(t *testing.T) {
	ts := newTestServer(t)
	if w := ts.req(t, http.MethodGet, "/api/delivery/stats", nil, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth delivery stats = %d, want 401", w.Code)
	}
	w := ts.req(t, http.MethodGet, "/api/delivery/stats", nil, adminHdr())
	if w.Code != http.StatusOK {
		t.Fatalf("delivery stats = %d", w.Code)
	}
	var got struct {
		Counts map[string]int       `json:"counts"`
		Dead   []store.DeadDelivery `json:"dead"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Dead == nil {
		t.Error("dead must serialize as [] not null, so the dashboard can map over it")
	}
}

// TestOnPresence_TracksRoster feeds the handler the retained status/<id> payloads
// the broker delivers and checks /api/devices reflects them.
func TestOnPresence_TracksRoster(t *testing.T) {
	ts := newTestServer(t)
	ts.srv.OnPresence("status/dev-1", []byte(`{"device_id":"dev-1","state":"online","at":1765238400,"client":"web/1.0"}`))
	ts.srv.OnPresence("status/dev-2", []byte(`{"device_id":"dev-2","state":"offline","at":1765238400,"client":"web/1.0"}`))
	// Neither garbage nor an unrecognised state may overwrite a good record —
	// a device may write its own status topic, so this is attacker-reachable.
	ts.srv.OnPresence("status/dev-1", []byte(`not json`))
	ts.srv.OnPresence("status/dev-1", []byte(`{"device_id":"dev-1","state":"weird"}`))

	w := ts.req(t, http.MethodGet, "/api/devices", nil, adminHdr())
	if w.Code != http.StatusOK {
		t.Fatalf("devices = %d", w.Code)
	}
	var devices []Presence
	if err := json.Unmarshal(w.Body.Bytes(), &devices); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("roster = %+v, want 2 devices", devices)
	}
	states := map[string]string{}
	for _, d := range devices {
		states[d.DeviceID] = d.State
	}
	if states["dev-1"] != "online" || states["dev-2"] != "offline" {
		t.Fatalf("states = %v", states)
	}
}

// TestSweeper_ClearsExpiredActive covers SPEC §5.2 TTL self-heal: once the active
// alert's TTL has passed, the retained slot is cleared so a reconnecting client
// does not resurrect a stale emergency.
func TestSweeper_ClearsExpiredActive(t *testing.T) {
	ts := newTestServer(t)
	a := &alert.Alert{
		SchemaVersion: alert.SchemaVersion, ID: "expired-1", Type: "alert",
		Category: "system", Severity: "critical", Title: "old",
		IssuedAt: 1, TTL: 1, // long past
	}
	ts.srv.mu.Lock()
	ts.srv.active = a
	ts.srv.mu.Unlock()

	ts.srv.sweepExpiredActive()

	ts.srv.mu.Lock()
	still := ts.srv.active
	ts.srv.mu.Unlock()
	if still != nil {
		t.Fatalf("sweeper left an expired alert in the active slot: %+v", still)
	}
}

// A live alert must survive the sweep.
func TestSweeper_KeepsLiveActive(t *testing.T) {
	ts := newTestServer(t)
	a := &alert.Alert{
		SchemaVersion: alert.SchemaVersion, ID: "live-1", Type: "alert",
		Category: "system", Severity: "critical", Title: "current",
		IssuedAt: time.Now().Unix(), TTL: 600,
	}
	ts.srv.mu.Lock()
	ts.srv.active = a
	ts.srv.mu.Unlock()

	ts.srv.sweepExpiredActive()

	ts.srv.mu.Lock()
	still := ts.srv.active
	ts.srv.mu.Unlock()
	if still == nil {
		t.Fatal("sweeper cleared an alert that is still within its TTL")
	}
}

// --- passkey: the paths reachable without a real authenticator --------------

func TestPasskey_RequiresSession(t *testing.T) {
	ts := newTestServer(t)
	for _, p := range []string{
		"/api/auth/passkey/register/begin",
		"/api/auth/passkey/register/finish",
		"/api/auth/passkey/list",
		"/api/auth/passkey/delete",
	} {
		if w := ts.req(t, http.MethodPost, p, map[string]any{}, nil); w.Code != http.StatusUnauthorized {
			t.Errorf("%s without a session = %d, want 401", p, w.Code)
		}
	}
}

// The static admin token authenticates but carries no user identity, so passkey
// registration — which must bind a credential to a specific user — has to refuse it.
func TestPasskeyRegister_RejectsAdminToken(t *testing.T) {
	ts := newTestServer(t)
	w := ts.req(t, http.MethodPost, "/api/auth/passkey/register/begin", nil, adminHdr())
	if w.Code != http.StatusBadRequest {
		t.Fatalf("register/begin with admin token = %d, want 400 (needs a real session)", w.Code)
	}
}

func TestPasskeyRegisterBegin_ReturnsOptions(t *testing.T) {
	ts := newTestServer(t)
	access := ts.seedUser(t, "pk@x", "viewer")
	w := ts.req(t, http.MethodPost, "/api/auth/passkey/register/begin", nil, userHdr(access))
	if w.Code != http.StatusOK {
		t.Fatalf("register/begin = %d; body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Options map[string]any `json:"options"`
		Session string         `json:"session"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Session == "" || got.Options == nil {
		t.Fatalf("register/begin must return both options and a session id: %+v", got)
	}
}

// Usernameless login begin is public by design — it must hand out a challenge
// without revealing whether any account exists.
func TestPasskeyLoginBegin_IsPublic(t *testing.T) {
	ts := newTestServer(t)
	w := ts.req(t, http.MethodPost, "/api/auth/passkey/login/begin", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login/begin = %d, want 200 (public)", w.Code)
	}
	var got struct {
		Session string `json:"session"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Session == "" {
		t.Fatal("login/begin must return a session id")
	}
}

func TestPasskeyLoginFinish_RejectsBogusSession(t *testing.T) {
	ts := newTestServer(t)
	w := ts.req(t, http.MethodPost, "/api/auth/passkey/login/finish?session=nope", map[string]any{}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("login/finish with an unknown session = %d, want 401", w.Code)
	}
}

func TestPasskeyList_EmptyForNewUser(t *testing.T) {
	ts := newTestServer(t)
	access := ts.seedUser(t, "pk2@x", "viewer")
	w := ts.req(t, http.MethodGet, "/api/auth/passkey/list", nil, userHdr(access))
	if w.Code != http.StatusOK {
		t.Fatalf("list = %d", w.Code)
	}
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("a new user's passkey list must serialize as [], got %s", w.Body.String())
	}
}

// --- 2FA enrollment start ---------------------------------------------------

func Test2FABegin_ReturnsEnrollmentSecret(t *testing.T) {
	ts := newTestServer(t)
	access := ts.seedUser(t, "totp@x", "viewer")
	w := ts.req(t, http.MethodPost, "/api/auth/2fa/begin", nil, userHdr(access))
	if w.Code != http.StatusOK {
		t.Fatalf("2fa/begin = %d; body=%s", w.Code, w.Body.String())
	}
	var got struct {
		URL    string `json:"otpauth_url"`
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(got.URL, "otpauth://") || got.Secret == "" {
		t.Fatalf("2fa/begin = %+v, want an otpauth:// URL and a secret", got)
	}
	// Enrolling is not the same as enabling: status must still report false until
	// a correct code is presented.
	s := ts.req(t, http.MethodGet, "/api/auth/2fa/status", nil, userHdr(access))
	if !strings.Contains(s.Body.String(), "false") {
		t.Fatalf("status after begin = %s, want enabled:false until confirmed", s.Body.String())
	}
}

func Test2FAEnable_RejectsWrongCode(t *testing.T) {
	ts := newTestServer(t)
	access := ts.seedUser(t, "totp2@x", "viewer")
	if w := ts.req(t, http.MethodPost, "/api/auth/2fa/begin", nil, userHdr(access)); w.Code != http.StatusOK {
		t.Fatalf("begin = %d", w.Code)
	}
	w := ts.req(t, http.MethodPost, "/api/auth/2fa/enable", codeReq{Code: "000000"}, userHdr(access))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("enable with a wrong code = %d, want 400", w.Code)
	}
}

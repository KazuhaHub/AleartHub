package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kazuha/alerthub/server/internal/alert"
	"github.com/kazuha/alerthub/server/internal/cap"
)

const capAlertXML = `<?xml version="1.0"?>
<alert xmlns="urn:oasis:names:tc:emergency:cap:1.2">
  <identifier>EQ-100</identifier><sender>jma</sender><sent>2026-06-14T14:23:01+09:00</sent>
  <status>Actual</status><msgType>Alert</msgType><scope>Public</scope>
  <info><category>Geo</category><event>Earthquake</event>
  <urgency>Immediate</urgency><severity>Severe</severity><certainty>Observed</certainty>
  <headline>強い揺れ</headline></info>
</alert>`

// capPost sends a raw CAP XML body to /api/cap with the admin token.
func (ts *testServer) capPost(t *testing.T, xml string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRequest(http.MethodPost, "/api/cap", strings.NewReader(xml))
	rr.Header.Set("Authorization", "Bearer "+testAdminToken)
	w := httptest.NewRecorder()
	ts.handler.ServeHTTP(w, rr)
	return w
}

func TestCAPIngest_DeterministicID(t *testing.T) {
	ts := newTestServer(t)
	w := ts.capPost(t, capAlertXML)
	if w.Code != http.StatusOK {
		t.Fatalf("cap ingest = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if want := cap.AlertID("jma", "EQ-100"); got["id"] != want {
		t.Fatalf("ingest id = %v, want deterministic %s (so Cancel can recall it)", got["id"], want)
	}
}

// TestCAPCancel_RecallsReferencedAlert is the 501→implemented proof: a CAP Cancel
// that <references> a prior alert recalls the exact alert we published, and emits a
// signed cancel envelope for it.
func TestCAPCancel_RecallsReferencedAlert(t *testing.T) {
	ts := newTestServer(t)
	if w := ts.capPost(t, capAlertXML); w.Code != http.StatusOK {
		t.Fatalf("ingest = %d; body=%s", w.Code, w.Body.String())
	}

	cancelXML := `<alert xmlns="urn:oasis:names:tc:emergency:cap:1.2">
	  <identifier>EQ-100-C</identifier><sender>jma</sender><sent>2026-06-14T14:30:00+09:00</sent>
	  <status>Actual</status><msgType>Cancel</msgType><scope>Public</scope>
	  <references>jma,EQ-100,2026-06-14T14:23:01+09:00</references>
	  <info><category>Geo</category><event>Earthquake</event>
	  <urgency>Past</urgency><severity>Minor</severity><certainty>Observed</certainty>
	  <headline>解除</headline></info>
	</alert>`
	w := ts.capPost(t, cancelXML)
	if w.Code != http.StatusOK {
		t.Fatalf("cap cancel = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Cancelled []string `json:"cancelled"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := cap.AlertID("jma", "EQ-100")
	if len(got.Cancelled) != 1 || got.Cancelled[0] != want {
		t.Fatalf("cancelled = %v, want [%s]", got.Cancelled, want)
	}

	// A signed cancel envelope for the original id must be recorded.
	hw := ts.req(t, http.MethodGet, "/api/history", nil, adminHdr())
	var hist []alert.Alert
	if err := json.Unmarshal(hw.Body.Bytes(), &hist); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	found := false
	for _, a := range hist {
		if a.Type == "cancel" && a.Cancels == want {
			if !alert.Verify(&a, ts.pub) {
				t.Fatal("cancel envelope must be signed and verify")
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a signed cancel with cancels=%s in history; got %+v", want, hist)
	}
}

func TestCAPCancel_NoReferences400(t *testing.T) {
	ts := newTestServer(t)
	cancelXML := `<alert xmlns="urn:oasis:names:tc:emergency:cap:1.2">
	  <identifier>C-1</identifier><sender>jma</sender><status>Actual</status><msgType>Cancel</msgType>
	  <info><category>Geo</category><event>x</event></info></alert>`
	if w := ts.capPost(t, cancelXML); w.Code != http.StatusBadRequest {
		t.Fatalf("cancel without <references> = %d, want 400", w.Code)
	}
}

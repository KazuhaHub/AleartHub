package cap_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kazuha/alerthub/server/internal/cap"
)

const eqXML = `<?xml version="1.0"?>
<alert xmlns="urn:oasis:names:tc:emergency:cap:1.2">
  <identifier>EQ-001</identifier><sender>jma</sender><sent>2026-06-14T14:23:01+09:00</sent>
  <status>Actual</status><msgType>Alert</msgType><scope>Public</scope>
  <info>
    <category>Geo</category><event>Earthquake Early Warning</event>
    <responseType>Shelter</responseType>
    <urgency>Immediate</urgency><severity>Severe</severity><certainty>Observed</certainty>
    <headline>緊急地震速報</headline>
    <description>強い揺れに警戒してください。</description>
    <instruction>頭を守り、机の下へ。</instruction>
  </info>
</alert>`

func TestMapEarthquakeEmergency(t *testing.T) {
	d, err := cap.Parse([]byte(eqXML))
	if err != nil {
		t.Fatal(err)
	}
	m := d.MapToAlert(time.Now())
	if m.Category != "earthquake" {
		t.Errorf("category = %q, want earthquake", m.Category)
	}
	if m.Severity != "emergency" {
		t.Errorf("severity = %q, want emergency (Immediate+Severe+Observed)", m.Severity)
	}
	if m.Title != "緊急地震速報" {
		t.Errorf("title = %q", m.Title)
	}
	if m.Action == "" {
		t.Error("expected an action from responseType Shelter")
	}
	if m.IsTest {
		t.Error("status Actual must not be a test")
	}
	if m.TTL <= 0 {
		t.Errorf("ttl = %d, want >0", m.TTL)
	}
}

const testXML = `<alert xmlns="urn:oasis:names:tc:emergency:cap:1.2">
  <identifier>EX-1</identifier><status>Exercise</status><msgType>Alert</msgType>
  <info><category>Fire</category><event>Drill</event>
  <urgency>Immediate</urgency><severity>Extreme</severity><certainty>Observed</certainty>
  <headline>Fire drill</headline></info></alert>`

func TestExerciseNeverRealAlarm(t *testing.T) {
	d, _ := cap.Parse([]byte(testXML))
	m := d.MapToAlert(time.Now())
	if !m.IsTest {
		t.Error("Exercise status must be flagged IsTest")
	}
	if m.Severity == "emergency" || m.Severity == "critical" {
		t.Errorf("a Test/Exercise must not map to %q (no real fullscreen alarm)", m.Severity)
	}
	if m.Category != "fire" {
		t.Errorf("category = %q, want fire", m.Category)
	}
}

func TestAlertID_DeterministicAndTopicSafe(t *testing.T) {
	a := cap.AlertID("jma", "EQ-1")
	if a != cap.AlertID("jma", "EQ-1") {
		t.Fatal("AlertID must be deterministic for the same (sender, identifier)")
	}
	if cap.AlertID("jma", "EQ-2") == a {
		t.Fatal("different identifiers must yield different ids")
	}
	if cap.AlertID("other", "EQ-1") == a {
		t.Fatal("different senders must yield different ids")
	}
	if !strings.HasPrefix(a, "cap-") {
		t.Fatalf("id %q must carry the cap- prefix", a)
	}
	if strings.ContainsAny(a, "/+# ") { // MQTT wildcards / separators / spaces
		t.Fatalf("id %q must be MQTT-topic-safe", a)
	}
}

func TestParseReferences(t *testing.T) {
	refs := cap.ParseReferences(
		"jma,EQ-1,2026-06-14T14:23:01+09:00 jma,EQ-2,2026-06-14T14:24:00+09:00")
	if len(refs) != 2 {
		t.Fatalf("want 2 refs, got %d (%+v)", len(refs), refs)
	}
	if refs[0].Sender != "jma" || refs[0].Identifier != "EQ-1" || refs[0].Sent == "" {
		t.Fatalf("ref0 = %+v", refs[0])
	}
	if refs[1].Identifier != "EQ-2" {
		t.Fatalf("ref1 = %+v", refs[1])
	}
	if len(cap.ParseReferences("")) != 0 {
		t.Fatal("empty references => no entries")
	}
	if len(cap.ParseReferences("senderonly")) != 0 {
		t.Fatal("a token with no identifier must be dropped")
	}
}

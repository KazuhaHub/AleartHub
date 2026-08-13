package eew

import "testing"

func TestMapWolfxSeverity(t *testing.T) {
	cases := []struct {
		name, json, wantSev string
		wantOK              bool
	}{
		{"warning 6+", `{"type":"jma_eew","EventID":"1","MaxIntensity":"6+","isWarn":true,"Hypocenter":"千葉県東方沖","Magunitude":"6.8","Title":"緊急地震速報（警報）"}`, "emergency", true},
		{"critical 5-", `{"type":"jma_eew","EventID":"2","MaxIntensity":"5-","Hypocenter":"x"}`, "critical", true},
		{"critical 4", `{"type":"jma_eew","EventID":"3","MaxIntensity":"4"}`, "critical", true},
		{"warning 3", `{"type":"jma_eew","EventID":"4","MaxIntensity":"3"}`, "warning", true},
		{"heartbeat", `{"type":"heartbeat"}`, "", false},
		{"no eventid", `{"type":"jma_eew","MaxIntensity":"7"}`, "", false},
	}
	for _, c := range cases {
		ev, ok := MapWolfx([]byte(c.json))
		if ok != c.wantOK {
			t.Errorf("%s: ok=%v want %v", c.name, ok, c.wantOK)
			continue
		}
		if ok && ev.Severity != c.wantSev {
			t.Errorf("%s: severity=%q want %q", c.name, ev.Severity, c.wantSev)
		}
	}
}

func TestTrainingNeverRealAlarm(t *testing.T) {
	ev, ok := MapWolfx([]byte(`{"type":"jma_eew","EventID":"T","MaxIntensity":"7","isWarn":true,"isTraining":true}`))
	if !ok || !ev.IsTest {
		t.Fatal("training EEW should be IsTest")
	}
	if ev.Severity == "emergency" || ev.Severity == "critical" {
		t.Errorf("training must not be %q", ev.Severity)
	}
}

func TestCancel(t *testing.T) {
	ev, ok := MapWolfx([]byte(`{"type":"jma_eew","EventID":"C","isCancel":true}`))
	if !ok || !ev.IsCancel {
		t.Fatal("cancel not detected")
	}
}

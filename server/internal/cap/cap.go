// Package cap parses OASIS CAP 1.2 alerts and maps them to AlertHub's envelope
// (ARCHITECTURE §7). CAP is the interop standard every serious emergency system
// speaks, so the public "other programs can call" API accepts it.
package cap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"strings"
	"time"
)

// Document is the subset of CAP 1.2 we consume (XML, default namespace matched by
// local name). alert → info(+area). We use the first info block.
type Document struct {
	XMLName    xml.Name `xml:"alert"`
	Identifier string   `xml:"identifier"`
	Sender     string   `xml:"sender"`
	Sent       string   `xml:"sent"`
	Status     string   `xml:"status"`  // Actual | Exercise | System | Test | Draft
	MsgType    string   `xml:"msgType"` // Alert | Update | Cancel | Ack | Error
	Scope      string   `xml:"scope"`
	References string   `xml:"references"`
	Info       []Info   `xml:"info"`
}

type Info struct {
	Language     string `xml:"language"`
	Category     string `xml:"category"` // Geo|Met|Safety|Security|Rescue|Fire|Health|Env|Transport|Infra|CBRNE|Other
	Event        string `xml:"event"`
	ResponseType string `xml:"responseType"` // Shelter|Evacuate|Prepare|Execute|Avoid|Monitor|Assess|AllClear|None
	Urgency      string `xml:"urgency"`      // Immediate|Expected|Future|Past|Unknown
	Severity     string `xml:"severity"`     // Extreme|Severe|Moderate|Minor|Unknown
	Certainty    string `xml:"certainty"`    // Observed|Likely|Possible|Unlikely|Unknown
	Headline     string `xml:"headline"`
	Description  string `xml:"description"`
	Instruction  string `xml:"instruction"`
	Expires      string `xml:"expires"`
	Onset        string `xml:"onset"`
}

// Mapped is the normalized result the API turns into an alert.Alert.
type Mapped struct {
	Identifier string
	MsgType    string // Alert|Update|Cancel
	IsTest     bool   // status != Actual → drill/test, must NOT be a real alarm
	Category   string // AlertHub category
	Severity   string // AlertHub severity
	Title      string
	Body       string
	Action     string
	TTL        int64 // seconds
	References string
}

func Parse(b []byte) (*Document, error) {
	var d Document
	if err := xml.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// AlertID derives a deterministic, MQTT-topic-safe AlertHub id from a CAP message's
// (sender, identifier). Using a stable id — not a random UUID — is what lets a later
// CAP Cancel, which references the original (sender, identifier), recall the exact
// alert we published. Mirrors the EEW "eew-"+EventID pattern. 12 bytes = 96 bits of
// SHA-256 is ample against collision; the "cap-" prefix + hex keeps it topic-safe.
func AlertID(sender, identifier string) string {
	sum := sha256.Sum256([]byte(sender + "\x00" + identifier))
	return "cap-" + hex.EncodeToString(sum[:12])
}

// Reference is one parsed CAP <references> entry.
type Reference struct{ Sender, Identifier, Sent string }

// ParseReferences parses a CAP 1.2 <references> value: a whitespace-separated list
// of "sender,identifier,sent" triples (CAP 1.2 §3.2.1 — identifier/sender contain
// no spaces or commas, so this split is unambiguous). Entries without an identifier
// are dropped since they cannot be resolved to an alert.
func ParseReferences(s string) []Reference {
	var out []Reference
	for _, tok := range strings.Fields(s) {
		parts := strings.SplitN(tok, ",", 3)
		r := Reference{Sender: parts[0]}
		if len(parts) > 1 {
			r.Identifier = parts[1]
		}
		if len(parts) > 2 {
			r.Sent = parts[2]
		}
		if r.Identifier == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}

var capCategory = map[string]string{
	"Fire": "fire", "Met": "weather", "Geo": "earthquake",
	"Security": "security", "Safety": "security", "Health": "system",
	"Infra": "system", "Env": "weather",
}

// MapToAlert collapses the CAP urgency/severity/certainty triple into AlertHub's
// single severity (ARCHITECTURE §7) and maps category/action.
func (d *Document) MapToAlert(now time.Time) Mapped {
	m := Mapped{
		Identifier: d.Identifier,
		MsgType:    firstNonEmpty(d.MsgType, "Alert"),
		IsTest:     d.Status != "" && d.Status != "Actual",
		References: d.References,
		Category:   "custom",
		Severity:   "notice",
	}
	if len(d.Info) == 0 {
		m.Title = firstNonEmpty(d.Identifier, "Alert")
		return m
	}
	i := d.Info[0]
	if c, ok := capCategory[i.Category]; ok {
		m.Category = c
	}
	m.Severity = collapseSeverity(i.Urgency, i.Severity, i.Certainty)
	if m.IsTest && rank(m.Severity) > rank("warning") {
		m.Severity = "warning" // never let a Test/Exercise fire a real fullscreen alarm
	}
	m.Title = firstNonEmpty(i.Headline, i.Event, "Alert")
	m.Body = strings.TrimSpace(i.Description)
	if i.Instruction != "" {
		if m.Body != "" {
			m.Body += "  "
		}
		m.Body += i.Instruction
	}
	m.Action = responseAction(i.ResponseType, i.Instruction)
	m.TTL = ttlFrom(i.Expires, now, m.Severity)
	return m
}

func collapseSeverity(urgency, severity, certainty string) string {
	if certainty == "Unlikely" {
		return "notice"
	}
	immediate := urgency == "Immediate"
	switch {
	case immediate && (severity == "Extreme" || severity == "Severe") && (certainty == "Observed" || certainty == "Likely"):
		return "emergency"
	case immediate && (severity == "Extreme" || severity == "Severe"):
		return "critical"
	case immediate && severity == "Moderate":
		return "critical"
	case urgency == "Expected" || urgency == "Future" || urgency == "":
		return "notice"
	case severity == "Moderate":
		return "warning"
	default:
		return "warning"
	}
}

func responseAction(rt, instruction string) string {
	switch rt {
	case "Shelter":
		return "就地避险 / Shelter in place"
	case "Evacuate":
		return "立即撤离 / Evacuate now"
	case "Execute":
		return "执行预案 / Execute the plan"
	case "Prepare":
		return "做好准备 / Prepare"
	case "Avoid":
		return "避开危险区域 / Avoid the area"
	case "AllClear":
		return "警报解除 / All clear"
	case "Monitor", "Assess":
		return "持续关注 / Monitor"
	}
	return strings.TrimSpace(instruction)
}

func ttlFrom(expires string, now time.Time, severity string) int64 {
	if expires != "" {
		if t, err := time.Parse(time.RFC3339, expires); err == nil {
			if d := int64(t.Sub(now).Seconds()); d > 0 {
				return d
			}
		}
	}
	switch severity {
	case "emergency", "critical":
		return 120
	default:
		return 600
	}
}

func rank(s string) int {
	return map[string]int{"notice": 0, "warning": 1, "critical": 2, "emergency": 3}[s]
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

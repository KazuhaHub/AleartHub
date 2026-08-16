// Package scenario holds the one-tap emergency templates (SPEC-SAFETY §6.3,
// borrowed from Alertus).
//
// They live on the SERVER, not in the panel, because every client must offer the
// same set: a "shelter in place" button that differs between the wall tablet and
// the phone is worse than having none, since in an emergency people act from
// muscle memory rather than reading.
//
// Each maps to a CAP responseType so an emitted alert says the same thing to
// downstream systems that it says to a person.
package scenario

import "strings"

type Scenario struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Icon         string `json:"icon"`
	Severity     string `json:"severity"`
	Category     string `json:"category"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	Action       string `json:"action"`
	ResponseType string `json:"response_type"` // CAP 1.2 responseType
	TTL          int64  `json:"ttl"`
}

// The canonical five from §6.3. The wording is deliberately imperative and short:
// these are read by someone who is frightened and has seconds.
var all = []Scenario{
	{
		ID: "evacuate", Label: "立即撤离", Icon: "🚪",
		Severity: "emergency", Category: "fire",
		Title: "立即撤离", Body: "请立即离开建筑，前往预定集合点。",
		Action: "立即撤离，不要乘电梯", ResponseType: "Evacuate", TTL: 600,
	},
	{
		ID: "shelter", Label: "就地避险", Icon: "🏠",
		Severity: "emergency", Category: "weather",
		Title: "就地避险", Body: "不要外出。留在室内，远离门窗。",
		Action: "就地避险，远离门窗", ResponseType: "Shelter", TTL: 900,
	},
	{
		ID: "lockdown", Label: "锁闭", Icon: "🔒",
		Severity: "emergency", Category: "security",
		Title: "锁闭", Body: "锁门、关灯、保持安静，等待解除通知。",
		Action: "锁门躲避，保持安静", ResponseType: "Shelter", TTL: 900,
	},
	{
		ID: "dropcover", Label: "趴下·掩护·抓牢", Icon: "🌐",
		Severity: "emergency", Category: "earthquake",
		Title: "正在发生地震", Body: "强烈摇晃即将到达。",
		Action: "趴下，掩护，抓牢", ResponseType: "Shelter", TTL: 120,
	},
	{
		// AllClear is a notice on purpose: an all-clear must never present as a
		// fullscreen emergency, or the relief itself becomes another alarm.
		ID: "allclear", Label: "警报解除", Icon: "✅",
		Severity: "notice", Category: "custom",
		Title: "警报解除", Body: "情况已解除，可恢复正常活动。",
		Action: "恢复正常活动", ResponseType: "AllClear", TTL: 600,
	},
}

// All returns the scenario set.
func All() []Scenario {
	out := make([]Scenario, len(all))
	copy(out, all)
	return out
}

// Get looks one up by id.
func Get(id string) (Scenario, bool) {
	for _, s := range all {
		if strings.EqualFold(s.ID, id) {
			return s, true
		}
	}
	return Scenario{}, false
}

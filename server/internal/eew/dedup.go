package eew

import "sync"

// Deduper collapses reports of the same earthquake. It is shared by every source
// so that two relays reporting one quake produce one alert, not two.
//
// It is NOT plain first-wins: a later report carrying a HIGHER intensity is let
// through as an upgrade. Suppressing it would mean a quake first reported as
// 震度4 and revised to 6弱 never re-alarms — the reader would be acting on the
// wrong instruction. Downgrades and repeats stay suppressed, because re-alarming
// for no new information trains people to ignore alerts.
//
// The upgrade is published under the SAME alert id, so SPEC §5.2 governs what the
// client does with it: higher severity re-presents, anything else extends quietly.
type Deduper struct {
	mu   sync.Mutex
	seen map[string]int // eventID -> highest severity rank emitted so far
}

func NewDeduper() *Deduper { return &Deduper{seen: map[string]int{}} }

// Emit reports whether this event should be published, and whether it is an
// upgrade of one already sent. Concurrent callers from different source
// goroutines are safe; exactly one wins a given (event, severity).
func (d *Deduper) Emit(eventID, severity string) (emit bool, upgrade bool) {
	r := rank(severity)
	d.mu.Lock()
	defer d.mu.Unlock()
	prev, known := d.seen[eventID]
	if !known {
		d.seen[eventID] = r
		return true, false
	}
	if r > prev {
		d.seen[eventID] = r
		return true, true // intensity was revised upward — must reach people again
	}
	return false, false
}

// FirstSeen is Emit without severity, kept for callers that only need "have I
// seen this event".
func (d *Deduper) FirstSeen(eventID string) bool {
	emit, _ := d.Emit(eventID, "")
	return emit
}

// Forget drops an event so a later report of it is emitted again. Used on cancel:
// after a recall, a re-issued warning for the same id must get through.
func (d *Deduper) Forget(eventID string) {
	d.mu.Lock()
	delete(d.seen, eventID)
	d.mu.Unlock()
}

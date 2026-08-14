package eew

import "sync"

// Deduper collapses reports of the same earthquake. It is shared by every source
// so that two relays reporting one quake produce one alert, not two.
//
// KNOWN LIMIT, stated rather than hidden: this is first-report-wins. If a slower
// relay later reports a HIGHER intensity for an event already seen, that upgrade
// is suppressed. Emitting it would need the SPEC §5.2 renewal mechanism (re-issue
// under the same id with a fresh issued_at), which is not built — and without it
// a second publish carrying the same "eew-<EventID>" id would be dropped by the
// client's own dedup gate anyway. Tracked in SPEC.md §0.
type Deduper struct {
	mu   sync.Mutex
	seen map[string]bool
}

func NewDeduper() *Deduper { return &Deduper{seen: map[string]bool{}} }

// FirstSeen reports whether this is the first time the event has been seen, and
// records it. Concurrent callers from different source goroutines are safe.
func (d *Deduper) FirstSeen(eventID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen[eventID] {
		return false
	}
	d.seen[eventID] = true
	return true
}

// Forget drops an event so a later report of it is emitted again. Used on cancel:
// after a recall, a re-issued warning for the same id must get through.
func (d *Deduper) Forget(eventID string) {
	d.mu.Lock()
	delete(d.seen, eventID)
	d.mu.Unlock()
}

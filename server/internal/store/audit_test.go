package store

import (
	"strings"
	"testing"
)

func TestAudit_AppendAndList(t *testing.T) {
	s := openTestSQLite(t)
	org, _ := s.EnsureDefaultOrg()
	other, _ := s.CreateOrg("other", "Other")

	for _, a := range []string{"alert.publish", "alert.cancel", "auth.login"} {
		if err := s.AppendAudit(&AuditEntry{
			OrgID: org, ActorType: ActorUser, ActorID: 7, ActorName: "alice",
			Action: a, TargetType: "alert", TargetID: "x", IP: "1.2.3.4",
		}); err != nil {
			t.Fatalf("append %s: %v", a, err)
		}
	}
	_ = s.AppendAudit(&AuditEntry{OrgID: other, Action: "alert.publish", ActorType: ActorSystem})

	list, err := s.ListAudit(org, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("org trail = %d entries, want 3 (other org must not leak)", len(list))
	}
	// newest first
	if list[0].Action != "auth.login" {
		t.Errorf("expected newest-first ordering, got %s first", list[0].Action)
	}
	if list[0].ActorName != "alice" || list[0].IP != "1.2.3.4" {
		t.Errorf("actor/ip not persisted: %+v", list[0])
	}
	if o, _ := s.ListAudit(other, 100); len(o) != 1 {
		t.Errorf("other org trail = %d, want 1", len(o))
	}
}

func TestAudit_RequiresAction(t *testing.T) {
	s := openTestSQLite(t)
	if err := s.AppendAudit(&AuditEntry{OrgID: 1}); err == nil {
		t.Fatal("an entry with no action must be rejected")
	}
}

func TestAudit_ChainLinks(t *testing.T) {
	s := openTestSQLite(t)
	org, _ := s.EnsureDefaultOrg()
	for i := 0; i < 4; i++ {
		if err := s.AppendAudit(&AuditEntry{OrgID: org, Action: "alert.publish", TargetID: "a"}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	rows, err := s.query(`SELECT ` + auditCols + ` FROM audit_log ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	prev := ""
	n := 0
	for rows.Next() {
		e, err := scanAudit(rows)
		if err != nil {
			t.Fatal(err)
		}
		if e.PrevHash != prev {
			t.Fatalf("entry %d prev_hash=%q, want %q", e.ID, e.PrevHash, prev)
		}
		if e.Hash == "" || len(e.Hash) != 64 {
			t.Fatalf("entry %d hash is not a sha256 hex: %q", e.ID, e.Hash)
		}
		prev = e.Hash
		n++
	}
	if n != 4 {
		t.Fatalf("walked %d entries, want 4", n)
	}
	res, err := s.VerifyAuditChain()
	if err != nil || !res.OK || res.Entries != 4 {
		t.Fatalf("VerifyAuditChain = %+v, %v; want OK with 4 entries", res, err)
	}
}

// TestAudit_DetectsEditedRow is the whole point of the chain: someone with
// database access edits a row to hide what they did, and the log must say so.
func TestAudit_DetectsEditedRow(t *testing.T) {
	s := openTestSQLite(t)
	org, _ := s.EnsureDefaultOrg()
	for _, name := range []string{"alice", "mallory", "bob"} {
		if err := s.AppendAudit(&AuditEntry{
			OrgID: org, Action: "alert.publish", ActorType: ActorUser, ActorName: name,
		}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if res, _ := s.VerifyAuditChain(); !res.OK {
		t.Fatal("chain should be intact before tampering")
	}

	// Rewrite history: make mallory's action look like alice's.
	if _, err := s.exec(`UPDATE audit_log SET actor_name = 'alice' WHERE actor_name = 'mallory'`); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	res, err := s.VerifyAuditChain()
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if res.OK {
		t.Fatal("SECURITY: an edited audit row went undetected")
	}
	if res.BadID == 0 || !strings.Contains(res.Reason, "hash") {
		t.Fatalf("verify result should name the offending row and why: %+v", res)
	}
}

// TestAudit_DetectsDeletedRow covers the other half: deleting the evidence must
// break the links of everything that came after it.
func TestAudit_DetectsDeletedRow(t *testing.T) {
	s := openTestSQLite(t)
	org, _ := s.EnsureDefaultOrg()
	for i := 0; i < 3; i++ {
		_ = s.AppendAudit(&AuditEntry{OrgID: org, Action: "alert.publish", ActorName: "x"})
	}
	if _, err := s.exec(`DELETE FROM audit_log WHERE id = (SELECT MIN(id) + 1 FROM audit_log)`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	res, _ := s.VerifyAuditChain()
	if res.OK {
		t.Fatal("SECURITY: a deleted audit row went undetected")
	}
	if !strings.Contains(res.Reason, "prev_hash") {
		t.Errorf("a deletion should be reported as a broken link, got %q", res.Reason)
	}
}

// An empty log is trivially valid — a fresh install must not look tampered with.
func TestAudit_EmptyChainIsValid(t *testing.T) {
	s := openTestSQLite(t)
	res, err := s.VerifyAuditChain()
	if err != nil || !res.OK || res.Entries != 0 {
		t.Fatalf("empty chain = %+v, %v; want OK with 0 entries", res, err)
	}
}

// Two entries with identical content must still get distinct hashes, because each
// links to a different predecessor. Otherwise duplicates could be swapped freely.
func TestAudit_IdenticalEntriesGetDistinctHashes(t *testing.T) {
	s := openTestSQLite(t)
	org, _ := s.EnsureDefaultOrg()
	mk := func() AuditEntry {
		return AuditEntry{OrgID: org, At: 1765238400, Action: "alert.publish",
			ActorType: ActorUser, ActorName: "same", TargetID: "same"}
	}
	a, b := mk(), mk()
	if err := s.AppendAudit(&a); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendAudit(&b); err != nil {
		t.Fatal(err)
	}
	if a.Hash == b.Hash {
		t.Fatal("identical entries must still differ by their chain position")
	}
	if b.PrevHash != a.Hash {
		t.Fatalf("second entry must link to the first: prev=%q first=%q", b.PrevHash, a.Hash)
	}
}

// --- retention ---------------------------------------------------------------

// TestAudit_PruneKeepsChainVerifiable is the point of the anchor: retention must
// not leave the log permanently reporting "tampered", or the operator learns to
// ignore the one alarm that means someone really did tamper with it.
func TestAudit_PruneKeepsChainVerifiable(t *testing.T) {
	s := openTestSQLite(t)
	org, _ := s.EnsureDefaultOrg()
	old := int64(1000)
	for i := 0; i < 4; i++ { // old entries
		if err := s.AppendAudit(&AuditEntry{OrgID: org, At: old + int64(i), Action: "alert.publish"}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ { // recent entries
		if err := s.AppendAudit(&AuditEntry{OrgID: org, At: 9000 + int64(i), Action: "auth.login"}); err != nil {
			t.Fatal(err)
		}
	}

	n, err := s.PruneAudit(5000, "test")
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 4 {
		t.Fatalf("pruned %d, want 4", n)
	}
	res, err := s.VerifyAuditChain()
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !res.OK {
		t.Fatalf("a pruned chain must still verify, got %+v", res)
	}
}

// The prune must itself be in the record: shortening the audit trail is exactly
// the kind of action an audit trail exists to capture.
func TestAudit_PruneRecordsItself(t *testing.T) {
	s := openTestSQLite(t)
	org, _ := s.EnsureDefaultOrg()
	_ = s.AppendAudit(&AuditEntry{OrgID: org, At: 1000, Action: "alert.publish"})
	_ = s.AppendAudit(&AuditEntry{OrgID: org, At: 9000, Action: "auth.login"})

	if _, err := s.PruneAudit(5000, "retention-worker"); err != nil {
		t.Fatalf("prune: %v", err)
	}
	rows, err := s.query(`SELECT action, actor_name FROM audit_log WHERE action = 'audit.prune'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var action, actor string
		_ = rows.Scan(&action, &actor)
		if action == "audit.prune" && actor == "retention-worker" {
			found = true
		}
	}
	if !found {
		t.Fatal("the prune itself was not recorded")
	}
}

// Tampering after a prune must STILL be detected — the anchor must not become a
// way to launder edits.
func TestAudit_PrunedChainStillDetectsTampering(t *testing.T) {
	s := openTestSQLite(t)
	org, _ := s.EnsureDefaultOrg()
	_ = s.AppendAudit(&AuditEntry{OrgID: org, At: 1000, Action: "alert.publish"})
	for i := 0; i < 3; i++ {
		_ = s.AppendAudit(&AuditEntry{OrgID: org, At: 9000 + int64(i), Action: "auth.login", ActorName: "alice"})
	}
	if _, err := s.PruneAudit(5000, "test"); err != nil {
		t.Fatal(err)
	}
	if res, _ := s.VerifyAuditChain(); !res.OK {
		t.Fatal("setup: pruned chain should verify")
	}
	if _, err := s.exec(`UPDATE audit_log SET actor_name = 'mallory' WHERE actor_name = 'alice'`); err != nil {
		t.Fatal(err)
	}
	res, _ := s.VerifyAuditChain()
	if res.OK {
		t.Fatal("SECURITY: tampering after a prune went undetected")
	}
}

// Pruning with nothing old enough is a no-op and must not disturb the chain.
func TestAudit_PruneNothingToDo(t *testing.T) {
	s := openTestSQLite(t)
	org, _ := s.EnsureDefaultOrg()
	_ = s.AppendAudit(&AuditEntry{OrgID: org, At: 9000, Action: "auth.login"})
	n, err := s.PruneAudit(1000, "test")
	if err != nil || n != 0 {
		t.Fatalf("prune = %d, %v; want 0, nil", n, err)
	}
	if res, _ := s.VerifyAuditChain(); !res.OK {
		t.Fatal("a no-op prune must leave the chain intact")
	}
}

package group

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"aim-chat/go-backend/internal/testutil/fsperm"
)

func TestSnapshotStoreBootstrapDefaultsWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "groups.enc")
	store := NewSnapshotStore()
	store.Configure(path, "test-secret")

	states, eventLog, err := store.Bootstrap()
	if err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("expected empty states, got %d", len(states))
	}
	if len(eventLog) != 0 {
		t.Fatalf("expected empty event log, got %d", len(eventLog))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected group state file to be created, err=%v", err)
	}
}

func TestSnapshotStorePersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "groups.enc")
	store := NewSnapshotStore()
	store.Configure(path, "test-secret")

	now := time.Now().UTC().Truncate(time.Second)
	states := map[string]GroupState{
		"group-1": {
			Group: Group{
				ID:        "group-1",
				Title:     "alpha",
				CreatedBy: "aim1owner",
				CreatedAt: now,
				UpdatedAt: now,
			},
			Version: 2,
			AppliedEventIDs: map[string]struct{}{
				"evt-1": {},
				"evt-2": {},
			},
			Members: map[string]GroupMember{
				"aim1owner": {
					GroupID:   "group-1",
					MemberID:  "aim1owner",
					Role:      GroupMemberRoleOwner,
					Status:    GroupMemberStatusActive,
					InvitedAt: now,
					UpdatedAt: now,
				},
				"aim1user": {
					GroupID:   "group-1",
					MemberID:  "aim1user",
					Role:      GroupMemberRoleUser,
					Status:    GroupMemberStatusInvited,
					InvitedAt: now,
					UpdatedAt: now,
				},
			},
			LastKeyVersion: 1,
		},
	}
	eventLog := map[string][]GroupEvent{
		"group-1": {
			{
				ID:         "evt-1",
				GroupID:    "group-1",
				Version:    1,
				Type:       GroupEventTypeMemberAdd,
				ActorID:    "aim1owner",
				OccurredAt: now,
				MemberID:   "aim1owner",
				Role:       GroupMemberRoleOwner,
			},
			{
				ID:         "evt-2",
				GroupID:    "group-1",
				Version:    2,
				Type:       GroupEventTypeMemberAdd,
				ActorID:    "aim1owner",
				OccurredAt: now.Add(time.Second),
				MemberID:   "aim1user",
				Role:       GroupMemberRoleUser,
			},
		},
	}

	if err := store.Persist(states, eventLog); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	reload := NewSnapshotStore()
	reload.Configure(path, "test-secret")
	gotStates, gotEventLog, err := reload.Bootstrap()
	if err != nil {
		t.Fatalf("reload bootstrap failed: %v", err)
	}
	if len(gotStates) != 1 {
		t.Fatalf("expected one group state, got %d", len(gotStates))
	}
	if gotStates["group-1"].Version != 2 {
		t.Fatalf("unexpected state version: got=%d want=2", gotStates["group-1"].Version)
	}
	if gotStates["group-1"].Group.Title != "alpha" {
		t.Fatalf("unexpected group title: got=%q want=%q", gotStates["group-1"].Group.Title, "alpha")
	}
	if gotStates["group-1"].Members["aim1user"].Status != GroupMemberStatusInvited {
		t.Fatalf("unexpected member status: got=%s want=%s", gotStates["group-1"].Members["aim1user"].Status, GroupMemberStatusInvited)
	}
	if len(gotEventLog["group-1"]) != 2 {
		t.Fatalf("expected two events in log, got %d", len(gotEventLog["group-1"]))
	}
	if gotEventLog["group-1"][0].ID != "evt-1" || gotEventLog["group-1"][1].ID != "evt-2" {
		t.Fatalf("unexpected event sequence: %+v", gotEventLog["group-1"])
	}
}

func TestSnapshotStoreBootstrapCorruptedPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "groups.enc")
	if err := os.WriteFile(path, []byte("corrupted"), 0o600); err != nil {
		t.Fatalf("write corrupted payload failed: %v", err)
	}

	store := NewSnapshotStore()
	store.Configure(path, "test-secret")
	_, _, err := store.Bootstrap()
	if err == nil {
		t.Fatal("expected bootstrap error for corrupted payload")
	}
}

func TestSnapshotStoreBootstrapIOError(t *testing.T) {
	dir := t.TempDir()
	store := NewSnapshotStore()
	store.Configure(dir, "test-secret")

	_, _, err := store.Bootstrap()
	if err == nil {
		t.Fatal("expected bootstrap io error")
	}
}

func TestSnapshotStorePersistCreatesPrivateDir(t *testing.T) {
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "secure", "groups.enc")
	store := NewSnapshotStore()
	store.Configure(path, "test-secret")

	if err := store.Persist(map[string]GroupState{}, map[string][]GroupEvent{}); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	fsperm.AssertPrivateDirPerm(t, filepath.Dir(path))
}

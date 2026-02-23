package usecase

import (
	"testing"
	"time"
)

func TestServiceListGroupsFiltersByActorMembership(t *testing.T) {
	actorID := "aim1me"
	now := time.Date(2026, time.February, 23, 0, 0, 0, 0, time.UTC)

	makeState := func(id string, createdAt time.Time, status GroupMemberStatus) GroupState {
		return GroupState{
			Group: Group{
				ID:        id,
				Title:     "group-" + id,
				CreatedBy: actorID,
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			},
			Members: map[string]GroupMember{
				actorID: {
					GroupID:  id,
					MemberID: actorID,
					Role:     GroupMemberRoleUser,
					Status:   status,
				},
			},
		}
	}

	states := map[string]GroupState{
		"active":  makeState("active", now.Add(1*time.Minute), GroupMemberStatusActive),
		"invited": makeState("invited", now.Add(2*time.Minute), GroupMemberStatusInvited),
		"left":    makeState("left", now.Add(3*time.Minute), GroupMemberStatusLeft),
		"removed": makeState("removed", now.Add(4*time.Minute), GroupMemberStatusRemoved),
		"other": {
			Group: Group{
				ID:        "other",
				Title:     "group-other",
				CreatedBy: "aim1other",
				CreatedAt: now.Add(5 * time.Minute),
				UpdatedAt: now.Add(5 * time.Minute),
			},
			Members: map[string]GroupMember{
				"aim1other": {
					GroupID:  "other",
					MemberID: "aim1other",
					Role:     GroupMemberRoleOwner,
					Status:   GroupMemberStatusActive,
				},
			},
		},
	}

	svc := &Service{
		IdentityID: func() string { return actorID },
		SnapshotStates: func() map[string]GroupState {
			return states
		},
	}

	groups, err := svc.ListGroups()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].ID != "active" {
		t.Fatalf("expected first group active, got %q", groups[0].ID)
	}
	if groups[1].ID != "invited" {
		t.Fatalf("expected second group invited, got %q", groups[1].ID)
	}
}

func TestServiceListGroupsWithoutActorReturnsAll(t *testing.T) {
	now := time.Date(2026, time.February, 23, 0, 0, 0, 0, time.UTC)
	states := map[string]GroupState{
		"g1": {
			Group: Group{
				ID:        "g1",
				Title:     "group-1",
				CreatedBy: "aim1user",
				CreatedAt: now,
				UpdatedAt: now,
			},
			Members: map[string]GroupMember{},
		},
		"g2": {
			Group: Group{
				ID:        "g2",
				Title:     "group-2",
				CreatedBy: "aim1user",
				CreatedAt: now.Add(1 * time.Minute),
				UpdatedAt: now.Add(1 * time.Minute),
			},
			Members: map[string]GroupMember{},
		},
	}

	svc := &Service{
		IdentityID: func() string { return "" },
		SnapshotStates: func() map[string]GroupState {
			return states
		},
	}

	groups, err := svc.ListGroups()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
}

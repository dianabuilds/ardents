package group

import (
	"errors"
	"testing"
)

func TestGroupMemberRoleValidation(t *testing.T) {
	cases := []struct {
		name string
		role GroupMemberRole
		want bool
	}{
		{name: "owner", role: GroupMemberRoleOwner, want: true},
		{name: "admin", role: GroupMemberRoleAdmin, want: true},
		{name: "user", role: GroupMemberRoleUser, want: true},
		{name: "invalid", role: "moderator", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.role.Valid(); got != tc.want {
				t.Fatalf("role validity mismatch: got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestParseGroupMemberRole(t *testing.T) {
	got, err := ParseGroupMemberRole(" owner ")
	if err != nil {
		t.Fatalf("parse role failed: %v", err)
	}
	if got != GroupMemberRoleOwner {
		t.Fatalf("unexpected role: got=%q want=%q", got, GroupMemberRoleOwner)
	}

	_, err = ParseGroupMemberRole("invalid")
	if !errors.Is(err, ErrInvalidGroupMemberRole) {
		t.Fatalf("expected ErrInvalidGroupMemberRole, got %v", err)
	}
}

func TestGroupMemberStatusValidation(t *testing.T) {
	cases := []struct {
		name   string
		status GroupMemberStatus
		want   bool
	}{
		{name: "invited", status: GroupMemberStatusInvited, want: true},
		{name: "active", status: GroupMemberStatusActive, want: true},
		{name: "left", status: GroupMemberStatusLeft, want: true},
		{name: "removed", status: GroupMemberStatusRemoved, want: true},
		{name: "invalid", status: "paused", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.status.Valid(); got != tc.want {
				t.Fatalf("status validity mismatch: got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestParseGroupMemberStatus(t *testing.T) {
	got, err := ParseGroupMemberStatus(" active ")
	if err != nil {
		t.Fatalf("parse status failed: %v", err)
	}
	if got != GroupMemberStatusActive {
		t.Fatalf("unexpected status: got=%q want=%q", got, GroupMemberStatusActive)
	}

	_, err = ParseGroupMemberStatus("invalid")
	if !errors.Is(err, ErrInvalidGroupMemberStatus) {
		t.Fatalf("expected ErrInvalidGroupMemberStatus, got %v", err)
	}
}

func TestValidateGroupMember(t *testing.T) {
	valid := GroupMember{
		GroupID:  "group-1",
		MemberID: "aim1member",
		Role:     GroupMemberRoleUser,
		Status:   GroupMemberStatusInvited,
	}
	if err := ValidateGroupMember(valid); err != nil {
		t.Fatalf("valid member rejected: %v", err)
	}

	cases := []struct {
		name string
		in   GroupMember
		err  error
	}{
		{name: "missing group id", in: GroupMember{MemberID: "m", Role: GroupMemberRoleUser, Status: GroupMemberStatusInvited}, err: ErrInvalidGroupID},
		{name: "missing member id", in: GroupMember{GroupID: "g", Role: GroupMemberRoleUser, Status: GroupMemberStatusInvited}, err: ErrInvalidGroupMemberID},
		{name: "invalid role", in: GroupMember{GroupID: "g", MemberID: "m", Role: "bad", Status: GroupMemberStatusInvited}, err: ErrInvalidGroupMemberRole},
		{name: "invalid status", in: GroupMember{GroupID: "g", MemberID: "m", Role: GroupMemberRoleUser, Status: "bad"}, err: ErrInvalidGroupMemberStatus},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateGroupMember(tc.in)
			if !errors.Is(err, tc.err) {
				t.Fatalf("expected %v, got %v", tc.err, err)
			}
		})
	}
}

func TestValidateGroupMemberStatusTransition(t *testing.T) {
	allowed := []struct {
		from GroupMemberStatus
		to   GroupMemberStatus
	}{
		{from: GroupMemberStatusInvited, to: GroupMemberStatusInvited},
		{from: GroupMemberStatusInvited, to: GroupMemberStatusActive},
		{from: GroupMemberStatusInvited, to: GroupMemberStatusRemoved},
		{from: GroupMemberStatusActive, to: GroupMemberStatusActive},
		{from: GroupMemberStatusActive, to: GroupMemberStatusLeft},
		{from: GroupMemberStatusActive, to: GroupMemberStatusRemoved},
		{from: GroupMemberStatusLeft, to: GroupMemberStatusLeft},
		{from: GroupMemberStatusLeft, to: GroupMemberStatusActive},
		{from: GroupMemberStatusLeft, to: GroupMemberStatusRemoved},
		{from: GroupMemberStatusRemoved, to: GroupMemberStatusRemoved},
	}
	for _, tc := range allowed {
		if err := ValidateGroupMemberStatusTransition(tc.from, tc.to); err != nil {
			t.Fatalf("transition %s -> %s should be allowed, got %v", tc.from, tc.to, err)
		}
	}

	denied := []struct {
		from GroupMemberStatus
		to   GroupMemberStatus
	}{
		{from: GroupMemberStatusInvited, to: GroupMemberStatusLeft},
		{from: GroupMemberStatusActive, to: GroupMemberStatusInvited},
		{from: GroupMemberStatusLeft, to: GroupMemberStatusInvited},
		{from: GroupMemberStatusRemoved, to: GroupMemberStatusInvited},
		{from: GroupMemberStatusRemoved, to: GroupMemberStatusActive},
		{from: GroupMemberStatusRemoved, to: GroupMemberStatusLeft},
	}
	for _, tc := range denied {
		err := ValidateGroupMemberStatusTransition(tc.from, tc.to)
		if !errors.Is(err, ErrInvalidGroupMemberStatusTransition) {
			t.Fatalf("transition %s -> %s should be rejected with ErrInvalidGroupMemberStatusTransition, got %v", tc.from, tc.to, err)
		}
	}

	if !errors.Is(ValidateGroupMemberStatusTransition("bad", GroupMemberStatusActive), ErrInvalidGroupMemberStatus) {
		t.Fatal("invalid source status must fail with ErrInvalidGroupMemberStatus")
	}
	if !errors.Is(ValidateGroupMemberStatusTransition(GroupMemberStatusInvited, "bad"), ErrInvalidGroupMemberStatus) {
		t.Fatal("invalid target status must fail with ErrInvalidGroupMemberStatus")
	}
}

func TestNormalizeGroupMemberID(t *testing.T) {
	id, err := NormalizeGroupMemberID("  aim1member  ")
	if err != nil {
		t.Fatalf("normalize member id failed: %v", err)
	}
	if id != "aim1member" {
		t.Fatalf("unexpected normalized id: %q", id)
	}
	if _, err := NormalizeGroupMemberID("   "); !errors.Is(err, ErrInvalidGroupMemberID) {
		t.Fatalf("expected ErrInvalidGroupMemberID, got %v", err)
	}
}

func TestGroupMemberCapabilities(t *testing.T) {
	owner := GroupMember{Role: GroupMemberRoleOwner, Status: GroupMemberStatusActive}
	admin := GroupMember{Role: GroupMemberRoleAdmin, Status: GroupMemberStatusInvited}
	user := GroupMember{Role: GroupMemberRoleUser, Status: GroupMemberStatusActive}
	leftUser := GroupMember{Role: GroupMemberRoleUser, Status: GroupMemberStatusLeft}

	if !owner.IsOwner() {
		t.Fatal("owner must report IsOwner=true")
	}
	if !owner.CanManageMembers() || !admin.CanManageMembers() {
		t.Fatal("owner/admin must be able to manage members")
	}
	if user.CanManageMembers() {
		t.Fatal("regular user must not manage members")
	}
	if !user.CanMutateRole() {
		t.Fatal("active user role should be mutable")
	}
	if !admin.CanMutateRole() {
		t.Fatal("invited admin role should be mutable")
	}
	if leftUser.CanMutateRole() {
		t.Fatal("left member role must not be mutable")
	}
}

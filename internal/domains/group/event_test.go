package group

import (
	"errors"
	"testing"
	"time"
)

func TestGroupEventTypeParseAndValid(t *testing.T) {
	cases := []struct {
		raw  string
		want GroupEventType
		ok   bool
	}{
		{raw: "member_add", want: GroupEventTypeMemberAdd, ok: true},
		{raw: "member_remove", want: GroupEventTypeMemberRemove, ok: true},
		{raw: "member_leave", want: GroupEventTypeMemberLeave, ok: true},
		{raw: "title_change", want: GroupEventTypeTitleChange, ok: true},
		{raw: "key_rotate", want: GroupEventTypeKeyRotate, ok: true},
		{raw: "bad", ok: false},
	}
	for _, tc := range cases {
		got, err := ParseGroupEventType(tc.raw)
		if tc.ok {
			if err != nil {
				t.Fatalf("expected parse success for %q, got %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("unexpected parsed type: got=%s want=%s", got, tc.want)
			}
			continue
		}
		if !errors.Is(err, ErrInvalidGroupEventType) {
			t.Fatalf("expected ErrInvalidGroupEventType for %q, got %v", tc.raw, err)
		}
	}
}

func TestValidateGroupEvent(t *testing.T) {
	now := time.Now().UTC()
	valid := GroupEvent{
		ID:         "evt-1",
		GroupID:    "group-1",
		Version:    1,
		Type:       GroupEventTypeMemberAdd,
		ActorID:    "aim1owner",
		OccurredAt: now,
		MemberID:   "aim1user",
		Role:       GroupMemberRoleUser,
	}
	if err := ValidateGroupEvent(valid); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}

	cases := []struct {
		name string
		in   GroupEvent
		err  error
	}{
		{name: "missing id", in: GroupEvent{GroupID: "g", Version: 1, Type: GroupEventTypeTitleChange, ActorID: "a", OccurredAt: now, Title: "x"}, err: ErrInvalidGroupEventID},
		{name: "missing group id", in: GroupEvent{ID: "e", Version: 1, Type: GroupEventTypeTitleChange, ActorID: "a", OccurredAt: now, Title: "x"}, err: ErrInvalidGroupID},
		{name: "zero version", in: GroupEvent{ID: "e", GroupID: "g", Version: 0, Type: GroupEventTypeTitleChange, ActorID: "a", OccurredAt: now, Title: "x"}, err: ErrInvalidGroupEventVersion},
		{name: "bad type", in: GroupEvent{ID: "e", GroupID: "g", Version: 1, Type: GroupEventType("bad"), ActorID: "a", OccurredAt: now, Title: "x"}, err: ErrInvalidGroupEventType},
		{name: "missing actor", in: GroupEvent{ID: "e", GroupID: "g", Version: 1, Type: GroupEventTypeTitleChange, OccurredAt: now, Title: "x"}, err: ErrInvalidGroupEventActorID},
		{name: "missing occurred at", in: GroupEvent{ID: "e", GroupID: "g", Version: 1, Type: GroupEventTypeTitleChange, ActorID: "a", Title: "x"}, err: ErrInvalidGroupEventPayload},
		{name: "member add missing member id", in: GroupEvent{ID: "e", GroupID: "g", Version: 1, Type: GroupEventTypeMemberAdd, ActorID: "a", OccurredAt: now, Role: GroupMemberRoleUser}, err: ErrInvalidGroupEventPayload},
		{name: "member add invalid role", in: GroupEvent{ID: "e", GroupID: "g", Version: 1, Type: GroupEventTypeMemberAdd, ActorID: "a", OccurredAt: now, MemberID: "m", Role: GroupMemberRole("bad")}, err: ErrInvalidGroupEventPayload},
		{name: "title change empty title", in: GroupEvent{ID: "e", GroupID: "g", Version: 1, Type: GroupEventTypeTitleChange, ActorID: "a", OccurredAt: now}, err: ErrInvalidGroupEventPayload},
		{name: "key rotate invalid version", in: GroupEvent{ID: "e", GroupID: "g", Version: 1, Type: GroupEventTypeKeyRotate, ActorID: "a", OccurredAt: now, KeyVersion: 0}, err: ErrInvalidGroupEventPayload},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateGroupEvent(tc.in)
			if !errors.Is(err, tc.err) {
				t.Fatalf("expected %v, got %v", tc.err, err)
			}
		})
	}
}

func TestApplyGroupEventMonotonicVersionAndIdempotency(t *testing.T) {
	state := NewGroupState(Group{
		ID:        "group-1",
		Title:     "first",
		CreatedBy: "aim1owner",
		CreatedAt: time.Now().UTC(),
	})
	now := time.Now().UTC()

	addEvent := GroupEvent{
		ID:         "evt-1",
		GroupID:    "group-1",
		Version:    1,
		Type:       GroupEventTypeMemberAdd,
		ActorID:    "aim1owner",
		OccurredAt: now,
		MemberID:   "aim1user",
		Role:       GroupMemberRoleUser,
	}
	applied, err := ApplyGroupEvent(&state, addEvent)
	if err != nil {
		t.Fatalf("apply add event failed: %v", err)
	}
	if !applied {
		t.Fatal("first apply must report applied=true")
	}
	if state.Version != 1 {
		t.Fatalf("unexpected state version: got=%d want=1", state.Version)
	}
	member, ok := state.Members["aim1user"]
	if !ok || member.Status != GroupMemberStatusInvited {
		t.Fatalf("expected invited member in state, got=%+v ok=%v", member, ok)
	}

	applied, err = ApplyGroupEvent(&state, addEvent)
	if err != nil {
		t.Fatalf("idempotent apply failed: %v", err)
	}
	if applied {
		t.Fatal("idempotent re-apply must report applied=false")
	}
	if state.Version != 1 {
		t.Fatalf("idempotent re-apply must not bump version, got=%d", state.Version)
	}

	outOfOrder := GroupEvent{
		ID:         "evt-3",
		GroupID:    "group-1",
		Version:    3,
		Type:       GroupEventTypeTitleChange,
		ActorID:    "aim1owner",
		OccurredAt: now.Add(time.Second),
		Title:      "v3",
	}
	_, err = ApplyGroupEvent(&state, outOfOrder)
	if !errors.Is(err, ErrOutOfOrderGroupEvent) {
		t.Fatalf("expected ErrOutOfOrderGroupEvent, got %v", err)
	}

	titleEvent := GroupEvent{
		ID:         "evt-2",
		GroupID:    "group-1",
		Version:    2,
		Type:       GroupEventTypeTitleChange,
		ActorID:    "aim1owner",
		OccurredAt: now.Add(2 * time.Second),
		Title:      "updated",
	}
	applied, err = ApplyGroupEvent(&state, titleEvent)
	if err != nil {
		t.Fatalf("apply title event failed: %v", err)
	}
	if !applied {
		t.Fatal("title event must report applied=true")
	}
	if state.Version != 2 {
		t.Fatalf("unexpected state version after title event: got=%d want=2", state.Version)
	}
	if state.Group.Title != "updated" {
		t.Fatalf("unexpected title: got=%q want=%q", state.Group.Title, "updated")
	}
}

func TestApplyGroupEventEventEffects(t *testing.T) {
	now := time.Now().UTC()
	state := NewGroupState(Group{
		ID:        "group-1",
		Title:     "group",
		CreatedBy: "aim1owner",
		CreatedAt: now,
	})

	events := []GroupEvent{
		{
			ID:         "evt-1",
			GroupID:    "group-1",
			Version:    1,
			Type:       GroupEventTypeMemberAdd,
			ActorID:    "aim1owner",
			OccurredAt: now,
			MemberID:   "aim1user",
			Role:       GroupMemberRoleUser,
		},
		{
			ID:         "evt-2",
			GroupID:    "group-1",
			Version:    2,
			Type:       GroupEventTypeMemberLeave,
			ActorID:    "aim1user",
			OccurredAt: now.Add(time.Second),
			MemberID:   "aim1user",
		},
		{
			ID:         "evt-3",
			GroupID:    "group-1",
			Version:    3,
			Type:       GroupEventTypeMemberRemove,
			ActorID:    "aim1owner",
			OccurredAt: now.Add(2 * time.Second),
			MemberID:   "aim1user",
		},
		{
			ID:         "evt-4",
			GroupID:    "group-1",
			Version:    4,
			Type:       GroupEventTypeKeyRotate,
			ActorID:    "aim1owner",
			OccurredAt: now.Add(3 * time.Second),
			KeyVersion: 7,
		},
	}

	for _, evt := range events {
		applied, err := ApplyGroupEvent(&state, evt)
		if err != nil {
			t.Fatalf("apply event %s failed: %v", evt.ID, err)
		}
		if !applied {
			t.Fatalf("event %s must be applied", evt.ID)
		}
	}

	member := state.Members["aim1user"]
	if member.Status != GroupMemberStatusRemoved {
		t.Fatalf("expected removed member, got %s", member.Status)
	}
	if state.LastKeyVersion != 7 {
		t.Fatalf("unexpected last key version: got=%d want=7", state.LastKeyVersion)
	}
	if state.Version != 4 {
		t.Fatalf("unexpected final state version: got=%d want=4", state.Version)
	}
}

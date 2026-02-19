package usecase

import (
	"aim-chat/go-backend/pkg/models"
	"errors"
	"testing"
	"time"
)

func TestGroupMessageFanout_ChannelRequiresAdminOrOwner(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	service := &GroupMessageFanoutService{
		States: map[string]GroupState{
			"group-1": {
				Group: Group{ID: "group-1", Title: "[channel] general"},
				Members: map[string]GroupMember{
					"actor": {MemberID: "actor", Role: GroupMemberRoleUser, Status: GroupMemberStatusActive},
				},
			},
		},
		IdentityID:     func() string { return "actor" },
		ActiveDeviceID: func() (string, error) { return "dev-1", nil },
		Now:            func() time.Time { return now },
	}

	_, err := service.SendGroupMessageFanout("group-1", "evt-1", "hello", "")
	if !errors.Is(err, ErrGroupPermissionDenied) {
		t.Fatalf("expected ErrGroupPermissionDenied, got %v", err)
	}
}

func TestGroupMessageFanout_MixedDuplicateAndDeliveredStatuses(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	recipientPendingID := DeriveRecipientMessageID("evt-1", "recipient-pending")
	recipientDeliveredID := DeriveRecipientMessageID("evt-1", "recipient-delivered")
	senderID := DeriveRecipientMessageID("evt-1", "actor")
	saved := map[string]models.Message{}
	saved[recipientPendingID] = models.Message{ID: recipientPendingID, Status: "pending"}

	service := &GroupMessageFanoutService{
		States: map[string]GroupState{
			"group-1": {
				Group: Group{ID: "group-1", Title: "general"},
				Members: map[string]GroupMember{
					"actor":               {MemberID: "actor", Role: GroupMemberRoleOwner, Status: GroupMemberStatusActive},
					"recipient-pending":   {MemberID: "recipient-pending", Role: GroupMemberRoleUser, Status: GroupMemberStatusActive},
					"recipient-delivered": {MemberID: "recipient-delivered", Role: GroupMemberRoleUser, Status: GroupMemberStatusActive},
				},
				Version: 3,
			},
		},
		IdentityID:     func() string { return "actor" },
		ActiveDeviceID: func() (string, error) { return "dev-1", nil },
		Now:            func() time.Time { return now },
		GetMessage: func(id string) (models.Message, bool) {
			m, ok := saved[id]
			return m, ok
		},
		SaveMessage: func(msg models.Message) error {
			saved[msg.ID] = msg
			return nil
		},
		PrepareAndPublish: func(msg models.Message, recipientID string, _ GroupMessageWireMeta) (string, string, error) {
			if recipientID != "recipient-delivered" {
				t.Fatalf("unexpected recipient %s", recipientID)
			}
			saved[msg.ID] = models.Message{ID: msg.ID, Status: "sent"}
			return msg.ID, "", nil
		},
	}

	result, err := service.SendGroupMessageFanout("group-1", "evt-1", "hello", "thread-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Attempted != 2 || result.Pending != 1 || result.Delivered != 1 || result.Failed != 0 {
		t.Fatalf("unexpected counters: %+v", result)
	}
	if _, ok := saved[senderID]; !ok {
		t.Fatalf("expected sender message to be persisted")
	}
	if len(result.Recipients) != 2 {
		t.Fatalf("expected 2 recipient statuses, got %d", len(result.Recipients))
	}
	if _, ok := saved[recipientDeliveredID]; !ok {
		t.Fatalf("expected delivered recipient message to be persisted")
	}
}

func TestGroupMessageFanout_SaveFailureMarksRecipientFailed(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	storageErr := errors.New("storage write failed")
	service := &GroupMessageFanoutService{
		States: map[string]GroupState{
			"group-1": {
				Group: Group{ID: "group-1", Title: "general"},
				Members: map[string]GroupMember{
					"actor":     {MemberID: "actor", Role: GroupMemberRoleOwner, Status: GroupMemberStatusActive},
					"recipient": {MemberID: "recipient", Role: GroupMemberRoleUser, Status: GroupMemberStatusActive},
				},
			},
		},
		IdentityID:     func() string { return "actor" },
		ActiveDeviceID: func() (string, error) { return "dev-1", nil },
		Now:            func() time.Time { return now },
		GetMessage:     func(string) (models.Message, bool) { return models.Message{}, false },
		SaveMessage: func(msg models.Message) error {
			if msg.ContentType == groupFanoutTransportContentType {
				return storageErr
			}
			return nil
		},
		PrepareAndPublish: func(msg models.Message, recipientID string, _ GroupMessageWireMeta) (string, string, error) {
			return msg.ID, "", nil
		},
	}

	result, err := service.SendGroupMessageFanout("group-1", "evt-1", "hello", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Failed != 1 || len(result.Recipients) != 1 || result.Recipients[0].Status != "failed" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

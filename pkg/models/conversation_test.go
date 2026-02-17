package models

import "testing"

func TestNormalizeConversationType(t *testing.T) {
	if got := NormalizeConversationType("group"); got != ConversationTypeGroup {
		t.Fatalf("expected group, got %q", got)
	}
	if got := NormalizeConversationType(" direct "); got != ConversationTypeDirect {
		t.Fatalf("expected direct, got %q", got)
	}
	if got := NormalizeConversationType("unknown"); got != ConversationTypeDirect {
		t.Fatalf("unknown type must fallback to direct, got %q", got)
	}
}

func TestNormalizeMessageConversationDefaultsToDirect(t *testing.T) {
	msg := NormalizeMessageConversation(Message{
		ID:        "m1",
		ContactID: "c1",
	})
	if msg.ConversationType != ConversationTypeDirect {
		t.Fatalf("expected direct conversation type, got %q", msg.ConversationType)
	}
	if msg.ConversationID != "c1" {
		t.Fatalf("expected conversation id to equal contact id, got %q", msg.ConversationID)
	}
}

func TestNormalizeMessageConversationPreservesGroup(t *testing.T) {
	msg := NormalizeMessageConversation(Message{
		ID:               "m2",
		ContactID:        "sender-1",
		ConversationID:   "group-1",
		ConversationType: ConversationTypeGroup,
	})
	if msg.ConversationType != ConversationTypeGroup {
		t.Fatalf("expected group conversation type, got %q", msg.ConversationType)
	}
	if msg.ConversationID != "group-1" {
		t.Fatalf("expected group conversation id, got %q", msg.ConversationID)
	}
}

package app

import (
	"errors"
	"sort"
	"strings"
	"time"

	"aim-chat/go-backend/pkg/models"
)

var ErrMessageRequestNotFound = errors.New("message request not found")

func BuildMessageRequestSummary(senderID string, messages []models.Message) (models.MessageRequest, error) {
	if len(messages) == 0 {
		return models.MessageRequest{}, ErrMessageRequestNotFound
	}
	first := messages[0]
	last := messages[0]
	for _, msg := range messages[1:] {
		if msg.Timestamp.Before(first.Timestamp) {
			first = msg
		}
		if msg.Timestamp.After(last.Timestamp) {
			last = msg
		}
	}
	return models.MessageRequest{
		SenderID:        senderID,
		FirstMessageAt:  first.Timestamp.UTC(),
		LastMessageAt:   last.Timestamp.UTC(),
		MessageCount:    len(messages),
		LastMessageID:   last.ID,
		LastContentType: last.ContentType,
		LastPreview:     MessagePreview(last.Content),
	}, nil
}

func SortMessageRequestsByRecency(requests []models.MessageRequest) {
	sort.Slice(requests, func(i, j int) bool {
		a := requests[i]
		b := requests[j]
		if a.LastMessageAt.Equal(b.LastMessageAt) {
			return a.SenderID < b.SenderID
		}
		return a.LastMessageAt.After(b.LastMessageAt)
	})
}

func CloneMessages(messages []models.Message) []models.Message {
	out := make([]models.Message, len(messages))
	copy(out, messages)
	return out
}

func CloneMessageRequestInbox(inbox map[string][]models.Message) map[string][]models.Message {
	if len(inbox) == 0 {
		return map[string][]models.Message{}
	}
	out := make(map[string][]models.Message, len(inbox))
	for senderID, thread := range inbox {
		out[senderID] = CloneMessages(thread)
	}
	return out
}

func MessagePreview(content []byte) string {
	trimmed := strings.TrimSpace(string(content))
	if len(trimmed) <= 96 {
		return trimmed
	}
	return trimmed[:96]
}

func HasMessageID(messages []models.Message, messageID string) bool {
	for _, msg := range messages {
		if msg.ID == messageID {
			return true
		}
	}
	return false
}

func NormalizeMessageTimestamp(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Now().UTC()
	}
	return ts.UTC()
}

package model

import (
	"errors"
	"strings"
)

var (
	ErrGroupNotFound                    = errors.New("group not found")
	ErrInvalidGroupTitle                = errors.New("group title is required")
	ErrGroupMembershipNotFound          = errors.New("group membership not found")
	ErrGroupPermissionDenied            = errors.New("group permission denied")
	ErrGroupCannotInviteSelf            = errors.New("cannot invite self to group")
	ErrGroupMemberBlocked               = errors.New("group member is blocked")
	ErrGroupSenderBlocked               = errors.New("group sender is blocked")
	ErrInvalidGroupMemberState          = errors.New("invalid group member state")
	ErrGroupRateLimitExceeded           = errors.New("group operation rate limit exceeded")
	ErrGroupMemberLimitExceeded         = errors.New("group member limit exceeded")
	ErrGroupPendingInvitesLimitExceeded = errors.New("group pending invites limit exceeded")
)

func NormalizeGroupID(groupID string) (string, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return "", ErrInvalidGroupID
	}
	return groupID, nil
}

func NormalizeGroupTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", ErrInvalidGroupTitle
	}
	return title, nil
}

var ErrInvalidGroupMessageContent = errors.New("group message content is required")

type GroupMessageRecipientStatus struct {
	RecipientID string `json:"recipient_id"`
	MessageID   string `json:"message_id"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	Duplicate   bool   `json:"duplicate"`
}

type GroupMessageFanoutResult struct {
	GroupID    string                        `json:"group_id"`
	EventID    string                        `json:"event_id"`
	Attempted  int                           `json:"attempted"`
	Delivered  int                           `json:"delivered"`
	Pending    int                           `json:"pending"`
	Failed     int                           `json:"failed"`
	Recipients []GroupMessageRecipientStatus `json:"recipients"`
}

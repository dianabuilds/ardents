package model

import (
	"errors"
	"strings"
	"time"
)

type GroupMemberRole string

const (
	GroupMemberRoleOwner GroupMemberRole = "owner"
	GroupMemberRoleAdmin GroupMemberRole = "admin"
	GroupMemberRoleUser  GroupMemberRole = "user"
)

type GroupMemberStatus string

const (
	GroupMemberStatusInvited GroupMemberStatus = "invited"
	GroupMemberStatusActive  GroupMemberStatus = "active"
	GroupMemberStatusLeft    GroupMemberStatus = "left"
	GroupMemberStatusRemoved GroupMemberStatus = "removed"
)

var (
	ErrInvalidGroupID                     = errors.New("invalid group id")
	ErrInvalidGroupMemberID               = errors.New("invalid group member id")
	ErrInvalidGroupMemberRole             = errors.New("invalid group member role")
	ErrInvalidGroupMemberStatus           = errors.New("invalid group member status")
	ErrInvalidGroupMemberStatusTransition = errors.New("invalid group member status transition")
)

// Group is a domain-level aggregate for group chat metadata.
type Group struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GroupMember describes member role and lifecycle state inside a group.
type GroupMember struct {
	GroupID     string            `json:"group_id"`
	MemberID    string            `json:"member_id"`
	Role        GroupMemberRole   `json:"role"`
	Status      GroupMemberStatus `json:"status"`
	InvitedAt   time.Time         `json:"invited_at"`
	ActivatedAt time.Time         `json:"activated_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

func (r GroupMemberRole) Valid() bool {
	switch r {
	case GroupMemberRoleOwner, GroupMemberRoleAdmin, GroupMemberRoleUser:
		return true
	default:
		return false
	}
}

func ParseGroupMemberRole(raw string) (GroupMemberRole, error) {
	role := GroupMemberRole(strings.TrimSpace(raw))
	if !role.Valid() {
		return "", ErrInvalidGroupMemberRole
	}
	return role, nil
}

func (s GroupMemberStatus) Valid() bool {
	switch s {
	case GroupMemberStatusInvited, GroupMemberStatusActive, GroupMemberStatusLeft, GroupMemberStatusRemoved:
		return true
	default:
		return false
	}
}

func ParseGroupMemberStatus(raw string) (GroupMemberStatus, error) {
	status := GroupMemberStatus(strings.TrimSpace(raw))
	if !status.Valid() {
		return "", ErrInvalidGroupMemberStatus
	}
	return status, nil
}

// ValidateGroupMember centralizes group member validation rules.
func ValidateGroupMember(member GroupMember) error {
	if strings.TrimSpace(member.GroupID) == "" {
		return ErrInvalidGroupID
	}
	if strings.TrimSpace(member.MemberID) == "" {
		return ErrInvalidGroupMemberID
	}
	if !member.Role.Valid() {
		return ErrInvalidGroupMemberRole
	}
	if !member.Status.Valid() {
		return ErrInvalidGroupMemberStatus
	}
	return nil
}

// ValidateGroupMemberStatusTransition checks lifecycle transitions:
// invited -> active|removed
// active  -> left|removed
// left    -> active|removed
// removed -> (terminal)
func ValidateGroupMemberStatusTransition(from, to GroupMemberStatus) error {
	if !from.Valid() || !to.Valid() {
		return ErrInvalidGroupMemberStatus
	}
	if from == to {
		return nil
	}
	switch from {
	case GroupMemberStatusInvited:
		if to == GroupMemberStatusActive || to == GroupMemberStatusRemoved {
			return nil
		}
	case GroupMemberStatusActive:
		if to == GroupMemberStatusLeft || to == GroupMemberStatusRemoved {
			return nil
		}
	case GroupMemberStatusLeft:
		if to == GroupMemberStatusActive || to == GroupMemberStatusRemoved {
			return nil
		}
	case GroupMemberStatusRemoved:
		// terminal status
	}
	return ErrInvalidGroupMemberStatusTransition
}

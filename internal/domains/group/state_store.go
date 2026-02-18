package group

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"

	"aim-chat/go-backend/internal/securestore"
)

type SnapshotStore struct {
	path   string
	secret string
}

func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{}
}

func (s *SnapshotStore) Configure(path, secret string) {
	s.path, s.secret = securestore.NormalizeStorageConfig(path, secret)
}

func (s *SnapshotStore) Bootstrap() (map[string]GroupState, map[string][]GroupEvent, error) {
	if !securestore.IsStorageConfigured(s.path, s.secret) {
		return map[string]GroupState{}, map[string][]GroupEvent{}, nil
	}
	plaintext, err := securestore.ReadDecryptedFile(s.path, s.secret)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			states := map[string]GroupState{}
			eventLog := map[string][]GroupEvent{}
			if err := s.Persist(states, eventLog); err != nil {
				return nil, nil, err
			}
			return states, eventLog, nil
		}
		return nil, nil, err
	}

	var state persistedGroupState
	if err := json.Unmarshal(plaintext, &state); err != nil {
		return nil, nil, err
	}
	if state.Version != 1 {
		return nil, nil, errors.New("group state persistence payload is invalid")
	}

	normalizedStates, normalizedEvents, err := NormalizeGroupSnapshot(state.States, state.EventLog)
	if err != nil {
		return nil, nil, err
	}
	return normalizedStates, normalizedEvents, nil
}

func (s *SnapshotStore) Persist(states map[string]GroupState, eventLog map[string][]GroupEvent) error {
	if !securestore.IsStorageConfigured(s.path, s.secret) {
		return nil
	}
	normalizedStates, normalizedEvents, err := NormalizeGroupSnapshot(states, eventLog)
	if err != nil {
		return err
	}
	state := persistedGroupState{
		Version:  1,
		States:   normalizedStates,
		EventLog: normalizedEvents,
	}
	return securestore.WriteEncryptedJSON(s.path, s.secret, state)
}

func (s *SnapshotStore) Wipe() error {
	if s.path == "" {
		return nil
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func NormalizeGroupSnapshot(
	states map[string]GroupState,
	eventLog map[string][]GroupEvent,
) (map[string]GroupState, map[string][]GroupEvent, error) {
	normalizedStates := cloneGroupStates(states)
	normalizedEvents := cloneGroupEventLog(eventLog)

	if normalizedStates == nil {
		normalizedStates = map[string]GroupState{}
	}
	if normalizedEvents == nil {
		normalizedEvents = map[string][]GroupEvent{}
	}

	for groupID, state := range normalizedStates {
		groupID = strings.TrimSpace(groupID)
		if groupID == "" {
			return nil, nil, ErrInvalidGroupID
		}
		if strings.TrimSpace(state.Group.ID) == "" || state.Group.ID != groupID {
			return nil, nil, ErrInvalidGroupID
		}
		normalizedState, err := normalizeGroupState(state)
		if err != nil {
			return nil, nil, err
		}
		normalizedStates[groupID] = normalizedState
	}

	for groupID, events := range normalizedEvents {
		groupID = strings.TrimSpace(groupID)
		if groupID == "" {
			return nil, nil, ErrInvalidGroupID
		}
		for i := range events {
			if err := ValidateGroupEvent(events[i]); err != nil {
				return nil, nil, err
			}
			if events[i].GroupID != groupID {
				return nil, nil, ErrInvalidGroupID
			}
		}
		sort.Slice(events, func(i, j int) bool {
			if events[i].Version == events[j].Version {
				return events[i].OccurredAt.Before(events[j].OccurredAt)
			}
			return events[i].Version < events[j].Version
		})
		normalizedEvents[groupID] = events
	}

	return normalizedStates, normalizedEvents, nil
}

func normalizeGroupState(state GroupState) (GroupState, error) {
	if strings.TrimSpace(state.Group.ID) == "" {
		return GroupState{}, ErrInvalidGroupID
	}
	if state.AppliedEventIDs == nil {
		state.AppliedEventIDs = make(map[string]struct{})
	}
	if state.Members == nil {
		state.Members = make(map[string]GroupMember)
	}
	for eventID := range state.AppliedEventIDs {
		if strings.TrimSpace(eventID) == "" {
			return GroupState{}, ErrInvalidGroupEventID
		}
	}
	for memberID, member := range state.Members {
		if strings.TrimSpace(memberID) == "" {
			return GroupState{}, ErrInvalidGroupMemberID
		}
		if strings.TrimSpace(member.MemberID) == "" || member.MemberID != memberID {
			return GroupState{}, ErrInvalidGroupMemberID
		}
		if member.GroupID != state.Group.ID {
			return GroupState{}, ErrInvalidGroupID
		}
		if err := ValidateGroupMember(member); err != nil {
			return GroupState{}, err
		}
	}
	return state, nil
}

func cloneGroupStates(src map[string]GroupState) map[string]GroupState {
	if src == nil {
		return nil
	}
	out := make(map[string]GroupState, len(src))
	for groupID, state := range src {
		cloned := state
		cloned.AppliedEventIDs = make(map[string]struct{}, len(state.AppliedEventIDs))
		for eventID := range state.AppliedEventIDs {
			cloned.AppliedEventIDs[eventID] = struct{}{}
		}
		cloned.Members = make(map[string]GroupMember, len(state.Members))
		for memberID, member := range state.Members {
			cloned.Members[memberID] = member
		}
		out[groupID] = cloned
	}
	return out
}

func cloneGroupEventLog(src map[string][]GroupEvent) map[string][]GroupEvent {
	if src == nil {
		return nil
	}
	out := make(map[string][]GroupEvent, len(src))
	for groupID, events := range src {
		out[groupID] = append([]GroupEvent(nil), events...)
	}
	return out
}

type persistedGroupState struct {
	Version  int                     `json:"version"`
	States   map[string]GroupState   `json:"states"`
	EventLog map[string][]GroupEvent `json:"event_log"`
}

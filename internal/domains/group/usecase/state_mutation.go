package usecase

type SnapshotPersist func(states map[string]GroupState, eventLog map[string][]GroupEvent) error

func LoadStateForActor(
	states map[string]GroupState,
	groupID,
	actorID string,
	requireActiveActor bool,
) (GroupState, error) {
	state, ok := states[groupID]
	if !ok {
		return GroupState{}, ErrGroupNotFound
	}
	normalizedActorID, err := NormalizeGroupMemberID(actorID)
	if err != nil {
		return GroupState{}, err
	}
	actor, exists := state.Members[normalizedActorID]
	if !exists {
		return GroupState{}, ErrGroupMembershipNotFound
	}
	if requireActiveActor && actor.Status != GroupMemberStatusActive {
		return GroupState{}, ErrGroupPermissionDenied
	}
	return state, nil
}

func CloneState(in GroupState) GroupState {
	out := in
	out.AppliedEventIDs = make(map[string]struct{}, len(in.AppliedEventIDs))
	for id := range in.AppliedEventIDs {
		out.AppliedEventIDs[id] = struct{}{}
	}
	out.Members = make(map[string]GroupMember, len(in.Members))
	for memberID, member := range in.Members {
		out.Members[memberID] = member
	}
	return out
}

func ApplyEventsWithRollback(
	state GroupState,
	states map[string]GroupState,
	eventLog map[string][]GroupEvent,
	persist SnapshotPersist,
	events ...GroupEvent,
) (GroupState, []GroupEvent, error) {
	if len(events) == 0 {
		return state, nil, nil
	}

	previousState := CloneState(state)
	_, hadState := states[state.Group.ID]
	previousLog := append([]GroupEvent(nil), eventLog[state.Group.ID]...)
	appliedEvents := make([]GroupEvent, 0, len(events))

	for _, event := range events {
		applied, err := ApplyGroupEvent(&state, event)
		if err != nil {
			return GroupState{}, nil, err
		}
		if applied {
			appliedEvents = append(appliedEvents, event)
		}
	}
	if len(appliedEvents) == 0 {
		return state, nil, nil
	}

	states[state.Group.ID] = state
	eventLog[state.Group.ID] = append(eventLog[state.Group.ID], appliedEvents...)

	if persist != nil {
		if err := persist(states, eventLog); err != nil {
			if hadState {
				states[state.Group.ID] = previousState
			} else {
				delete(states, state.Group.ID)
			}
			eventLog[state.Group.ID] = previousLog
			return GroupState{}, nil, err
		}
	}

	return state, appliedEvents, nil
}

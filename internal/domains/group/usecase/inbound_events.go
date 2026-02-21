package usecase

func AuthorizeInboundGroupEvent(state GroupState, event GroupEvent) error {
	actor, actorExists := state.Members[event.ActorID]
	switch event.Type {
	case GroupEventTypeMemberAdd:
		target, targetExists := state.Members[event.MemberID]
		if !actorExists {
			// bootstrap invite/initial add is handled by caller for unknown groups
			return ErrGroupPermissionDenied
		}
		if targetExists && target.Status == GroupMemberStatusInvited && event.ActorID == event.MemberID {
			return nil
		}
		if actor.Status != GroupMemberStatusActive {
			return ErrGroupPermissionDenied
		}
		if !targetExists {
			if actor.Role == GroupMemberRoleOwner || actor.Role == GroupMemberRoleAdmin {
				return nil
			}
			return ErrGroupPermissionDenied
		}
		// Role changes are owner-only.
		if target.Role != event.Role {
			if actor.Role != GroupMemberRoleOwner {
				return ErrGroupPermissionDenied
			}
			if target.Role == GroupMemberRoleOwner {
				return ErrGroupPermissionDenied
			}
		}
		return nil
	case GroupEventTypeMemberRemove:
		if !actorExists || actor.Status != GroupMemberStatusActive {
			return ErrGroupPermissionDenied
		}
		target, targetExists := state.Members[event.MemberID]
		if !targetExists {
			return ErrGroupMembershipNotFound
		}
		if actor.Role != GroupMemberRoleOwner && actor.Role != GroupMemberRoleAdmin {
			return ErrGroupPermissionDenied
		}
		if target.Role == GroupMemberRoleOwner {
			return ErrGroupPermissionDenied
		}
		if actor.Role == GroupMemberRoleAdmin && target.Role == GroupMemberRoleAdmin {
			return ErrGroupPermissionDenied
		}
		return nil
	case GroupEventTypeMemberLeave:
		if !actorExists {
			return ErrGroupPermissionDenied
		}
		if event.ActorID != event.MemberID {
			return ErrGroupPermissionDenied
		}
		return nil
	case GroupEventTypeTitleChange:
		if !actorExists || actor.Status != GroupMemberStatusActive {
			return ErrGroupPermissionDenied
		}
		if actor.Role != GroupMemberRoleOwner && actor.Role != GroupMemberRoleAdmin {
			return ErrGroupPermissionDenied
		}
		return nil
	case GroupEventTypeProfileChange:
		if !actorExists || actor.Status != GroupMemberStatusActive {
			return ErrGroupPermissionDenied
		}
		if actor.Role != GroupMemberRoleOwner && actor.Role != GroupMemberRoleAdmin {
			return ErrGroupPermissionDenied
		}
		return nil
	case GroupEventTypeKeyRotate:
		if !actorExists || actor.Status != GroupMemberStatusActive {
			return ErrGroupPermissionDenied
		}
		if actor.Role != GroupMemberRoleOwner {
			return ErrGroupPermissionDenied
		}
		return nil
	default:
		return ErrGroupPermissionDenied
	}
}

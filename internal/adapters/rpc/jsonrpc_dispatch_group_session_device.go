package rpc

import "encoding/json"

func (s *Server) dispatchGroupRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "group.create":
		result, rpcErr := callWithSingleStringParam(rawParams, -32100, func(title string) (any, error) {
			return s.service.CreateGroup(title)
		})
		return result, rpcErr, true
	case "group.get":
		result, rpcErr := callWithSingleStringParam(rawParams, -32101, func(groupID string) (any, error) {
			return s.service.GetGroup(groupID)
		})
		return result, rpcErr, true
	case "group.list":
		result, rpcErr := callWithoutParams(-32102, func() (any, error) {
			return s.service.ListGroups()
		})
		return result, rpcErr, true
	default:
		if result, rpcErr, ok := s.dispatchGroupMembershipRPC(method, rawParams); ok {
			return result, rpcErr, true
		}
		return s.dispatchGroupMessageRPC(method, rawParams)
	}
}

func (s *Server) dispatchGroupMembershipRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "group.members.list":
		result, rpcErr := callWithSingleStringParam(rawParams, -32103, func(groupID string) (any, error) {
			return s.service.ListGroupMembers(groupID)
		})
		return result, rpcErr, true
	case "group.leave":
		result, rpcErr := callWithSingleStringParam(rawParams, -32104, func(groupID string) (any, error) {
			left, err := s.service.LeaveGroup(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"left": left}, nil
		})
		return result, rpcErr, true
	case "group.update_title":
		result, rpcErr := callWithTwoStringParams(rawParams, -32105, func(groupID, title string) (any, error) {
			return s.service.UpdateGroupTitle(groupID, title)
		})
		return result, rpcErr, true
	case "group.update_profile":
		result, rpcErr := callWithFourStringParams(rawParams, -32107, func(groupID, title, description, avatar string) (any, error) {
			return s.service.UpdateGroupProfile(groupID, title, description, avatar)
		})
		return result, rpcErr, true
	case "group.delete":
		result, rpcErr := callWithSingleStringParam(rawParams, -32106, func(groupID string) (any, error) {
			deleted, err := s.service.DeleteGroup(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"deleted": deleted}, nil
		})
		return result, rpcErr, true
	case "group.invite":
		result, rpcErr := callWithTwoStringParams(rawParams, -32110, func(groupID, memberID string) (any, error) {
			return s.service.InviteToGroup(groupID, memberID)
		})
		return result, rpcErr, true
	case "group.accept_invite":
		return s.dispatchGroupAcceptInvite(rawParams)
	case "group.decline_invite":
		return s.dispatchGroupDeclineInvite(rawParams)
	case "group.remove_member":
		return s.dispatchGroupRemoveMember(rawParams)
	case "group.promote":
		result, rpcErr := callWithTwoStringParams(rawParams, -32114, func(groupID, memberID string) (any, error) {
			return s.service.PromoteGroupMember(groupID, memberID)
		})
		return result, rpcErr, true
	case "group.demote":
		result, rpcErr := callWithTwoStringParams(rawParams, -32115, func(groupID, memberID string) (any, error) {
			return s.service.DemoteGroupMember(groupID, memberID)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchGroupAcceptInvite(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithSingleStringParam(rawParams, -32111, func(groupID string) (any, error) {
		accepted, err := s.service.AcceptGroupInvite(groupID)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"accepted": accepted}, nil
	})
	return result, rpcErr, true
}

func (s *Server) dispatchGroupDeclineInvite(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithSingleStringParam(rawParams, -32112, func(groupID string) (any, error) {
		declined, err := s.service.DeclineGroupInvite(groupID)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"declined": declined}, nil
	})
	return result, rpcErr, true
}

func (s *Server) dispatchGroupRemoveMember(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithTwoStringParams(rawParams, -32113, func(groupID, memberID string) (any, error) {
		removed, err := s.service.RemoveGroupMember(groupID, memberID)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"removed": removed}, nil
	})
	return result, rpcErr, true
}

func (s *Server) dispatchGroupMessageRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "group.send":
		result, rpcErr := callWithTwoStringParams(rawParams, -32120, func(groupID, content string) (any, error) {
			return s.service.SendGroupMessage(groupID, content)
		})
		return result, rpcErr, true
	case "group.thread.send":
		result, rpcErr := callWithThreadSendParams(rawParams, -32124, func(groupID, content, threadID string) (any, error) {
			return s.service.SendGroupMessageInThread(groupID, content, threadID)
		})
		return result, rpcErr, true
	case "group.messages.list":
		result, rpcErr := callWithMessageListParams(rawParams, -32121, func(groupID string, limit, offset int) (any, error) {
			return s.service.ListGroupMessages(groupID, limit, offset)
		})
		return result, rpcErr, true
	case "group.thread.list":
		result, rpcErr := callWithThreadListParams(rawParams, -32125, func(groupID, threadID string, limit, offset int) (any, error) {
			return s.service.ListGroupMessagesByThread(groupID, threadID, limit, offset)
		})
		return result, rpcErr, true
	case "group.message.status":
		result, rpcErr := callWithTwoStringParams(rawParams, -32122, func(groupID, messageID string) (any, error) {
			return s.service.GetGroupMessageStatus(groupID, messageID)
		})
		return result, rpcErr, true
	case "group.message.delete":
		result, rpcErr := callWithTwoStringParams(rawParams, -32123, func(groupID, messageID string) (any, error) {
			if err := s.service.DeleteGroupMessage(groupID, messageID); err != nil {
				return nil, err
			}
			return map[string]bool{"deleted": true}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchSessionMessageRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "session.init":
		result, rpcErr := callWithSessionInitParams(rawParams, -32030, func(contactID string, peerPublicKey []byte) (any, error) {
			return s.service.InitSession(contactID, peerPublicKey)
		})
		return result, rpcErr, true
	case "message.send":
		result, rpcErr := callWithTwoStringParams(rawParams, -32040, func(contactID, content string) (any, error) {
			messageID, err := s.service.SendMessage(contactID, content)
			if err != nil {
				return nil, err
			}
			return map[string]string{"message_id": messageID}, nil
		})
		return result, rpcErr, true
	case "message.thread.send":
		result, rpcErr := callWithThreadSendParams(rawParams, -32046, func(contactID, content, threadID string) (any, error) {
			messageID, err := s.service.SendMessageInThread(contactID, content, threadID)
			if err != nil {
				return nil, err
			}
			return map[string]string{"message_id": messageID}, nil
		})
		return result, rpcErr, true
	case "message.edit":
		result, rpcErr := callWithMessageEditParams(rawParams, -32043, func(contactID, messageID, content string) (any, error) {
			return s.service.EditMessage(contactID, messageID, content)
		})
		return result, rpcErr, true
	case "message.delete":
		result, rpcErr := callWithTwoStringParams(rawParams, -32044, func(contactID, messageID string) (any, error) {
			if err := s.service.DeleteMessage(contactID, messageID); err != nil {
				return nil, err
			}
			return map[string]bool{"deleted": true}, nil
		})
		return result, rpcErr, true
	case "message.clear":
		result, rpcErr := callWithSingleStringParam(rawParams, -32045, func(contactID string) (any, error) {
			cleared, err := s.service.ClearMessages(contactID)
			if err != nil {
				return nil, err
			}
			return map[string]int{"cleared": cleared}, nil
		})
		return result, rpcErr, true
	case "message.list":
		result, rpcErr := callWithMessageListParams(rawParams, -32041, func(contactID string, limit, offset int) (any, error) {
			return s.service.GetMessages(contactID, limit, offset)
		})
		return result, rpcErr, true
	case "message.thread.list":
		result, rpcErr := callWithThreadListParams(rawParams, -32047, func(contactID, threadID string, limit, offset int) (any, error) {
			return s.service.GetMessagesByThread(contactID, threadID, limit, offset)
		})
		return result, rpcErr, true
	case "message.status":
		result, rpcErr := callWithSingleStringParam(rawParams, -32042, func(messageID string) (any, error) {
			return s.service.GetMessageStatus(messageID)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchDeviceRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "device.list":
		result, rpcErr := callWithoutParams(-32050, func() (any, error) {
			return s.service.ListDevices()
		})
		return result, rpcErr, true
	case "device.add":
		result, rpcErr := callWithSingleStringParam(rawParams, -32051, func(name string) (any, error) {
			return s.service.AddDevice(name)
		})
		return result, rpcErr, true
	case "device.revoke":
		result, rpcErr := callWithSingleStringParamAndErrorMapper(rawParams, mapDeviceRevokeRPCError, func(deviceID string) (any, error) {
			return s.service.RevokeDevice(deviceID)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

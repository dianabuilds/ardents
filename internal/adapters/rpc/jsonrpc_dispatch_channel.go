package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	groupdomain "aim-chat/go-backend/internal/domains/group"
)

var channelGroupTitlePrefixRe = regexp.MustCompile(`^\[channel(?::(public|private))?]\s*`)

func normalizeChannelVisibility(visibility string) string {
	if strings.EqualFold(strings.TrimSpace(visibility), "private") {
		return "private"
	}
	return "public"
}

func encodeChannelGroupTitle(name, visibility string) string {
	return fmt.Sprintf("[channel:%s] %s", normalizeChannelVisibility(visibility), strings.TrimSpace(name))
}

func isChannelGroupTitle(title string) bool {
	return channelGroupTitlePrefixRe.MatchString(strings.TrimSpace(title))
}

func (s *Server) ensureChannelGroup(groupID string) (groupdomain.Group, error) {
	group, err := s.service.GetGroup(groupID)
	if err != nil {
		return groupdomain.Group{}, err
	}
	if !isChannelGroupTitle(group.Title) {
		return groupdomain.Group{}, errors.New("group not found")
	}
	return group, nil
}

func (s *Server) dispatchChannelRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "channel.create":
		result, rpcErr := callWithChannelCreateParams(rawParams, -32200, func(name, visibility, description string) (any, error) {
			title := encodeChannelGroupTitle(name, visibility)
			group, err := s.service.CreateGroup(title)
			if err != nil {
				return nil, err
			}
			description = strings.TrimSpace(description)
			if description == "" {
				return group, nil
			}
			return s.service.UpdateGroupProfile(group.ID, group.Title, description, group.Avatar)
		})
		return result, rpcErr, true
	case "channel.get":
		result, rpcErr := callWithSingleStringParam(rawParams, -32201, func(groupID string) (any, error) {
			return s.ensureChannelGroup(groupID)
		})
		return result, rpcErr, true
	case "channel.list":
		result, rpcErr := callWithoutParams(-32202, func() (any, error) {
			groups, err := s.service.ListGroups()
			if err != nil {
				return nil, err
			}
			out := make([]groupdomain.Group, 0, len(groups))
			for _, group := range groups {
				if isChannelGroupTitle(group.Title) {
					out = append(out, group)
				}
			}
			return out, nil
		})
		return result, rpcErr, true
	default:
		if result, rpcErr, ok := s.dispatchChannelMembershipRPC(method, rawParams); ok {
			return result, rpcErr, true
		}
		return s.dispatchChannelMessageRPC(method, rawParams)
	}
}

func (s *Server) dispatchChannelMembershipRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "channel.members.list":
		result, rpcErr := callWithSingleStringParam(rawParams, -32203, func(groupID string) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			return s.service.ListGroupMembers(groupID)
		})
		return result, rpcErr, true
	case "channel.rename":
		result, rpcErr := callWithTwoStringParams(rawParams, -32205, func(groupID, name string) (any, error) {
			channelGroup, err := s.ensureChannelGroup(groupID)
			if err != nil {
				return nil, err
			}
			visibility := "public"
			if matches := channelGroupTitlePrefixRe.FindStringSubmatch(strings.TrimSpace(channelGroup.Title)); len(matches) > 1 && strings.TrimSpace(matches[1]) != "" {
				visibility = normalizeChannelVisibility(matches[1])
			}
			return s.service.UpdateGroupTitle(groupID, encodeChannelGroupTitle(name, visibility))
		})
		return result, rpcErr, true
	case "channel.update_profile":
		result, rpcErr := callWithFourStringParams(rawParams, -32207, func(groupID, name, description, avatar string) (any, error) {
			channelGroup, err := s.ensureChannelGroup(groupID)
			if err != nil {
				return nil, err
			}
			visibility := "public"
			if matches := channelGroupTitlePrefixRe.FindStringSubmatch(strings.TrimSpace(channelGroup.Title)); len(matches) > 1 && strings.TrimSpace(matches[1]) != "" {
				visibility = normalizeChannelVisibility(matches[1])
			}
			return s.service.UpdateGroupProfile(groupID, encodeChannelGroupTitle(name, visibility), description, avatar)
		})
		return result, rpcErr, true
	case "channel.delete":
		result, rpcErr := callWithSingleStringParam(rawParams, -32206, func(groupID string) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			deleted, err := s.service.DeleteGroup(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"deleted": deleted}, nil
		})
		return result, rpcErr, true
	case "channel.leave":
		return s.dispatchChannelLeave(rawParams)
	case "channel.invite":
		return s.dispatchChannelInvite(rawParams)
	case "channel.accept_invite":
		return s.dispatchChannelAcceptInvite(rawParams)
	case "channel.decline_invite":
		return s.dispatchChannelDeclineInvite(rawParams)
	case "channel.remove_member":
		return s.dispatchChannelRemoveMember(rawParams)
	case "channel.promote":
		return s.dispatchChannelPromote(rawParams)
	case "channel.demote":
		return s.dispatchChannelDemote(rawParams)
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchChannelLeave(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithSingleStringParam(rawParams, -32204, func(groupID string) (any, error) {
		if _, err := s.ensureChannelGroup(groupID); err != nil {
			return nil, err
		}
		left, err := s.service.LeaveGroup(groupID)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"left": left}, nil
	})
	return result, rpcErr, true
}

func (s *Server) dispatchChannelInvite(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithTwoStringParams(rawParams, -32210, func(groupID, memberID string) (any, error) {
		if _, err := s.ensureChannelGroup(groupID); err != nil {
			return nil, err
		}
		return s.service.InviteToGroup(groupID, memberID)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchChannelAcceptInvite(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithSingleStringParam(rawParams, -32211, func(groupID string) (any, error) {
		if _, err := s.ensureChannelGroup(groupID); err != nil {
			return nil, err
		}
		accepted, err := s.service.AcceptGroupInvite(groupID)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"accepted": accepted}, nil
	})
	return result, rpcErr, true
}

func (s *Server) dispatchChannelDeclineInvite(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithSingleStringParam(rawParams, -32212, func(groupID string) (any, error) {
		if _, err := s.ensureChannelGroup(groupID); err != nil {
			return nil, err
		}
		declined, err := s.service.DeclineGroupInvite(groupID)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"declined": declined}, nil
	})
	return result, rpcErr, true
}

func (s *Server) dispatchChannelRemoveMember(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithTwoStringParams(rawParams, -32213, func(groupID, memberID string) (any, error) {
		if _, err := s.ensureChannelGroup(groupID); err != nil {
			return nil, err
		}
		removed, err := s.service.RemoveGroupMember(groupID, memberID)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"removed": removed}, nil
	})
	return result, rpcErr, true
}

func (s *Server) dispatchChannelPromote(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithTwoStringParams(rawParams, -32214, func(groupID, memberID string) (any, error) {
		if _, err := s.ensureChannelGroup(groupID); err != nil {
			return nil, err
		}
		return s.service.PromoteGroupMember(groupID, memberID)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchChannelDemote(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithTwoStringParams(rawParams, -32215, func(groupID, memberID string) (any, error) {
		if _, err := s.ensureChannelGroup(groupID); err != nil {
			return nil, err
		}
		return s.service.DemoteGroupMember(groupID, memberID)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchChannelMessageRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "channel.send":
		result, rpcErr := callWithTwoStringParams(rawParams, -32220, func(groupID, content string) (any, error) {
			return s.service.SendGroupMessage(groupID, content)
		})
		return result, rpcErr, true
	case "channel.thread.send":
		result, rpcErr := callWithThreadSendParams(rawParams, -32224, func(groupID, content, threadID string) (any, error) {
			return s.service.SendGroupMessageInThread(groupID, content, threadID)
		})
		return result, rpcErr, true
	case "channel.messages.list":
		return s.dispatchChannelMessagesList(rawParams)
	case "channel.thread.list":
		return s.dispatchChannelThreadList(rawParams)
	case "channel.message.status":
		return s.dispatchChannelMessageStatus(rawParams)
	case "channel.message.delete":
		return s.dispatchChannelMessageDelete(rawParams)
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchChannelMessagesList(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithMessageListParams(rawParams, -32221, func(groupID string, limit, offset int) (any, error) {
		if _, err := s.ensureChannelGroup(groupID); err != nil {
			return nil, err
		}
		return s.service.ListGroupMessages(groupID, limit, offset)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchChannelThreadList(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithThreadListParams(rawParams, -32225, func(groupID, threadID string, limit, offset int) (any, error) {
		if _, err := s.ensureChannelGroup(groupID); err != nil {
			return nil, err
		}
		return s.service.ListGroupMessagesByThread(groupID, threadID, limit, offset)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchChannelMessageStatus(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithTwoStringParams(rawParams, -32222, func(groupID, messageID string) (any, error) {
		if _, err := s.ensureChannelGroup(groupID); err != nil {
			return nil, err
		}
		return s.service.GetGroupMessageStatus(groupID, messageID)
	})
	return result, rpcErr, true
}

func (s *Server) dispatchChannelMessageDelete(rawParams json.RawMessage) (any, *rpcError, bool) {
	result, rpcErr := callWithTwoStringParams(rawParams, -32223, func(groupID, messageID string) (any, error) {
		if _, err := s.ensureChannelGroup(groupID); err != nil {
			return nil, err
		}
		if err := s.service.DeleteGroupMessage(groupID, messageID); err != nil {
			return nil, err
		}
		return map[string]bool{"deleted": true}, nil
	})
	return result, rpcErr, true
}

func callWithChannelCreateParams(rawParams json.RawMessage, serviceErrCode int, call func(name, visibility, description string) (any, error)) (any, *rpcError) {
	name, visibility, description, err := decodeChannelCreateParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	result, err := call(name, visibility, description)
	if err != nil {
		return nil, rpcServiceError(serviceErrCode, err)
	}
	return result, nil
}

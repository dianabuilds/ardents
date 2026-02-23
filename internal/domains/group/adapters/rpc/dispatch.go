package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"aim-chat/go-backend/internal/domains/contracts"
	groupdomain "aim-chat/go-backend/internal/domains/group"
	"aim-chat/go-backend/internal/domains/rpckit"
)

const (
	maxMessageListLimit  = 1000
	maxMessageListOffset = 1_000_000
)

var channelGroupTitlePrefixRe = regexp.MustCompile(`^\[channel(?::(public|private))?]\s*`)

func Dispatch(service contracts.DaemonService, method string, rawParams json.RawMessage) (any, *rpckit.Error, bool) {
	if result, rpcErr, ok := dispatchGroupRPC(service, method, rawParams); ok {
		return result, rpcErr, true
	}
	return dispatchChannelRPC(service, method, rawParams)
}

func dispatchGroupRPC(service contracts.DaemonService, method string, rawParams json.RawMessage) (any, *rpckit.Error, bool) {
	switch method {
	case "group.create":
		result, rpcErr := callWithSingleStringParam(rawParams, -32100, func(title string) (any, error) {
			return service.CreateGroup(title)
		})
		return result, rpcErr, true
	case "group.get":
		result, rpcErr := callWithSingleStringParam(rawParams, -32101, func(groupID string) (any, error) {
			return service.GetGroup(groupID)
		})
		return result, rpcErr, true
	case "group.list":
		result, rpcErr := callWithoutParams(-32102, func() (any, error) {
			return service.ListGroups()
		})
		return result, rpcErr, true
	case "group.members.list":
		result, rpcErr := callWithSingleStringParam(rawParams, -32103, func(groupID string) (any, error) {
			return service.ListGroupMembers(groupID)
		})
		return result, rpcErr, true
	case "group.leave":
		result, rpcErr := callWithSingleStringParam(rawParams, -32104, func(groupID string) (any, error) {
			left, err := service.LeaveGroup(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"left": left}, nil
		})
		return result, rpcErr, true
	case "group.update_title":
		result, rpcErr := callWithTwoStringParams(rawParams, -32105, func(groupID, title string) (any, error) {
			return service.UpdateGroupTitle(groupID, title)
		})
		return result, rpcErr, true
	case "group.update_profile":
		result, rpcErr := callWithFourStringParams(rawParams, -32107, func(groupID, title, description, avatar string) (any, error) {
			return service.UpdateGroupProfile(groupID, title, description, avatar)
		})
		return result, rpcErr, true
	case "group.delete":
		result, rpcErr := callWithSingleStringParam(rawParams, -32106, func(groupID string) (any, error) {
			deleted, err := service.DeleteGroup(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"deleted": deleted}, nil
		})
		return result, rpcErr, true
	case "group.invite":
		result, rpcErr := callWithTwoStringParams(rawParams, -32110, func(groupID, memberID string) (any, error) {
			return service.InviteToGroup(groupID, memberID)
		})
		return result, rpcErr, true
	case "group.accept_invite":
		result, rpcErr := callWithSingleStringParam(rawParams, -32111, func(groupID string) (any, error) {
			accepted, err := service.AcceptGroupInvite(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"accepted": accepted}, nil
		})
		return result, rpcErr, true
	case "group.decline_invite":
		result, rpcErr := callWithSingleStringParam(rawParams, -32112, func(groupID string) (any, error) {
			declined, err := service.DeclineGroupInvite(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"declined": declined}, nil
		})
		return result, rpcErr, true
	case "group.remove_member":
		result, rpcErr := callWithTwoStringParams(rawParams, -32113, func(groupID, memberID string) (any, error) {
			removed, err := service.RemoveGroupMember(groupID, memberID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"removed": removed}, nil
		})
		return result, rpcErr, true
	case "group.promote":
		result, rpcErr := callWithTwoStringParams(rawParams, -32114, func(groupID, memberID string) (any, error) {
			return service.PromoteGroupMember(groupID, memberID)
		})
		return result, rpcErr, true
	case "group.demote":
		result, rpcErr := callWithTwoStringParams(rawParams, -32115, func(groupID, memberID string) (any, error) {
			return service.DemoteGroupMember(groupID, memberID)
		})
		return result, rpcErr, true
	case "group.send":
		result, rpcErr := callWithTwoStringParams(rawParams, -32120, func(groupID, content string) (any, error) {
			return service.SendGroupMessage(groupID, content)
		})
		return result, rpcErr, true
	case "group.thread.send":
		result, rpcErr := callWithThreadSendParams(rawParams, -32124, func(groupID, content, threadID string) (any, error) {
			return service.SendGroupMessageInThread(groupID, content, threadID)
		})
		return result, rpcErr, true
	case "group.messages.list":
		result, rpcErr := callWithMessageListParams(rawParams, -32121, func(groupID string, limit, offset int) (any, error) {
			return service.ListGroupMessages(groupID, limit, offset)
		})
		return result, rpcErr, true
	case "group.thread.list":
		result, rpcErr := callWithThreadListParams(rawParams, -32125, func(groupID, threadID string, limit, offset int) (any, error) {
			return service.ListGroupMessagesByThread(groupID, threadID, limit, offset)
		})
		return result, rpcErr, true
	case "group.message.status":
		result, rpcErr := callWithTwoStringParams(rawParams, -32122, func(groupID, messageID string) (any, error) {
			return service.GetGroupMessageStatus(groupID, messageID)
		})
		return result, rpcErr, true
	case "group.message.delete":
		result, rpcErr := callWithTwoStringParams(rawParams, -32123, func(groupID, messageID string) (any, error) {
			if err := service.DeleteGroupMessage(groupID, messageID); err != nil {
				return nil, err
			}
			return map[string]bool{"deleted": true}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func dispatchChannelRPC(service contracts.DaemonService, method string, rawParams json.RawMessage) (any, *rpckit.Error, bool) {
	switch method {
	case "channel.create":
		result, rpcErr := callWithChannelCreateParams(rawParams, -32200, func(name, visibility, description string) (any, error) {
			title := encodeChannelGroupTitle(name, visibility)
			group, err := service.CreateGroup(title)
			if err != nil {
				return nil, err
			}
			description = strings.TrimSpace(description)
			if description == "" {
				return group, nil
			}
			return service.UpdateGroupProfile(group.ID, group.Title, description, group.Avatar)
		})
		return result, rpcErr, true
	case "channel.get":
		result, rpcErr := callWithSingleStringParam(rawParams, -32201, func(groupID string) (any, error) {
			return ensureChannelGroup(service, groupID)
		})
		return result, rpcErr, true
	case "channel.list":
		result, rpcErr := callWithoutParams(-32202, func() (any, error) {
			groups, err := service.ListGroups()
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
	case "channel.members.list":
		result, rpcErr := callWithSingleStringParam(rawParams, -32203, func(groupID string) (any, error) {
			if _, err := ensureChannelGroup(service, groupID); err != nil {
				return nil, err
			}
			return service.ListGroupMembers(groupID)
		})
		return result, rpcErr, true
	case "channel.rename":
		result, rpcErr := callWithTwoStringParams(rawParams, -32205, func(groupID, name string) (any, error) {
			channelGroup, err := ensureChannelGroup(service, groupID)
			if err != nil {
				return nil, err
			}
			visibility := channelVisibilityFromTitle(channelGroup.Title)
			return service.UpdateGroupTitle(groupID, encodeChannelGroupTitle(name, visibility))
		})
		return result, rpcErr, true
	case "channel.update_profile":
		result, rpcErr := callWithFourStringParams(rawParams, -32207, func(groupID, name, description, avatar string) (any, error) {
			channelGroup, err := ensureChannelGroup(service, groupID)
			if err != nil {
				return nil, err
			}
			visibility := channelVisibilityFromTitle(channelGroup.Title)
			return service.UpdateGroupProfile(groupID, encodeChannelGroupTitle(name, visibility), description, avatar)
		})
		return result, rpcErr, true
	case "channel.delete":
		result, rpcErr := callWithSingleStringParam(rawParams, -32206, func(groupID string) (any, error) {
			if _, err := ensureChannelGroup(service, groupID); err != nil {
				return nil, err
			}
			deleted, err := service.DeleteGroup(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"deleted": deleted}, nil
		})
		return result, rpcErr, true
	case "channel.leave":
		result, rpcErr := callWithSingleStringParam(rawParams, -32204, func(groupID string) (any, error) {
			if _, err := ensureChannelGroup(service, groupID); err != nil {
				return nil, err
			}
			left, err := service.LeaveGroup(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"left": left}, nil
		})
		return result, rpcErr, true
	case "channel.invite":
		result, rpcErr := callWithTwoStringParams(rawParams, -32210, func(groupID, memberID string) (any, error) {
			if _, err := ensureChannelGroup(service, groupID); err != nil {
				return nil, err
			}
			return service.InviteToGroup(groupID, memberID)
		})
		return result, rpcErr, true
	case "channel.accept_invite":
		result, rpcErr := callWithSingleStringParam(rawParams, -32211, func(groupID string) (any, error) {
			if _, err := ensureChannelGroup(service, groupID); err != nil {
				return nil, err
			}
			accepted, err := service.AcceptGroupInvite(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"accepted": accepted}, nil
		})
		return result, rpcErr, true
	case "channel.decline_invite":
		result, rpcErr := callWithSingleStringParam(rawParams, -32212, func(groupID string) (any, error) {
			if _, err := ensureChannelGroup(service, groupID); err != nil {
				return nil, err
			}
			declined, err := service.DeclineGroupInvite(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"declined": declined}, nil
		})
		return result, rpcErr, true
	case "channel.remove_member":
		result, rpcErr := callWithTwoStringParams(rawParams, -32213, func(groupID, memberID string) (any, error) {
			if _, err := ensureChannelGroup(service, groupID); err != nil {
				return nil, err
			}
			removed, err := service.RemoveGroupMember(groupID, memberID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"removed": removed}, nil
		})
		return result, rpcErr, true
	case "channel.promote":
		result, rpcErr := callWithTwoStringParams(rawParams, -32214, func(groupID, memberID string) (any, error) {
			if _, err := ensureChannelGroup(service, groupID); err != nil {
				return nil, err
			}
			return service.PromoteGroupMember(groupID, memberID)
		})
		return result, rpcErr, true
	case "channel.demote":
		result, rpcErr := callWithTwoStringParams(rawParams, -32215, func(groupID, memberID string) (any, error) {
			if _, err := ensureChannelGroup(service, groupID); err != nil {
				return nil, err
			}
			return service.DemoteGroupMember(groupID, memberID)
		})
		return result, rpcErr, true
	case "channel.send":
		result, rpcErr := callWithTwoStringParams(rawParams, -32220, func(groupID, content string) (any, error) {
			return service.SendGroupMessage(groupID, content)
		})
		return result, rpcErr, true
	case "channel.thread.send":
		result, rpcErr := callWithThreadSendParams(rawParams, -32224, func(groupID, content, threadID string) (any, error) {
			return service.SendGroupMessageInThread(groupID, content, threadID)
		})
		return result, rpcErr, true
	case "channel.messages.list":
		result, rpcErr := callWithMessageListParams(rawParams, -32221, func(groupID string, limit, offset int) (any, error) {
			if _, err := ensureChannelGroup(service, groupID); err != nil {
				return nil, err
			}
			return service.ListGroupMessages(groupID, limit, offset)
		})
		return result, rpcErr, true
	case "channel.thread.list":
		result, rpcErr := callWithThreadListParams(rawParams, -32225, func(groupID, threadID string, limit, offset int) (any, error) {
			if _, err := ensureChannelGroup(service, groupID); err != nil {
				return nil, err
			}
			return service.ListGroupMessagesByThread(groupID, threadID, limit, offset)
		})
		return result, rpcErr, true
	case "channel.message.status":
		result, rpcErr := callWithTwoStringParams(rawParams, -32222, func(groupID, messageID string) (any, error) {
			if _, err := ensureChannelGroup(service, groupID); err != nil {
				return nil, err
			}
			return service.GetGroupMessageStatus(groupID, messageID)
		})
		return result, rpcErr, true
	case "channel.message.delete":
		result, rpcErr := callWithTwoStringParams(rawParams, -32223, func(groupID, messageID string) (any, error) {
			if _, err := ensureChannelGroup(service, groupID); err != nil {
				return nil, err
			}
			if err := service.DeleteGroupMessage(groupID, messageID); err != nil {
				return nil, err
			}
			return map[string]bool{"deleted": true}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func callWithoutParams(serviceErrCode int, call func() (any, error)) (any, *rpckit.Error) {
	result, err := call()
	if err != nil {
		return nil, rpckit.ServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithSingleStringParam(rawParams json.RawMessage, serviceErrCode int, call func(string) (any, error)) (any, *rpckit.Error) {
	param, err := decodeSingleStringParam(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, err := call(param)
	if err != nil {
		return nil, rpckit.ServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithTwoStringParams(rawParams json.RawMessage, serviceErrCode int, call func(string, string) (any, error)) (any, *rpckit.Error) {
	a, b, err := decodeTwoStringParams(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, err := call(a, b)
	if err != nil {
		return nil, rpckit.ServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithFourStringParams(rawParams json.RawMessage, serviceErrCode int, call func(string, string, string, string) (any, error)) (any, *rpckit.Error) {
	a, b, c, d, err := decodeFourStringParams(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, callErr := call(a, b, c, d)
	if callErr != nil {
		return nil, rpckit.ServiceError(serviceErrCode, callErr)
	}
	return result, nil
}

func callWithMessageListParams(
	rawParams json.RawMessage,
	serviceErrCode int,
	call func(contactID string, limit, offset int) (any, error),
) (any, *rpckit.Error) {
	contactID, limit, offset, err := decodeMessageListParams(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, err := call(contactID, limit, offset)
	if err != nil {
		return nil, rpckit.ServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithThreadSendParams(
	rawParams json.RawMessage,
	serviceErrCode int,
	call func(targetID, content, threadID string) (any, error),
) (any, *rpckit.Error) {
	targetID, content, threadID, err := decodeThreadSendParams(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, err := call(targetID, content, threadID)
	if err != nil {
		return nil, rpckit.ServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithThreadListParams(
	rawParams json.RawMessage,
	serviceErrCode int,
	call func(targetID, threadID string, limit, offset int) (any, error),
) (any, *rpckit.Error) {
	targetID, threadID, limit, offset, err := decodeThreadListParams(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, err := call(targetID, threadID, limit, offset)
	if err != nil {
		return nil, rpckit.ServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithChannelCreateParams(rawParams json.RawMessage, serviceErrCode int, call func(name, visibility, description string) (any, error)) (any, *rpckit.Error) {
	name, visibility, description, err := decodeChannelCreateParams(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, err := call(name, visibility, description)
	if err != nil {
		return nil, rpckit.ServiceError(serviceErrCode, err)
	}
	return result, nil
}

func decodeSingleStringParam(raw json.RawMessage) (string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 && arr[0] != "" {
		return arr[0], nil
	}
	return "", errors.New("invalid params")
}

func decodeTwoStringParams(raw json.RawMessage) (string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 2 && arr[0] != "" && arr[1] != "" {
		return arr[0], arr[1], nil
	}
	return "", "", errors.New("invalid params")
}

func decodeFourStringParams(raw json.RawMessage) (string, string, string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 4 || arr[0] == "" || arr[1] == "" {
		return "", "", "", "", errors.New("invalid params")
	}
	return arr[0], arr[1], arr[2], arr[3], nil
}

func decodeMessageListParams(raw json.RawMessage) (string, int, int, error) {
	var arr []any
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 3 {
		return "", 0, 0, errors.New("invalid params")
	}
	contactID, ok := arr[0].(string)
	if !ok || contactID == "" {
		return "", 0, 0, errors.New("invalid params")
	}
	limit, err := decodeStrictNonNegativeInt(arr[1])
	if err != nil {
		return "", 0, 0, errors.New("invalid params")
	}
	offset, err := decodeStrictNonNegativeInt(arr[2])
	if err != nil {
		return "", 0, 0, errors.New("invalid params")
	}
	if limit > maxMessageListLimit || offset > maxMessageListOffset {
		return "", 0, 0, errors.New("invalid params")
	}
	return contactID, limit, offset, nil
}

func decodeThreadSendParams(raw json.RawMessage) (string, string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 3 {
		return "", "", "", errors.New("invalid params")
	}
	targetID := strings.TrimSpace(arr[0])
	content := strings.TrimSpace(arr[1])
	threadID := strings.TrimSpace(arr[2])
	if targetID == "" || content == "" || threadID == "" {
		return "", "", "", errors.New("invalid params")
	}
	return targetID, content, threadID, nil
}

func decodeThreadListParams(raw json.RawMessage) (string, string, int, int, error) {
	var arr []any
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 4 {
		return "", "", 0, 0, errors.New("invalid params")
	}
	targetID, ok := arr[0].(string)
	if !ok || strings.TrimSpace(targetID) == "" {
		return "", "", 0, 0, errors.New("invalid params")
	}
	threadID, ok := arr[1].(string)
	if !ok || strings.TrimSpace(threadID) == "" {
		return "", "", 0, 0, errors.New("invalid params")
	}
	limit, err := decodeStrictNonNegativeInt(arr[2])
	if err != nil {
		return "", "", 0, 0, errors.New("invalid params")
	}
	offset, err := decodeStrictNonNegativeInt(arr[3])
	if err != nil {
		return "", "", 0, 0, errors.New("invalid params")
	}
	if limit > maxMessageListLimit || offset > maxMessageListOffset {
		return "", "", 0, 0, errors.New("invalid params")
	}
	return strings.TrimSpace(targetID), strings.TrimSpace(threadID), limit, offset, nil
}

func decodeStrictNonNegativeInt(raw any) (int, error) {
	v, ok := raw.(float64)
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("invalid params")
	}
	if v < 0 || math.Trunc(v) != v {
		return 0, errors.New("invalid params")
	}
	maxInt := float64(^uint(0) >> 1)
	if v > maxInt {
		return 0, errors.New("invalid params")
	}
	return int(v), nil
}

func decodeChannelCreateParams(raw json.RawMessage) (string, string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		switch len(arr) {
		case 1:
			name := strings.TrimSpace(arr[0])
			if name == "" {
				return "", "", "", errors.New("invalid params")
			}
			return name, "public", "", nil
		case 2:
			name := strings.TrimSpace(arr[0])
			visibility := strings.TrimSpace(arr[1])
			if name == "" || visibility == "" {
				return "", "", "", errors.New("invalid params")
			}
			return name, visibility, "", nil
		case 3:
			name := strings.TrimSpace(arr[0])
			visibility := strings.TrimSpace(arr[1])
			description := strings.TrimSpace(arr[2])
			if name == "" || visibility == "" {
				return "", "", "", errors.New("invalid params")
			}
			return name, visibility, description, nil
		}
	}

	var payload struct {
		Name        string `json:"name"`
		Visibility  string `json:"visibility"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", "", errors.New("invalid params")
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		return "", "", "", errors.New("invalid params")
	}
	visibility := strings.TrimSpace(payload.Visibility)
	if visibility == "" {
		visibility = "public"
	}
	return name, visibility, strings.TrimSpace(payload.Description), nil
}

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

func channelVisibilityFromTitle(title string) string {
	matches := channelGroupTitlePrefixRe.FindStringSubmatch(strings.TrimSpace(title))
	if len(matches) > 1 && strings.TrimSpace(matches[1]) != "" {
		return normalizeChannelVisibility(matches[1])
	}
	return "public"
}

func ensureChannelGroup(service contracts.DaemonService, groupID string) (groupdomain.Group, error) {
	group, err := service.GetGroup(groupID)
	if err != nil {
		return groupdomain.Group{}, err
	}
	if !isChannelGroupTitle(group.Title) {
		return groupdomain.Group{}, errors.New("group not found")
	}
	return group, nil
}

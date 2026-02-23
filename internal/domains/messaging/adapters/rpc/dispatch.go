package rpc

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/internal/domains/rpckit"
)

const (
	maxMessageListLimit  = 1000
	maxMessageListOffset = 1_000_000
)

func Dispatch(service contracts.DaemonService, method string, rawParams json.RawMessage) (any, *rpckit.Error, bool) {
	switch method {
	case "session.init":
		result, rpcErr := callWithSessionInitParams(rawParams, -32030, func(contactID string, peerPublicKey []byte) (any, error) {
			return service.InitSession(contactID, peerPublicKey)
		})
		return result, rpcErr, true
	case "message.send":
		result, rpcErr := callWithTwoStringParams(rawParams, -32040, func(contactID, content string) (any, error) {
			messageID, err := service.SendMessage(contactID, content)
			if err != nil {
				return nil, err
			}
			return map[string]string{"message_id": messageID}, nil
		})
		return result, rpcErr, true
	case "message.thread.send":
		result, rpcErr := callWithThreadSendParams(rawParams, -32046, func(contactID, content, threadID string) (any, error) {
			messageID, err := service.SendMessageInThread(contactID, content, threadID)
			if err != nil {
				return nil, err
			}
			return map[string]string{"message_id": messageID}, nil
		})
		return result, rpcErr, true
	case "message.edit":
		result, rpcErr := callWithMessageEditParams(rawParams, -32043, func(contactID, messageID, content string) (any, error) {
			return service.EditMessage(contactID, messageID, content)
		})
		return result, rpcErr, true
	case "message.delete":
		result, rpcErr := callWithTwoStringParams(rawParams, -32044, func(contactID, messageID string) (any, error) {
			if err := service.DeleteMessage(contactID, messageID); err != nil {
				return nil, err
			}
			return map[string]bool{"deleted": true}, nil
		})
		return result, rpcErr, true
	case "message.clear":
		result, rpcErr := callWithSingleStringParam(rawParams, -32045, func(contactID string) (any, error) {
			cleared, err := service.ClearMessages(contactID)
			if err != nil {
				return nil, err
			}
			return map[string]int{"cleared": cleared}, nil
		})
		return result, rpcErr, true
	case "message.list":
		result, rpcErr := callWithMessageListParams(rawParams, -32041, func(contactID string, limit, offset int) (any, error) {
			return service.GetMessages(contactID, limit, offset)
		})
		return result, rpcErr, true
	case "message.thread.list":
		result, rpcErr := callWithThreadListParams(rawParams, -32047, func(contactID, threadID string, limit, offset int) (any, error) {
			return service.GetMessagesByThread(contactID, threadID, limit, offset)
		})
		return result, rpcErr, true
	case "message.status":
		result, rpcErr := callWithSingleStringParam(rawParams, -32042, func(messageID string) (any, error) {
			return service.GetMessageStatus(messageID)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
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

func callWithSessionInitParams(
	rawParams json.RawMessage,
	serviceErrCode int,
	call func(contactID string, peerPublicKey []byte) (any, error),
) (any, *rpckit.Error) {
	contactID, peerPublicKey, err := decodeSessionInitParams(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, err := call(contactID, peerPublicKey)
	if err != nil {
		return nil, rpckit.ServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithMessageEditParams(
	rawParams json.RawMessage,
	serviceErrCode int,
	call func(contactID, messageID, content string) (any, error),
) (any, *rpckit.Error) {
	contactID, messageID, content, err := decodeMessageEditParams(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, err := call(contactID, messageID, content)
	if err != nil {
		return nil, rpckit.ServiceError(serviceErrCode, err)
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

func decodeSingleStringParam(raw json.RawMessage) (string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 && strings.TrimSpace(arr[0]) != "" {
		return arr[0], nil
	}
	return "", errors.New("invalid params")
}

func decodeTwoStringParams(raw json.RawMessage) (string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 2 && strings.TrimSpace(arr[0]) != "" && strings.TrimSpace(arr[1]) != "" {
		return arr[0], arr[1], nil
	}
	return "", "", errors.New("invalid params")
}

func decodeSessionInitParams(raw json.RawMessage) (string, []byte, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 2 || strings.TrimSpace(arr[0]) == "" || strings.TrimSpace(arr[1]) == "" {
		return "", nil, errors.New("invalid params")
	}
	peerKey, err := base64.StdEncoding.DecodeString(arr[1])
	if err != nil {
		return "", nil, errors.New("invalid params")
	}
	return arr[0], peerKey, nil
}

func decodeMessageEditParams(raw json.RawMessage) (string, string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 3 {
		return "", "", "", errors.New("invalid params")
	}
	if strings.TrimSpace(arr[0]) == "" || strings.TrimSpace(arr[1]) == "" || strings.TrimSpace(arr[2]) == "" {
		return "", "", "", errors.New("invalid params")
	}
	return arr[0], arr[1], arr[2], nil
}

func decodeMessageListParams(raw json.RawMessage) (string, int, int, error) {
	var arr []any
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) != 3 {
		return "", 0, 0, errors.New("invalid params")
	}
	contactID, ok := arr[0].(string)
	if !ok || strings.TrimSpace(contactID) == "" {
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

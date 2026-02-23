package rpc

import (
	"encoding/json"
	"errors"
	"strings"

	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/internal/domains/rpckit"
)

func Dispatch(service contracts.DaemonService, method string, rawParams json.RawMessage) (any, *rpckit.Error, bool) {
	switch method {
	case "request.list":
		result, rpcErr := callWithoutParams(-32093, func() (any, error) {
			return service.ListMessageRequests()
		})
		return result, rpcErr, true
	case "request.get":
		result, rpcErr := callWithSingleStringParam(rawParams, -32094, func(senderID string) (any, error) {
			return service.GetMessageRequest(senderID)
		})
		return result, rpcErr, true
	case "request.accept":
		result, rpcErr := callWithSingleStringParam(rawParams, -32095, func(senderID string) (any, error) {
			accepted, err := service.AcceptMessageRequest(senderID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"accepted": accepted}, nil
		})
		return result, rpcErr, true
	case "request.decline":
		result, rpcErr := callWithSingleStringParam(rawParams, -32096, func(senderID string) (any, error) {
			declined, err := service.DeclineMessageRequest(senderID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"declined": declined}, nil
		})
		return result, rpcErr, true
	case "request.block":
		result, rpcErr := callWithSingleStringParam(rawParams, -32097, func(senderID string) (any, error) {
			return service.BlockSender(senderID)
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

func decodeSingleStringParam(raw json.RawMessage) (string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 && strings.TrimSpace(arr[0]) != "" {
		return arr[0], nil
	}
	return "", errors.New("invalid params")
}

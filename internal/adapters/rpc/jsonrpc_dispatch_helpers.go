package rpc

import (
	"encoding/json"

	"aim-chat/go-backend/pkg/models"
)

func callWithoutParams(serviceErrCode int, call func() (any, error)) (any, *rpcError) {
	result, err := call()
	if err != nil {
		return nil, rpcServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithSingleStringParam(rawParams json.RawMessage, serviceErrCode int, call func(string) (any, error)) (any, *rpcError) {
	param, err := decodeSingleStringParam(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	result, err := call(param)
	if err != nil {
		return nil, rpcServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithSingleStringParamAndErrorMapper(
	rawParams json.RawMessage,
	mapErr func(error) *rpcError,
	call func(string) (any, error),
) (any, *rpcError) {
	param, err := decodeSingleStringParam(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	result, err := call(param)
	if err != nil {
		return nil, mapErr(err)
	}
	return result, nil
}

func callWithSessionInitParams(
	rawParams json.RawMessage,
	serviceErrCode int,
	call func(contactID string, peerPublicKey []byte) (any, error),
) (any, *rpcError) {
	contactID, peerPublicKey, err := decodeSessionInitParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	result, err := call(contactID, peerPublicKey)
	if err != nil {
		return nil, rpcServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithMessageEditParams(
	rawParams json.RawMessage,
	serviceErrCode int,
	call func(contactID, messageID, content string) (any, error),
) (any, *rpcError) {
	contactID, messageID, content, err := decodeMessageEditParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	result, err := call(contactID, messageID, content)
	if err != nil {
		return nil, rpcServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithMessageListParams(
	rawParams json.RawMessage,
	serviceErrCode int,
	call func(contactID string, limit, offset int) (any, error),
) (any, *rpcError) {
	contactID, limit, offset, err := decodeMessageListParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	result, err := call(contactID, limit, offset)
	if err != nil {
		return nil, rpcServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithCardParam(
	rawParams json.RawMessage,
	serviceErrCode int,
	call func(card models.ContactCard) (any, error),
) (any, *rpcError) {
	card, err := decodeCardParam(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	result, err := call(card)
	if err != nil {
		return nil, rpcServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithFilePutParams(
	rawParams json.RawMessage,
	serviceErrCode int,
	call func(name, mimeType, dataBase64 string) (any, error),
) (any, *rpcError) {
	name, mimeType, dataBase64, err := decodeFilePutParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	result, err := call(name, mimeType, dataBase64)
	if err != nil {
		return nil, rpcServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithContactByIDParams(
	rawParams json.RawMessage,
	call func(contactID, displayName string) (any, *rpcError),
) (any, *rpcError) {
	contactID, displayName, err := decodeContactByIDParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	return call(contactID, displayName)
}

func callWithTwoStringParams(rawParams json.RawMessage, serviceErrCode int, call func(string, string) (any, error)) (any, *rpcError) {
	a, b, err := decodeTwoStringParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	result, err := call(a, b)
	if err != nil {
		return nil, rpcServiceError(serviceErrCode, err)
	}
	return result, nil
}

package rpc

import (
	"encoding/json"

	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/internal/domains/rpckit"
)

func dispatchContactFileBlobNodeDevice(service contracts.DaemonService, method string, rawParams json.RawMessage) (any, *rpckit.Error, bool) {
	if result, rpcErr, ok := dispatchContactRPC(service, method, rawParams); ok {
		return result, rpcErr, true
	}
	if result, rpcErr, ok := dispatchFileUploadRPC(service, method, rawParams); ok {
		return result, rpcErr, true
	}
	if result, rpcErr, ok := dispatchBlobRPC(service, method, rawParams); ok {
		return result, rpcErr, true
	}
	if result, rpcErr, ok := dispatchNodeBindingRPC(service, method, rawParams); ok {
		return result, rpcErr, true
	}
	return dispatchDeviceRPC(service, method, rawParams)
}

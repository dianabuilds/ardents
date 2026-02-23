package rpc

import (
	"encoding/json"
	"errors"

	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/internal/domains/rpckit"
)

func dispatchDeviceRPC(service contracts.DaemonService, method string, rawParams json.RawMessage) (any, *rpckit.Error, bool) {
	switch method {
	case "device.list":
		result, rpcErr := callWithoutParams(-32050, func() (any, error) {
			return service.ListDevices()
		})
		return result, rpcErr, true
	case "device.add":
		result, rpcErr := callWithSingleStringParam(rawParams, -32051, func(name string) (any, error) {
			return service.AddDevice(name)
		})
		return result, rpcErr, true
	case "device.revoke":
		result, rpcErr := callWithSingleStringParamAndErrorMapper(rawParams, mapDeviceRevokeRPCError, func(deviceID string) (any, error) {
			return service.RevokeDevice(deviceID)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func mapDeviceRevokeRPCError(err error) *rpckit.Error {
	var deliveryErr *contracts.DeviceRevocationDeliveryError
	if errors.As(err, &deliveryErr) {
		if deliveryErr.IsFullFailure() {
			return &rpckit.Error{Code: -32054, Message: err.Error()}
		}
		return &rpckit.Error{Code: -32053, Message: err.Error()}
	}
	return &rpckit.Error{Code: -32052, Message: err.Error()}
}

func callWithSingleStringParamAndErrorMapper(
	rawParams json.RawMessage,
	mapErr func(error) *rpckit.Error,
	call func(string) (any, error),
) (any, *rpckit.Error) {
	param, err := decodeSingleStringParam(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, err := call(param)
	if err != nil {
		return nil, mapErr(err)
	}
	return result, nil
}

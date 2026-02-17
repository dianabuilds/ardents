package rpc

import (
	"aim-chat/go-backend/internal/domains/contracts"
	"errors"
)

func rpcInvalidParams() *rpcError {
	return &rpcError{Code: -32602, Message: "invalid params"}
}

func rpcServiceError(code int, err error) *rpcError {
	return &rpcError{Code: code, Message: err.Error()}
}

func mapDeviceRevokeRPCError(err error) *rpcError {
	var deliveryErr *contracts.DeviceRevocationDeliveryError
	if errors.As(err, &deliveryErr) {
		if deliveryErr.IsFullFailure() {
			return &rpcError{Code: -32054, Message: err.Error()}
		}
		return &rpcError{Code: -32053, Message: err.Error()}
	}
	return &rpcError{Code: -32052, Message: err.Error()}
}

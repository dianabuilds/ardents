package rpc

import (
	"errors"

	"aim-chat/go-backend/pkg/models"
)

func (s *Server) dispatchNetworkRPC(method string) (any, *rpcError, bool) {
	switch method {
	case "network.status":
		return serviceCall(-32031, func() (any, error) {
			return s.service.GetNetworkStatus(), nil
		})
	case "network.listen_addresses":
		return serviceCall(-32032, func() (any, error) {
			return map[string]any{"addresses": s.service.ListenAddresses()}, nil
		})
	case "metrics.get":
		return serviceCall(-32070, func() (any, error) {
			return s.service.GetMetrics(), nil
		})
	case "diagnostics.export":
		return serviceCall(-32071, func() (any, error) {
			exporter, ok := s.service.(interface {
				ExportDiagnosticsBundle(windowMinutes int) (models.DiagnosticsExportPackage, error)
			})
			if !ok {
				return nil, errors.New("diagnostics export is not supported")
			}
			return exporter.ExportDiagnosticsBundle(0)
		})
	default:
		return nil, nil, false
	}
}

func serviceCall(serviceErrCode int, call func() (any, error)) (any, *rpcError, bool) {
	result, err := call()
	if err != nil {
		return nil, &rpcError{Code: serviceErrCode, Message: err.Error()}, true
	}
	return result, nil, true
}

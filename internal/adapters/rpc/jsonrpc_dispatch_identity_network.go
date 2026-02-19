package rpc

import (
	"encoding/json"
	"errors"
)

func (s *Server) dispatchIdentityRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "identity.get":
		result, rpcErr := callWithoutParams(-32000, func() (any, error) {
			return s.service.GetIdentity()
		})
		return result, rpcErr, true
	case "identity.self_contact_card":
		result, rpcErr := callWithSingleStringParam(rawParams, -32025, func(displayName string) (any, error) {
			return s.service.SelfContactCard(displayName)
		})
		return result, rpcErr, true
	case "identity.create":
		result, rpcErr := callWithSingleStringParam(rawParams, -32020, func(password string) (any, error) {
			identity, mnemonic, err := s.service.CreateIdentity(password)
			if err != nil {
				return nil, err
			}
			return map[string]any{"identity": identity, "mnemonic": mnemonic}, nil
		})
		return result, rpcErr, true
	case "identity.export_seed":
		result, rpcErr := callWithSingleStringParam(rawParams, -32021, func(password string) (any, error) {
			mnemonic, err := s.service.ExportSeed(password)
			if err != nil {
				return nil, err
			}
			return map[string]string{"mnemonic": mnemonic}, nil
		})
		return result, rpcErr, true
	case "backup.export":
		result, rpcErr := callWithTwoStringParams(rawParams, -32024, func(consent, passphrase string) (any, error) {
			blob, err := s.service.ExportBackup(consent, passphrase)
			if err != nil {
				return nil, err
			}
			return map[string]string{"backup_blob": blob}, nil
		})
		return result, rpcErr, true
	case "backup.restore":
		result, rpcErr := callWithThreeStringParams(rawParams, -32028, func(consent, passphrase, backupBlob string) (any, error) {
			identity, err := s.service.RestoreBackup(consent, passphrase, backupBlob)
			if err != nil {
				return nil, err
			}
			return map[string]any{"identity": identity}, nil
		})
		return result, rpcErr, true
	case "data.wipe":
		result, rpcErr := callWithSingleStringParam(rawParams, -32027, func(consentToken string) (any, error) {
			wiper, ok := s.service.(interface {
				WipeData(consentToken string) (bool, error)
			})
			if !ok {
				return nil, errors.New("data wipe is not supported")
			}
			wiped, err := wiper.WipeData(consentToken)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"wiped": wiped}, nil
		})
		return result, rpcErr, true
	case "identity.import_seed":
		result, rpcErr := callWithTwoStringParams(rawParams, -32022, func(mnemonic, password string) (any, error) {
			identity, err := s.service.ImportIdentity(mnemonic, password)
			if err != nil {
				return nil, err
			}
			return map[string]any{"identity": identity}, nil
		})
		return result, rpcErr, true
	case "identity.validate_mnemonic":
		result, rpcErr := callWithSingleStringParam(rawParams, -32026, func(mnemonic string) (any, error) {
			return map[string]bool{"valid": s.service.ValidateMnemonic(mnemonic)}, nil
		})
		return result, rpcErr, true
	case "identity.change_password":
		result, rpcErr := callWithTwoStringParams(rawParams, -32023, func(oldPassword, newPassword string) (any, error) {
			if err := s.service.ChangePassword(oldPassword, newPassword); err != nil {
				return nil, err
			}
			return map[string]bool{"changed": true}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchNetworkRPC(method string) (any, *rpcError, bool) {
	switch method {
	case "network.status":
		result, rpcErr := callWithoutParams(-32031, func() (any, error) {
			return s.service.GetNetworkStatus(), nil
		})
		return result, rpcErr, true
	case "network.listen_addresses":
		result, rpcErr := callWithoutParams(-32032, func() (any, error) {
			return map[string]any{"addresses": s.service.ListenAddresses()}, nil
		})
		return result, rpcErr, true
	case "metrics.get":
		result, rpcErr := callWithoutParams(-32070, func() (any, error) {
			return s.service.GetMetrics(), nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

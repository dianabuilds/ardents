package rpc

import (
	"encoding/json"
	"errors"

	"aim-chat/go-backend/internal/domains/contracts"
	identitytransport "aim-chat/go-backend/internal/domains/identity/transport"
	"aim-chat/go-backend/internal/domains/rpckit"
)

func Dispatch(service contracts.DaemonService, method string, rawParams json.RawMessage) (any, *rpckit.Error, bool) {
	switch method {
	case identitytransport.MethodIdentityGet:
		result, rpcErr := callWithoutParams(-32000, func() (any, error) {
			return service.GetIdentity()
		})
		return result, rpcErr, true
	case identitytransport.MethodIdentitySelfCard:
		result, rpcErr := callWithSingleStringParam(rawParams, -32025, func(displayName string) (any, error) {
			return service.SelfContactCard(displayName)
		})
		return result, rpcErr, true
	case identitytransport.MethodIdentityLogin:
		result, rpcErr := callWithTwoStringParams(rawParams, -32029, func(identityID, seedPassword string) (any, error) {
			if err := service.Login(identityID, seedPassword); err != nil {
				return nil, err
			}
			return map[string]bool{"logged_in": true}, nil
		})
		return result, rpcErr, true
	case identitytransport.MethodIdentityCreate:
		result, rpcErr := callWithSingleStringParam(rawParams, -32020, func(password string) (any, error) {
			identity, mnemonic, err := service.CreateIdentity(password)
			if err != nil {
				return nil, err
			}
			return map[string]any{"identity": identity, "mnemonic": mnemonic}, nil
		})
		return result, rpcErr, true
	case identitytransport.MethodIdentityExportSeed:
		result, rpcErr := callWithSingleStringParam(rawParams, -32021, func(password string) (any, error) {
			mnemonic, err := service.ExportSeed(password)
			if err != nil {
				return nil, err
			}
			return map[string]string{"mnemonic": mnemonic}, nil
		})
		return result, rpcErr, true
	case identitytransport.MethodBackupExport:
		result, rpcErr := callWithTwoStringParams(rawParams, -32024, func(consent, password string) (any, error) {
			blob, err := service.ExportBackup(consent, password)
			if err != nil {
				return nil, err
			}
			return map[string]string{"backup_blob": blob}, nil
		})
		return result, rpcErr, true
	case identitytransport.MethodBackupRestore:
		result, rpcErr := callWithThreeStringParams(rawParams, -32028, func(consent, password, backupBlob string) (any, error) {
			identity, err := service.RestoreBackup(consent, password, backupBlob)
			if err != nil {
				return nil, err
			}
			return map[string]any{"identity": identity}, nil
		})
		return result, rpcErr, true
	case identitytransport.MethodDataWipe:
		result, rpcErr := callWithSingleStringParam(rawParams, -32027, func(consentToken string) (any, error) {
			wiper, ok := service.(interface {
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
	case identitytransport.MethodIdentityImportSeed:
		result, rpcErr := callWithTwoStringParams(rawParams, -32022, func(mnemonic, password string) (any, error) {
			identity, err := service.ImportIdentity(mnemonic, password)
			if err != nil {
				return nil, err
			}
			return map[string]any{"identity": identity}, nil
		})
		return result, rpcErr, true
	case identitytransport.MethodIdentityMnemonic:
		result, rpcErr := callWithSingleStringParam(rawParams, -32026, func(mnemonic string) (any, error) {
			return map[string]bool{"valid": service.ValidateMnemonic(mnemonic)}, nil
		})
		return result, rpcErr, true
	case identitytransport.MethodIdentityChangePwd:
		result, rpcErr := callWithTwoStringParams(rawParams, -32023, func(oldPassword, newPassword string) (any, error) {
			if err := service.ChangePassword(oldPassword, newPassword); err != nil {
				return nil, err
			}
			return map[string]bool{"changed": true}, nil
		})
		return result, rpcErr, true
	case identitytransport.MethodAccountList:
		result, rpcErr := callWithoutParams(-32040, func() (any, error) {
			accountSvc, ok := service.(contracts.AccountAPI)
			if !ok {
				return nil, errors.New("account profiles are not supported")
			}
			return accountSvc.ListAccounts()
		})
		return result, rpcErr, true
	case identitytransport.MethodAccountCurrent:
		result, rpcErr := callWithoutParams(-32041, func() (any, error) {
			accountSvc, ok := service.(contracts.AccountAPI)
			if !ok {
				return nil, errors.New("account profiles are not supported")
			}
			return accountSvc.GetCurrentAccount()
		})
		return result, rpcErr, true
	case identitytransport.MethodAccountSwitch:
		result, rpcErr := callWithSingleStringParam(rawParams, -32042, func(accountID string) (any, error) {
			accountSvc, ok := service.(contracts.AccountAPI)
			if !ok {
				return nil, errors.New("account profiles are not supported")
			}
			identity, err := accountSvc.SwitchAccount(accountID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"identity": identity}, nil
		})
		return result, rpcErr, true
	default:
		return dispatchContactFileBlobNodeDevice(service, method, rawParams)
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

func callWithThreeStringParams(rawParams json.RawMessage, serviceErrCode int, call func(string, string, string) (any, error)) (any, *rpckit.Error) {
	a, b, c, err := decodeThreeStringParams(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, err := call(a, b, c)
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

func decodeThreeStringParams(raw json.RawMessage) (string, string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 3 && arr[0] != "" && arr[1] != "" && arr[2] != "" {
		return arr[0], arr[1], arr[2], nil
	}
	return "", "", "", errors.New("invalid params")
}

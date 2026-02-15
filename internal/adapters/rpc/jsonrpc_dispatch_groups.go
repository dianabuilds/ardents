package rpc

import (
	"encoding/json"

	"aim-chat/go-backend/pkg/models"
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

func (s *Server) dispatchSessionMessageRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "session.init":
		result, rpcErr := callWithSessionInitParams(rawParams, -32030, func(contactID string, peerPublicKey []byte) (any, error) {
			return s.service.InitSession(contactID, peerPublicKey)
		})
		return result, rpcErr, true
	case "message.send":
		result, rpcErr := callWithTwoStringParams(rawParams, -32040, func(contactID, content string) (any, error) {
			messageID, err := s.service.SendMessage(contactID, content)
			if err != nil {
				return nil, err
			}
			return map[string]string{"message_id": messageID}, nil
		})
		return result, rpcErr, true
	case "message.edit":
		result, rpcErr := callWithMessageEditParams(rawParams, -32043, func(contactID, messageID, content string) (any, error) {
			return s.service.EditMessage(contactID, messageID, content)
		})
		return result, rpcErr, true
	case "message.list":
		result, rpcErr := callWithMessageListParams(rawParams, -32041, func(contactID string, limit, offset int) (any, error) {
			return s.service.GetMessages(contactID, limit, offset)
		})
		return result, rpcErr, true
	case "message.status":
		result, rpcErr := callWithSingleStringParam(rawParams, -32042, func(messageID string) (any, error) {
			return s.service.GetMessageStatus(messageID)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchContactFileRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "contact.verify":
		result, rpcErr := callWithCardParam(rawParams, -32010, func(card models.ContactCard) (any, error) {
			ok, err := s.service.VerifyContactCard(card)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"valid": ok}, nil
		})
		return result, rpcErr, true
	case "contact.add":
		result, rpcErr := s.dispatchContactAdd(rawParams)
		return result, rpcErr, true
	case "contact.add_by_id":
		result, rpcErr := callWithContactByIDParams(rawParams, func(contactID, displayName string) (any, *rpcError) {
			return s.dispatchAddContactByID(contactID, displayName)
		})
		return result, rpcErr, true
	case "file.put":
		result, rpcErr := callWithFilePutParams(rawParams, -32060, func(name, mimeType, dataBase64 string) (any, error) {
			return s.service.PutAttachment(name, mimeType, dataBase64)
		})
		return result, rpcErr, true
	case "contact.list":
		result, rpcErr := callWithoutParams(-32012, func() (any, error) {
			return s.service.GetContacts()
		})
		return result, rpcErr, true
	case "contact.remove":
		result, rpcErr := callWithSingleStringParam(rawParams, -32014, func(contactID string) (any, error) {
			if err := s.service.RemoveContact(contactID); err != nil {
				return nil, err
			}
			return map[string]bool{"removed": true}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchContactAdd(rawParams json.RawMessage) (any, *rpcError) {
	card, contactID, displayName, err := decodeAddContactParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	if card.IdentityID != "" {
		if err := s.service.AddContactCard(card); err != nil {
			return nil, rpcServiceError(-32011, err)
		}
		return map[string]bool{"added": true}, nil
	}
	return s.dispatchAddContactByID(contactID, displayName)
}

func (s *Server) dispatchAddContactByID(contactID, displayName string) (any, *rpcError) {
	if err := s.service.AddContact(contactID, displayName); err != nil {
		return nil, rpcServiceError(-32013, err)
	}
	return map[string]bool{"added": true}, nil
}

func (s *Server) dispatchDeviceRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "device.list":
		result, rpcErr := callWithoutParams(-32050, func() (any, error) {
			return s.service.ListDevices()
		})
		return result, rpcErr, true
	case "device.add":
		result, rpcErr := callWithSingleStringParam(rawParams, -32051, func(name string) (any, error) {
			return s.service.AddDevice(name)
		})
		return result, rpcErr, true
	case "device.revoke":
		result, rpcErr := callWithSingleStringParamAndErrorMapper(rawParams, mapDeviceRevokeRPCError, func(deviceID string) (any, error) {
			return s.service.RevokeDevice(deviceID)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

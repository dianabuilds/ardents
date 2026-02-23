package rpc

import (
	"encoding/json"
	"errors"

	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/internal/domains/rpckit"
	"aim-chat/go-backend/pkg/models"
)

func dispatchContactRPC(service contracts.DaemonService, method string, rawParams json.RawMessage) (any, *rpckit.Error, bool) {
	switch method {
	case "contact.verify":
		result, rpcErr := callWithCardParam(rawParams, -32010, func(card models.ContactCard) (any, error) {
			ok, err := service.VerifyContactCard(card)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"valid": ok}, nil
		})
		return result, rpcErr, true
	case "contact.add":
		result, rpcErr := dispatchContactAdd(service, rawParams)
		return result, rpcErr, true
	case "contact.add_by_id":
		result, rpcErr := callWithContactByIDParams(rawParams, func(contactID, displayName string) (any, *rpckit.Error) {
			return dispatchAddContactByID(service, contactID, displayName)
		})
		return result, rpcErr, true
	case "contact.list":
		result, rpcErr := callWithoutParams(-32012, func() (any, error) {
			return service.GetContacts()
		})
		return result, rpcErr, true
	case "contact.remove":
		result, rpcErr := callWithSingleStringParam(rawParams, -32014, func(contactID string) (any, error) {
			if err := service.RemoveContact(contactID); err != nil {
				return nil, err
			}
			return map[string]bool{"removed": true}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func callWithCardParam(
	rawParams json.RawMessage,
	serviceErrCode int,
	call func(card models.ContactCard) (any, error),
) (any, *rpckit.Error) {
	card, err := decodeCardParam(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	result, err := call(card)
	if err != nil {
		return nil, rpckit.ServiceError(serviceErrCode, err)
	}
	return result, nil
}

func callWithContactByIDParams(
	rawParams json.RawMessage,
	call func(contactID, displayName string) (any, *rpckit.Error),
) (any, *rpckit.Error) {
	contactID, displayName, err := decodeContactByIDParams(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	return call(contactID, displayName)
}

func dispatchContactAdd(service contracts.DaemonService, rawParams json.RawMessage) (any, *rpckit.Error) {
	card, contactID, displayName, err := decodeAddContactParams(rawParams)
	if err != nil {
		return nil, rpckit.InvalidParams()
	}
	if card.IdentityID != "" {
		if err := service.AddContactCard(card); err != nil {
			return nil, rpckit.ServiceError(-32011, err)
		}
		return map[string]bool{"added": true}, nil
	}
	return dispatchAddContactByID(service, contactID, displayName)
}

func dispatchAddContactByID(service contracts.DaemonService, contactID, displayName string) (any, *rpckit.Error) {
	if err := service.AddContact(contactID, displayName); err != nil {
		return nil, rpckit.ServiceError(-32013, err)
	}
	return map[string]bool{"added": true}, nil
}

func decodeCardParam(raw json.RawMessage) (models.ContactCard, error) {
	var arr []models.ContactCard
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		return arr[0], nil
	}
	var wrapper struct {
		Card models.ContactCard `json:"card"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Card.IdentityID != "" {
		return wrapper.Card, nil
	}
	return models.ContactCard{}, errors.New("invalid params")
}

func decodeAddContactParams(raw json.RawMessage) (models.ContactCard, string, string, error) {
	if card, err := decodeCardParam(raw); err == nil {
		return card, "", "", nil
	}
	contactID, displayName, err := decodeContactByIDParams(raw)
	if err != nil {
		return models.ContactCard{}, "", "", errors.New("invalid params")
	}
	return models.ContactCard{}, contactID, displayName, nil
}

func decodeContactByIDParams(raw json.RawMessage) (string, string, error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return "", "", errors.New("invalid params")
	}
	if len(arr) == 1 && arr[0] != "" {
		return arr[0], "", nil
	}
	if len(arr) == 2 && arr[0] != "" {
		return arr[0], arr[1], nil
	}
	return "", "", errors.New("invalid params")
}

package rpc

import (
	"encoding/json"
	"errors"
	"strings"

	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/internal/domains/rpckit"
	"aim-chat/go-backend/pkg/models"
)

func dispatchNodeBindingRPC(service contracts.DaemonService, method string, rawParams json.RawMessage) (any, *rpckit.Error, bool) {
	switch method {
	case "node.getPolicies":
		result, rpcErr := callWithoutParams(-32089, func() (any, error) {
			policyAPI, ok := service.(interface {
				GetNodePolicies() models.NodePolicies
			})
			if !ok {
				return nil, errors.New("node policies are not supported")
			}
			return policyAPI.GetNodePolicies(), nil
		})
		return result, rpcErr, true
	case "node.updatePolicies":
		patch, err := decodeNodePoliciesPatchParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32090, func() (any, error) {
			policyAPI, ok := service.(interface {
				UpdateNodePolicies(patch models.NodePoliciesPatch) (models.NodePolicies, error)
			})
			if !ok {
				return nil, errors.New("node policies are not supported")
			}
			return policyAPI.UpdateNodePolicies(patch)
		})
		return result, rpcErr, true
	case "node.binding.link.create":
		ttlSeconds, err := decodeNodeBindingLinkCreateParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32073, func() (any, error) {
			bindingAPI, ok := service.(interface {
				CreateNodeBindingLinkCode(ttlSeconds int) (models.NodeBindingLinkCode, error)
			})
			if !ok {
				return nil, errors.New("node binding is not supported")
			}
			return bindingAPI.CreateNodeBindingLinkCode(ttlSeconds)
		})
		return result, rpcErr, true
	case "node.binding.complete":
		linkCode, nodeID, nodePub, nodeSig, rebind, err := decodeNodeBindingCompleteParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32074, func() (any, error) {
			bindingAPI, ok := service.(interface {
				CompleteNodeBinding(linkCode, nodeID, nodePublicKeyBase64, nodeSignatureBase64 string, allowRebind bool) (models.NodeBindingRecord, error)
			})
			if !ok {
				return nil, errors.New("node binding is not supported")
			}
			return bindingAPI.CompleteNodeBinding(linkCode, nodeID, nodePub, nodeSig, rebind)
		})
		return result, rpcErr, true
	case "node.binding.get":
		result, rpcErr := callWithoutParams(-32075, func() (any, error) {
			bindingAPI, ok := service.(interface {
				GetNodeBinding() (models.NodeBindingRecord, bool, error)
			})
			if !ok {
				return nil, errors.New("node binding is not supported")
			}
			record, exists, err := bindingAPI.GetNodeBinding()
			if err != nil {
				return nil, err
			}
			return map[string]any{"exists": exists, "binding": record}, nil
		})
		return result, rpcErr, true
	case "node.binding.unbind":
		nodeID, confirm, err := decodeNodeBindingUnbindParams(rawParams)
		if err != nil {
			return nil, rpckit.InvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32076, func() (any, error) {
			bindingAPI, ok := service.(interface {
				UnbindNode(nodeID string, confirm bool) (bool, error)
			})
			if !ok {
				return nil, errors.New("node binding is not supported")
			}
			removed, err := bindingAPI.UnbindNode(nodeID, confirm)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"removed": removed}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func decodeNodePoliciesPatchParams(raw json.RawMessage) (models.NodePoliciesPatch, error) {
	type payload struct {
		PersonalPolicy *models.NodePersonalPolicyPatch `json:"personal_policy"`
		PublicPolicy   *models.NodePublicPolicyPatch   `json:"public_policy"`
		Personal       *models.NodePersonalPolicyPatch `json:"personal"`
		Public         *models.NodePublicPolicyPatch   `json:"public"`
	}
	parse := func(p payload) (models.NodePoliciesPatch, error) {
		personal := p.PersonalPolicy
		if personal == nil {
			personal = p.Personal
		}
		public := p.PublicPolicy
		if public == nil {
			public = p.Public
		}
		if personal == nil && public == nil {
			return models.NodePoliciesPatch{}, errors.New("invalid params")
		}
		return models.NodePoliciesPatch{
			Personal: personal,
			Public:   public,
		}, nil
	}
	var arr []payload
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		return parse(arr[0])
	}
	var direct payload
	if err := json.Unmarshal(raw, &direct); err == nil {
		return parse(direct)
	}
	return models.NodePoliciesPatch{}, errors.New("invalid params")
}

func decodeNodeBindingLinkCreateParams(raw json.RawMessage) (int, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	type payload struct {
		TTLSeconds *int `json:"ttl_seconds"`
	}
	var arr []payload
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		if arr[0].TTLSeconds == nil {
			return 0, nil
		}
		if *arr[0].TTLSeconds < 1 {
			return 0, errors.New("invalid params")
		}
		return *arr[0].TTLSeconds, nil
	}
	var direct payload
	if err := json.Unmarshal(raw, &direct); err == nil {
		if direct.TTLSeconds == nil {
			return 0, nil
		}
		if *direct.TTLSeconds < 1 {
			return 0, errors.New("invalid params")
		}
		return *direct.TTLSeconds, nil
	}
	return 0, errors.New("invalid params")
}

func decodeNodeBindingCompleteParams(raw json.RawMessage) (string, string, string, string, bool, error) {
	type payload struct {
		LinkCode            string `json:"link_code"`
		NodeID              string `json:"node_id"`
		NodePublicKeyBase64 string `json:"node_public_key_base64"`
		NodeSignatureBase64 string `json:"node_signature_base64"`
		Rebind              bool   `json:"rebind"`
	}
	parse := func(p payload) (string, string, string, string, bool, error) {
		linkCode := strings.TrimSpace(p.LinkCode)
		nodeID := strings.TrimSpace(p.NodeID)
		pub := strings.TrimSpace(p.NodePublicKeyBase64)
		sig := strings.TrimSpace(p.NodeSignatureBase64)
		if linkCode == "" || nodeID == "" || pub == "" || sig == "" {
			return "", "", "", "", false, errors.New("invalid params")
		}
		return linkCode, nodeID, pub, sig, p.Rebind, nil
	}
	var arr []payload
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		return parse(arr[0])
	}
	var direct payload
	if err := json.Unmarshal(raw, &direct); err == nil {
		return parse(direct)
	}
	return "", "", "", "", false, errors.New("invalid params")
}

func decodeNodeBindingUnbindParams(raw json.RawMessage) (string, bool, error) {
	type payload struct {
		NodeID  string `json:"node_id"`
		Confirm bool   `json:"confirm"`
	}
	parse := func(p payload) (string, bool, error) {
		return strings.TrimSpace(p.NodeID), p.Confirm, nil
	}
	var arr []payload
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) == 1 {
		return parse(arr[0])
	}
	var direct payload
	if err := json.Unmarshal(raw, &direct); err == nil {
		return parse(direct)
	}
	return "", false, errors.New("invalid params")
}

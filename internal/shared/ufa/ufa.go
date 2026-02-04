package ufa

import (
	"errors"
	"strings"

	"github.com/dianabuilds/ardents/internal/shared/ids"
)

var (
	ErrUFAUnsupported      = errors.New("ERR_UFA_UNSUPPORTED")
	ErrUFATypeMismatch     = errors.New("ERR_UFA_TYPE_MISMATCH")
	ErrServiceNameRequired = errors.New("ERR_SERVICE_NAME_REQUIRED")
)

const (
	TargetNode     = "node"
	TargetService  = "service"
	TargetIdentity = "identity"
)

type Result struct {
	TargetType string
	TargetID   string
}

type AliasResolver func(alias string, nowMs int64) (targetType, targetID string, ok bool, err error)

func Parse(raw string, nowMs int64, resolveAlias AliasResolver) (Result, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Result{}, ErrUFAUnsupported
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return Result{}, ErrUFAUnsupported
	}
	normalized := strings.ToLower(s)
	if resolveAlias != nil {
		targetType, targetID, ok, err := resolveAlias(normalized, nowMs)
		if err != nil {
			return Result{}, err
		}
		if ok {
			return Result{TargetType: targetType, TargetID: targetID}, nil
		}
	}
	if ids.ValidateNodeID(normalized) == nil {
		return Result{TargetType: TargetNode, TargetID: normalized}, nil
	}
	if ids.ValidateServiceID(normalized) == nil {
		return Result{TargetType: TargetService, TargetID: normalized}, nil
	}
	if ids.ValidateIdentityID(s) == nil {
		return Result{TargetType: TargetIdentity, TargetID: s}, nil
	}
	return Result{}, ErrUFAUnsupported
}

func Resolve(raw string, nowMs int64, resolveAlias AliasResolver, serviceName string) (Result, error) {
	res, err := Parse(raw, nowMs, resolveAlias)
	if err != nil {
		return Result{}, err
	}
	if res.TargetType != TargetIdentity {
		return res, nil
	}
	if serviceName == "" || ids.ValidateServiceName(serviceName) != nil {
		return Result{}, ErrServiceNameRequired
	}
	serviceID, err := ids.NewServiceID(res.TargetID, serviceName)
	if err != nil {
		return Result{}, err
	}
	return Result{TargetType: TargetService, TargetID: serviceID}, nil
}

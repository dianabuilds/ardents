package cliutil

import (
	"errors"
	"strings"

	"github.com/dianabuilds/ardents/internal/core/infra/addressbook"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/ufa"
)

func aliasResolverFromBook(book addressbook.Book) ufa.AliasResolver {
	return func(alias string, nowMs int64) (string, string, bool, error) {
		entry, ok, err := book.ResolveAlias(alias, nowMs)
		if err != nil {
			if errors.Is(err, addressbook.ErrAliasInvalid) {
				// Treat invalid alias as "not an alias", so raw parsing errors are stable.
				return "", "", false, nil
			}
			return "", "", false, err
		}
		if !ok {
			return "", "", false, nil
		}
		return entry.TargetType, entry.TargetID, true, nil
	}
}

func ResolveNodeID(raw string, book addressbook.Book, nowMs int64) (string, error) {
	res, err := ufa.Parse(raw, nowMs, aliasResolverFromBook(book))
	if err != nil {
		return "", err
	}
	if res.TargetType != ufa.TargetNode {
		return "", ufa.ErrUFATypeMismatch
	}
	return res.TargetID, nil
}

// ResolveServiceID resolves raw UFA (alias|service_id|identity_id) into a service_id.
// If raw is an identity_id, serviceID is computed via ids.NewServiceID(identity_id, serviceName).
func ResolveServiceID(raw string, serviceName string, book addressbook.Book, nowMs int64) (serviceID string, targetIdentityID string, err error) {
	res, err := ufa.Resolve(raw, nowMs, aliasResolverFromBook(book), serviceName)
	if err != nil {
		return "", "", err
	}
	if res.TargetType != ufa.TargetService {
		return "", "", ufa.ErrUFATypeMismatch
	}
	if err := ids.ValidateServiceID(res.TargetID); err != nil {
		return "", "", err
	}
	s := strings.TrimSpace(raw)
	if ids.ValidateIdentityID(s) == nil {
		targetIdentityID = s
	}
	return res.TargetID, targetIdentityID, nil
}

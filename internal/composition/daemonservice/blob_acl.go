package daemonservice

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"

	"aim-chat/go-backend/internal/domains/contracts"
	groupdomain "aim-chat/go-backend/internal/domains/group"
	"aim-chat/go-backend/pkg/models"
)

const (
	blobACLModeEnv      = "AIM_BLOB_ACL_MODE"
	blobACLAllowlistEnv = "AIM_BLOB_ACL_ALLOWLIST"
)

type blobACLMode string

const (
	blobACLModeOwnerOnly         blobACLMode = "owner_only"
	blobACLModeOwnerContacts     blobACLMode = "owner_contacts"
	blobACLModeOwnerGroupsMember blobACLMode = "owner_groups_members"
	blobACLModeAllowlist         blobACLMode = "allowlist"
)

type blobACLPolicy struct {
	Mode      blobACLMode
	Allowlist map[string]struct{}
}

func resolveBlobACLPolicyFromEnv() blobACLPolicy {
	mode, err := parseBlobACLMode(envString(blobACLModeEnv))
	if err != nil {
		mode = blobACLModeOwnerContacts
	}
	return blobACLPolicy{
		Mode:      mode,
		Allowlist: normalizeBlobACLAllowlist(envCSV(blobACLAllowlistEnv)),
	}
}

func parseBlobACLMode(raw string) (blobACLMode, error) {
	switch blobACLMode(strings.ToLower(strings.TrimSpace(raw))) {
	case blobACLModeOwnerOnly:
		return blobACLModeOwnerOnly, nil
	case blobACLModeOwnerContacts:
		return blobACLModeOwnerContacts, nil
	case blobACLModeOwnerGroupsMember:
		return blobACLModeOwnerGroupsMember, nil
	case blobACLModeAllowlist:
		return blobACLModeAllowlist, nil
	default:
		return "", errors.New("invalid blob acl mode")
	}
}

func normalizeBlobACLAllowlist(raw []string) map[string]struct{} {
	out := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		peerID := strings.TrimSpace(item)
		if peerID == "" {
			continue
		}
		out[peerID] = struct{}{}
	}
	return out
}

func (s *Service) GetBlobACLPolicy() models.BlobACLPolicy {
	s.blobACLMu.RLock()
	policy := s.blobACL
	s.blobACLMu.RUnlock()
	return models.BlobACLPolicy{
		Mode:      string(policy.Mode),
		Allowlist: blobACLAllowlistSlice(policy.Allowlist),
		Enforced:  s.isBlobACLEnforced(),
	}
}

func (s *Service) SetBlobACLPolicy(mode string, allowlist []string) (models.BlobACLPolicy, error) {
	parsedMode, err := parseBlobACLMode(mode)
	if err != nil {
		return models.BlobACLPolicy{}, err
	}
	s.blobACLMu.Lock()
	s.blobACL = blobACLPolicy{
		Mode:      parsedMode,
		Allowlist: normalizeBlobACLAllowlist(allowlist),
	}
	policy := s.blobACL
	s.blobACLMu.Unlock()
	return models.BlobACLPolicy{
		Mode:      string(policy.Mode),
		Allowlist: blobACLAllowlistSlice(policy.Allowlist),
		Enforced:  s.isBlobACLEnforced(),
	}, nil
}

func blobACLAllowlistSlice(input map[string]struct{}) []string {
	out := make([]string, 0, len(input))
	for peerID := range input {
		out = append(out, peerID)
	}
	sort.Strings(out)
	return out
}

func (s *Service) authorizeBlobOperation(requesterPeerID, operation string) error {
	allowed, reason := s.isBlobOperationAllowed(requesterPeerID)
	if allowed {
		return nil
	}
	s.auditBlobACLDenied(operation, requesterPeerID, reason)
	return contracts.ErrAttachmentAccessDenied
}

func (s *Service) isBlobOperationAllowed(requesterPeerID string) (bool, string) {
	if !s.isBlobACLEnforced() {
		return true, ""
	}
	ownerID := strings.TrimSpace(s.localPeerID())
	requesterPeerID = strings.TrimSpace(requesterPeerID)
	if ownerID == "" || requesterPeerID == "" {
		return false, "missing_subject"
	}
	if requesterPeerID == ownerID {
		return true, ""
	}

	s.blobACLMu.RLock()
	policy := s.blobACL
	s.blobACLMu.RUnlock()

	switch policy.Mode {
	case blobACLModeOwnerOnly:
		return false, "owner_only"
	case blobACLModeOwnerContacts:
		if s.identityManager.HasContact(requesterPeerID) {
			return true, ""
		}
		return false, "not_contact"
	case blobACLModeOwnerGroupsMember:
		if s.identityManager.HasContact(requesterPeerID) {
			return true, ""
		}
		if s.hasSharedActiveGroupWithOwner(requesterPeerID, ownerID) {
			return true, ""
		}
		return false, "no_shared_group"
	case blobACLModeAllowlist:
		if _, ok := policy.Allowlist[requesterPeerID]; ok {
			return true, ""
		}
		return false, "not_in_allowlist"
	default:
		return false, "invalid_mode"
	}
}

func (s *Service) hasSharedActiveGroupWithOwner(requesterPeerID, ownerID string) bool {
	groups, err := s.groupCore.ListGroups()
	if err != nil {
		return false
	}
	for _, group := range groups {
		members, err := s.groupCore.ListGroupMembers(group.ID)
		if err != nil {
			continue
		}
		ownerActive := false
		requesterActive := false
		for _, member := range members {
			memberID := strings.TrimSpace(member.MemberID)
			if member.Status != groupdomain.GroupMemberStatusActive {
				continue
			}
			if memberID == ownerID {
				ownerActive = true
			}
			if memberID == requesterPeerID {
				requesterActive = true
			}
		}
		if ownerActive && requesterActive {
			return true
		}
	}
	return false
}

func (s *Service) isBlobACLEnforced() bool {
	identityID := strings.TrimSpace(s.identityManager.GetIdentity().ID)
	if identityID == "" {
		return false
	}
	_, ok := s.bindingStore.Get(identityID)
	return ok
}

func (s *Service) auditBlobACLDenied(operation, requesterPeerID, reason string) {
	fingerprint := "unknown"
	requesterPeerID = strings.TrimSpace(requesterPeerID)
	if requesterPeerID != "" {
		sum := sha256.Sum256([]byte(requesterPeerID))
		fingerprint = hex.EncodeToString(sum[:6])
	}
	s.logger.Warn("blob acl denied", "operation", operation, "reason", reason, "requester_fpr", fingerprint)
}

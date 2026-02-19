package daemonservice

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"aim-chat/go-backend/pkg/models"
)

const (
	defaultNodeBindingLinkTTL = 90 * time.Second
	maxNodeBindingLinkTTL     = 10 * time.Minute
)

func (s *Service) CreateNodeBindingLinkCode(ttlSeconds int) (models.NodeBindingLinkCode, error) {
	identityID := strings.TrimSpace(s.identityManager.GetIdentity().ID)
	if identityID == "" {
		return models.NodeBindingLinkCode{}, errors.New("identity is not initialized")
	}
	ttl := defaultNodeBindingLinkTTL
	if ttlSeconds > 0 {
		ttl = time.Duration(ttlSeconds) * time.Second
	}
	if ttl > maxNodeBindingLinkTTL {
		ttl = maxNodeBindingLinkTTL
	}
	code, err := randomBase64URL(18)
	if err != nil {
		return models.NodeBindingLinkCode{}, err
	}
	challenge, err := randomBase64URL(24)
	if err != nil {
		return models.NodeBindingLinkCode{}, err
	}
	expiresAt := time.Now().UTC().Add(ttl)

	s.bindingLinkMu.Lock()
	s.bindingLinks[code] = pendingNodeBindingLink{
		IdentityID: identityID,
		Code:       code,
		Challenge:  challenge,
		ExpiresAt:  expiresAt,
	}
	s.bindingLinkMu.Unlock()

	return models.NodeBindingLinkCode{
		LinkCode:   code,
		Challenge:  challenge,
		ExpiresAt:  expiresAt,
		IdentityID: identityID,
	}, nil
}

func (s *Service) CompleteNodeBinding(linkCode, nodeID, nodePublicKeyBase64, nodeSignatureBase64 string, allowRebind bool) (models.NodeBindingRecord, error) {
	linkCode = strings.TrimSpace(linkCode)
	nodeID = strings.TrimSpace(nodeID)
	if linkCode == "" || nodeID == "" {
		return models.NodeBindingRecord{}, errors.New("link code and node id are required")
	}
	pending, ok := s.consumeValidBindingLink(linkCode)
	if !ok {
		return models.NodeBindingRecord{}, errors.New("link code is invalid or expired")
	}
	nodePub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(nodePublicKeyBase64))
	if err != nil || len(nodePub) != ed25519.PublicKeySize {
		return models.NodeBindingRecord{}, errors.New("node public key is invalid")
	}
	nodeSig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(nodeSignatureBase64))
	if err != nil || len(nodeSig) != ed25519.SignatureSize {
		return models.NodeBindingRecord{}, errors.New("node signature is invalid")
	}
	challengePayload := nodeBindingChallengeBytes(pending.IdentityID, pending.Code, nodeID, pending.Challenge)
	if !ed25519.Verify(nodePub, challengePayload, nodeSig) {
		return models.NodeBindingRecord{}, errors.New("node signature verification failed")
	}

	existing, exists := s.bindingStore.Get(pending.IdentityID)
	if exists && strings.TrimSpace(existing.NodeID) != nodeID && !allowRebind {
		return models.NodeBindingRecord{}, errors.New("node binding already exists; explicit rebind confirmation is required")
	}

	_, identityPrivateKey := s.identityManager.SnapshotIdentityKeys()
	if len(identityPrivateKey) != ed25519.PrivateKeySize {
		return models.NodeBindingRecord{}, errors.New("identity private key is unavailable")
	}
	now := time.Now().UTC()
	accountSignature := ed25519.Sign(identityPrivateKey, nodeBindingAccountBytes(pending.IdentityID, nodeID, pending.Challenge, now))
	record := models.NodeBindingRecord{
		IdentityID:       pending.IdentityID,
		NodeID:           nodeID,
		NodePublicKey:    base64.StdEncoding.EncodeToString(nodePub),
		AccountSignature: base64.StdEncoding.EncodeToString(accountSignature),
		NodeSignature:    base64.StdEncoding.EncodeToString(nodeSig),
		BoundAt:          now,
		UpdatedAt:        now,
	}
	if exists {
		record.BoundAt = existing.BoundAt
		if record.BoundAt.IsZero() {
			record.BoundAt = now
		}
	}
	if err := s.bindingStore.Upsert(record); err != nil {
		return models.NodeBindingRecord{}, err
	}
	return record, nil
}

func (s *Service) GetNodeBinding() (models.NodeBindingRecord, bool, error) {
	identityID := strings.TrimSpace(s.identityManager.GetIdentity().ID)
	if identityID == "" {
		return models.NodeBindingRecord{}, false, errors.New("identity is not initialized")
	}
	record, ok := s.bindingStore.Get(identityID)
	return record, ok, nil
}

func (s *Service) UnbindNode(nodeID string, confirm bool) (bool, error) {
	if !confirm {
		return false, errors.New("unbind confirmation is required")
	}
	identityID := strings.TrimSpace(s.identityManager.GetIdentity().ID)
	if identityID == "" {
		return false, errors.New("identity is not initialized")
	}
	record, ok := s.bindingStore.Get(identityID)
	if !ok {
		return false, nil
	}
	if strings.TrimSpace(nodeID) != "" && strings.TrimSpace(record.NodeID) != strings.TrimSpace(nodeID) {
		return false, errors.New("node id does not match current binding")
	}
	if err := s.bindingStore.Delete(identityID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) consumeValidBindingLink(code string) (pendingNodeBindingLink, bool) {
	now := time.Now().UTC()
	s.bindingLinkMu.Lock()
	defer s.bindingLinkMu.Unlock()
	link, ok := s.bindingLinks[code]
	if !ok {
		return pendingNodeBindingLink{}, false
	}
	delete(s.bindingLinks, code)
	if link.ExpiresAt.Before(now) {
		return pendingNodeBindingLink{}, false
	}
	return link, true
}

func nodeBindingChallengeBytes(identityID, linkCode, nodeID, challenge string) []byte {
	return []byte(fmt.Sprintf("aim-bind-v1|challenge|%s|%s|%s|%s", identityID, linkCode, nodeID, challenge))
}

func nodeBindingAccountBytes(identityID, nodeID, challenge string, ts time.Time) []byte {
	return []byte(fmt.Sprintf("aim-bind-v1|account|%s|%s|%s|%s", identityID, nodeID, challenge, ts.UTC().Format(time.RFC3339Nano)))
}

func randomBase64URL(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

package daemonservice

import (
	"context"
	"errors"
	"strings"
	"time"

	"aim-chat/go-backend/internal/domains/contracts"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
)

const (
	defaultBlobProviderTTL  = 5 * time.Minute
	blobFetchMaxAttempts    = 3
	blobFetchInitialBackoff = 100 * time.Millisecond
	blobFetchRequestTimeout = 2 * time.Second
)

func (s *Service) PutAttachment(name, mimeType, dataBase64 string) (models.AttachmentMeta, error) {
	if err := s.authorizeBlobOperation(s.localPeerID(), "upload"); err != nil {
		return models.AttachmentMeta{}, err
	}
	meta, err := s.identityCore.PutAttachment(name, mimeType, dataBase64)
	if err != nil {
		return models.AttachmentMeta{}, err
	}
	s.announceLocalBlobProvider(meta)
	return meta, nil
}

func (s *Service) CommitAttachmentUpload(uploadID string) (models.AttachmentMeta, error) {
	if err := s.authorizeBlobOperation(s.localPeerID(), "upload"); err != nil {
		return models.AttachmentMeta{}, err
	}
	meta, err := s.identityCore.CommitAttachmentUpload(uploadID)
	if err != nil {
		return models.AttachmentMeta{}, err
	}
	s.announceLocalBlobProvider(meta)
	return meta, nil
}

func (s *Service) GetAttachment(attachmentID string) (models.AttachmentMeta, []byte, error) {
	attachmentID = strings.TrimSpace(attachmentID)
	if attachmentID == "" {
		return models.AttachmentMeta{}, nil, errors.New("attachment id is required")
	}
	meta, data, err := s.identityCore.GetAttachment(attachmentID)
	if err == nil {
		return meta, data, nil
	}
	if cachedMeta, cachedData, ok := s.getEphemeralPublicBlob(attachmentID); ok {
		return cachedMeta, cachedData, nil
	}
	fetchStarted := time.Now()
	peerID := s.localPeerID()
	if peerID == "" {
		s.recordBlobFetchFailure(fetchStarted)
		return models.AttachmentMeta{}, nil, err
	}
	if !s.shouldFetchBlobFromPeers(peerID) {
		return models.AttachmentMeta{}, nil, err
	}
	fetchCtx, cancel := context.WithTimeout(context.Background(), blobFetchRequestTimeout)
	defer cancel()
	fetchedMeta, fetchedData, fetchErr := s.fetchAttachmentFromProviders(fetchCtx, attachmentID, peerID)
	if fetchErr != nil {
		if errors.Is(fetchErr, contracts.ErrAttachmentTemporarilyUnavailable) {
			return models.AttachmentMeta{}, nil, fetchErr
		}
		if errors.Is(fetchErr, contracts.ErrAttachmentAccessDenied) {
			return models.AttachmentMeta{}, nil, fetchErr
		}
		s.recordBlobFetchFailure(fetchStarted)
		return models.AttachmentMeta{}, nil, err
	}
	if s.shouldCacheFetchedBlob() {
		s.cacheFetchedAttachment(fetchedMeta, fetchedData)
	}
	s.announceLocalBlobProvider(fetchedMeta)
	return fetchedMeta, fetchedData, nil
}

func (s *Service) announceLocalBlobProvider(meta models.AttachmentMeta) {
	if !s.runtime.IsNetworking() {
		return
	}
	if !s.shouldAnnounceBlob(meta) {
		return
	}
	peerID := s.localPeerID()
	if peerID == "" {
		return
	}
	if !s.shouldAnnounceBlobFromPeer(peerID) {
		return
	}
	s.announceBlobProvider(meta, peerID, time.Now().UTC())
}

func (s *Service) announceAllLocalBlobProviders() {
	peerID := s.localPeerID()
	if peerID == "" {
		return
	}
	if !s.shouldAnnounceBlobFromPeer(peerID) {
		return
	}
	lister, ok := s.attachmentStore.(interface {
		ListMetas() []models.AttachmentMeta
	})
	if !ok {
		return
	}
	now := time.Now().UTC()
	for _, meta := range lister.ListMetas() {
		if !s.shouldAnnounceBlob(meta) {
			continue
		}
		s.announceBlobProvider(meta, peerID, now)
	}
}

func (s *Service) announceBlobProvider(meta models.AttachmentMeta, peerID string, now time.Time) {
	_ = s.blobProviders.announceBlob(meta.ID, peerID, defaultBlobProviderTTL, s.localBlobFetchProvider(), now)
}

func (s *Service) localBlobFetchProvider() func(string, string) (models.AttachmentMeta, []byte, error) {
	return func(requestBlobID, requesterPeerID string) (models.AttachmentMeta, []byte, error) {
		if err := s.authorizeBlobOperation(requesterPeerID, "fetch"); err != nil {
			return models.AttachmentMeta{}, nil, err
		}
		if !s.isPublicServingAllowed() {
			return models.AttachmentMeta{}, nil, contracts.ErrAttachmentTemporarilyUnavailable
		}
		if !s.allowPublicServeRequest(requesterPeerID) {
			return models.AttachmentMeta{}, nil, contracts.ErrAttachmentTemporarilyUnavailable
		}
		releaseSlot, acquired := s.acquirePublicServeSlot()
		if !acquired {
			return models.AttachmentMeta{}, nil, contracts.ErrAttachmentTemporarilyUnavailable
		}
		defer releaseSlot()
		fetchMeta, fetchData, err := s.getLocalAttachmentOnly(requestBlobID)
		if err != nil {
			return models.AttachmentMeta{}, nil, err
		}
		if s.serveSoftLimiter != nil && !s.serveSoftLimiter.AllowBytes(len(fetchData)) {
			s.markPublicServeSoftCapExceeded()
		}
		if !s.serveLimiter.AllowBytes(len(fetchData)) {
			return models.AttachmentMeta{}, nil, contracts.ErrAttachmentTemporarilyUnavailable
		}
		return fetchMeta, fetchData, nil
	}
}

func (s *Service) fetchAttachmentFromProviders(ctx context.Context, blobID, requesterPeerID string) (models.AttachmentMeta, []byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	hadCandidates := false
	hadAllowedCandidates := false
	hadForbiddenCandidates := false
	hadProviderErrors := false
	timedOut := false
	cancelled := false
	backoff := blobFetchInitialBackoff
	for attempt := 0; attempt < blobFetchMaxAttempts; attempt++ {
		if cerr := ctx.Err(); cerr != nil {
			if errors.Is(cerr, context.DeadlineExceeded) {
				timedOut = true
			} else {
				cancelled = true
			}
			break
		}
		candidates := s.blobProviders.listProviders(blobID, time.Now().UTC())
		if len(candidates) > 0 {
			hadCandidates = true
		}
		for _, candidate := range candidates {
			if cerr := ctx.Err(); cerr != nil {
				if errors.Is(cerr, context.DeadlineExceeded) {
					timedOut = true
				} else {
					cancelled = true
				}
				break
			}
			if strings.TrimSpace(candidate.peerID) == requesterPeerID {
				continue
			}
			if !s.blobProviders.allowFetch(requesterPeerID, candidate.peerID, time.Now().UTC()) {
				continue
			}
			hadAllowedCandidates = true
			meta, data, err := candidate.fetchFn(blobID, requesterPeerID)
			if err != nil {
				if errors.Is(err, contracts.ErrAttachmentAccessDenied) {
					hadForbiddenCandidates = true
					continue
				}
				hadProviderErrors = true
				continue
			}
			if !s.fetchLimiter.AllowBytes(len(data)) {
				continue
			}
			if meta.ID == "" {
				meta.ID = blobID
			}
			s.recordBlobFetchSuccess(started)
			return meta, data, nil
		}
		if attempt < blobFetchMaxAttempts-1 {
			if err := waitWithContext(ctx, backoff); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					timedOut = true
				} else {
					cancelled = true
				}
				break
			}
			backoff *= 2
		}
	}
	reason := "unavailable"
	switch {
	case timedOut:
		reason = "timeout"
	case cancelled:
		reason = "cancelled"
	case !hadCandidates:
		reason = "no_providers"
	case !hadAllowedCandidates:
		reason = "rate_limited"
	case hadForbiddenCandidates:
		reason = "forbidden"
	case hadProviderErrors:
		reason = "providers_failed"
	}
	s.recordBlobFetchUnavailable(reason, started)
	if hadForbiddenCandidates {
		return models.AttachmentMeta{}, nil, contracts.ErrAttachmentAccessDenied
	}
	return models.AttachmentMeta{}, nil, contracts.ErrAttachmentTemporarilyUnavailable
}

func (s *Service) cacheFetchedAttachment(meta models.AttachmentMeta, data []byte) {
	if len(data) == 0 {
		return
	}
	if !s.isPublicStoreEnabled() {
		if s.publicBlobCache != nil {
			_ = s.publicBlobCache.Put(meta, data, time.Now().UTC())
		}
		return
	}
	upserter, ok := s.attachmentStore.(interface {
		PutExisting(meta models.AttachmentMeta, data []byte) error
	})
	if !ok {
		return
	}
	_ = upserter.PutExisting(meta, data)
}

func (s *Service) getLocalAttachmentOnly(attachmentID string) (models.AttachmentMeta, []byte, error) {
	meta, data, err := s.identityCore.GetAttachment(attachmentID)
	if err == nil {
		return meta, data, nil
	}
	if cachedMeta, cachedData, ok := s.getEphemeralPublicBlob(attachmentID); ok {
		return cachedMeta, cachedData, nil
	}
	return models.AttachmentMeta{}, nil, err
}

func (s *Service) getEphemeralPublicBlob(attachmentID string) (models.AttachmentMeta, []byte, bool) {
	if s == nil || s.publicBlobCache == nil {
		return models.AttachmentMeta{}, nil, false
	}
	return s.publicBlobCache.Get(attachmentID, time.Now().UTC())
}

func (s *Service) localPeerID() string {
	identity := s.identityManager.GetIdentity()
	return strings.TrimSpace(identity.ID)
}

func (s *Service) ListBlobProviders(blobID string) ([]models.BlobProviderInfo, error) {
	blobID = strings.TrimSpace(blobID)
	if blobID == "" {
		return nil, errors.New("blob id is required")
	}
	now := time.Now().UTC()
	candidates := s.blobProviders.listProviders(blobID, now)
	out := make([]models.BlobProviderInfo, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, models.BlobProviderInfo{
			PeerID:    candidate.peerID,
			ExpiresAt: candidate.expires,
		})
	}
	return out, nil
}

func (s *Service) PinBlob(blobID string) (models.AttachmentMeta, error) {
	return s.setBlobPinState(blobID, string(models.AttachmentPinStatePinned))
}

func (s *Service) UnpinBlob(blobID string) (models.AttachmentMeta, error) {
	return s.setBlobPinState(blobID, string(models.AttachmentPinStateUnpinned))
}

func (s *Service) GetBlobReplicationMode() string {
	s.replicationMu.RLock()
	mode := s.replicationMode
	s.replicationMu.RUnlock()
	return string(mode)
}

func (s *Service) SetBlobReplicationMode(mode string) error {
	parsedMode, err := parseBlobReplicationMode(mode)
	if err != nil {
		return err
	}
	s.replicationMu.Lock()
	s.replicationMode = parsedMode
	s.replicationMu.Unlock()
	if !s.runtime.IsNetworking() {
		return nil
	}
	s.blobProviders.removePeer(s.localPeerID())
	s.announceAllLocalBlobProviders()
	return nil
}

func (s *Service) GetBlobFeatureFlags() models.BlobFeatureFlags {
	s.replicationMu.RLock()
	flags := s.blobFlags
	s.replicationMu.RUnlock()
	return flags.toModel()
}

func (s *Service) SetBlobFeatureFlags(announceEnabled, fetchEnabled bool, rolloutPercent int) (models.BlobFeatureFlags, error) {
	rolloutPercent = clampRolloutPercent(rolloutPercent)
	s.replicationMu.Lock()
	s.blobFlags = blobFeatureFlags{
		announceEnabled: announceEnabled,
		fetchEnabled:    fetchEnabled,
		rolloutPercent:  rolloutPercent,
	}
	flags := s.blobFlags
	s.replicationMu.Unlock()
	if !s.runtime.IsNetworking() {
		return flags.toModel(), nil
	}
	peerID := s.localPeerID()
	s.blobProviders.removePeer(peerID)
	s.announceAllLocalBlobProviders()
	return flags.toModel(), nil
}

func (s *Service) setBlobPinState(blobID, pinState string) (models.AttachmentMeta, error) {
	if err := s.authorizeBlobOperation(s.localPeerID(), "pin"); err != nil {
		return models.AttachmentMeta{}, err
	}
	blobID = strings.TrimSpace(blobID)
	if blobID == "" {
		return models.AttachmentMeta{}, errors.New("blob id is required")
	}
	pinner, ok := s.attachmentStore.(interface {
		SetPinState(id, pinState string) error
	})
	if !ok {
		return models.AttachmentMeta{}, errors.New("blob pinning is not supported")
	}
	if err := pinner.SetPinState(blobID, pinState); err != nil {
		if errors.Is(err, storage.ErrAttachmentNotFound) {
			return models.AttachmentMeta{}, storage.ErrAttachmentNotFound
		}
		return models.AttachmentMeta{}, err
	}
	meta, _, err := s.identityCore.GetAttachment(blobID)
	if err != nil {
		return models.AttachmentMeta{}, err
	}
	if !s.runtime.IsNetworking() {
		return meta, nil
	}
	if s.shouldAnnounceBlob(meta) {
		s.announceLocalBlobProvider(meta)
		return meta, nil
	}
	s.blobProviders.removeBlobPeer(blobID, s.localPeerID())
	return meta, nil
}

func (s *Service) shouldAnnounceBlob(meta models.AttachmentMeta) bool {
	if strings.TrimSpace(meta.ID) == "" {
		return false
	}
	switch s.currentBlobReplicationMode() {
	case blobReplicationModeNone:
		return false
	case blobReplicationModePinnedOnly:
		return strings.EqualFold(strings.TrimSpace(meta.PinState), string(models.AttachmentPinStatePinned))
	default:
		return true
	}
}

func (s *Service) shouldCacheFetchedBlob() bool {
	return s.currentBlobReplicationMode() != blobReplicationModeNone
}

func (s *Service) shouldAnnounceBlobFromPeer(peerID string) bool {
	s.replicationMu.RLock()
	flags := s.blobFlags
	s.replicationMu.RUnlock()
	return flags.announceEnabled && flags.allowsPeer(peerID)
}

func (s *Service) shouldFetchBlobFromPeers(peerID string) bool {
	s.replicationMu.RLock()
	flags := s.blobFlags
	s.replicationMu.RUnlock()
	return flags.fetchEnabled && flags.allowsPeer(peerID)
}

func (s *Service) currentBlobReplicationMode() blobReplicationMode {
	s.replicationMu.RLock()
	mode := s.replicationMode
	s.replicationMu.RUnlock()
	if mode == "" {
		return blobReplicationModeOnDemand
	}
	return mode
}

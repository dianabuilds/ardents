package daemonservice

import (
	"context"
	"errors"
	"time"

	"aim-chat/go-backend/internal/bootstrap/bootstrapmanager"
	"aim-chat/go-backend/internal/domains/contracts"
	messagingapp "aim-chat/go-backend/internal/domains/messaging"
	runtimeapp "aim-chat/go-backend/internal/platform/runtime"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

func (s *Service) StartNetworking(ctx context.Context) error {
	s.startStopMu.Lock()
	defer s.startStopMu.Unlock()

	if s.runtime.IsNetworking() {
		return nil
	}

	if err := s.wakuNode.Start(ctx); err != nil {
		s.recordError(contracts.ErrorCategoryNetwork, err)
		return err
	}
	localIdentity := s.identityManager.GetIdentity()
	s.wakuNode.SetIdentity(localIdentity.ID)
	if err := s.wakuNode.SubscribePrivate(s.handleIncomingPrivateMessage); err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = s.wakuNode.Stop(stopCtx)
		cancel()
		s.recordError(contracts.ErrorCategoryNetwork, err)
		return err
	}
	s.syncMissedInboundMessages(localIdentity.ID)
	s.announceAllLocalBlobProviders()
	networkCtx, networkCancel := context.WithCancel(ctx)
	s.recoverPendingOnStartup(networkCtx)

	retryCtx, cancel := context.WithCancel(networkCtx)
	if !s.runtime.TryActivate(networkCtx, networkCancel, cancel) {
		cancel()
		networkCancel()
		return nil
	}
	s.startBootstrapRefreshLoop(networkCtx)
	go func() {
		defer s.runtime.RetryLoopDone()
		s.runRetryLoop(retryCtx)
	}()
	s.notifyNetworkStatus(true)
	return nil
}

func (s *Service) syncMissedInboundMessages(identityID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	missed, err := s.wakuNode.FetchPrivateSince(ctx, identityID, time.Now().Add(-24*time.Hour), 500)
	if err != nil {
		s.recordError(contracts.ErrorCategoryNetwork, err)
		return
	}
	for _, msg := range missed {
		s.handleIncomingPrivateMessage(msg)
	}
}

func (s *Service) StopNetworking(ctx context.Context) error {
	s.startStopMu.Lock()
	defer s.startStopMu.Unlock()

	retryCancel, networkCancel, wasRunning := s.runtime.Deactivate()
	if !wasRunning {
		s.blobProviders.removePeer(s.localPeerID())
		return s.cleanupOnStopIfZeroRetention()
	}

	if retryCancel != nil {
		retryCancel()
		s.runtime.WaitRetryLoop()
	}
	s.stopBootstrapRefreshLoop()
	if networkCancel != nil {
		networkCancel()
	}
	if err := s.wakuNode.Stop(ctx); err != nil {
		s.recordError(contracts.ErrorCategoryNetwork, err)
		return err
	}
	s.blobProviders.removePeer(s.localPeerID())
	s.notifyNetworkStatus(true)
	if err := s.cleanupOnStopIfZeroRetention(); err != nil {
		return err
	}
	return nil
}

func (s *Service) runRetryLoop(ctx context.Context) {
	ticker := time.NewTicker(messagingapp.RetryLoopTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.notifyNetworkStatus(false)
			s.enforceRetentionPolicies(time.Now())
			now := time.Now()
			pending := s.messageStore.DuePending(now)
			s.processPendingBatch(ctx, pending, s.handleRetryPublishError)
		}
	}
}

func (s *Service) notifyNetworkStatus(force bool) {
	current := s.GetNetworkStatus()
	shouldNotify := s.runtime.UpdateLastNetworkStatus(current, force)
	if shouldNotify {
		s.notify("notify.network", current)
	}
}

func (s *Service) startBootstrapRefreshLoop(ctx context.Context) {
	if s.wakuCfg == nil {
		return
	}
	if s.wakuCfg.BootstrapManifestPath == "" || s.wakuCfg.BootstrapTrustBundlePath == "" {
		return
	}
	if s.bootstrapCancel != nil {
		return
	}
	baked := bootstrapmanager.BootstrapSet{
		Source:         bootstrapmanager.SourceBaked,
		BootstrapNodes: append([]string(nil), s.wakuCfg.BootstrapNodes...),
		MinPeers:       s.wakuCfg.MinPeers,
		ReconnectPolicy: bootstrapmanager.ReconnectPolicy{
			BaseIntervalMS: int(s.wakuCfg.ReconnectInterval / time.Millisecond),
			MaxIntervalMS:  int(s.wakuCfg.ReconnectBackoffMax / time.Millisecond),
			JitterRatio:    0.2,
		},
	}
	s.bootstrapManager = bootstrapmanager.New(
		s.wakuCfg.BootstrapManifestPath,
		s.wakuCfg.BootstrapTrustBundlePath,
		s.wakuCfg.BootstrapCachePath,
		baked,
	)
	s.bootstrapRefresher = bootstrapmanager.NewRefresher(s.bootstrapManager, s.wakuCfg, func(cfg waku.Config) {
		if applier, ok := s.wakuNode.(interface{ ApplyBootstrapConfig(waku.Config) }); ok {
			applier.ApplyBootstrapConfig(cfg)
		}
	})
	refreshCtx, cancel := context.WithCancel(ctx)
	s.bootstrapCancel = cancel
	s.bootstrapWG.Add(1)
	go func() {
		defer s.bootstrapWG.Done()
		s.bootstrapRefresher.Run(refreshCtx)
	}()
}

func (s *Service) stopBootstrapRefreshLoop() {
	cancel := s.bootstrapCancel
	s.bootstrapCancel = nil
	if cancel != nil {
		cancel()
		s.bootstrapWG.Wait()
	}
}

func (s *Service) recoverPendingOnStartup(ctx context.Context) {
	pending := s.messageStore.DuePending(time.Now().Add(messagingapp.StartupRecoveryLookahead))
	if len(pending) == 0 {
		return
	}
	s.logger.Info("startup recovery", "pending_count", len(pending))
	s.processPendingBatch(ctx, pending, s.handleStartupPublishError)
}

func (s *Service) publishSignedWireWithContext(ctx context.Context, messageID, recipient string, wire contracts.WirePayload) error {
	hardenedWire, delay, err := s.metaHardening.harden(wire)
	if err != nil {
		return contracts.WrapCategorizedError(contracts.ErrorCategoryAPI, err)
	}
	if err := waitWithContext(ctx, delay); err != nil {
		return contracts.WrapCategorizedError(contracts.ErrorCategoryNetwork, err)
	}
	wmsg, err := messagingapp.ComposeSignedPrivateMessage(messageID, recipient, hardenedWire, s.identityManager)
	if err != nil {
		return err
	}
	if err := s.publishWithTimeout(ctx, wmsg); err != nil {
		return contracts.WrapCategorizedError(contracts.ErrorCategoryNetwork, err)
	}
	return nil
}

func (s *Service) markMessageAsSent(messageID string) {
	s.updateMessageStatusAndNotify(messageID, "sent")
	if err := s.messageStore.RemovePending(messageID); err != nil {
		s.recordError(contracts.ErrorCategoryStorage, err)
	}
}

func (s *Service) publishWithTimeout(parent context.Context, msg waku.PrivateMessage) error {
	if parent == nil {
		return errors.New("publish context is not available")
	}
	publishCtx, cancel := context.WithTimeout(parent, runtimeapp.PublishTimeout)
	defer cancel()
	return s.wakuNode.PublishPrivate(publishCtx, msg)
}

func (s *Service) networkContext(category string) (context.Context, error) {
	ctx, ok := s.runtime.CurrentNetworkContext()
	if ok {
		return ctx, nil
	}
	err := errors.New("networking is not started")
	if category == "" {
		return nil, err
	}
	return nil, contracts.WrapCategorizedError(category, err)
}

func (s *Service) GetNetworkStatus() models.NetworkStatus {
	status := s.wakuNode.Status()
	return models.NetworkStatus{
		Status:                   status.State,
		PeerCount:                status.PeerCount,
		LastSync:                 status.LastSync,
		BootstrapSource:          status.BootstrapSource,
		BootstrapManifestVersion: status.BootstrapManifestVersion,
		BootstrapManifestKeyID:   status.BootstrapManifestKeyID,
	}
}

func (s *Service) ListenAddresses() []string {
	return s.wakuNode.ListenAddresses()
}

func (s *Service) processPendingBatch(
	ctx context.Context,
	pending []storage.PendingMessage,
	onPublishError func(storage.PendingMessage, error),
) {
	messagingapp.ProcessPendingMessages(
		ctx,
		pending,
		func(msg models.Message) (contracts.WirePayload, error) {
			return s.buildStoredMessageWire(msg)
		},
		func(parent context.Context, messageID, recipient string, wire contracts.WirePayload) error {
			return s.publishSignedWireWithContext(parent, messageID, recipient, wire)
		},
		onPublishError,
		func(messageID string) {
			s.markMessageAsSent(messageID)
		},
	)
}

func (s *Service) handleRetryPublishError(p storage.PendingMessage, err error) {
	s.recordError(messagingapp.ErrorCategory(err), err)
	nextCount := p.RetryCount + 1
	correlationID := messageCorrelationID(p.Message.ID, p.Message.ContactID)
	if nextCount > 8 {
		s.logWarn("message.retry_limit", correlationID, "message retry limit reached", "message_id", p.Message.ID, "contact_id", p.Message.ContactID, "retry_count", nextCount)
		s.updateMessageStatusAndNotify(p.Message.ID, "failed")
		if remErr := s.messageStore.RemovePending(p.Message.ID); remErr != nil {
			s.recordError(contracts.ErrorCategoryStorage, remErr)
		}
		return
	}
	s.recordRetryAttempt()
	s.logWarn("message.retry_scheduled", correlationID, "message retry scheduled", "message_id", p.Message.ID, "contact_id", p.Message.ContactID, "retry_count", nextCount)
	if perr := s.messageStore.AddOrUpdatePending(p.Message, nextCount, messagingapp.NextRetryTime(nextCount), err.Error()); perr != nil {
		s.recordError(contracts.ErrorCategoryStorage, perr)
	}
}

func (s *Service) handleStartupPublishError(_ storage.PendingMessage, err error) {
	s.recordError(messagingapp.ErrorCategory(err), err)
}

func (s *Service) SubscribeNotifications(cursor int64) ([]contracts.NotificationEvent, <-chan contracts.NotificationEvent, func()) {
	return s.notifier.Subscribe(cursor)
}

func (s *Service) notify(method string, payload any) {
	s.notifier.Publish(method, payload)
}

func (s *Service) notifyMessageStatus(messageID, status string) {
	msg, ok := s.messageStore.GetMessage(messageID)
	if !ok {
		return
	}
	s.notify("notify.message.status", map[string]any{
		"message_id": messageID,
		"contact_id": msg.ContactID,
		"status":     status,
	})
}

func (s *Service) notifySecurityAlert(kind, contactID, message string) {
	s.notify("notify.security.alert", map[string]any{
		"kind":       kind,
		"contact_id": contactID,
		"message":    message,
	})
}

func (s *Service) updateMessageStatusAndNotify(messageID, status string) bool {
	if _, err := s.messageStore.UpdateMessageStatus(messageID, status); err != nil {
		s.recordError(contracts.ErrorCategoryStorage, err)
		return false
	}
	s.notifyMessageStatus(messageID, status)
	return true
}

func (s *Service) GetMetrics() models.MetricsSnapshot {
	status := s.wakuNode.Status()
	counters, groupAggregates, gcEvictionByClass, blobStats, opStats, retries, lastAt := s.metrics.Snapshot()
	usageByClass := map[string]int64{}
	guardrails := map[string]int{}
	if usageReader, ok := s.attachmentStore.(interface {
		UsageByClass() map[string]int64
	}); ok {
		usageByClass = usageReader.UsageByClass()
	}
	if guardrailReader, ok := s.attachmentStore.(interface {
		HardCapStats() map[string]int
	}); ok {
		guardrails = guardrailReader.HardCapStats()
	}
	return models.MetricsSnapshot{
		PeerCount:              status.PeerCount,
		PendingQueueSize:       s.messageStore.PendingCount(),
		ErrorCounters:          counters,
		GroupAggregates:        groupAggregates,
		NetworkMetrics:         s.wakuNode.NetworkMetrics(),
		DiskUsageByClass:       usageByClass,
		GCEvictionCountByClass: gcEvictionByClass,
		BlobFetchStats:         blobStats,
		StorageGuardrails:      guardrails,
		OperationStats:         opStats,
		RetryAttemptsTotal:     retries,
		LastUpdatedAt:          lastAt,
		NotificationBacklog:    s.notifier.BacklogSize(),
	}
}

func (s *Service) recordError(category string, err error) {
	s.recordErrorWithContext(category, err, "service.error", "n/a")
}

func (s *Service) recordRetryAttempt() {
	s.metrics.RecordRetryAttempt()
}

func (s *Service) recordGroupAggregate(name string) {
	s.metrics.RecordGroupAggregate(name)
}

func (s *Service) recordGCEvictions(evictedByClass map[string]int) {
	s.metrics.RecordGCEvictions(evictedByClass)
}

func (s *Service) recordOp(operation string, started time.Time) {
	s.metrics.RecordOp(operation, started)
}

func (s *Service) recordOpError(operation string) {
	s.metrics.RecordOpError(operation)
}

func (s *Service) recordBlobFetchSuccess(started time.Time) {
	s.metrics.RecordBlobFetchSuccess(time.Since(started))
}

func (s *Service) recordBlobFetchUnavailable(reason string, started time.Time) {
	s.metrics.RecordBlobFetchUnavailable(reason, time.Since(started))
}

func (s *Service) recordBlobFetchFailure(started time.Time) {
	s.metrics.RecordBlobFetchFailure(time.Since(started))
}

func (s *Service) trackOperation(operation string, errRef *error) func() {
	started := time.Now()
	return func() {
		s.recordOp(operation, started)
		if errRef != nil && *errRef != nil {
			s.recordOpError(operation)
		}
	}
}

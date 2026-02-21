package rpc

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aim-chat/go-backend/internal/domains/contracts"
	groupdomain "aim-chat/go-backend/internal/domains/group"
	identityusecase "aim-chat/go-backend/internal/domains/identity/usecase"
	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/pkg/models"
)

type channelMockService struct {
	createGroupFn              func(title string) (groupdomain.Group, error)
	getGroupFn                 func(groupID string) (groupdomain.Group, error)
	listGroupsFn               func() ([]groupdomain.Group, error)
	updateGroupTitleFn         func(groupID, title string) (groupdomain.Group, error)
	updateGroupProfileFn       func(groupID, title, description, avatar string) (groupdomain.Group, error)
	deleteGroupFn              func(groupID string) (bool, error)
	listMembersFn              func(groupID string) ([]groupdomain.GroupMember, error)
	sendGroupMessageFn         func(groupID, content string) (groupdomain.GroupMessageFanoutResult, error)
	sendGroupMessageInThreadFn func(groupID, content, threadID string) (groupdomain.GroupMessageFanoutResult, error)
	listGroupThreadFn          func(groupID, threadID string, limit, offset int) ([]models.Message, error)
	getIdentityFn              func() (models.Identity, error)
	restoreBackupFn            func(consentToken, passphrase, backupBlob string) (models.Identity, error)
	getStoragePolicyFn         func() (privacydomain.StoragePolicy, error)
	updateStoragePolicyFn      func(
		storageProtection string,
		retention string,
		messageTTLSeconds int,
		imageTTLSeconds int,
		fileTTLSeconds int,
		imageQuotaMB int,
		fileQuotaMB int,
		imageMaxItemSizeMB int,
		fileMaxItemSizeMB int,
	) (privacydomain.StoragePolicy, error)
	setStorageScopeOverrideFn func(
		scope string,
		scopeID string,
		storageProtection string,
		retention string,
		messageTTLSeconds int,
		imageTTLSeconds int,
		fileTTLSeconds int,
		imageQuotaMB int,
		fileQuotaMB int,
		imageMaxItemSizeMB int,
		fileMaxItemSizeMB int,
		infiniteTTL bool,
		pinRequiredForInfinite bool,
	) (privacydomain.StoragePolicyOverride, error)
	getStorageScopeOverrideFn    func(scope string, scopeID string) (privacydomain.StoragePolicyOverride, bool, error)
	removeStorageScopeOverrideFn func(scope string, scopeID string) (bool, error)
	resolveStoragePolicyFn       func(scope string, scopeID string, isPinned bool) (privacydomain.StoragePolicy, error)
	initAttachmentUploadFn       func(name, mimeType string, totalSize int64, totalChunks, chunkSize int, fileSHA256 string) (identityusecase.AttachmentUploadInitResult, error)
	putAttachmentChunkFn         func(uploadID string, chunkIndex int, dataBase64, chunkSHA256 string) (identityusecase.AttachmentUploadChunkResult, error)
	getAttachmentUploadStatusFn  func(uploadID string) (identityusecase.AttachmentUploadStatus, error)
	commitAttachmentUploadFn     func(uploadID string) (models.AttachmentMeta, error)
	listBlobProvidersFn          func(blobID string) ([]models.BlobProviderInfo, error)
	pinBlobFn                    func(blobID string) (models.AttachmentMeta, error)
	unpinBlobFn                  func(blobID string) (models.AttachmentMeta, error)
	setBlobReplicationModeFn     func(mode string) error
	getBlobReplicationModeFn     func() string
	setBlobFeatureFlagsFn        func(announceEnabled, fetchEnabled bool, rolloutPercent int) (models.BlobFeatureFlags, error)
	getBlobFeatureFlagsFn        func() models.BlobFeatureFlags
	setBlobACLPolicyFn           func(mode string, allowlist []string) (models.BlobACLPolicy, error)
	getBlobACLPolicyFn           func() models.BlobACLPolicy
	setBlobNodePresetFn          func(preset string) (models.BlobNodePresetConfig, error)
	getBlobNodePresetFn          func() models.BlobNodePresetConfig
	createNodeBindingLinkFn      func(ttlSeconds int) (models.NodeBindingLinkCode, error)
	completeNodeBindingFn        func(linkCode, nodeID, nodePublicKeyBase64, nodeSignatureBase64 string, allowRebind bool) (models.NodeBindingRecord, error)
	getNodeBindingFn             func() (models.NodeBindingRecord, bool, error)
	unbindNodeFn                 func(nodeID string, confirm bool) (bool, error)
	listAccountsFn               func() ([]contracts.AccountProfile, error)
	getCurrentAccountFn          func() (contracts.AccountProfile, error)
	switchAccountFn              func(accountID string) (models.Identity, error)
}

func (m *channelMockService) Logout() error { return nil }
func (m *channelMockService) GetIdentity() (models.Identity, error) {
	if m.getIdentityFn != nil {
		return m.getIdentityFn()
	}
	return models.Identity{}, nil
}
func (m *channelMockService) SelfContactCard(_ string) (models.ContactCard, error) {
	return models.ContactCard{}, nil
}
func (m *channelMockService) CreateIdentity(_ string) (models.Identity, string, error) {
	return models.Identity{}, "", nil
}
func (m *channelMockService) ExportSeed(_ string) (string, error) { return "", nil }
func (m *channelMockService) ExportBackup(_, _ string) (string, error) {
	return "", nil
}
func (m *channelMockService) RestoreBackup(consentToken, passphrase, backupBlob string) (models.Identity, error) {
	if m.restoreBackupFn != nil {
		return m.restoreBackupFn(consentToken, passphrase, backupBlob)
	}
	return models.Identity{}, nil
}
func (m *channelMockService) ImportIdentity(_, _ string) (models.Identity, error) {
	return models.Identity{}, nil
}
func (m *channelMockService) ValidateMnemonic(_ string) bool { return true }
func (m *channelMockService) ChangePassword(_, _ string) error {
	return nil
}
func (m *channelMockService) ListAccounts() ([]contracts.AccountProfile, error) {
	if m.listAccountsFn != nil {
		return m.listAccountsFn()
	}
	return nil, nil
}
func (m *channelMockService) GetCurrentAccount() (contracts.AccountProfile, error) {
	if m.getCurrentAccountFn != nil {
		return m.getCurrentAccountFn()
	}
	return contracts.AccountProfile{}, nil
}
func (m *channelMockService) SwitchAccount(accountID string) (models.Identity, error) {
	if m.switchAccountFn != nil {
		return m.switchAccountFn(accountID)
	}
	return models.Identity{}, nil
}
func (m *channelMockService) AddContactCard(_ models.ContactCard) error { return nil }
func (m *channelMockService) VerifyContactCard(_ models.ContactCard) (bool, error) {
	return true, nil
}
func (m *channelMockService) PutAttachment(_, _, _ string) (models.AttachmentMeta, error) {
	return models.AttachmentMeta{}, nil
}
func (m *channelMockService) GetAttachment(_ string) (models.AttachmentMeta, []byte, error) {
	return models.AttachmentMeta{}, nil, nil
}
func (m *channelMockService) AddContact(_, _ string) error           { return nil }
func (m *channelMockService) RemoveContact(_ string) error           { return nil }
func (m *channelMockService) GetContacts() ([]models.Contact, error) { return nil, nil }
func (m *channelMockService) ListDevices() ([]models.Device, error)  { return nil, nil }
func (m *channelMockService) AddDevice(_ string) (models.Device, error) {
	return models.Device{}, nil
}
func (m *channelMockService) RevokeDevice(_ string) (models.DeviceRevocation, error) {
	return models.DeviceRevocation{}, nil
}
func (m *channelMockService) InitAttachmentUpload(name, mimeType string, totalSize int64, totalChunks, chunkSize int, fileSHA256 string) (identityusecase.AttachmentUploadInitResult, error) {
	if m.initAttachmentUploadFn != nil {
		return m.initAttachmentUploadFn(name, mimeType, totalSize, totalChunks, chunkSize, fileSHA256)
	}
	return identityusecase.AttachmentUploadInitResult{}, nil
}
func (m *channelMockService) PutAttachmentChunk(uploadID string, chunkIndex int, dataBase64, chunkSHA256 string) (identityusecase.AttachmentUploadChunkResult, error) {
	if m.putAttachmentChunkFn != nil {
		return m.putAttachmentChunkFn(uploadID, chunkIndex, dataBase64, chunkSHA256)
	}
	return identityusecase.AttachmentUploadChunkResult{}, nil
}
func (m *channelMockService) GetAttachmentUploadStatus(uploadID string) (identityusecase.AttachmentUploadStatus, error) {
	if m.getAttachmentUploadStatusFn != nil {
		return m.getAttachmentUploadStatusFn(uploadID)
	}
	return identityusecase.AttachmentUploadStatus{}, nil
}
func (m *channelMockService) CommitAttachmentUpload(uploadID string) (models.AttachmentMeta, error) {
	if m.commitAttachmentUploadFn != nil {
		return m.commitAttachmentUploadFn(uploadID)
	}
	return models.AttachmentMeta{}, nil
}
func (m *channelMockService) ListBlobProviders(blobID string) ([]models.BlobProviderInfo, error) {
	if m.listBlobProvidersFn != nil {
		return m.listBlobProvidersFn(blobID)
	}
	return nil, nil
}
func (m *channelMockService) PinBlob(blobID string) (models.AttachmentMeta, error) {
	if m.pinBlobFn != nil {
		return m.pinBlobFn(blobID)
	}
	return models.AttachmentMeta{}, nil
}
func (m *channelMockService) UnpinBlob(blobID string) (models.AttachmentMeta, error) {
	if m.unpinBlobFn != nil {
		return m.unpinBlobFn(blobID)
	}
	return models.AttachmentMeta{}, nil
}
func (m *channelMockService) SetBlobReplicationMode(mode string) error {
	if m.setBlobReplicationModeFn != nil {
		return m.setBlobReplicationModeFn(mode)
	}
	return nil
}
func (m *channelMockService) GetBlobReplicationMode() string {
	if m.getBlobReplicationModeFn != nil {
		return m.getBlobReplicationModeFn()
	}
	return "on_demand"
}
func (m *channelMockService) SetBlobFeatureFlags(announceEnabled, fetchEnabled bool, rolloutPercent int) (models.BlobFeatureFlags, error) {
	if m.setBlobFeatureFlagsFn != nil {
		return m.setBlobFeatureFlagsFn(announceEnabled, fetchEnabled, rolloutPercent)
	}
	return models.BlobFeatureFlags{
		AnnounceEnabled: announceEnabled,
		FetchEnabled:    fetchEnabled,
		RolloutPercent:  rolloutPercent,
	}, nil
}
func (m *channelMockService) GetBlobFeatureFlags() models.BlobFeatureFlags {
	if m.getBlobFeatureFlagsFn != nil {
		return m.getBlobFeatureFlagsFn()
	}
	return models.BlobFeatureFlags{
		AnnounceEnabled: true,
		FetchEnabled:    true,
		RolloutPercent:  100,
	}
}
func (m *channelMockService) SetBlobACLPolicy(mode string, allowlist []string) (models.BlobACLPolicy, error) {
	if m.setBlobACLPolicyFn != nil {
		return m.setBlobACLPolicyFn(mode, allowlist)
	}
	return models.BlobACLPolicy{
		Mode:      mode,
		Allowlist: allowlist,
		Enforced:  true,
	}, nil
}
func (m *channelMockService) GetBlobACLPolicy() models.BlobACLPolicy {
	if m.getBlobACLPolicyFn != nil {
		return m.getBlobACLPolicyFn()
	}
	return models.BlobACLPolicy{
		Mode:     "owner_contacts",
		Enforced: true,
	}
}
func (m *channelMockService) SetBlobNodePreset(preset string) (models.BlobNodePresetConfig, error) {
	if m.setBlobNodePresetFn != nil {
		return m.setBlobNodePresetFn(preset)
	}
	return models.BlobNodePresetConfig{Preset: preset}, nil
}
func (m *channelMockService) GetBlobNodePreset() models.BlobNodePresetConfig {
	if m.getBlobNodePresetFn != nil {
		return m.getBlobNodePresetFn()
	}
	return models.BlobNodePresetConfig{Preset: "custom"}
}
func (m *channelMockService) CreateNodeBindingLinkCode(ttlSeconds int) (models.NodeBindingLinkCode, error) {
	if m.createNodeBindingLinkFn != nil {
		return m.createNodeBindingLinkFn(ttlSeconds)
	}
	return models.NodeBindingLinkCode{}, nil
}
func (m *channelMockService) CompleteNodeBinding(linkCode, nodeID, nodePublicKeyBase64, nodeSignatureBase64 string, allowRebind bool) (models.NodeBindingRecord, error) {
	if m.completeNodeBindingFn != nil {
		return m.completeNodeBindingFn(linkCode, nodeID, nodePublicKeyBase64, nodeSignatureBase64, allowRebind)
	}
	return models.NodeBindingRecord{}, nil
}
func (m *channelMockService) GetNodeBinding() (models.NodeBindingRecord, bool, error) {
	if m.getNodeBindingFn != nil {
		return m.getNodeBindingFn()
	}
	return models.NodeBindingRecord{}, false, nil
}
func (m *channelMockService) UnbindNode(nodeID string, confirm bool) (bool, error) {
	if m.unbindNodeFn != nil {
		return m.unbindNodeFn(nodeID, confirm)
	}
	return false, nil
}
func (m *channelMockService) SendMessage(_, _ string) (string, error) {
	return "", nil
}
func (m *channelMockService) SendMessageInThread(_, _, _ string) (string, error) {
	return "", nil
}
func (m *channelMockService) EditMessage(_, _, _ string) (models.Message, error) {
	return models.Message{}, nil
}
func (m *channelMockService) DeleteMessage(_, _ string) error { return nil }
func (m *channelMockService) ClearMessages(_ string) (int, error) {
	return 0, nil
}
func (m *channelMockService) GetMessages(_ string, _, _ int) ([]models.Message, error) {
	return nil, nil
}
func (m *channelMockService) GetMessagesByThread(_, _ string, _, _ int) ([]models.Message, error) {
	return nil, nil
}
func (m *channelMockService) GetMessageStatus(_ string) (models.MessageStatus, error) {
	return models.MessageStatus{}, nil
}
func (m *channelMockService) InitSession(_ string, _ []byte) (models.SessionState, error) {
	return models.SessionState{}, nil
}
func (m *channelMockService) CreateGroup(title string) (groupdomain.Group, error) {
	if m.createGroupFn != nil {
		return m.createGroupFn(title)
	}
	return groupdomain.Group{}, nil
}
func (m *channelMockService) GetGroup(groupID string) (groupdomain.Group, error) {
	if m.getGroupFn != nil {
		return m.getGroupFn(groupID)
	}
	return groupdomain.Group{}, nil
}
func (m *channelMockService) ListGroups() ([]groupdomain.Group, error) {
	if m.listGroupsFn != nil {
		return m.listGroupsFn()
	}
	return nil, nil
}
func (m *channelMockService) UpdateGroupTitle(groupID, title string) (groupdomain.Group, error) {
	if m.updateGroupTitleFn != nil {
		return m.updateGroupTitleFn(groupID, title)
	}
	return groupdomain.Group{ID: groupID, Title: title}, nil
}
func (m *channelMockService) UpdateGroupProfile(groupID, title, description, avatar string) (groupdomain.Group, error) {
	if m.updateGroupProfileFn != nil {
		return m.updateGroupProfileFn(groupID, title, description, avatar)
	}
	return groupdomain.Group{ID: groupID, Title: title, Description: description, Avatar: avatar}, nil
}
func (m *channelMockService) DeleteGroup(groupID string) (bool, error) {
	if m.deleteGroupFn != nil {
		return m.deleteGroupFn(groupID)
	}
	return true, nil
}
func (m *channelMockService) ListGroupMembers(groupID string) ([]groupdomain.GroupMember, error) {
	if m.listMembersFn != nil {
		return m.listMembersFn(groupID)
	}
	return nil, nil
}
func (m *channelMockService) LeaveGroup(_ string) (bool, error) { return false, nil }
func (m *channelMockService) InviteToGroup(_, _ string) (groupdomain.GroupMember, error) {
	return groupdomain.GroupMember{}, nil
}
func (m *channelMockService) AcceptGroupInvite(_ string) (bool, error)  { return false, nil }
func (m *channelMockService) DeclineGroupInvite(_ string) (bool, error) { return false, nil }
func (m *channelMockService) RemoveGroupMember(_, _ string) (bool, error) {
	return false, nil
}
func (m *channelMockService) PromoteGroupMember(_, _ string) (groupdomain.GroupMember, error) {
	return groupdomain.GroupMember{}, nil
}
func (m *channelMockService) DemoteGroupMember(_, _ string) (groupdomain.GroupMember, error) {
	return groupdomain.GroupMember{}, nil
}
func (m *channelMockService) SendGroupMessage(groupID, content string) (groupdomain.GroupMessageFanoutResult, error) {
	if m.sendGroupMessageFn != nil {
		return m.sendGroupMessageFn(groupID, content)
	}
	return groupdomain.GroupMessageFanoutResult{}, nil
}
func (m *channelMockService) SendGroupMessageInThread(groupID, content, threadID string) (groupdomain.GroupMessageFanoutResult, error) {
	if m.sendGroupMessageInThreadFn != nil {
		return m.sendGroupMessageInThreadFn(groupID, content, threadID)
	}
	if m.sendGroupMessageFn != nil {
		return m.sendGroupMessageFn(groupID, content)
	}
	return groupdomain.GroupMessageFanoutResult{}, nil
}
func (m *channelMockService) ListGroupMessages(_ string, _, _ int) ([]models.Message, error) {
	return nil, nil
}
func (m *channelMockService) ListGroupMessagesByThread(groupID, threadID string, limit, offset int) ([]models.Message, error) {
	if m.listGroupThreadFn != nil {
		return m.listGroupThreadFn(groupID, threadID, limit, offset)
	}
	return nil, nil
}
func (m *channelMockService) GetGroupMessageStatus(_, _ string) (models.MessageStatus, error) {
	return models.MessageStatus{}, nil
}
func (m *channelMockService) DeleteGroupMessage(_, _ string) error { return nil }
func (m *channelMockService) ListMessageRequests() ([]models.MessageRequest, error) {
	return nil, nil
}
func (m *channelMockService) GetMessageRequest(_ string) (models.MessageRequestThread, error) {
	return models.MessageRequestThread{}, nil
}
func (m *channelMockService) AcceptMessageRequest(_ string) (bool, error)  { return false, nil }
func (m *channelMockService) DeclineMessageRequest(_ string) (bool, error) { return false, nil }
func (m *channelMockService) BlockSender(_ string) (models.BlockSenderResult, error) {
	return models.BlockSenderResult{}, nil
}
func (m *channelMockService) GetPrivacySettings() (privacydomain.PrivacySettings, error) {
	return privacydomain.PrivacySettings{}, nil
}
func (m *channelMockService) UpdatePrivacySettings(_ string) (privacydomain.PrivacySettings, error) {
	return privacydomain.PrivacySettings{}, nil
}
func (m *channelMockService) GetStoragePolicy() (privacydomain.StoragePolicy, error) {
	if m.getStoragePolicyFn != nil {
		return m.getStoragePolicyFn()
	}
	return privacydomain.StoragePolicy{}, nil
}
func (m *channelMockService) UpdateStoragePolicy(
	storageProtection string,
	retention string,
	messageTTLSeconds int,
	imageTTLSeconds int,
	fileTTLSeconds int,
	imageQuotaMB int,
	fileQuotaMB int,
	imageMaxItemSizeMB int,
	fileMaxItemSizeMB int,
) (privacydomain.StoragePolicy, error) {
	if m.updateStoragePolicyFn != nil {
		return m.updateStoragePolicyFn(
			storageProtection,
			retention,
			messageTTLSeconds,
			imageTTLSeconds,
			fileTTLSeconds,
			imageQuotaMB,
			fileQuotaMB,
			imageMaxItemSizeMB,
			fileMaxItemSizeMB,
		)
	}
	return privacydomain.StoragePolicy{}, nil
}
func (m *channelMockService) SetStorageScopeOverride(
	scope string,
	scopeID string,
	storageProtection string,
	retention string,
	messageTTLSeconds int,
	imageTTLSeconds int,
	fileTTLSeconds int,
	imageQuotaMB int,
	fileQuotaMB int,
	imageMaxItemSizeMB int,
	fileMaxItemSizeMB int,
	infiniteTTL bool,
	pinRequiredForInfinite bool,
) (privacydomain.StoragePolicyOverride, error) {
	if m.setStorageScopeOverrideFn != nil {
		return m.setStorageScopeOverrideFn(
			scope,
			scopeID,
			storageProtection,
			retention,
			messageTTLSeconds,
			imageTTLSeconds,
			fileTTLSeconds,
			imageQuotaMB,
			fileQuotaMB,
			imageMaxItemSizeMB,
			fileMaxItemSizeMB,
			infiniteTTL,
			pinRequiredForInfinite,
		)
	}
	return privacydomain.StoragePolicyOverride{}, nil
}
func (m *channelMockService) GetStorageScopeOverride(scope string, scopeID string) (privacydomain.StoragePolicyOverride, bool, error) {
	if m.getStorageScopeOverrideFn != nil {
		return m.getStorageScopeOverrideFn(scope, scopeID)
	}
	return privacydomain.StoragePolicyOverride{}, false, nil
}
func (m *channelMockService) RemoveStorageScopeOverride(scope string, scopeID string) (bool, error) {
	if m.removeStorageScopeOverrideFn != nil {
		return m.removeStorageScopeOverrideFn(scope, scopeID)
	}
	return false, nil
}
func (m *channelMockService) ResolveStoragePolicy(scope string, scopeID string, isPinned bool) (privacydomain.StoragePolicy, error) {
	if m.resolveStoragePolicyFn != nil {
		return m.resolveStoragePolicyFn(scope, scopeID, isPinned)
	}
	return privacydomain.StoragePolicy{}, nil
}
func (m *channelMockService) GetBlocklist() ([]string, error) { return nil, nil }
func (m *channelMockService) AddToBlocklist(_ string) ([]string, error) {
	return nil, nil
}
func (m *channelMockService) RemoveFromBlocklist(_ string) ([]string, error) {
	return nil, nil
}
func (m *channelMockService) GetNetworkStatus() models.NetworkStatus  { return models.NetworkStatus{} }
func (m *channelMockService) GetMetrics() models.MetricsSnapshot      { return models.MetricsSnapshot{} }
func (m *channelMockService) StartNetworking(_ context.Context) error { return nil }
func (m *channelMockService) StopNetworking(_ context.Context) error  { return nil }
func (m *channelMockService) SubscribeNotifications(_ int64) ([]contracts.NotificationEvent, <-chan contracts.NotificationEvent, func()) {
	ch := make(chan contracts.NotificationEvent)
	close(ch)
	return nil, ch, func() {}
}
func (m *channelMockService) ListenAddresses() []string { return nil }

func TestDispatchRPCChannelCreateEncodesTitle(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	var createdTitle string
	svc := &channelMockService{
		createGroupFn: func(title string) (groupdomain.Group, error) {
			createdTitle = title
			return groupdomain.Group{ID: "g1", Title: title}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	params, _ := json.Marshal([]string{"news", "private", "Announcements"})
	result, rpcErr := s.dispatchRPC("channel.create", params)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if createdTitle != "[channel:private] news" {
		t.Fatalf("unexpected encoded title: %q", createdTitle)
	}
	group, ok := result.(groupdomain.Group)
	if !ok {
		t.Fatalf("expected group result")
	}
	if group.Title != "[channel:private] news" {
		t.Fatalf("unexpected group title: %q", group.Title)
	}
}

func TestDispatchRPCBackupRestore(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	var gotConsent, gotPassphrase, gotBlob string
	svc := &channelMockService{
		restoreBackupFn: func(consentToken, passphrase, backupBlob string) (models.Identity, error) {
			gotConsent = consentToken
			gotPassphrase = passphrase
			gotBlob = backupBlob
			return models.Identity{ID: "aim1restored"}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	params, _ := json.Marshal([]string{"I_UNDERSTAND_BACKUP_RISK", "secret-pass", "blob-data"})
	result, rpcErr := s.dispatchRPC("backup.restore", params)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if gotConsent != "I_UNDERSTAND_BACKUP_RISK" || gotPassphrase != "secret-pass" || gotBlob != "blob-data" {
		t.Fatalf("unexpected restore input: consent=%q passphrase=%q blob=%q", gotConsent, gotPassphrase, gotBlob)
	}
	payload, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result payload type: %#v", result)
	}
	identity, ok := payload["identity"].(models.Identity)
	if !ok {
		t.Fatalf("unexpected identity payload: %#v", payload["identity"])
	}
	if identity.ID != "aim1restored" {
		t.Fatalf("unexpected identity id: %q", identity.ID)
	}
}

func TestDispatchRPCChannelListFiltersGroups(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	svc := &channelMockService{
		listGroupsFn: func() ([]groupdomain.Group, error) {
			return []groupdomain.Group{
				{ID: "c1", Title: "[channel:public] Updates"},
				{ID: "g1", Title: "Plain Group"},
				{ID: "c2", Title: "[channel:private] Team"},
			}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	result, rpcErr := s.dispatchRPC("channel.list", nil)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	channels, ok := result.([]groupdomain.Group)
	if !ok {
		t.Fatalf("expected []groupdomain.Group result")
	}
	if len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(channels))
	}
}

func TestDispatchRPCChannelSendDelegatesRoleChecksToDomain(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	sendCalled := false
	svc := &channelMockService{
		getGroupFn: func(groupID string) (groupdomain.Group, error) {
			return groupdomain.Group{ID: groupID, Title: "[channel:public] General"}, nil
		},
		getIdentityFn: func() (models.Identity, error) {
			return models.Identity{ID: "aim1user"}, nil
		},
		listMembersFn: func(groupID string) ([]groupdomain.GroupMember, error) {
			return []groupdomain.GroupMember{
				{GroupID: groupID, MemberID: "aim1user", Role: groupdomain.GroupMemberRoleUser, Status: groupdomain.GroupMemberStatusActive},
			}, nil
		},
		sendGroupMessageFn: func(groupID, content string) (groupdomain.GroupMessageFanoutResult, error) {
			sendCalled = true
			return groupdomain.GroupMessageFanoutResult{GroupID: groupID, EventID: "evt"}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	params, _ := json.Marshal([]string{"g1", "hello"})
	result, rpcErr := s.dispatchRPC("channel.send", params)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if !sendCalled {
		t.Fatalf("send should be delegated to service")
	}
	if _, ok := result.(groupdomain.GroupMessageFanoutResult); !ok {
		t.Fatalf("expected fanout result")
	}
}

func TestDispatchRPCChannelSendForAdmin(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	svc := &channelMockService{
		getGroupFn: func(groupID string) (groupdomain.Group, error) {
			return groupdomain.Group{ID: groupID, Title: "[channel:public] General"}, nil
		},
		getIdentityFn: func() (models.Identity, error) {
			return models.Identity{ID: "aim1admin"}, nil
		},
		listMembersFn: func(groupID string) ([]groupdomain.GroupMember, error) {
			return []groupdomain.GroupMember{
				{GroupID: groupID, MemberID: "aim1admin", Role: groupdomain.GroupMemberRoleAdmin, Status: groupdomain.GroupMemberStatusActive},
			}, nil
		},
		sendGroupMessageFn: func(groupID, content string) (groupdomain.GroupMessageFanoutResult, error) {
			return groupdomain.GroupMessageFanoutResult{
				GroupID:   groupID,
				EventID:   "evt1",
				Attempted: 1,
				Delivered: 1,
			}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	params, _ := json.Marshal([]string{"g1", "hello"})
	result, rpcErr := s.dispatchRPC("channel.send", params)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	fanout, ok := result.(groupdomain.GroupMessageFanoutResult)
	if !ok {
		t.Fatalf("expected fanout result")
	}
	if fanout.Delivered != 1 {
		t.Fatalf("expected delivered=1, got %d", fanout.Delivered)
	}
}

func TestDispatchRPCChannelGetRejectsNonChannel(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	svc := &channelMockService{
		getGroupFn: func(groupID string) (groupdomain.Group, error) {
			return groupdomain.Group{ID: groupID, Title: "Plain Group"}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	params, _ := json.Marshal([]string{"g1"})
	_, rpcErr := s.dispatchRPC("channel.get", params)
	if rpcErr == nil {
		t.Fatalf("expected rpc error")
	}
	if rpcErr.Code != -32201 {
		t.Fatalf("expected rpc code -32201, got %d", rpcErr.Code)
	}
	if rpcErr.Message != "group not found" {
		t.Fatalf("unexpected message: %q", rpcErr.Message)
	}
}

func TestDispatchRPCChannelThreadSendForAdmin(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	var gotThreadID string
	svc := &channelMockService{
		getGroupFn: func(groupID string) (groupdomain.Group, error) {
			return groupdomain.Group{ID: groupID, Title: "[channel:public] General"}, nil
		},
		getIdentityFn: func() (models.Identity, error) {
			return models.Identity{ID: "aim1admin"}, nil
		},
		listMembersFn: func(groupID string) ([]groupdomain.GroupMember, error) {
			return []groupdomain.GroupMember{
				{GroupID: groupID, MemberID: "aim1admin", Role: groupdomain.GroupMemberRoleAdmin, Status: groupdomain.GroupMemberStatusActive},
			}, nil
		},
		sendGroupMessageInThreadFn: func(groupID, content, threadID string) (groupdomain.GroupMessageFanoutResult, error) {
			gotThreadID = threadID
			return groupdomain.GroupMessageFanoutResult{GroupID: groupID, EventID: "evt-thread"}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	params, _ := json.Marshal([]string{"g1", "hello", "th-1"})
	_, rpcErr := s.dispatchRPC("channel.thread.send", params)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if gotThreadID != "th-1" {
		t.Fatalf("expected thread id th-1, got %q", gotThreadID)
	}
}

func TestDispatchRPCPrivacyStorageGet(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	svc := &channelMockService{
		getStoragePolicyFn: func() (privacydomain.StoragePolicy, error) {
			return privacydomain.StoragePolicy{
				StorageProtection:    privacydomain.StorageProtectionProtected,
				ContentRetentionMode: privacydomain.RetentionZeroRetention,
			}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	result, rpcErr := s.dispatchRPC("privacy.storage.get", nil)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	policy, ok := result.(privacydomain.StoragePolicy)
	if !ok {
		t.Fatalf("expected privacydomain.StoragePolicy result")
	}
	if policy.ContentRetentionMode != privacydomain.RetentionZeroRetention {
		t.Fatalf("unexpected retention mode: %q", policy.ContentRetentionMode)
	}
}

func TestDispatchRPCPrivacyStorageSet(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	var gotProtection string
	var gotRetention string
	var gotMessageTTL int
	var gotImageTTL int
	var gotFileTTL int
	var gotImageQuotaMB int
	var gotFileQuotaMB int
	var gotImageMaxItemSizeMB int
	var gotFileMaxItemSizeMB int

	svc := &channelMockService{
		updateStoragePolicyFn: func(
			storageProtection string,
			retention string,
			messageTTLSeconds int,
			imageTTLSeconds int,
			fileTTLSeconds int,
			imageQuotaMB int,
			fileQuotaMB int,
			imageMaxItemSizeMB int,
			fileMaxItemSizeMB int,
		) (privacydomain.StoragePolicy, error) {
			gotProtection = storageProtection
			gotRetention = retention
			gotMessageTTL = messageTTLSeconds
			gotImageTTL = imageTTLSeconds
			gotFileTTL = fileTTLSeconds
			gotImageQuotaMB = imageQuotaMB
			gotFileQuotaMB = fileQuotaMB
			gotImageMaxItemSizeMB = imageMaxItemSizeMB
			gotFileMaxItemSizeMB = fileMaxItemSizeMB
			return privacydomain.StoragePolicy{
				StorageProtection:    privacydomain.StorageProtectionMode(storageProtection),
				ContentRetentionMode: privacydomain.ContentRetentionMode(retention),
				MessageTTLSeconds:    messageTTLSeconds,
				ImageTTLSeconds:      imageTTLSeconds,
				FileTTLSeconds:       fileTTLSeconds,
				ImageQuotaMB:         imageQuotaMB,
				FileQuotaMB:          fileQuotaMB,
				ImageMaxItemSizeMB:   imageMaxItemSizeMB,
				FileMaxItemSizeMB:    fileMaxItemSizeMB,
			}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	params, _ := json.Marshal([]map[string]any{{
		"storage_protection_mode": "standard",
		"content_retention_mode":  "ephemeral",
		"message_ttl_seconds":     30,
		"image_ttl_seconds":       45,
		"file_ttl_seconds":        60,
		"image_quota_mb":          512,
		"file_quota_mb":           256,
		"image_max_item_size_mb":  25,
		"file_max_item_size_mb":   10,
	}})
	result, rpcErr := s.dispatchRPC("privacy.storage.set", params)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if gotProtection != "standard" || gotRetention != "ephemeral" || gotMessageTTL != 30 || gotImageTTL != 45 || gotFileTTL != 60 {
		t.Fatalf("unexpected ttl params: protection=%q retention=%q messageTTL=%d imageTTL=%d fileTTL=%d", gotProtection, gotRetention, gotMessageTTL, gotImageTTL, gotFileTTL)
	}
	if gotImageQuotaMB != 512 || gotFileQuotaMB != 256 || gotImageMaxItemSizeMB != 25 || gotFileMaxItemSizeMB != 10 {
		t.Fatalf("unexpected class params: imageQuota=%d fileQuota=%d imageMaxItem=%d fileMaxItem=%d", gotImageQuotaMB, gotFileQuotaMB, gotImageMaxItemSizeMB, gotFileMaxItemSizeMB)
	}
	policy, ok := result.(privacydomain.StoragePolicy)
	if !ok {
		t.Fatalf("expected privacydomain.StoragePolicy result")
	}
	if policy.MessageTTLSeconds != 30 || policy.ImageTTLSeconds != 45 || policy.FileTTLSeconds != 60 {
		t.Fatalf("unexpected ttl in result: %+v", policy)
	}
	if policy.ImageQuotaMB != 512 || policy.FileQuotaMB != 256 || policy.ImageMaxItemSizeMB != 25 || policy.FileMaxItemSizeMB != 10 {
		t.Fatalf("unexpected class policy in result: %+v", policy)
	}
}

func TestDispatchRPCPrivacyStorageScopeFlow(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	stored := privacydomain.StoragePolicyOverride{}
	svc := &channelMockService{
		setStorageScopeOverrideFn: func(
			scope string,
			scopeID string,
			storageProtection string,
			retention string,
			messageTTLSeconds int,
			imageTTLSeconds int,
			fileTTLSeconds int,
			imageQuotaMB int,
			fileQuotaMB int,
			imageMaxItemSizeMB int,
			fileMaxItemSizeMB int,
			infiniteTTL bool,
			pinRequiredForInfinite bool,
		) (privacydomain.StoragePolicyOverride, error) {
			if scope != "group" || scopeID != "g1" || !infiniteTTL || !pinRequiredForInfinite {
				t.Fatalf("unexpected scope override args")
			}
			stored = privacydomain.StoragePolicyOverride{
				StorageProtection:      privacydomain.StorageProtectionProtected,
				ContentRetentionMode:   privacydomain.RetentionPersistent,
				InfiniteTTL:            true,
				PinRequiredForInfinite: true,
			}
			return stored, nil
		},
		getStorageScopeOverrideFn: func(scope string, scopeID string) (privacydomain.StoragePolicyOverride, bool, error) {
			return stored, true, nil
		},
		resolveStoragePolicyFn: func(scope string, scopeID string, isPinned bool) (privacydomain.StoragePolicy, error) {
			if scope != "group" || scopeID != "g1" || !isPinned {
				t.Fatalf("unexpected resolve args")
			}
			return privacydomain.StoragePolicy{
				StorageProtection:    privacydomain.StorageProtectionProtected,
				ContentRetentionMode: privacydomain.RetentionPersistent,
			}, nil
		},
		removeStorageScopeOverrideFn: func(scope string, scopeID string) (bool, error) {
			if scope != "group" || scopeID != "g1" {
				t.Fatalf("unexpected remove args")
			}
			return true, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	setParams, _ := json.Marshal([]map[string]any{{
		"scope":                     "group",
		"scope_id":                  "g1",
		"storage_protection_mode":   "protected",
		"content_retention_mode":    "persistent",
		"infinite_ttl":              true,
		"pin_required_for_infinite": true,
	}})
	setResult, rpcErr := s.dispatchRPC("privacy.storage.scope.set", setParams)
	if rpcErr != nil {
		t.Fatalf("unexpected set rpc error: %+v", rpcErr)
	}
	if _, ok := setResult.(privacydomain.StoragePolicyOverride); !ok {
		t.Fatalf("expected scope override result, got %#v", setResult)
	}

	refParams, _ := json.Marshal([]map[string]any{{
		"scope":    "group",
		"scope_id": "g1",
	}})
	getResult, rpcErr := s.dispatchRPC("privacy.storage.scope.get", refParams)
	if rpcErr != nil {
		t.Fatalf("unexpected get rpc error: %+v", rpcErr)
	}
	getPayload, ok := getResult.(map[string]any)
	if !ok {
		t.Fatalf("unexpected get payload type: %#v", getResult)
	}
	if exists, _ := getPayload["exists"].(bool); !exists {
		t.Fatalf("expected exists=true, payload=%#v", getPayload)
	}

	resolveParams, _ := json.Marshal([]map[string]any{{
		"scope":     "group",
		"scope_id":  "g1",
		"is_pinned": true,
	}})
	resolveResult, rpcErr := s.dispatchRPC("privacy.storage.scope.resolve", resolveParams)
	if rpcErr != nil {
		t.Fatalf("unexpected resolve rpc error: %+v", rpcErr)
	}
	if _, ok := resolveResult.(privacydomain.StoragePolicy); !ok {
		t.Fatalf("expected resolved storage policy, got %#v", resolveResult)
	}

	deleteResult, rpcErr := s.dispatchRPC("privacy.storage.scope.delete", refParams)
	if rpcErr != nil {
		t.Fatalf("unexpected delete rpc error: %+v", rpcErr)
	}
	deleted, ok := deleteResult.(map[string]bool)
	if !ok || !deleted["removed"] {
		t.Fatalf("unexpected delete result: %#v", deleteResult)
	}
}

func TestDispatchRPCFileUploadChunkFlow(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	svc := &channelMockService{
		initAttachmentUploadFn: func(name, mimeType string, totalSize int64, totalChunks, chunkSize int, fileSHA256 string) (identityusecase.AttachmentUploadInitResult, error) {
			if name != "doc.txt" || totalChunks != 2 || chunkSize != 256 {
				t.Fatalf("unexpected init params: name=%q totalChunks=%d chunkSize=%d", name, totalChunks, chunkSize)
			}
			return identityusecase.AttachmentUploadInitResult{
				UploadID:    "upl-1",
				TotalChunks: totalChunks,
				ChunkSize:   chunkSize,
			}, nil
		},
		putAttachmentChunkFn: func(uploadID string, chunkIndex int, dataBase64, chunkSHA256 string) (identityusecase.AttachmentUploadChunkResult, error) {
			if uploadID != "upl-1" || chunkIndex != 0 || dataBase64 == "" {
				t.Fatalf("unexpected chunk params")
			}
			return identityusecase.AttachmentUploadChunkResult{
				UploadID:      uploadID,
				ReceivedChunk: chunkIndex,
				ReceivedCount: 1,
				TotalChunks:   2,
			}, nil
		},
		commitAttachmentUploadFn: func(uploadID string) (models.AttachmentMeta, error) {
			if uploadID != "upl-1" {
				t.Fatalf("unexpected upload id at commit: %q", uploadID)
			}
			return models.AttachmentMeta{ID: "att-1", Name: "doc.txt", MimeType: "text/plain", Class: "file", Size: 12}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	initParams, _ := json.Marshal([]map[string]any{{
		"name":         "doc.txt",
		"mime_type":    "text/plain",
		"total_size":   12,
		"total_chunks": 2,
		"chunk_size":   256,
	}})
	initResult, rpcErr := s.dispatchRPC("file.upload.init", initParams)
	if rpcErr != nil {
		t.Fatalf("unexpected init rpc error: %+v", rpcErr)
	}
	initResp, ok := initResult.(identityusecase.AttachmentUploadInitResult)
	if !ok || initResp.UploadID != "upl-1" {
		t.Fatalf("unexpected init result: %#v", initResult)
	}

	chunkParams, _ := json.Marshal([]map[string]any{{
		"upload_id":   "upl-1",
		"chunk_index": 0,
		"data_base64": "aGVsbG8=",
	}})
	_, rpcErr = s.dispatchRPC("file.upload.chunk", chunkParams)
	if rpcErr != nil {
		t.Fatalf("unexpected chunk rpc error: %+v", rpcErr)
	}

	commitParams, _ := json.Marshal([]map[string]any{{"upload_id": "upl-1"}})
	commitResult, rpcErr := s.dispatchRPC("file.upload.commit", commitParams)
	if rpcErr != nil {
		t.Fatalf("unexpected commit rpc error: %+v", rpcErr)
	}
	meta, ok := commitResult.(models.AttachmentMeta)
	if !ok || meta.ID != "att-1" {
		t.Fatalf("unexpected commit result: %#v", commitResult)
	}
	if meta.Class != "file" {
		t.Fatalf("expected attachment class=file, got %q", meta.Class)
	}
}

func TestDispatchRPCFileUploadStatus(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	svc := &channelMockService{
		getAttachmentUploadStatusFn: func(uploadID string) (identityusecase.AttachmentUploadStatus, error) {
			if uploadID != "upl-42" {
				t.Fatalf("unexpected upload id: %q", uploadID)
			}
			return identityusecase.AttachmentUploadStatus{
				UploadID:      uploadID,
				ReceivedCount: 3,
				TotalChunks:   5,
				NextChunk:     3,
			}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	params, _ := json.Marshal([]map[string]any{{"upload_id": "upl-42"}})
	result, rpcErr := s.dispatchRPC("file.upload.status", params)
	if rpcErr != nil {
		t.Fatalf("unexpected status rpc error: %+v", rpcErr)
	}
	status, ok := result.(identityusecase.AttachmentUploadStatus)
	if !ok {
		t.Fatalf("expected AttachmentUploadStatus result")
	}
	if status.ReceivedCount != 3 || status.TotalChunks != 5 || status.NextChunk != 3 {
		t.Fatalf("unexpected status payload: %#v", status)
	}
}

func TestDispatchRPCBlobProvidersList(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	svc := &channelMockService{
		listBlobProvidersFn: func(blobID string) ([]models.BlobProviderInfo, error) {
			if blobID != "att1_blob" {
				t.Fatalf("unexpected blob id: %q", blobID)
			}
			return []models.BlobProviderInfo{
				{PeerID: "aim1peer", ExpiresAt: time.Unix(1700000000, 0).UTC()},
			}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)
	params, _ := json.Marshal([]string{"att1_blob"})
	result, rpcErr := s.dispatchRPC("blob.providers.list", params)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	providers, ok := result.([]models.BlobProviderInfo)
	if !ok {
		t.Fatalf("expected []models.BlobProviderInfo result")
	}
	if len(providers) != 1 || providers[0].PeerID != "aim1peer" {
		t.Fatalf("unexpected providers result: %#v", providers)
	}
}

func TestDispatchRPCBlobPinUnpin(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	svc := &channelMockService{
		pinBlobFn: func(blobID string) (models.AttachmentMeta, error) {
			if blobID != "att1_blob" {
				t.Fatalf("unexpected blob id for pin: %q", blobID)
			}
			return models.AttachmentMeta{ID: blobID, PinState: "pinned"}, nil
		},
		unpinBlobFn: func(blobID string) (models.AttachmentMeta, error) {
			if blobID != "att1_blob" {
				t.Fatalf("unexpected blob id for unpin: %q", blobID)
			}
			return models.AttachmentMeta{ID: blobID, PinState: "unpinned"}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)
	params, _ := json.Marshal([]string{"att1_blob"})

	pinResult, rpcErr := s.dispatchRPC("blob.pin", params)
	if rpcErr != nil {
		t.Fatalf("unexpected pin rpc error: %+v", rpcErr)
	}
	pinMeta, ok := pinResult.(models.AttachmentMeta)
	if !ok || pinMeta.PinState != "pinned" {
		t.Fatalf("unexpected pin result: %#v", pinResult)
	}

	unpinResult, rpcErr := s.dispatchRPC("blob.unpin", params)
	if rpcErr != nil {
		t.Fatalf("unexpected unpin rpc error: %+v", rpcErr)
	}
	unpinMeta, ok := unpinResult.(models.AttachmentMeta)
	if !ok || unpinMeta.PinState != "unpinned" {
		t.Fatalf("unexpected unpin result: %#v", unpinResult)
	}
}

func TestDispatchRPCBlobReplicationModeGetSet(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	currentMode := "on_demand"
	svc := &channelMockService{
		setBlobReplicationModeFn: func(mode string) error {
			currentMode = mode
			return nil
		},
		getBlobReplicationModeFn: func() string {
			return currentMode
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	getResult, rpcErr := s.dispatchRPC("blob.replication.get", nil)
	if rpcErr != nil {
		t.Fatalf("unexpected get rpc error: %+v", rpcErr)
	}
	getPayload, ok := getResult.(map[string]string)
	if !ok || getPayload["mode"] != "on_demand" {
		t.Fatalf("unexpected get payload: %#v", getResult)
	}

	params, _ := json.Marshal([]string{"pinned_only"})
	setResult, rpcErr := s.dispatchRPC("blob.replication.set", params)
	if rpcErr != nil {
		t.Fatalf("unexpected set rpc error: %+v", rpcErr)
	}
	setPayload, ok := setResult.(map[string]string)
	if !ok || setPayload["mode"] != "pinned_only" {
		t.Fatalf("unexpected set payload: %#v", setResult)
	}
}

func TestDispatchRPCBlobFeatureFlagsGetSet(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	flags := models.BlobFeatureFlags{
		AnnounceEnabled: true,
		FetchEnabled:    true,
		RolloutPercent:  100,
	}
	svc := &channelMockService{
		setBlobFeatureFlagsFn: func(announceEnabled, fetchEnabled bool, rolloutPercent int) (models.BlobFeatureFlags, error) {
			flags = models.BlobFeatureFlags{
				AnnounceEnabled: announceEnabled,
				FetchEnabled:    fetchEnabled,
				RolloutPercent:  rolloutPercent,
			}
			return flags, nil
		},
		getBlobFeatureFlagsFn: func() models.BlobFeatureFlags {
			return flags
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	getResult, rpcErr := s.dispatchRPC("blob.features.get", nil)
	if rpcErr != nil {
		t.Fatalf("unexpected get rpc error: %+v", rpcErr)
	}
	getPayload, ok := getResult.(models.BlobFeatureFlags)
	if !ok || !getPayload.AnnounceEnabled || !getPayload.FetchEnabled || getPayload.RolloutPercent != 100 {
		t.Fatalf("unexpected get payload: %#v", getResult)
	}

	params, _ := json.Marshal([]map[string]any{{
		"announce_enabled": false,
		"fetch_enabled":    true,
		"rollout_percent":  25,
	}})
	setResult, rpcErr := s.dispatchRPC("blob.features.set", params)
	if rpcErr != nil {
		t.Fatalf("unexpected set rpc error: %+v", rpcErr)
	}
	setPayload, ok := setResult.(models.BlobFeatureFlags)
	if !ok {
		t.Fatalf("unexpected set payload type: %#v", setResult)
	}
	if setPayload.AnnounceEnabled || !setPayload.FetchEnabled || setPayload.RolloutPercent != 25 {
		t.Fatalf("unexpected set payload: %#v", setPayload)
	}
}

func TestDispatchRPCBlobACLPolicyGetSet(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	acl := models.BlobACLPolicy{
		Mode:      "owner_contacts",
		Allowlist: nil,
		Enforced:  true,
	}
	svc := &channelMockService{
		setBlobACLPolicyFn: func(mode string, allowlist []string) (models.BlobACLPolicy, error) {
			acl = models.BlobACLPolicy{
				Mode:      mode,
				Allowlist: append([]string(nil), allowlist...),
				Enforced:  true,
			}
			return acl, nil
		},
		getBlobACLPolicyFn: func() models.BlobACLPolicy {
			return acl
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	getResult, rpcErr := s.dispatchRPC("blob.acl.get", nil)
	if rpcErr != nil {
		t.Fatalf("unexpected get rpc error: %+v", rpcErr)
	}
	getPayload, ok := getResult.(models.BlobACLPolicy)
	if !ok || getPayload.Mode != "owner_contacts" || !getPayload.Enforced {
		t.Fatalf("unexpected get payload: %#v", getResult)
	}

	params, _ := json.Marshal([]map[string]any{{
		"mode":      "allowlist",
		"allowlist": []string{"aim1peerA", "aim1peerB"},
	}})
	setResult, rpcErr := s.dispatchRPC("blob.acl.set", params)
	if rpcErr != nil {
		t.Fatalf("unexpected set rpc error: %+v", rpcErr)
	}
	setPayload, ok := setResult.(models.BlobACLPolicy)
	if !ok {
		t.Fatalf("unexpected set payload type: %#v", setResult)
	}
	if setPayload.Mode != "allowlist" || len(setPayload.Allowlist) != 2 {
		t.Fatalf("unexpected set payload: %#v", setPayload)
	}
}

func TestDispatchRPCBlobNodePresetGetSet(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	preset := models.BlobNodePresetConfig{Preset: "custom"}
	svc := &channelMockService{
		setBlobNodePresetFn: func(name string) (models.BlobNodePresetConfig, error) {
			preset = models.BlobNodePresetConfig{
				Preset:             name,
				ServeBandwidthKBps: 1024,
				FetchBandwidthKBps: 2048,
			}
			return preset, nil
		},
		getBlobNodePresetFn: func() models.BlobNodePresetConfig {
			return preset
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	getResult, rpcErr := s.dispatchRPC("blob.preset.get", nil)
	if rpcErr != nil {
		t.Fatalf("unexpected get rpc error: %+v", rpcErr)
	}
	getPayload, ok := getResult.(models.BlobNodePresetConfig)
	if !ok || getPayload.Preset != "custom" {
		t.Fatalf("unexpected get payload: %#v", getResult)
	}

	params, _ := json.Marshal([]string{"cache"})
	setResult, rpcErr := s.dispatchRPC("blob.preset.set", params)
	if rpcErr != nil {
		t.Fatalf("unexpected set rpc error: %+v", rpcErr)
	}
	setPayload, ok := setResult.(models.BlobNodePresetConfig)
	if !ok || setPayload.Preset != "cache" || setPayload.ServeBandwidthKBps != 1024 {
		t.Fatalf("unexpected set payload: %#v", setResult)
	}
}

func TestDispatchRPCNodeBindingFlow(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	challenge := "ch_123"
	link := models.NodeBindingLinkCode{
		LinkCode:   "lk_abc",
		Challenge:  challenge,
		ExpiresAt:  time.Now().UTC().Add(1 * time.Minute),
		IdentityID: "aim1identity",
	}
	var stored models.NodeBindingRecord
	svc := &channelMockService{
		createNodeBindingLinkFn: func(ttlSeconds int) (models.NodeBindingLinkCode, error) {
			if ttlSeconds != 120 {
				t.Fatalf("unexpected ttl: %d", ttlSeconds)
			}
			return link, nil
		},
		completeNodeBindingFn: func(linkCode, nodeID, nodePublicKeyBase64, nodeSignatureBase64 string, allowRebind bool) (models.NodeBindingRecord, error) {
			if linkCode != "lk_abc" || nodeID != "node-1" || !allowRebind {
				t.Fatalf("unexpected complete params")
			}
			sigRaw, err := base64.StdEncoding.DecodeString(nodeSignatureBase64)
			if err != nil || len(sigRaw) != ed25519.SignatureSize {
				t.Fatalf("invalid node signature")
			}
			stored = models.NodeBindingRecord{
				IdentityID:       "aim1identity",
				NodeID:           nodeID,
				NodePublicKey:    nodePublicKeyBase64,
				NodeSignature:    nodeSignatureBase64,
				AccountSignature: "acc-sig",
				BoundAt:          time.Now().UTC(),
				UpdatedAt:        time.Now().UTC(),
			}
			return stored, nil
		},
		getNodeBindingFn: func() (models.NodeBindingRecord, bool, error) {
			return stored, true, nil
		},
		unbindNodeFn: func(nodeID string, confirm bool) (bool, error) {
			if nodeID != "node-1" || !confirm {
				t.Fatalf("unexpected unbind params")
			}
			stored = models.NodeBindingRecord{}
			return true, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	createParams, _ := json.Marshal([]map[string]any{{"ttl_seconds": 120}})
	createResult, rpcErr := s.dispatchRPC("node.binding.link.create", createParams)
	if rpcErr != nil {
		t.Fatalf("unexpected link create rpc error: %+v", rpcErr)
	}
	if created, ok := createResult.(models.NodeBindingLinkCode); !ok || created.LinkCode != "lk_abc" {
		t.Fatalf("unexpected link result: %#v", createResult)
	}

	nodePayload := []byte("node-proof")
	nodeSig := ed25519.Sign(priv, nodePayload)
	completeParams, _ := json.Marshal([]map[string]any{{
		"link_code":              "lk_abc",
		"node_id":                "node-1",
		"node_public_key_base64": base64.StdEncoding.EncodeToString(pub),
		"node_signature_base64":  base64.StdEncoding.EncodeToString(nodeSig),
		"rebind":                 true,
	}})
	completeResult, rpcErr := s.dispatchRPC("node.binding.complete", completeParams)
	if rpcErr != nil {
		t.Fatalf("unexpected complete rpc error: %+v", rpcErr)
	}
	if record, ok := completeResult.(models.NodeBindingRecord); !ok || record.NodeID != "node-1" {
		t.Fatalf("unexpected complete result: %#v", completeResult)
	}

	getResult, rpcErr := s.dispatchRPC("node.binding.get", nil)
	if rpcErr != nil {
		t.Fatalf("unexpected get rpc error: %+v", rpcErr)
	}
	payload, ok := getResult.(map[string]any)
	if !ok {
		t.Fatalf("unexpected get payload type: %#v", getResult)
	}
	if exists, _ := payload["exists"].(bool); !exists {
		t.Fatalf("expected exists=true, payload=%#v", payload)
	}

	unbindParams, _ := json.Marshal([]map[string]any{{
		"node_id": "node-1",
		"confirm": true,
	}})
	unbindResult, rpcErr := s.dispatchRPC("node.binding.unbind", unbindParams)
	if rpcErr != nil {
		t.Fatalf("unexpected unbind rpc error: %+v", rpcErr)
	}
	removedPayload, ok := unbindResult.(map[string]bool)
	if !ok || !removedPayload["removed"] {
		t.Fatalf("unexpected unbind result: %#v", unbindResult)
	}
}

func TestRPCIdempotencyKeyPreventsDuplicateWriteSideEffects(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	createCalls := 0
	svc := &channelMockService{
		createGroupFn: func(title string) (groupdomain.Group, error) {
			createCalls++
			return groupdomain.Group{ID: "g-idem", Title: title}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	body := `{"jsonrpc":"2.0","id":1,"method":"channel.create","params":["news","private","Announcements"]}`
	req1 := httptest.NewRequest("POST", "/rpc", strings.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set(rpcIdempotencyHeader, "idem-key-1")
	rec1 := httptest.NewRecorder()
	s.HandleRPC(rec1, req1)
	if rec1.Code != 200 {
		t.Fatalf("expected first call status 200, got %d", rec1.Code)
	}

	body2 := `{"jsonrpc":"2.0","id":2,"method":"channel.create","params":["news","private","Announcements"]}`
	req2 := httptest.NewRequest("POST", "/rpc", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set(rpcIdempotencyHeader, "idem-key-1")
	rec2 := httptest.NewRecorder()
	s.HandleRPC(rec2, req2)
	if rec2.Code != 200 {
		t.Fatalf("expected second call status 200, got %d", rec2.Code)
	}

	if createCalls != 1 {
		t.Fatalf("expected createGroup to be called once, got %d", createCalls)
	}
}

func TestRPCIdempotencyKeyRejectsPayloadMismatch(t *testing.T) {
	t.Setenv("AIM_ENV", "test")

	createCalls := 0
	svc := &channelMockService{
		createGroupFn: func(title string) (groupdomain.Group, error) {
			createCalls++
			return groupdomain.Group{ID: "g-idem-mismatch", Title: title}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	first := `{"jsonrpc":"2.0","id":1,"method":"channel.create","params":["news","private","Announcements"]}`
	req1 := httptest.NewRequest("POST", "/rpc", strings.NewReader(first))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set(rpcIdempotencyHeader, "idem-key-2")
	rec1 := httptest.NewRecorder()
	s.HandleRPC(rec1, req1)
	if rec1.Code != 200 {
		t.Fatalf("expected first call status 200, got %d", rec1.Code)
	}

	second := `{"jsonrpc":"2.0","id":2,"method":"channel.create","params":["alerts","public","General"]}`
	req2 := httptest.NewRequest("POST", "/rpc", strings.NewReader(second))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set(rpcIdempotencyHeader, "idem-key-2")
	rec2 := httptest.NewRecorder()
	s.HandleRPC(rec2, req2)
	if rec2.Code != 200 {
		t.Fatalf("expected second call status 200, got %d", rec2.Code)
	}

	var resp rpcResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if resp.Error == nil {
		t.Fatalf("expected error for idempotency mismatch")
	}
	if resp.Error.Code != -32082 {
		t.Fatalf("expected code -32082, got %d", resp.Error.Code)
	}
	if createCalls != 1 {
		t.Fatalf("expected createGroup to be called once, got %d", createCalls)
	}
}

func TestRPCAccountListDispatchesToService(t *testing.T) {
	svc := &channelMockService{
		listAccountsFn: func() ([]contracts.AccountProfile, error) {
			return []contracts.AccountProfile{
				{ID: "legacy", Active: false},
				{ID: "acct_1", Active: true},
			}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	result, rpcErr := s.dispatchRPC("account.list", nil)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	accounts, ok := result.([]contracts.AccountProfile)
	if !ok {
		t.Fatalf("unexpected result type: %#v", result)
	}
	if len(accounts) != 2 || !accounts[1].Active || accounts[1].ID != "acct_1" {
		t.Fatalf("unexpected accounts payload: %#v", accounts)
	}
}

func TestRPCAccountSwitchReturnsIdentityPayload(t *testing.T) {
	svc := &channelMockService{
		switchAccountFn: func(accountID string) (models.Identity, error) {
			if accountID != "acct_2" {
				t.Fatalf("unexpected account id: %q", accountID)
			}
			return models.Identity{ID: "aim1new"}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	params, _ := json.Marshal([]string{"acct_2"})
	result, rpcErr := s.dispatchRPC("account.switch", params)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	payload, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type: %#v", result)
	}
	identity, ok := payload["identity"].(models.Identity)
	if !ok {
		t.Fatalf("unexpected identity payload: %#v", payload)
	}
	if identity.ID != "aim1new" {
		t.Fatalf("unexpected identity id: %q", identity.ID)
	}
}

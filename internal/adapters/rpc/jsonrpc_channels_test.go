package rpc

import (
	"context"
	"encoding/json"
	"testing"

	"aim-chat/go-backend/internal/domains/contracts"
	groupdomain "aim-chat/go-backend/internal/domains/group"
	identityusecase "aim-chat/go-backend/internal/domains/identity/usecase"
	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/pkg/models"
)

type channelMockService struct {
	createGroupFn               func(title string) (groupdomain.Group, error)
	getGroupFn                  func(groupID string) (groupdomain.Group, error)
	listGroupsFn                func() ([]groupdomain.Group, error)
	listMembersFn               func(groupID string) ([]groupdomain.GroupMember, error)
	sendGroupMessageFn          func(groupID, content string) (groupdomain.GroupMessageFanoutResult, error)
	sendGroupMessageInThreadFn  func(groupID, content, threadID string) (groupdomain.GroupMessageFanoutResult, error)
	listGroupThreadFn           func(groupID, threadID string, limit, offset int) ([]models.Message, error)
	getIdentityFn               func() (models.Identity, error)
	getStoragePolicyFn          func() (privacydomain.StoragePolicy, error)
	updateStoragePolicyFn       func(storageProtection string, retention string, messageTTLSeconds int, fileTTLSeconds int) (privacydomain.StoragePolicy, error)
	initAttachmentUploadFn      func(name, mimeType string, totalSize int64, totalChunks, chunkSize int, fileSHA256 string) (identityusecase.AttachmentUploadInitResult, error)
	putAttachmentChunkFn        func(uploadID string, chunkIndex int, dataBase64, chunkSHA256 string) (identityusecase.AttachmentUploadChunkResult, error)
	getAttachmentUploadStatusFn func(uploadID string) (identityusecase.AttachmentUploadStatus, error)
	commitAttachmentUploadFn    func(uploadID string) (models.AttachmentMeta, error)
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
func (m *channelMockService) ImportIdentity(_, _ string) (models.Identity, error) {
	return models.Identity{}, nil
}
func (m *channelMockService) ValidateMnemonic(_ string) bool { return true }
func (m *channelMockService) ChangePassword(_, _ string) error {
	return nil
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
func (m *channelMockService) UpdateStoragePolicy(storageProtection string, retention string, messageTTLSeconds int, fileTTLSeconds int) (privacydomain.StoragePolicy, error) {
	if m.updateStoragePolicyFn != nil {
		return m.updateStoragePolicyFn(storageProtection, retention, messageTTLSeconds, fileTTLSeconds)
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

func TestDispatchRPCChannelSendRequiresAdminOrOwner(t *testing.T) {
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
	_, rpcErr := s.dispatchRPC("channel.send", params)
	if rpcErr == nil {
		t.Fatalf("expected rpc error")
	}
	if rpcErr.Code != -32220 {
		t.Fatalf("expected rpc code -32220, got %d", rpcErr.Code)
	}
	if sendCalled {
		t.Fatalf("send should not be called for non-admin member")
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
	var gotFileTTL int

	svc := &channelMockService{
		updateStoragePolicyFn: func(storageProtection string, retention string, messageTTLSeconds int, fileTTLSeconds int) (privacydomain.StoragePolicy, error) {
			gotProtection = storageProtection
			gotRetention = retention
			gotMessageTTL = messageTTLSeconds
			gotFileTTL = fileTTLSeconds
			return privacydomain.StoragePolicy{
				StorageProtection:    privacydomain.StorageProtectionMode(storageProtection),
				ContentRetentionMode: privacydomain.ContentRetentionMode(retention),
				MessageTTLSeconds:    messageTTLSeconds,
				FileTTLSeconds:       fileTTLSeconds,
			}, nil
		},
	}
	s := newServerWithService(DefaultRPCAddr, svc, "", false)

	params, _ := json.Marshal([]map[string]any{{
		"storage_protection_mode": "standard",
		"content_retention_mode":  "ephemeral",
		"message_ttl_seconds":     30,
		"file_ttl_seconds":        60,
	}})
	result, rpcErr := s.dispatchRPC("privacy.storage.set", params)
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if gotProtection != "standard" || gotRetention != "ephemeral" || gotMessageTTL != 30 || gotFileTTL != 60 {
		t.Fatalf("unexpected params: protection=%q retention=%q messageTTL=%d fileTTL=%d", gotProtection, gotRetention, gotMessageTTL, gotFileTTL)
	}
	policy, ok := result.(privacydomain.StoragePolicy)
	if !ok {
		t.Fatalf("expected privacydomain.StoragePolicy result")
	}
	if policy.MessageTTLSeconds != 30 || policy.FileTTLSeconds != 60 {
		t.Fatalf("unexpected ttl in result: %+v", policy)
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
			return models.AttachmentMeta{ID: "att-1", Name: "doc.txt", MimeType: "text/plain", Size: 12}, nil
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

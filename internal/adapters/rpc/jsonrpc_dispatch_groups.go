package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	groupdomain "aim-chat/go-backend/internal/domains/group"
	identityusecase "aim-chat/go-backend/internal/domains/identity/usecase"
	privacydomain "aim-chat/go-backend/internal/domains/privacy"
	"aim-chat/go-backend/pkg/models"
)

func (s *Server) dispatchIdentityRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "identity.get":
		result, rpcErr := callWithoutParams(-32000, func() (any, error) {
			return s.service.GetIdentity()
		})
		return result, rpcErr, true
	case "identity.self_contact_card":
		result, rpcErr := callWithSingleStringParam(rawParams, -32025, func(displayName string) (any, error) {
			return s.service.SelfContactCard(displayName)
		})
		return result, rpcErr, true
	case "identity.create":
		result, rpcErr := callWithSingleStringParam(rawParams, -32020, func(password string) (any, error) {
			identity, mnemonic, err := s.service.CreateIdentity(password)
			if err != nil {
				return nil, err
			}
			return map[string]any{"identity": identity, "mnemonic": mnemonic}, nil
		})
		return result, rpcErr, true
	case "identity.export_seed":
		result, rpcErr := callWithSingleStringParam(rawParams, -32021, func(password string) (any, error) {
			mnemonic, err := s.service.ExportSeed(password)
			if err != nil {
				return nil, err
			}
			return map[string]string{"mnemonic": mnemonic}, nil
		})
		return result, rpcErr, true
	case "backup.export":
		result, rpcErr := callWithTwoStringParams(rawParams, -32024, func(consent, passphrase string) (any, error) {
			blob, err := s.service.ExportBackup(consent, passphrase)
			if err != nil {
				return nil, err
			}
			return map[string]string{"backup_blob": blob}, nil
		})
		return result, rpcErr, true
	case "data.wipe":
		result, rpcErr := callWithSingleStringParam(rawParams, -32027, func(consentToken string) (any, error) {
			wiper, ok := s.service.(interface {
				WipeData(consentToken string) (bool, error)
			})
			if !ok {
				return nil, errors.New("data wipe is not supported")
			}
			wiped, err := wiper.WipeData(consentToken)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"wiped": wiped}, nil
		})
		return result, rpcErr, true
	case "identity.import_seed":
		result, rpcErr := callWithTwoStringParams(rawParams, -32022, func(mnemonic, password string) (any, error) {
			identity, err := s.service.ImportIdentity(mnemonic, password)
			if err != nil {
				return nil, err
			}
			return map[string]any{"identity": identity}, nil
		})
		return result, rpcErr, true
	case "identity.validate_mnemonic":
		result, rpcErr := callWithSingleStringParam(rawParams, -32026, func(mnemonic string) (any, error) {
			return map[string]bool{"valid": s.service.ValidateMnemonic(mnemonic)}, nil
		})
		return result, rpcErr, true
	case "identity.change_password":
		result, rpcErr := callWithTwoStringParams(rawParams, -32023, func(oldPassword, newPassword string) (any, error) {
			if err := s.service.ChangePassword(oldPassword, newPassword); err != nil {
				return nil, err
			}
			return map[string]bool{"changed": true}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchNetworkRPC(method string) (any, *rpcError, bool) {
	switch method {
	case "network.status":
		result, rpcErr := callWithoutParams(-32031, func() (any, error) {
			return s.service.GetNetworkStatus(), nil
		})
		return result, rpcErr, true
	case "network.listen_addresses":
		result, rpcErr := callWithoutParams(-32032, func() (any, error) {
			return map[string]any{"addresses": s.service.ListenAddresses()}, nil
		})
		return result, rpcErr, true
	case "metrics.get":
		result, rpcErr := callWithoutParams(-32070, func() (any, error) {
			return s.service.GetMetrics(), nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchPrivacyRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "privacy.get":
		result, rpcErr := callWithoutParams(-32080, func() (any, error) {
			return s.service.GetPrivacySettings()
		})
		return result, rpcErr, true
	case "privacy.set":
		result, rpcErr := callWithSingleStringParam(rawParams, -32081, func(mode string) (any, error) {
			return s.service.UpdatePrivacySettings(mode)
		})
		return result, rpcErr, true
	case "privacy.storage.get":
		result, rpcErr := callWithoutParams(-32082, func() (any, error) {
			storageAPI, ok := s.service.(interface {
				GetStoragePolicy() (privacydomain.StoragePolicy, error)
			})
			if !ok {
				return nil, errors.New("privacy storage policy is not supported")
			}
			return storageAPI.GetStoragePolicy()
		})
		return result, rpcErr, true
	case "privacy.storage.set":
		protection, retention, messageTTL, fileTTL, err := decodeStoragePolicyParams(rawParams)
		if err != nil {
			return nil, rpcInvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32083, func() (any, error) {
			storageAPI, ok := s.service.(interface {
				UpdateStoragePolicy(storageProtection string, retention string, messageTTLSeconds int, fileTTLSeconds int) (privacydomain.StoragePolicy, error)
			})
			if !ok {
				return nil, errors.New("privacy storage policy is not supported")
			}
			return storageAPI.UpdateStoragePolicy(protection, retention, messageTTL, fileTTL)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchBlocklistRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "blocklist.list":
		result, rpcErr := callWithoutParams(-32090, func() (any, error) {
			blocked, err := s.service.GetBlocklist()
			if err != nil {
				return nil, err
			}
			return map[string]any{"blocked": blocked}, nil
		})
		return result, rpcErr, true
	case "blocklist.add":
		result, rpcErr := callWithSingleStringParam(rawParams, -32091, func(identityID string) (any, error) {
			blocked, err := s.service.AddToBlocklist(identityID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"blocked": blocked}, nil
		})
		return result, rpcErr, true
	case "blocklist.remove":
		result, rpcErr := callWithSingleStringParam(rawParams, -32092, func(identityID string) (any, error) {
			blocked, err := s.service.RemoveFromBlocklist(identityID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"blocked": blocked}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchRequestRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "request.list":
		result, rpcErr := callWithoutParams(-32093, func() (any, error) {
			return s.service.ListMessageRequests()
		})
		return result, rpcErr, true
	case "request.get":
		result, rpcErr := callWithSingleStringParam(rawParams, -32094, func(senderID string) (any, error) {
			return s.service.GetMessageRequest(senderID)
		})
		return result, rpcErr, true
	case "request.accept":
		result, rpcErr := callWithSingleStringParam(rawParams, -32095, func(senderID string) (any, error) {
			accepted, err := s.service.AcceptMessageRequest(senderID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"accepted": accepted}, nil
		})
		return result, rpcErr, true
	case "request.decline":
		result, rpcErr := callWithSingleStringParam(rawParams, -32096, func(senderID string) (any, error) {
			declined, err := s.service.DeclineMessageRequest(senderID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"declined": declined}, nil
		})
		return result, rpcErr, true
	case "request.block":
		result, rpcErr := callWithSingleStringParam(rawParams, -32097, func(senderID string) (any, error) {
			return s.service.BlockSender(senderID)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchGroupRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "group.create":
		result, rpcErr := callWithSingleStringParam(rawParams, -32100, func(title string) (any, error) {
			return s.service.CreateGroup(title)
		})
		return result, rpcErr, true
	case "group.get":
		result, rpcErr := callWithSingleStringParam(rawParams, -32101, func(groupID string) (any, error) {
			return s.service.GetGroup(groupID)
		})
		return result, rpcErr, true
	case "group.list":
		result, rpcErr := callWithoutParams(-32102, func() (any, error) {
			return s.service.ListGroups()
		})
		return result, rpcErr, true
	case "group.members.list":
		result, rpcErr := callWithSingleStringParam(rawParams, -32103, func(groupID string) (any, error) {
			return s.service.ListGroupMembers(groupID)
		})
		return result, rpcErr, true
	case "group.leave":
		result, rpcErr := callWithSingleStringParam(rawParams, -32104, func(groupID string) (any, error) {
			left, err := s.service.LeaveGroup(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"left": left}, nil
		})
		return result, rpcErr, true
	case "group.invite":
		result, rpcErr := callWithTwoStringParams(rawParams, -32110, func(groupID, memberID string) (any, error) {
			return s.service.InviteToGroup(groupID, memberID)
		})
		return result, rpcErr, true
	case "group.accept_invite":
		result, rpcErr := callWithSingleStringParam(rawParams, -32111, func(groupID string) (any, error) {
			accepted, err := s.service.AcceptGroupInvite(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"accepted": accepted}, nil
		})
		return result, rpcErr, true
	case "group.decline_invite":
		result, rpcErr := callWithSingleStringParam(rawParams, -32112, func(groupID string) (any, error) {
			declined, err := s.service.DeclineGroupInvite(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"declined": declined}, nil
		})
		return result, rpcErr, true
	case "group.remove_member":
		result, rpcErr := callWithTwoStringParams(rawParams, -32113, func(groupID, memberID string) (any, error) {
			removed, err := s.service.RemoveGroupMember(groupID, memberID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"removed": removed}, nil
		})
		return result, rpcErr, true
	case "group.promote":
		result, rpcErr := callWithTwoStringParams(rawParams, -32114, func(groupID, memberID string) (any, error) {
			return s.service.PromoteGroupMember(groupID, memberID)
		})
		return result, rpcErr, true
	case "group.demote":
		result, rpcErr := callWithTwoStringParams(rawParams, -32115, func(groupID, memberID string) (any, error) {
			return s.service.DemoteGroupMember(groupID, memberID)
		})
		return result, rpcErr, true
	case "group.send":
		result, rpcErr := callWithTwoStringParams(rawParams, -32120, func(groupID, content string) (any, error) {
			return s.service.SendGroupMessage(groupID, content)
		})
		return result, rpcErr, true
	case "group.thread.send":
		result, rpcErr := callWithThreadSendParams(rawParams, -32124, func(groupID, content, threadID string) (any, error) {
			return s.service.SendGroupMessageInThread(groupID, content, threadID)
		})
		return result, rpcErr, true
	case "group.messages.list":
		result, rpcErr := callWithMessageListParams(rawParams, -32121, func(groupID string, limit, offset int) (any, error) {
			return s.service.ListGroupMessages(groupID, limit, offset)
		})
		return result, rpcErr, true
	case "group.thread.list":
		result, rpcErr := callWithThreadListParams(rawParams, -32125, func(groupID, threadID string, limit, offset int) (any, error) {
			return s.service.ListGroupMessagesByThread(groupID, threadID, limit, offset)
		})
		return result, rpcErr, true
	case "group.message.status":
		result, rpcErr := callWithTwoStringParams(rawParams, -32122, func(groupID, messageID string) (any, error) {
			return s.service.GetGroupMessageStatus(groupID, messageID)
		})
		return result, rpcErr, true
	case "group.message.delete":
		result, rpcErr := callWithTwoStringParams(rawParams, -32123, func(groupID, messageID string) (any, error) {
			if err := s.service.DeleteGroupMessage(groupID, messageID); err != nil {
				return nil, err
			}
			return map[string]bool{"deleted": true}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchSessionMessageRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "session.init":
		result, rpcErr := callWithSessionInitParams(rawParams, -32030, func(contactID string, peerPublicKey []byte) (any, error) {
			return s.service.InitSession(contactID, peerPublicKey)
		})
		return result, rpcErr, true
	case "message.send":
		result, rpcErr := callWithTwoStringParams(rawParams, -32040, func(contactID, content string) (any, error) {
			messageID, err := s.service.SendMessage(contactID, content)
			if err != nil {
				return nil, err
			}
			return map[string]string{"message_id": messageID}, nil
		})
		return result, rpcErr, true
	case "message.thread.send":
		result, rpcErr := callWithThreadSendParams(rawParams, -32046, func(contactID, content, threadID string) (any, error) {
			messageID, err := s.service.SendMessageInThread(contactID, content, threadID)
			if err != nil {
				return nil, err
			}
			return map[string]string{"message_id": messageID}, nil
		})
		return result, rpcErr, true
	case "message.edit":
		result, rpcErr := callWithMessageEditParams(rawParams, -32043, func(contactID, messageID, content string) (any, error) {
			return s.service.EditMessage(contactID, messageID, content)
		})
		return result, rpcErr, true
	case "message.delete":
		result, rpcErr := callWithTwoStringParams(rawParams, -32044, func(contactID, messageID string) (any, error) {
			if err := s.service.DeleteMessage(contactID, messageID); err != nil {
				return nil, err
			}
			return map[string]bool{"deleted": true}, nil
		})
		return result, rpcErr, true
	case "message.clear":
		result, rpcErr := callWithSingleStringParam(rawParams, -32045, func(contactID string) (any, error) {
			cleared, err := s.service.ClearMessages(contactID)
			if err != nil {
				return nil, err
			}
			return map[string]int{"cleared": cleared}, nil
		})
		return result, rpcErr, true
	case "message.list":
		result, rpcErr := callWithMessageListParams(rawParams, -32041, func(contactID string, limit, offset int) (any, error) {
			return s.service.GetMessages(contactID, limit, offset)
		})
		return result, rpcErr, true
	case "message.thread.list":
		result, rpcErr := callWithThreadListParams(rawParams, -32047, func(contactID, threadID string, limit, offset int) (any, error) {
			return s.service.GetMessagesByThread(contactID, threadID, limit, offset)
		})
		return result, rpcErr, true
	case "message.status":
		result, rpcErr := callWithSingleStringParam(rawParams, -32042, func(messageID string) (any, error) {
			return s.service.GetMessageStatus(messageID)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchContactFileRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "contact.verify":
		result, rpcErr := callWithCardParam(rawParams, -32010, func(card models.ContactCard) (any, error) {
			ok, err := s.service.VerifyContactCard(card)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"valid": ok}, nil
		})
		return result, rpcErr, true
	case "contact.add":
		result, rpcErr := s.dispatchContactAdd(rawParams)
		return result, rpcErr, true
	case "contact.add_by_id":
		result, rpcErr := callWithContactByIDParams(rawParams, func(contactID, displayName string) (any, *rpcError) {
			return s.dispatchAddContactByID(contactID, displayName)
		})
		return result, rpcErr, true
	case "file.put":
		result, rpcErr := callWithFilePutParams(rawParams, -32060, func(name, mimeType, dataBase64 string) (any, error) {
			return s.service.PutAttachment(name, mimeType, dataBase64)
		})
		return result, rpcErr, true
	case "file.upload.init":
		params, err := decodeFileUploadInitParams(rawParams)
		if err != nil {
			return nil, rpcInvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32061, func() (any, error) {
			uploader, ok := s.service.(interface {
				InitAttachmentUpload(name, mimeType string, totalSize int64, totalChunks, chunkSize int, fileSHA256 string) (identityusecase.AttachmentUploadInitResult, error)
			})
			if !ok {
				return nil, errors.New("chunked file upload is not supported")
			}
			return uploader.InitAttachmentUpload(
				params.Name,
				params.MimeType,
				params.TotalSize,
				params.TotalChunks,
				params.ChunkSize,
				params.FileSHA256,
			)
		})
		return result, rpcErr, true
	case "file.upload.chunk":
		params, err := decodeFileUploadChunkParams(rawParams)
		if err != nil {
			return nil, rpcInvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32062, func() (any, error) {
			uploader, ok := s.service.(interface {
				PutAttachmentChunk(uploadID string, chunkIndex int, dataBase64, chunkSHA256 string) (identityusecase.AttachmentUploadChunkResult, error)
			})
			if !ok {
				return nil, errors.New("chunked file upload is not supported")
			}
			return uploader.PutAttachmentChunk(params.UploadID, params.ChunkIndex, params.DataBase64, params.ChunkSHA256)
		})
		return result, rpcErr, true
	case "file.upload.status":
		params, err := decodeFileUploadCommitParams(rawParams)
		if err != nil {
			return nil, rpcInvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32063, func() (any, error) {
			uploader, ok := s.service.(interface {
				GetAttachmentUploadStatus(uploadID string) (identityusecase.AttachmentUploadStatus, error)
			})
			if !ok {
				return nil, errors.New("chunked file upload is not supported")
			}
			return uploader.GetAttachmentUploadStatus(params.UploadID)
		})
		return result, rpcErr, true
	case "file.upload.commit":
		params, err := decodeFileUploadCommitParams(rawParams)
		if err != nil {
			return nil, rpcInvalidParams(), true
		}
		result, rpcErr := callWithoutParams(-32064, func() (any, error) {
			uploader, ok := s.service.(interface {
				CommitAttachmentUpload(uploadID string) (models.AttachmentMeta, error)
			})
			if !ok {
				return nil, errors.New("chunked file upload is not supported")
			}
			return uploader.CommitAttachmentUpload(params.UploadID)
		})
		return result, rpcErr, true
	case "contact.list":
		result, rpcErr := callWithoutParams(-32012, func() (any, error) {
			return s.service.GetContacts()
		})
		return result, rpcErr, true
	case "contact.remove":
		result, rpcErr := callWithSingleStringParam(rawParams, -32014, func(contactID string) (any, error) {
			if err := s.service.RemoveContact(contactID); err != nil {
				return nil, err
			}
			return map[string]bool{"removed": true}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func (s *Server) dispatchContactAdd(rawParams json.RawMessage) (any, *rpcError) {
	card, contactID, displayName, err := decodeAddContactParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	if card.IdentityID != "" {
		if err := s.service.AddContactCard(card); err != nil {
			return nil, rpcServiceError(-32011, err)
		}
		return map[string]bool{"added": true}, nil
	}
	return s.dispatchAddContactByID(contactID, displayName)
}

func (s *Server) dispatchAddContactByID(contactID, displayName string) (any, *rpcError) {
	if err := s.service.AddContact(contactID, displayName); err != nil {
		return nil, rpcServiceError(-32013, err)
	}
	return map[string]bool{"added": true}, nil
}

func (s *Server) dispatchDeviceRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "device.list":
		result, rpcErr := callWithoutParams(-32050, func() (any, error) {
			return s.service.ListDevices()
		})
		return result, rpcErr, true
	case "device.add":
		result, rpcErr := callWithSingleStringParam(rawParams, -32051, func(name string) (any, error) {
			return s.service.AddDevice(name)
		})
		return result, rpcErr, true
	case "device.revoke":
		result, rpcErr := callWithSingleStringParamAndErrorMapper(rawParams, mapDeviceRevokeRPCError, func(deviceID string) (any, error) {
			return s.service.RevokeDevice(deviceID)
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

var channelGroupTitlePrefixRe = regexp.MustCompile(`^\[channel(?::(public|private))?\]\s*`)

func normalizeChannelVisibility(visibility string) string {
	if strings.EqualFold(strings.TrimSpace(visibility), "private") {
		return "private"
	}
	return "public"
}

func encodeChannelGroupTitle(name, visibility string) string {
	return fmt.Sprintf("[channel:%s] %s", normalizeChannelVisibility(visibility), strings.TrimSpace(name))
}

func isChannelGroupTitle(title string) bool {
	return channelGroupTitlePrefixRe.MatchString(strings.TrimSpace(title))
}

func (s *Server) ensureChannelGroup(groupID string) (groupdomain.Group, error) {
	group, err := s.service.GetGroup(groupID)
	if err != nil {
		return groupdomain.Group{}, err
	}
	if !isChannelGroupTitle(group.Title) {
		return groupdomain.Group{}, errors.New("group not found")
	}
	return group, nil
}

func (s *Server) ensureCanPublishToChannel(groupID string) error {
	_, err := s.ensureChannelGroup(groupID)
	if err != nil {
		return err
	}
	identity, err := s.service.GetIdentity()
	if err != nil {
		return err
	}
	actorID := strings.TrimSpace(identity.ID)
	if actorID == "" {
		return errors.New("group permission denied")
	}
	members, err := s.service.ListGroupMembers(groupID)
	if err != nil {
		return err
	}
	for _, member := range members {
		if strings.TrimSpace(member.MemberID) != actorID {
			continue
		}
		if member.Status != groupdomain.GroupMemberStatusActive {
			return errors.New("invalid group member state")
		}
		if member.Role == groupdomain.GroupMemberRoleOwner || member.Role == groupdomain.GroupMemberRoleAdmin {
			return nil
		}
		return errors.New("group permission denied")
	}
	return errors.New("group membership not found")
}

func (s *Server) dispatchChannelRPC(method string, rawParams json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "channel.create":
		result, rpcErr := callWithChannelCreateParams(rawParams, -32200, func(name, visibility, _ string) (any, error) {
			title := encodeChannelGroupTitle(name, visibility)
			return s.service.CreateGroup(title)
		})
		return result, rpcErr, true
	case "channel.get":
		result, rpcErr := callWithSingleStringParam(rawParams, -32201, func(groupID string) (any, error) {
			return s.ensureChannelGroup(groupID)
		})
		return result, rpcErr, true
	case "channel.list":
		result, rpcErr := callWithoutParams(-32202, func() (any, error) {
			groups, err := s.service.ListGroups()
			if err != nil {
				return nil, err
			}
			out := make([]groupdomain.Group, 0, len(groups))
			for _, group := range groups {
				if isChannelGroupTitle(group.Title) {
					out = append(out, group)
				}
			}
			return out, nil
		})
		return result, rpcErr, true
	case "channel.members.list":
		result, rpcErr := callWithSingleStringParam(rawParams, -32203, func(groupID string) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			return s.service.ListGroupMembers(groupID)
		})
		return result, rpcErr, true
	case "channel.leave":
		result, rpcErr := callWithSingleStringParam(rawParams, -32204, func(groupID string) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			left, err := s.service.LeaveGroup(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"left": left}, nil
		})
		return result, rpcErr, true
	case "channel.invite":
		result, rpcErr := callWithTwoStringParams(rawParams, -32210, func(groupID, memberID string) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			return s.service.InviteToGroup(groupID, memberID)
		})
		return result, rpcErr, true
	case "channel.accept_invite":
		result, rpcErr := callWithSingleStringParam(rawParams, -32211, func(groupID string) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			accepted, err := s.service.AcceptGroupInvite(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"accepted": accepted}, nil
		})
		return result, rpcErr, true
	case "channel.decline_invite":
		result, rpcErr := callWithSingleStringParam(rawParams, -32212, func(groupID string) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			declined, err := s.service.DeclineGroupInvite(groupID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"declined": declined}, nil
		})
		return result, rpcErr, true
	case "channel.remove_member":
		result, rpcErr := callWithTwoStringParams(rawParams, -32213, func(groupID, memberID string) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			removed, err := s.service.RemoveGroupMember(groupID, memberID)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"removed": removed}, nil
		})
		return result, rpcErr, true
	case "channel.promote":
		result, rpcErr := callWithTwoStringParams(rawParams, -32214, func(groupID, memberID string) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			return s.service.PromoteGroupMember(groupID, memberID)
		})
		return result, rpcErr, true
	case "channel.demote":
		result, rpcErr := callWithTwoStringParams(rawParams, -32215, func(groupID, memberID string) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			return s.service.DemoteGroupMember(groupID, memberID)
		})
		return result, rpcErr, true
	case "channel.send":
		result, rpcErr := callWithTwoStringParams(rawParams, -32220, func(groupID, content string) (any, error) {
			if err := s.ensureCanPublishToChannel(groupID); err != nil {
				return nil, err
			}
			return s.service.SendGroupMessage(groupID, content)
		})
		return result, rpcErr, true
	case "channel.thread.send":
		result, rpcErr := callWithThreadSendParams(rawParams, -32224, func(groupID, content, threadID string) (any, error) {
			if err := s.ensureCanPublishToChannel(groupID); err != nil {
				return nil, err
			}
			return s.service.SendGroupMessageInThread(groupID, content, threadID)
		})
		return result, rpcErr, true
	case "channel.messages.list":
		result, rpcErr := callWithMessageListParams(rawParams, -32221, func(groupID string, limit, offset int) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			return s.service.ListGroupMessages(groupID, limit, offset)
		})
		return result, rpcErr, true
	case "channel.thread.list":
		result, rpcErr := callWithThreadListParams(rawParams, -32225, func(groupID, threadID string, limit, offset int) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			return s.service.ListGroupMessagesByThread(groupID, threadID, limit, offset)
		})
		return result, rpcErr, true
	case "channel.message.status":
		result, rpcErr := callWithTwoStringParams(rawParams, -32222, func(groupID, messageID string) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			return s.service.GetGroupMessageStatus(groupID, messageID)
		})
		return result, rpcErr, true
	case "channel.message.delete":
		result, rpcErr := callWithTwoStringParams(rawParams, -32223, func(groupID, messageID string) (any, error) {
			if _, err := s.ensureChannelGroup(groupID); err != nil {
				return nil, err
			}
			if err := s.service.DeleteGroupMessage(groupID, messageID); err != nil {
				return nil, err
			}
			return map[string]bool{"deleted": true}, nil
		})
		return result, rpcErr, true
	default:
		return nil, nil, false
	}
}

func callWithChannelCreateParams(rawParams json.RawMessage, serviceErrCode int, call func(name, visibility, description string) (any, error)) (any, *rpcError) {
	name, visibility, description, err := decodeChannelCreateParams(rawParams)
	if err != nil {
		return nil, rpcInvalidParams()
	}
	result, err := call(name, visibility, description)
	if err != nil {
		return nil, rpcServiceError(serviceErrCode, err)
	}
	return result, nil
}

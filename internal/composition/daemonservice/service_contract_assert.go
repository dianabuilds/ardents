package daemonservice

import "aim-chat/go-backend/internal/domains/contracts"

var _ contracts.IdentityAPI = (*Service)(nil)
var _ contracts.MessagingAPI = (*Service)(nil)
var _ contracts.GroupAPI = (*Service)(nil)
var _ contracts.InboxAPI = (*Service)(nil)
var _ contracts.PrivacyAPI = (*Service)(nil)
var _ contracts.NetworkAPI = (*Service)(nil)
var _ contracts.DaemonService = (*Service)(nil)

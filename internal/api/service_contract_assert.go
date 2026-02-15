package api

import "aim-chat/go-backend/internal/app/contracts"

var _ contracts.CoreAPI = (*Service)(nil)
var _ contracts.DaemonService = (*Service)(nil)

package api

import "aim-chat/go-backend/internal/app/contracts"

// CoreAPI is kept for backward compatibility inside api package.
// New transport-neutral interface lives in internal/app/contracts.
type CoreAPI = contracts.CoreAPI

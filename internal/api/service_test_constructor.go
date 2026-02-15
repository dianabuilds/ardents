package api

import (
	"aim-chat/go-backend/internal/app/contracts"
	"aim-chat/go-backend/internal/waku"
)

// NewServiceForTesting allows tests to configure service dependencies without
// depending on unexported constructors.
func NewServiceForTesting(wakuCfg waku.Config, opts contracts.ServiceOptions) (*Service, error) {
	return newServiceWithOptions(wakuCfg, opts)
}

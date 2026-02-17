// Package daemonservice provides transport-facing adapters (JSON-RPC/HTTP) and
// boundary orchestration for the daemon service.
//
// Responsibilities:
// - Decode/validate transport requests and map them to core service calls.
// - Translate domain/service errors into transport protocol errors.
// - Trigger side effects at the boundary (publish/store/notify) via app ports.
//
// Non-responsibilities:
// - Domain rules and business workflows (implemented in internal/domains/*).
// - Persistence or crypto implementation details.
//
//goland:noinspection GoCommentStart
package daemonservice

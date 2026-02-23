// Package identity provides the public domain facade for account identity flows.
//
// Package layout:
// - adapters: protocol-specific adapters (e.g., RPC dispatch helpers)
// - domain: domain-level constants and shared invariants
// - ports: boundary interfaces used by usecases
// - transport: method identifiers and transport-facing metadata
// - usecase: application usecases orchestrating domain operations
//
// External callers should use exports.go as the stable entrypoint.
package identity

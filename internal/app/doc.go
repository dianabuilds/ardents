// Package app contains core application contracts, domain helpers, and
// use-case level orchestration that are independent of transport protocols.
//
// Responsibilities:
// - Define service ports/interfaces between domain logic and infrastructure.
// - Implement reusable business rules and application workflows.
// - Provide shared runtime/state abstractions used by adapters.
//
// Non-responsibilities:
// - JSON-RPC/HTTP protocol handling and endpoint-level mapping.
package app

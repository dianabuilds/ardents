# Duplicate Implementations Inventory (Updated)

**Goal:** identify potential parallel implementations and track whether they are true duplicates or protocol-version differences.

---

## 1) Envelope (delivery container)

**Where:**
- `internal/shared/envelope/envelope.go` (v1)
- `internal/shared/envelopev2/envelopev2.go` (v2)
- `internal/core/app/runtime/handler.go` (v1 handling)
- `internal/core/app/runtime/envelopev2.go` (v2 handling)
- `internal/core/app/runtime/garlic.go` (v2 envelope inside garlic)

**What differs:**
- v2 has different fields and validation rules; v1 includes `from.peer_id` and `ack` semantics, v2 uses `from.identity_id`/`to.service_id` and no direct ACK.

**Status:** acceptable protocol-version difference (pipeline unified in JIRA-66/67).

---

## 2) Discovery & routing layer

**Where:**
- `internal/core/app/netdb/*` (NetDB records)
- `internal/core/app/runtime/service_publish.go` (service head/lease set publish)
- `internal/core/app/runtime/tunnel.go`, `internal/core/domain/tunnel/*`
- `internal/core/domain/garlic/*`

**What differs:**
- v2 uses NetDB + tunnel/garlic routing for delivery; v1 direct delivery via envelope v1.
- Service discovery uses `service.head/lease_set` (v2) vs addressbook or direct endpoints (v1).

**Status:** acceptable protocol-version difference. Discovery surface unified (JIRA-68), delivery API unified (JIRA-67).

---

## 3) Service descriptor

**Where:**
- `internal/core/app/services/servicedesc/servicedesc.go` (unified Descriptor)
- `internal/core/app/runtime/service_publish.go`

**What differs:**
- descriptor v2 lacks endpoints, relies on NetDB/directory.

**Status:** unified model and validation (JIRA-69). Only payload versions remain.

---

## 4) Simulators / tooling

**Where:**
- `cmd/sim/main.go` (shared runner)
- `cmd/sim/v2.go`, `cmd/sim/v2_dirquery.go`, `cmd/sim/v2_checks.go`, `cmd/sim/v2_reseed.go` (scenario modules)
- `internal/core/app/runtime/sim_v2.go` (v2-only helpers)

**What differs:**
- v2 scenarios require tunnel/garlic/NetDB helpers; v1 uses direct envelope flow.

**Status:** unified runner/reporting (JIRA-70). Scenario modules remain as protocol-specific test vectors.

---

## 5) Specs / docs with explicit v2

**Where (specs):**
- `spec/SPEC-510-netdb-and-records.md`
- `spec/SPEC-520-tunnels-and-garlic-routing.md`
- `spec/SPEC-530-anonymous-services-and-directory.md`
- `spec/SPEC-550-anonymous-envelope.md`

**Where (docs):**
- `docs/TECH-030-dynamic-testing.md` (v2 suite)

**Status:** documentation of protocol versions; not code duplication.

---

## 6) Tests with v2 code paths

**Where:**
- `internal/core/app/runtime/dirquery_test.go`
- `internal/core/app/runtime/garlic_integration_test.go`
- `internal/core/app/runtime/tunnel_integration_test.go`

**Status:** protocol-specific tests; not duplicate logic.

---

## 7) Legacy

**Where:**
- `legacy/` (reference-only per SPEC-000)

**Status:** not part of active runtime; keep isolated or remove when no longer needed.

---

Summary: remaining parallel paths are protocol-version differences, not duplicated business logic. The concrete duplicate implementations in delivery/discovery/descriptors/sim were unified in JIRA-67..70.

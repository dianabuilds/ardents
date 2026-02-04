# SPEC-400: Client Console/Chat (ARCHIVED)

**Status:** Archived (2026-02-04)  
**Dependencies:** SPEC-000, SPEC-120, SPEC-140, SPEC-420  
**Purpose:** Historical note only. This document used to describe an early "chat-console" UX.

This SPEC is **NOT** part of the active spec set and **MUST NOT** be implemented.

Replaced by:

* SPEC-410 (Client Node Browser)
* SPEC-340 (Web service profile) + `cmd/webclient`
* SPEC-415 (User-facing resolution / UFA)
* SPEC-420 (Diagnostics and observability)

## Historical notes (non-normative)

The old idea included:

* a local CLI/TUI that selects `alias|peer|service` from Address Book;
* a `chat.msg.v1` payload with a simple text body;
* basic delivery diagnostics (ACK/latency/error_code).

This approach was dropped in favor of task-based services and content node workflows.


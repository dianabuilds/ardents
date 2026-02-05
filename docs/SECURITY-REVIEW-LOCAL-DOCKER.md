# Local Docker Security Review (Timeboxed)

**Scope:** Local Docker compose stack (`docker/docker-compose.yml`) for Ardents runtime + integration services.  
**Timebox:** <= 30 minutes.  
**Date:** 2026-02-05.  
**Reviewer:** Codex (automated).

---

## 1) Release Security Checklist (Pass/Fail/Notes)

**Access Control & Permissions**
- [PASS] Containers run as non-root (`user: 10001:10001`).
- [PASS] Runtime data (`/var/lib/ardents/data`, `/var/lib/ardents/run`) owner-only perms inside container (`0700` dirs, `0600` files).
- [PASS] `run` volumes are tmpfs with `mode=700` (see docker compose `driver_opts`).
- [PASS] Health/Metrics bound to `127.0.0.1` (inside container).
- [WARN] Compose uses deprecated `version` field (non-security, should remove).

**Keys / Tokens**
- [PASS] Identity and transport keys stored with owner-only perms.
- [PASS] IPC token file `run/peer.token` present with owner-only permissions (IPC enabled).

**IPC / Integration**
- [PASS] IPC enabled in docker config (`integration.enabled: true` in `node.json`).
- [PASS] Token enforcement negative test executed: invalid token yields `ERR_IPC_REGISTER_FAILED`.

**Rate Limits / Abuse**
- [PASS] Rate limits configured in `node.json` for handshake and dirquery.
- [INFO] Runtime emits `net.degraded` due to low peers (expected in isolated local stack).

**Observability / Redaction**
- [PASS] Health endpoint responds OK (`/healthz`).
- [PASS] Metrics endpoint responds and includes baseline counters.
- [SKIP] Log redaction for sensitive payloads not actively tested (static rule exists in SPEC/TECH).
- [SKIP] Pcap redaction not tested (pcap disabled in config).

**Container Hardening**
- [PASS] `read_only: true` for containers.
- [PASS] `cap_drop: ALL` and `no-new-privileges:true`.
- [PASS] `/tmp` and `/run` tmpfs.

---

## 2) Threat Model (STRIDE-style, condensed)

### Assets
- `data/identity/identity.key` (identity private key)
- `data/keys/peer.key` / `peer.crt` (transport keys)
- `run/peer.token` (IPC token)
- `run/pcap.jsonl` (sensitive traffic)
- `addressbook.json` (trusted identities/aliases)
- `service descriptors` / `router.info` records

### Trust Boundaries
- External network (QUIC overlay)
- Local IPC (named pipe / unix socket)
- Local filesystem (keys/tokens)
- Local observability endpoints (health/metrics)

### Threats & Mitigations
- **Spoofing (IPC / services)**: Token-based IPC auth + owner-only ACLs. IPC disabled by default in docker config.
- **Tampering (data/keys)**: Owner-only file permissions enforced. Volume mounted with proper ownership.
- **Repudiation**: JSONL logs with event fields; avoid logging secrets.
- **Information Disclosure**: Prohibit secrets in logs/pcap; keep metrics/health loopback only.
- **Denial of Service**: Rate limits for handshake and dirquery; PoW enforcement; bans/dedup.
- **Elevation of Privilege**: Non-root containers; drop caps; no-new-privileges.

### Primary Residual Risks
- IPC enabled here; token/ACL enforcement should still be verified with an active negative test.
- Log redaction is policy-driven; need active tests to ensure payloads/secrets never logged.

---

## 3) Active Local Tests Performed (Local Docker)

**Environment**
- `docker compose up -d` (local)
- Running services: `peer`, `peer-2`, `peer-3`, `integration*`

**Checks**
1. **Permissions**
   - `/var/lib/ardents/run` -> `0700` dir, `status.json` `0600`
   - `/var/lib/ardents/data/*` -> `0700` dir, key files `0600`

2. **Observability endpoints**
   - `http://127.0.0.1:8081/healthz` => OK
   - `http://127.0.0.1:9090` => metrics rendered

3. **Network exposure (inside container)**
   - `netstat` shows `127.0.0.1:8081` and `127.0.0.1:9090` only

4. **IPC enabled + token/socket created**
   - `/var/lib/ardents/run/peer.sock` and `/var/lib/ardents/run/peer.token` present with owner-only perms
   - Integration service registers after peer restart (observed in logs)

5. **IPC auth negative test**
   - Temporarily replaced `peer.token` with invalid value and invoked `integration-ipc`
   - Result: `ERR_IPC_REGISTER_FAILED` (expected)

---

## 4) Gaps / Follow-ups (IPC)

IPC enabled; remaining follow-ups:
- `run/peer.token` exists and is `0600`.
- `run` directory is `0700`.
- IPC rejects connections without correct token.
- If ACL enforcement fails, runtime must refuse IPC with `ERR_GATEWAY_UNAUTHORIZED`.

---

## 5) Summary

Local docker stack is hardened (non-root, read-only, no caps), permissions are correct, observability endpoints are loopback-only, IPC token/socket are present, and invalid token auth fails as expected. Remaining gap: missing-token/ACL failure path.

## 6) Automated Smoke Script

Run: `scripts/security-smoke.sh`

This script covers health/metrics, perms, IPC token/socket presence, and a negative IPC auth test.

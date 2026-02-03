# TECH-050: Эксплуатация, директории и запуск (v1)

**Статус:** Draft (2026-02-02)  
**Назначение:** зафиксировать целевую схему “удобного продукта” для v1 (headless-first): где лежат файлы, как запускать и как оформить daemon/service.

---

## 1) Целевой режим v1

* Primary user v1: **server/node (headless-first)**.
* Основной интерфейс: CLI + конфиги, минимум интерактива.
* Автозапуск после reboot: **НЕ по умолчанию**, только по явному действию пользователя (systemd unit).
* Gateway: **выключен по умолчанию** (отдельный процесс, включение — явным действием).
* Логи: **stdout/journald по умолчанию** (структурированные JSON lines). Запись логов в файл — опциональная настройка/флаг.

---

## 2) Директории и раскладка (XDG/portable)

### 2.1 Linux default (XDG)

* Config: `${XDG_CONFIG_HOME:-~/.config}/ardents/`
* Data: `${XDG_DATA_HOME:-~/.local/share}/ardents/`
* State: `${XDG_STATE_HOME:-~/.local/state}/ardents/`
* Run: `${XDG_STATE_HOME:-~/.local/state}/ardents/run/`

### 2.2 Portable mode

Если задан `ARDENTS_HOME` или флаг `--home`, то используется portable раскладка:

* `<home>/config/`
* `<home>/data/`
* `<home>/run/`

### 2.3 Файлы v1

* config: `node.json` (в `config/`)
* address book: `addressbook.json` (в `data/`)
* identity: `data/identity/identity.key`
* transport keys: `data/keys/peer.key`, `data/keys/peer.crt`
* status: `run/status.json`
* gateway token: `run/peer.token`
* packet capture: `run/pcap.jsonl`

---

## 3) Запуск/настройка (CLI)

### 3.1 Инициализация

* `peer init` — создаёт директории, config (если нет), identity, transport keys и address book.

Portable пример:

* `peer init --home ./ardents-home`

### 3.2 Запуск

* `peer start` — запускает узел, пишет status в `run/status.json`, пишет логи в stdout.
* `peer status` — показывает status и `/healthz` (если включён).

Portable пример:

* `peer start --home ./ardents-home`

Опционально (локальный файл логов):

* `peer start --log.file log.jsonl` (пишет в `<run>/log.jsonl` для текущего home/XDG)
* `peer start --log.format json|text`

### 3.3 Gateway (опционально)

По умолчанию gateway не запускается.

Чтобы подготовить token (owner-only) и явно включить gateway-интеграцию:

* `peer start --enable-gateway ...`
* затем `gateway --home <same>` (или `gateway --token <path>`)

---

## 5) Support bundle (диагностика без секретов)

Для поддержки и воспроизводимости багов есть команда:

* `peer support bundle`

Что попадает в ZIP:

* метаданные (OS/arch/go version/build info)
* снимок путей (`meta/paths.json`)
* `config/node.json` (редактирование “секретов” зарезервировано; в v1 секретов в конфиге нет)
* `run/status.json` (если есть)
* tail файла логов (если включён `observability.log_file`)
* метаданные packet capture (`run/pcap.meta.json`), если существует `run/pcap.jsonl` (raw payload не включается)
* `data/addressbook.meta.json` по умолчанию, или полный `data/addressbook.json` по флагу `--include-addressbook` (поле `note` редактируется)

Не попадает (всегда):

* `run/peer.token`
* приватные ключи identity и transport keys (`data/identity/*`, `data/keys/*`)

---

## 4) systemd units (Linux)

### 4.1 Генерация unit файла

`peer systemd unit --mode=user` печатает unit в stdout.

Для system-wide (обычно server/VPS) рекомендуется portable home:

* `peer systemd unit --mode=system --home /var/lib/ardents`

### 4.1.1 Установка unit файла (удобная команда)

* `peer install-service --mode=user` — пишет unit в `${XDG_CONFIG_HOME:-~/.config}/systemd/user/ardents.service`.
* `peer install-service --mode=system --home /var/lib/ardents` — пытается записать unit в `/etc/systemd/system/ardents.service` (обычно требует root).

### 4.2 Установка/включение

Проект не включает автозапуск по умолчанию. Пользователь сам решает:

* `systemctl --user enable --now ardents.service` (user service)
* `sudo systemctl enable --now ardents.service` (system service)

---

## 5) Runbook: init -> start -> verify -> stop

### 5.1 Initialization (one-time)

1) `peer init --home /var/lib/ardents`
2) Ensure files exist:
   - `config/node.json`
   - `data/identity/identity.key`
   - `data/keys/peer.key`, `data/keys/peer.crt`

### 5.2 Start

1) `peer start --home /var/lib/ardents`
2) Verify:
   - `peer status --home /var/lib/ardents`
   - `/healthz` should be `OK` (if enabled)

### 5.3 Stop

1) Stop the service (systemd or signal):
   - `systemctl --user stop ardents.service` or `sudo systemctl stop ardents.service`
2) Check that `run/status.json` updated.

---

## 6) Upgrade and rollback

### 6.1 Upgrade

1) Stop service (see 5.3).
2) Backup data (see 7.1).
3) Update `peer` binary and related tools.
4) Start service (see 5.2).
5) Verify `peer status` and `/healthz`.

### 6.2 Rollback

1) Stop service.
2) Restore previous `peer` binary.
3) Restore backup (if data changed).
4) Start service and verify `/healthz`.

### 6.3 Config compatibility

* `config/node.json` must be backward compatible within minor versions.
* If incompatible, add an explicit migration in SPEC/TECH.

---

## 7) Backup / Restore

### 7.1 What to back up

* `config/node.json`
* `data/identity/identity.key`
* `data/addressbook.json`
* `data/keys/peer.key`, `data/keys/peer.crt`
* `data/lkeys/` (if v2 services are used)

### 7.2 Restore

1) Restore the same paths under `ARDENTS_HOME` or XDG.
2) Put files back from backup.
3) Start `peer` and verify `/healthz`.

### 7.3 Restore validation (clean node)

1) Create a clean `ARDENTS_HOME`.
2) Restore backup files (see 7.1).
3) Start `peer start --home <restored>` and verify:
   - `peer status` shows the same `identity_id`;
   - `/healthz` = `OK`;
   - `data/addressbook.json` loads without errors.

---

## 8) Observability baseline and alerts (SPEC-420)

### 8.1 Logs

* JSON Lines format.
* External boundary errors must be `ERR_*`.
* Logs must not include secrets (tokens, private keys, sensitive plaintext).

### 8.2 Metrics (minimum)

* Network state: `net_inbound_conns`, `net_outbound_conns`, `peers_connected`.
* IPC/Tasks errors: `ipc_errors_total`, `task_fail_total{code}`.
* Timeouts: `task_timeout_total`, `ipc_timeout_total`.
* Latency: `ack_latency_ms_bucket` (p50/p95 computed by monitoring).

### 8.3 Alert thresholds (recommended)

* `healthz != ok` for 2+ minutes.
* `peers_connected == 0` for 10+ minutes.
* `ack_rejected_total / msg_received_total > 5%` over 10 minutes.
* `task_fail_total` > 1% of `task_request_total` over 10 minutes.
* `ipc_errors_total` > 1% of `ipc_requests_total` over 10 minutes.
* `ack_latency_p95_ms > 2000` for 10 minutes.

---


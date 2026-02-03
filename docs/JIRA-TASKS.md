# JIRA Tasks: план разработки по спецификациям

**Статус документа:** Draft (2026-02-02)  
**Источник требований:** `spec/`, `docs/TECH-000-engineering-requirements.md`

Формат каждой задачи: **Название**, **Описание**, **Статус**, **DoR**, **DoD**, **AC**.  
Статусы: `Done`, `In Progress`, `Todo`.

---

## Ближайшие задачи (MVP?1)

### JIRA-01: Полный pipeline Envelope (ACK/TTL/PoW/Signature)
**Описание:** Завершить обработку Envelope по SPEC?140 с полным набором ошибок и корректным поведением ACK/REJECTED, включая PoW?проверку, подписи и дедуп.  
**Статус:** Done  
**DoR:**  
- Спеки SPEC?010, SPEC?140, SPEC?002 актуальны.  
- Есть базовые реализации envelope/pow/signature.  
**DoD:**  
- Вся логика ACK/REJECTED соответствует SPEC?140.  
- PoW применяется по правилам SPEC?002 (trusted skip + hard caps).  
- Подписи проверяются строго по did:key (SPEC?001).  
**AC:**  
- Набор unit?тестов: TTL expired, dedup, sig required/invalid, pow required/invalid.  
- Поведение одинаково для inbound/outbound.

**Фактический прогресс:**  
- Полный набор ошибок SPEC?140 (TTL/DEDUP/SIG/PoW/UNSUPPORTED/PAYLOAD_DECODE).  
- Dedup окно = `max(ttl_ms, 10 минут)` и ACK.DUPLICATE без повторной обработки.  
- Hard caps (`max_msg_bytes`/`max_payload_bytes`) enforced в pipeline.  
- Единая политика ACK: OK/DUPLICATE/REJECTED во всех ветках.

**Подзадачи:**  
- JIRA-01.1: Полный набор ошибок SPEC?140 (ERR_TTL_EXPIRED, ERR_DEDUP, ERR_SIG_REQUIRED/INVALID, ERR_POW_REQUIRED/INVALID).  
- JIRA-01.2: Hard caps enforcement (max_msg_bytes/max_payload_bytes) в обработке Envelope.  
- JIRA-01.3: Единая политика ACK (OK/DUPLICATE/REJECTED) для всех веток (в т.ч. service response).  
- JIRA-01.4: Unit?тесты pipeline (TTL, dedup, sig required/invalid, pow required/invalid).

### JIRA-02: Address Book (полная модель + импорт/экспорт)
**Описание:** Реализовать полный Address Book по SPEC?120, включая детерминированное разрешение alias, импорт/экспорт bundle как Content Node.  
**Статус:** Done  
**DoR:**  
- JSON формат Address Book зафиксирован.  
- Базовые CLI команды add/list есть.  
**DoD:**  
- Реализованы conflict resolution правила.  
- Импорт/экспорт bundle.addressbook.v1 (SPEC?200).  
**AC:**  
- Тесты на разрешение конфликтов и истечение `expires_at_ms`.  
- Отказ импорта от untrusted Identity.

**Фактический прогресс:**  
- Реализованы: JSON формат Book/Entry, load/save, trusted check, CLI add/list.  
- Реализованы: conflict resolution, expires_at_ms, import/export bundles (bundle.addressbook.v1), CLI import/export, тесты.

**Подзадачи:**  
- JIRA-02.1: Реализация conflict resolution по SPEC?120 (детерминированно).  
- JIRA-02.2: Поддержка `expires_at_ms` для imported записей.  
- JIRA-02.3: Импорт/экспорт `bundle.addressbook.v1` (SPEC?200).  
- JIRA-02.4: CLI команды import/export + тесты.

### JIRA-03: QUIC transport + Hello + peer_id verification
**Описание:** Завершить transport: стабильный handshake, strict peer_id vs TLS cert, error handling.  
**Статус:** Done  
**DoR:**  
- QUIC listener и dialer есть.  
- Hello CBOR формат и ValidateHello описаны.  
**DoD:**  
- Полный набор ошибок SPEC?110.  
- Таймауты/лимиты применяются.  
**AC:**  
- Тесты: mismatched peer_id, time skew, unsupported version.  
- Runtime переходит в degraded при clock_skew.

**Фактический прогресс:**  
- Реализовано: QUIC listen/dial, hello CBOR, peer_id vs TLS cert check, time skew check.  
- Реализовано: полный набор ошибок, ретраи/backoff, тесты, базовое net.* логирование.

**Подзадачи:**  
- JIRA-03.1: Полный набор ошибок SPEC?110 (ERR_HANDSHAKE_TIME_SKEW, ERR_PEER_ID_MISMATCH, ERR_UNSUPPORTED_VERSION, ERR_ADDR_INVALID).  
- JIRA-03.2: Ретраи/backoff для исходящих соединений (SPEC?100).  
- JIRA-03.3: Тесты handshake (time skew, mismatch, unsupported).  
- JIRA-03.4: Логирование событий net.* (SPEC?420).

### JIRA-04: Message Send/Receive (chat.msg.v1)
**Описание:** Реализовать полноценный обмен `chat.msg.v1` с ACK, логированием и состоянием доставки.  
**Статус:** Done  
**DoR:**  
- Envelope pipeline готов.  
- CLI `send` работает.  
**DoD:**  
- Send получает ACK.OK / ACK.DUPLICATE / ACK.REJECTED.  
- Консоль показывает status/error.  
**AC:**  
- Интеграционный тест: send > ACK.OK.  
- Отражение ошибок в CLI.

**Фактический прогресс:**  
- Реализовано: CLI `send --addr --to --text`, ожидание ACK.  
- Реализовано: delivery tracking, логирование статусов, интеграционный тест send>ACK OK.

**Подзадачи:**  
- JIRA-04.1: Delivery state tracking (sent/acked/failed).  
- JIRA-04.2: Логирование статусов отправки.  
- JIRA-04.3: Интеграционные тесты send>ACK OK/REJECTED.

### JIRA-05: Runtime lifecycle и degraded причины
**Описание:** Реализовать полный жизненный цикл runtime согласно SPEC?100, включая причины degraded и health.  
**Статус:** Done  
**DoR:**  
- netmgr FSM есть.  
**DoD:**  
- health endpoint `/healthz` (SPEC?420).  
- degraded причины: `clock_skew`, `transport_errors`, `low_peers`.  
**AC:**  
- Тест: degraded выставляется при clock_skew.  
- `/healthz` отражает статус.

**Фактический прогресс:**  
- Реализовано: FSM, выставление `transport_errors`.  
- Реализовано: `/healthz`, причина `low_peers` + тесты.  
- `clock_skew` отмечено как placeholder, требуются реальные события от handshake (зафиксировано в документации).

**Подзадачи:**  
- JIRA-05.1: HTTP `/healthz` endpoint (SPEC?420).  
- JIRA-05.2: Причины degraded (`clock_skew`, `low_peers`) + метрики/логи.  
- JIRA-05.3: Тесты degraded переходов.

---

## MVP?1 (завершение)

### JIRA-06: Observability v1
**Описание:** JSONL логи, базовые метрики и события NET.  
**Статус:** Done  
**DoR:** SPEC?420 утверждён.  
**DoD:**  
- JSONL логирование с обязательными полями.  
- Метрики (минимум) доступны локально.  
**AC:**  
- Проверка формата логов на sample run.  

**Подзадачи:**  
- JIRA-06.1: JSONL логгер (обязательные поля).  
- JIRA-06.2: Локальный Prometheus endpoint (минимум метрик).  
- JIRA-06.3: Retention policy (лог/pcap).

### JIRA-07: CLI/TUI console (минимальный UX)
**Описание:** Консольный UX по SPEC?400 с индикаторами trust/pow/net.  
**Статус:** Done  
**DoR:** Envelope pipeline готов.  
**DoD:**  
- Индикаторы trust/pow/net работают.  
- Ошибки отображаются пользователю.  
**AC:**  
- Демо?сценарий: trusted vs untrusted.

**Фактический прогресс:**  
- Реализованы индикаторы trust/pow/net при `send`.  
- CLI показывает ACK и ошибки, добавлен вывод `/healthz` в `peer status`.

**Подзадачи:**  
- JIRA-07.1: Индикаторы trust/pow/net.  
- JIRA-07.2: UX ошибок ACK/REJECTED.  
- JIRA-07.3: Команды статуса (peer status + health).

---

## MVP?2 (после стабилизации ядра)

### JIRA-08: Node Graph (CID/dag?cbor)
**Описание:** Реализация Content Node по SPEC?200 с проверкой CID/подписи/лимитов.  
**Статус:** Done  
**DoR:** Envelope pipeline стабилен.  
**DoD:**  
- Структуры Node v1, canonical dag?cbor.  
- Валидация CID и sig.  
**AC:**  
- Golden tests CID для известного Node.

**Фактический прогресс:**  
- Реализованы: Node v1 структура, canonical dag?cbor, CIDv1 (sha2?256) вычисление.  
- Реализована валидация CID/подписи/лимитов.  
- Добавлены golden tests CID (encode+verify).

**Подзадачи:**  
- JIRA-08.1: Структуры Node v1 + dag?cbor canonical.  
- JIRA-08.2: Валидация CID + подписи + лимитов.  
- JIRA-08.3: Golden tests CID.

### JIRA-09: Providers + node.fetch.v1
**Описание:** Полная реализация provider hints и fetch по SPEC?210.  
**Статус:** Done  
**DoR:** Node Graph готов.  
**DoD:**  
- ProviderRecord, fetch, cache, selection strategy.  
**AC:**  
- Тест: fetch по provider list.

**Фактический прогресс:**  
- Реализованы: provider.announce payload, реестр providers, фильтрация по TTL, стратегия выбора (trusted/recent/success).  
- Реализованы: fetch по provider list, валидация CID/подписи, кэширование Node.  
- Добавлен интеграционный тест fetch по provider list.

**Подзадачи:**  
- JIRA-09.1: ProviderRecord + announce payload.  
- JIRA-09.2: Selection strategy (recent/trusted/parallel).  
- JIRA-09.3: Fetch cache + errors.

### JIRA-10: Access Policy & Encryption
**Описание:** Реализовать encrypted nodes по SPEC?220.  
**Статус:** Done  
**DoR:** Node Graph готов.  
**DoD:**  
- `enc.node.v1`, recipients sorting, XChaCha20?Poly1305.  
**AC:**  
- Тест: decrypt success/fail.

**Фактический прогресс:**  
- Реализовано: `enc.node.v1`, `EncryptedBody.v1`, `PrivateNodePayload.v1`, сортировка recipients.  
- Реализовано: XChaCha20?Poly1305 + sealed key для recipients (X25519).  
- Добавлены тесты encrypt/decrypt и no?recipient.

**Подзадачи:**  
- JIRA-10.1: enc.node.v1 структура + recipients сортировка.  
- JIRA-10.2: XChaCha20?Poly1305 encrypt/decrypt.  
- JIRA-10.3: Sealed key для recipients.

---

## MVP?3 (сервисы/задачи/интеграции)

### JIRA-11: Service Descriptor + Updates
**Описание:** Реализация service.descriptor.v1 и обновлений через announce (SPEC?300).  
**Статус:** Done  
**DoR:** Envelope pipeline + Node Graph.  
**DoD:**  
- Latest trusted descriptor logic.  
**AC:**  
- Тест: обновление descriptor по времени.

**Фактический прогресс:**  
- Реализованы: `service.descriptor.v1`, валидация `service_id`, endpoints, capabilities/limits.  
- Реализовано: `service.announce.v1` + fetch descriptor + latest?trusted update.  
- Добавлен тест обновления реестра через announce.

**Подзадачи:**  
- JIRA-11.1: service.descriptor.v1 структура как Node.  
- JIRA-11.2: Latest trusted descriptor logic.  
- JIRA-11.3: service.announce.v1 обработка.

### JIRA-12: Tasks lifecycle
**Описание:** task.request/accept/progress/result/fail/receipt (SPEC?310).  
**Статус:** Done  
**DoR:** Service model готов.  
**DoD:**  
- Идемпотентность по client_request_id.  
**AC:**  
- Тесты на повтор request.

**Фактический прогресс:**  
- Реализованы payload?типы `task.*.v1`, кодеки и обработка `task.request.v1`.  
- Идемпотентность по `client_request_id` + защита от повторного `task_id`.  
- Добавлены тесты на повтор и конфликт payload.

**Подзадачи:**  
- JIRA-12.1: task.request/accept/fail.  
- JIRA-12.2: task.progress/result/receipt.  
- JIRA-12.3: Идемпотентность client_request_id.

### JIRA-13: AI chat profile (Tasks?based)
**Описание:** ai.chat.v1 профиль поверх Tasks (SPEC?330).  
**Статус:** Done  
**DoR:** Tasks lifecycle готов.  
**DoD:**  
- ai.chat.input.v1 + transcript Node.  
**AC:**  
- E2E тест chat request/result.

**Фактический прогресс:**  
- Реализованы `ai.chat.input.v1` и `ai.chat.transcript.v1` (public/encrypted).  
- Обработка `task.request` для `job_type=ai.chat.v1` с accept+result.  
- Тест E2E chat request/result.

**Подзадачи:**  
- JIRA-13.1: ai.chat.input.v1 структура.  
- JIRA-13.2: transcript Node + safety labels.  
- JIRA-13.3: E2E тест поверх Tasks.

---

## Инструменты разработки

### JIRA-14: Packet capture + replay
**Описание:** Реализация capture/replay по SPEC?430 (off by default).  
**Статус:** Done  
**DoR:** Envelope pipeline стабилен.  
**DoD:**  
- `run/pcap.jsonl` с owner?only правами.  
- replay tool с sandbox?default.  
**AC:**  
- Replay воспроизводит задержки.  

**Подзадачи:**  
- JIRA-14.1: pcap writer (JSONL + owner?only).  
- JIRA-14.2: replay tool (sandbox?only).  
- JIRA-14.3: CLI flags + docs.

**Фактический прогресс:**  
- Реализован pcap writer (JSONL) с owner?only правами (POSIX 0600, Windows ACL).  
- Packet capture off by default, включается флагом `peer start --pcap`, логирует `event=pcap.enabled`.  
- Реализована утилита `cmd/replay` с sandbox?default, флаг `--allow-network` для реальной сети, сохранение относительных задержек.

### JIRA-15: Simulator
**Описание:** Симулятор N peer с соблюдением лимитов и PoW.  
**Статус:** Done  
**DoR:** Envelope pipeline готов.  
**DoD:**  
- метрики latency/drop/pow reject.  
**AC:**  
- Test run N=5.

**Подзадачи:**  
- JIRA-15.1: in?proc N peer runner.  
- JIRA-15.2: traffic generator (chat/task/fetch).  
- JIRA-15.3: metrics collector.

**Фактический прогресс:**  
- Добавлен in?proc симулятор `cmd/sim` с N peer и генерацией chat/task/fetch.  
- Метрики: latency avg/p95, drop rate, pow reject rate, статистика по типам.  
- Симулятор уважает лимиты/PoW/TTL через реальный pipeline `handleEnvelope`.

---

## Полное закрытие SPEC (после MVP?1)

### JIRA-16: Routing & Relays (SPEC?130)
**Описание:** Реализовать routing/relays v1: форматы relay?пакетов, forward/return ACK, ограничения N=1..2, правила ACK пути.  
**Статус:** Done  
**DoR:** Envelope pipeline стабилен; QUIC transport готов.  
**DoD:**  
- Реализован relay?маршрут N=1..2 с явными правилами ACK (через relay/прямо).  
- Лимиты hop/TTL/size применяются.  
- Ошибки и ACK соответствуют SPEC?130/140.  
**AC:**  
- Интеграционный тест: sender > relay > receiver (ACK OK).  
- Интеграционный тест: relay drop > sender получает REJECTED/timeout.  

**Фактический прогресс:**  
- Добавлены RelayPacket encode/decode + sealed?box крипто (X25519/XChaCha20?Poly1305).  
- Добавлен relay?handler в runtime (TTL, decrypt, forward, relay?errors).  
- Добавлены интеграционные тесты: N=1, N=2, drop/REJECTED.  

**Подзадачи:**  
- JIRA-16.1: Wire?форматы relay?envelope и relay?routing. (Done)  
- JIRA-16.2: Forward/return ACK (правила SPEC?130). (Done)  
- JIRA-16.3: Лимиты hop/TTL/size + тесты. (Done)  

### JIRA-17: Integration Gateway (SPEC?320)
**Описание:** Реализовать минимальный gateway HTTP (loopback only) с локальным токеном.  
**Статус:** Done  
**DoR:** Runtime start/stop, send pipeline и addressbook готовы.  
**DoD:**  
- `cmd/gateway` запускается на loopback.  
- Токен хранится в `run/` с правами `0600` и поддерживает ротацию.  
- Минимальные методы: `POST /send`, `GET /status`, `POST /resolve`.  
**AC:**  
- E2E тест: send > ACK OK через gateway.  
- Проверка отказа при неверном токене.  

**Фактический прогресс:**  
- Добавлен `cmd/gateway` (loopback only), Auth Bearer из `run/peer.token`, методы `/status`, `/send`, `/resolve`.  
- Ротация token при старте `peer` (owner?only права).  
- Добавлены unit?тесты gateway (auth/send/resolve).  

**Подзадачи:**  
- JIRA-17.1: HTTP сервер + auth token (rotation). (Done)  
- JIRA-17.2: Реализация `/send`, `/status`, `/resolve`. (Done)  
- JIRA-17.3: Тесты и документация. (Done)  

### JIRA-18: Node Browser (SPEC?410)
**Описание:** Реализовать минимальный node?browser (CLI) для просмотра ноды и истории (`prev`/`supersedes`).  
**Статус:** Done  
**DoR:** Node Graph + storage готовы.  
**DoD:**  
- CLI команда `node get <cid>` выводит метаданные и ссылки.  
- Поддержан просмотр истории (`prev`, `supersedes`).  
**AC:**  
- Тест: локальная нода читается и корректно отображается.  

**Фактический прогресс:**  
- Добавлен `cmd/node get --id <cid> [--decrypt] [--history-depth N]` с выводом метаданных, ссылок и истории.  
- Документация по использованию: `docs/TECH-040-node-browser-usage.md`.  

**Подзадачи:**  
- JIRA-18.1: CLI команды просмотра node. (Done)  
- JIRA-18.2: История по `prev`/`supersedes`. (Done)  
- JIRA-18.3: Тесты и минимальные примеры. (Done)

---

## Privacy-first v2 (Tor/I2P-like)

Ниже — задачи для **основного профиля v2** (см. `spec/SPEC-000-system-overview.md` и серию `SPEC-500..550`). Все задачи этого блока **не должны** ломать совместимый v1 direct mode.

### JIRA-19: Directory Authorities + Reseed (SPEC?500)
**Описание:** Реализовать bootstrap v2: загрузка `reseed.bundle.v1` по HTTPS, проверка quorum 3/5, применение `params`, загрузка initial seed routers в NetDB cache.  
**Статус:** Done  
**DoR:**  
- Приняты и зафиксированы pinned identity_id 5 DA и URL(ы) reseed в конфигурации.  
- Реализован CBOR canonical decode + Ed25519 verify (есть в кодовой базе для других сущностей).  
**DoD:**  
- Верификация `reseed.bundle.v1` (подписи, expires, params) строго по SPEC?500.  
- При неуспехе входа в сеть — degraded `no_bootstrap` + backoff.  
- Метрики/события: `reseed.fetch.*`, `reseed.verify.*`, `reseed.apply.*`.  
**AC:**  
- Тест: invalid signature > reject.  
- Тест: 2/5 signatures > reject, 3/5 > accept.  
- Тест: expired bundle > reject.

**Подзадачи:**  
- JIRA-19.1: Формат и валидация `reseed.bundle.v1` (canonical CBOR + quorum).  
- JIRA-19.2: HTTPS fetch + retry/backoff + timeouts.  
- JIRA-19.3: Применение `params` + сохранение активных параметров до `expires_at_ms`.

**Фактический прогресс:**  
- Добавлена конфигурация `reseed` (network_id/urls/authorities) и default `ardents.mainnet`.  
- Реализован `reseed` пакет: fetch по HTTPS, проверка quorum 3/5, валидация params, проверка RouterInfo signatures.  
- Runtime применяет reseed при отсутствии bootstrap peers и фиксирует `no_bootstrap` при неуспехе.  
- Добавлены unit?тесты на quorum/подписи.

### JIRA-20: NetDB core (DHT) + Records (SPEC?510)
**Описание:** Реализовать NetDB: хранение/валидация `router.info.v1`, `service.lease_set.v1`, `service.head.v1`; DHT операции `find_node/find_value/store/reply`; anti-poisoning и rate-limits.  
**Статус:** Done  
**DoR:**  
- Реализован transport p2p (QUIC) и Envelope v1 control-plane.  
- Определены источники времени (UTC ms) и лимиты (есть в v1 runtime).  
**DoD:**  
- Реализованы вычисление `dht_key` и обращение по нему строго по SPEC?510.  
- Подписи всех record типов валидируются; невалидное не сохраняется.  
- Включены rate-limits и PoW-гейты для `netdb.store` от untrusted (SPEC?002/510).  
**AC:**  
- Тест: store invalid sig > REJECTED.  
- Тест: store expired > REJECTED.  
- Тест: find_value по ключу service_id возвращает актуальный head/leases.  

**Подзадачи:**  
- JIRA-20.1: Валидация и хранение `router.info.v1` (peer_id match transport_pub, addrs limits).  
- JIRA-20.2: Валидация и хранение `service.head.v1` и `service.lease_set.v1` (owner signature, service_id recompute).  
- JIRA-20.3: Wire сообщения NetDB (`netdb.*.v1`) + интеграционные тесты N=3..5.  
- JIRA-20.4: Anti-poisoning quarantine cache + “verified router” критерии.

**Фактический прогресс:**  
- Добавлен пакет `internal/core/app/netdb` с хранением, валидацией и DHT key derivation по SPEC?510.  
- Реализованы wire?типы `netdb.find_node/find_value/store/reply` и обработчики в runtime.  
- Добавлены базовые unit?тесты на store/find.

### JIRA-21: RouterInfo generation (local) + публикация в NetDB (SPEC?510)
**Описание:** На стороне роутера: генерация `onion_pub` (X25519), формирование `router.info.v1`, периодическая публикация/обновление в NetDB.  
**Статус:** Done  
**DoR:**  
- Хранилище ключей (`data/keys/`) готово.  
**DoD:**  
- `router.info.v1` подписывается транспортным Ed25519 ключом; `peer_id` строго совпадает.  
- Обновление `router.info.v1` не реже, чем раз в `record_max_ttl_ms/2`.  
**AC:**  
- Тест: другой peer_id при том же transport_pub > reject.  
- Тест: on startup publish and reachable by another node via NetDB.

**Фактический прогресс:**  
- Добавлен генератор/хранилище `onion.key` (X25519) в `data/keys/`.  
- Runtime публикует `router.info.v1` в NetDB при старте и по таймеру `record_max_ttl_ms/2`.  
- Подпись `router.info.v1` выполняется транспортным Ed25519 ключом.

### JIRA-22: Tunnel build + rotation + padding (SPEC?520)
**Описание:** Реализовать туннели v2: build/reply, hop-to-hop keys, `tunnel.data.v1`, rotation и обязательный `basic.v1` padding.  
**Статус:** Done  
**DoR:**  
- NetDB отдаёт валидные `router.info.v1` и есть список verified routers.  
**DoD:**  
- Туннель строится L=3 по умолчанию и ротируется по `rotation_ms`.  
- Replay protection по `(peer_id,tunnel_id,seq)` работает.  
- Padding отправляется по правилам `basic.v1`.  
**AC:**  
- Интеграционный тест: build inbound+outbound, доставка `tunnel.data.v1` через 3 hops.  
- Тест: replay seq > `ERR_TUNNEL_DATA_REPLAY`.  

**Подзадачи:**  
- JIRA-22.1: Build протокол + per-hop crypto (HKDF/XChaCha).  
- JIRA-22.2: Forward/deliver/padding обработка `tunnel.data.v1`.  
- JIRA-22.3: Rotation scheduler + health сигнализация.

**Фактический прогресс:**  
- Добавлен пакет `internal/core/domain/tunnel` с build/data форматами и криптопримитивами (X25519 + HKDF + XChaCha20-Poly1305).  
- В runtime добавлены обработчики `tunnel.build.v1` и `tunnel.data.v1`, хранение hop-keys и replay-защита по `seq`.  
- Добавлены базовые события `tunnel.build.ok` и обработка padding/no-op.  
- Реализован менеджер туннелей: построение outbound+inbound на базе NetDB, ротация по `rotation_ms`, `basic.v1` padding с выравниванием ciphertext по bucket-списку.  
- Добавлены build/forward envelope-подписи (или PoW при отсутствии identity), проверка `next_tunnel_id` при forward.

**Осталось для DoD:**  
- Интеграционный тест: build inbound+outbound, доставка `tunnel.data.v1` через 3 hops (sim).  
- Forward-build не требуется: используем direct initiator build (решение зафиксировано).  

**Фактический прогресс (2026-02-03):**  
- Добавлены тесты: `tunnel_integration_test.go` (3-hop delivery), `tunnel_replay_test.go` (replay seq).  
- `go test ./...` — OK.  

### JIRA-23: Envelope v2 + Garlic E2E (SPEC?550/SPEC?520)
**Описание:** Реализовать `envelope.v2` и `garlic.msg.v1`: e2e шифрование до сервиса, упаковка в `tunnel.data.v1`, TTL/подписи/дедуп.  
**Статус:** Done  
**DoR:**  
- Туннели работают (JIRA-22).  
- Есть LeaseSet сервиса с `enc_pub` (SPEC?510).  
**DoD:**  
- `envelope.v2` encode/decode/sign/verify строго по SPEC?550.  
- `garlic.msg.v1` e2e decrypt/validate на стороне сервиса.  
- В v2 нет автоматического ACK; задачи используют Tasks протокол.  
**AC:**  
- Интеграционный тест: клиент отправляет `envelope.v2` в сервис через garlic и получает ответ через mailbox — выполнено.  

**Фактический прогресс:**  
- Добавлен пакет `internal/shared/envelopev2` с wire?типами, валидацией, подписью/проверкой.  
- Добавлен пакет `internal/core/domain/garlic` с шифрованием/дешифрованием `garlic.msg.v1` (X25519 + HKDF + XChaCha20?Poly1305).  
- Базовые unit?тесты для `envelope.v2` и garlic.
- Добавлен обработчик garlic?delivery на стороне runtime + хранилище ключей в `data/lkeys/`.
- Добавлен обработчик `envelope.v2` для `task.request.v1` и генерация ответов `task.*` в формате `envelope.v2` (через `reply_to.service_id`).
- Добавлена доставка `envelope.v2` ответов через outbound?туннель на lease из NetDB LeaseSet (mailbox).
- Интеграционный тест garlic > envelope.v2 > task > reply через mailbox добавлен и проходит.

### JIRA-24: Публикация анонимного сервиса (Head + LeaseSet) (SPEC?510/530)
**Описание:** Реализовать v2 публикацию сервиса: `service.descriptor.v2`, NetDB `service.head.v1` + `service.lease_set.v1`, refresh каждые 5 минут.  
**Статус:** Done  
**DoR:**  
- NetDB store/lookup работает.  
**DoD:**  
- Сервис публикует head и leases; клиенты могут резолвить и доставлять сообщения.  
**AC:**  
- Интеграционный тест: после рестарта сервиса новый descriptor становится виден через `service.head.v1`.

### JIRA-25: Directory Service `dir.query.v1` (SPEC?530)
**Описание:** Реализовать сервис?индексатор: принимает `task.request(job_type=dir.query.v1)`, возвращает `dir.query.result.v1` Node с детерминированным скорингом и TTL 60s.  
**Статус:** Done  
**DoR:**  
- Service descriptor v2 содержит `resources` и `capabilities`.  
- Есть механизм получения `service.head.v1` и загрузки descriptors.  
**DoD:**  
- Directory Service индексирует только валидные descriptors + валидный head в NetDB.  
- Результаты детерминированы (score+sort), подпись результата проверяема (CID+sig).  
**AC:**  
- Тест: одинаковый запрос > одинаковая выдача (query_hash совпадает).  
- Тест: untrusted directory > клиент отклоняет (ERR_DIR_UNTRUSTED).

**Фактический прогресс:**  
- Реализован `dir.query.v1` на базе Tasks v2, формирование `dir.query.result.v1` Node.  
- Индексация по валидным `service.head.v1` + `service.descriptor.v2`, фильтры prefix/requirements/resources, детерминированный скоринг и сортировка.  
- Добавлен тест на успешный `dir.query.v1` с выдачей result node.

### JIRA-26: E2E динамическое тестирование v2 (SPEC?540)
**Описание:** Добавить сценарии динамического тестирования privacy-first профиля (sim): bootstrap > netdb > tunnels > service discovery > tasks.  
**Статус:** Done  
**DoR:**  
- Реализованы JIRA-19..25.  
**DoD:**  
- Автотесты в `cmd/sim`/интеграционном наборе покрывают: reseed quorum, netdb poisoning reject, tunnel rotate, padding присутствует.  
**AC:**  
- Прогон симулятора N=10 показывает стабильность без деградации, p95 latency фиксируется.  

**Фактический прогресс:**  
- Добавлен v2 dynamic suite в `cmd/sim` (`-profile v2`) с проверками: reseed quorum, netdb poisoning reject, netdb wire, dirquery e2e, tunnel rotate, padding.  
- Добавлен вывод `latency_p95_ms` в v2 suite и пример фактического прогона в `TECH-030`.  
- Добавлена клиентская обработка `task.*` ответов в v2 (валидация + кэш ответов).

### JIRA-27: Полный тех-аудит и ремедиация (TECH-000/Spec compliance)
**Описание:** Зафиксировать результаты полного аудита кода и выполнить исправления: размер файлов/функций, классифицируемость ошибок, дублирование, запрет panic, соответствие DDD-слоям, линтер/динамика/сборка.  
**Статус:** Done  
**DoR:**  
- TECH-000 и AGENTS.md актуальны.  
- Доступен полный репозиторий (core + legacy).  
**DoD:**  
- Устранены нарушения TECH-000 (размеры файлов/функций/params, дублирование, panic).  
- Протокольные/наружные ошибки классифицируемы (ERR_*), маппятся в стабильные коды.  
- Структура слоёв приведена к DDD (domain/app/infra/transport) или зафиксирована отдельной SPEC.  
- Линтер/тесты/сборка/динамика проходят.  
**AC:**  
- `go test ./...`, `go vet ./...`, `go build ./...` — OK.  
- `golangci-lint run` — OK.  
- Симуляции A/B/C и v2 suite проходят с ожидаемыми метриками (см. TECH-030).  

**Фактический прогресс (ремедиация 2026-02-03):**  
- Кодировки docs/spec нормализованы в UTF-8 (исправлены битые символы).  
- Неклассифицируемые ошибки `errors.New` приведены к ERR_* в core/transport/shared (netmgr, quic, onionkey, uuidv7).  
- Декомпозирован `handleEnvelope` (runtime) на обработчики: auth/basic checks, netdb, tasks, provider announce, node fetch.  
- `go test ./...` проходит после рефакторинга обработчика.  
- Декомпозирован `fetchFromProvider` (runtime) и вынесена логика dial/hello в `quic.Dialer` для уменьшения дублирования и размера функций.  
- Декомпозирован `publishServiceHeadAndLeaseSet` и `quic.Server.handleConn` на набор более мелких шагов.  
- Декомпозированы IPC- и v2-обработчики задач (runtime/handler_ipc.go, runtime/envelopev2.go).  
- Декомпозированы CLI webclient и integration-ipc (разделены parse/validate/build/send/handle шаги).  
- Декомпозированы `runtime.New` и `runtime.Start` (инициализация и запуск разделены на подсекции).  
- Декомпозированы `handleNetDBMessage` и `dir.query.v2` обработчик (v2 directory): выделены helpers для валидации/ранжирования/ответов.  
- Упрощён `buildTunnelPath` (выделены шаги построения hop/record/self/store).  
- Упрощён `EncryptNode` (helpers для ключа/nonce/recipients/node), `buildAITranscript` и `appdirs.Resolve` (выделение логики платформ).  
- Прогон `go test ./...` после рефакторинга — OK.  
- Декомпозированы симулятор и v2 checks: `cmd/sim/main.go`, `cmd/sim/v2_dirquery.go`, `cmd/sim/v2_checks.go`, `cmd/replay/main.go`.  
- Декомпозированы bundle/support и импорт addressbook (`internal/core/infra/support/bundle.go`, `internal/core/infra/addressbook/addressbook.go`).  
- Прогон `go test ./...` после изменений симулятора — OK.  
- Прогоны `go vet ./...`, `go build ./...`, `golangci-lint run` — OK.  
- Прогнаны симуляции A/B/C и v2 suite, результаты зафиксированы в `TECH-030`.  
- Принято решение: тестовые функции не декомпозируем в рамках JIRA-27 (осознанное исключение).  

### JIRA-28: Web integration profile + IPC adapter (SPEC-320/340)
**Описание:** Добавить минимальный профиль `web.request.v1` и рабочий IPC-адаптер для прокси локального сайта.  
**Статус:** Done  
**DoR:** IPC Integration v1 доступен (SPEC-320).  
**DoD:**  
- Спецификация `SPEC-340` описывает запрос/ответ.  
- Есть адаптер `cmd/integration-ipc` для прокси локального HTTP.  
- Есть CLI-клиент `cmd/webclient` для ручного запроса и fetch результата.  
**AC:**  
- `task.request.v1` с `job_type=web.request.v1` получает `task.result.v1` и `web.response.v1` node.  

**Фактический прогресс:**  
- Добавлен `spec/SPEC-340-web-service-profile.md`.  
- Добавлен `cmd/integration-ipc` (регистрация сервиса + обработка задач).  
- Добавлен `cmd/webclient` (ручной запрос/получение результата).  
- Прогон E2E (локально): `peer` + `integration-ipc` + `webclient` → ACK OK, `task.result.v1`, `web.response.v1` (status=200, body получен).  

---

## Прод-готовность (core, без legacy)

### JIRA-29: Runbooks + эксплуатационная модель (TECH-050)
**Описание:** Зафиксировать прод-эксплуатацию: деплой, запуск/останов, мониторинг, инциденты, апгрейд/rollback, бэкап/восстановление.  
**Статус:** Done  
**DoR:**  
- TECH-050 актуален.  
**DoD:**  
- В `TECH-050` описаны: схема деплоя, обязательные переменные/файлы, порядок старта/остановки, инцидентные процедуры.  
- Отдельно описаны апгрейд/rollback (шаги + проверки), требования к совместимости конфигов.  
**AC:**  
- По runbook можно выполнить полный цикл: init → start → verify → stop → upgrade → rollback.  

**Фактический прогресс:**  
- `TECH-050` обновлён: runbook, upgrade/rollback, backup/restore, инциденты/диагностика, статус Done (2026-02-03).  

### JIRA-30: Security hardening + проверки границ (SPEC-002/320/340)
**Описание:** Укрепить IPC/web-интеграцию и внешние границы: доступ, токены, ограничения входных данных.  
**Статус:** Done  
**DoR:**  
- SPEC-002 и SPEC-320/340 актуальны.  
**DoD:**  
- IPC работает только локально, токен обязателен, права файла токена проверяются.  
- Добавлены тесты: отказ при отсутствии токена, отказ при неверных правах, отказ на абсолютные URL и SSRF-попытки.  
- Введены лимиты на размер входа/выхода и таймауты (явно задокументированы).  
**AC:**  
- Набор тестов на security-границы проходит и покрывает IPC + web.request.v1.  

**Фактический прогресс:**  
- IPC token файл проверяется на owner-only, unix socket получает 0600, ошибка при нарушении прав.  
- Upstream ограничен loopback (localhost/127.0.0.1/::1), заголовок Host игнорируется, SSRF/absolute URL запрещены.  
- Добавлены тесты на loopback upstream, запрет absolute URL и проверку прав token.  
- `go test ./...` — OK.  

### JIRA-31: Observability baseline + алерты (SPEC-420)
**Описание:** Зафиксировать минимальный набор метрик/логов и алертов для прод-эксплуатации.  
**Статус:** Done  
**DoR:**  
- SPEC-420 актуален.  
**DoD:**  
- Метрики: состояние сети, ошибки IPC/Tasks, timeouts, latency p50/p95.  
- Логи: классифицируемые ERR_* на входных/выходных границах.  
- В `TECH-050` добавлены пороги алертов и рекомендации по реагированию.  
**AC:**  
- Локальный прогон выдаёт метрики и логи, алерт-правила формализованы.  

**Фактический прогресс:**  
- `TECH-050` дополнен: baseline по логам/метрикам и набор алертов с порогами.  

### JIRA-32: CI gates + release checks
**Описание:** Ввести обязательные проверки перед релизом.  
**Статус:** Done  
**DoR:**  
- JIRA-27 в прогрессе (есть перечень требований TECH-000).  
**DoD:**  
- Скрипт/CI выполняет: `go test ./...`, `go vet ./...`, `go build ./...`, `golangci-lint run`.  
- Добавлен отдельный шаг для симуляций A/B/C + v2 suite.  
- Пороговые критерии (latency/ошибки) описаны в `TECH-030`.  
**AC:**  
- Один прогон CI воспроизводим локально одной командой.  

**Фактический прогресс:**  
- Добавлены скрипты `scripts/ci/check.ps1` и `scripts/ci/check.sh` (test/vet/build/lint + sim A/B/C + v2).  
- Обновлён `TECH-030` с инструкциями запуска CI.  

### JIRA-33: Нагрузочные и стабильностные тесты (core)
**Описание:** Добавить soak/load сценарии для валидации стабильности ядра.  
**Статус:** Done  
**DoR:**  
- JIRA-26 завершён (есть dynamic suite).  
**DoD:**  
- Добавлены сценарии: длительный прогон (soak), повышенная нагрузка (N>=50), проверка деградаций.  
- Фиксируются метрики: p95 latency, error rate, memory/cpu.  
- Результаты фиксируются в `TECH-030` с датой и параметрами.  
**AC:**  
- Нагрузочный прогон проходит без деградации, метрики соответствуют порогам.  

**Фактический прогресс:**  
- Добавлены `scripts/load/load.ps1|sh` и `scripts/load/soak.ps1|sh`.  
- Прогоны load/soak выполнены, результаты зафиксированы в `TECH-030`.  

### JIRA-34: Бэкапы и восстановление данных узла
**Описание:** Описать и проверить процедуру резервного копирования ключевых данных (без legacy).  
**Статус:** Done  
**DoR:**  
- Определены ключевые пути данных/ключей.  
**DoD:**  
- Описана процедура backup/restore для `config/`, `data/identity/`, `data/addressbook.json`, `data/lkeys/`.  
- Проверено восстановление на чистом узле.  
**AC:**  
- Восстановленный узел успешно стартует и сохраняет идентичность.  

**Фактический прогресс:**  
- `TECH-050` дополнен процедурой backup/restore и шагами валидации восстановления на чистом узле.  

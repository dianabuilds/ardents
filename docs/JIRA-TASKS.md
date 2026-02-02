# JIRA Tasks: план разработки по спецификациям

**Статус документа:** Draft (2026-02-02)  
**Источник требований:** `spec/`, `docs/TECH-000-engineering-requirements.md`, `docs/TECH-010-development-questions.md`

Формат каждой задачи: **Название**, **Описание**, **Статус**, **DoR**, **DoD**, **AC**.  
Статусы: `Done`, `In Progress`, `Todo`.

---

## Ближайшие задачи (MVP‑1)

### JIRA‑01: Полный pipeline Envelope (ACK/TTL/PoW/Signature)
**Описание:** Завершить обработку Envelope по SPEC‑140 с полным набором ошибок и корректным поведением ACK/REJECTED, включая PoW‑проверку, подписи и дедуп.  
**Статус:** In Progress (частично реализовано)  
**DoR:**  
- Спеки SPEC‑010, SPEC‑140, SPEC‑002 актуальны.  
- Есть базовые реализации envelope/pow/signature.  
**DoD:**  
- Вся логика ACK/REJECTED соответствует SPEC‑140.  
- PoW применяется по правилам SPEC‑002 (trusted skip + hard caps).  
- Подписи проверяются строго по did:key (SPEC‑001).  
**AC:**  
- Набор unit‑тестов: TTL expired, dedup, sig required/invalid, pow required/invalid.  
- Поведение одинаково для inbound/outbound.

**Фактический прогресс:**  
- Реализованы: ACK payload, TTL/dedup, подпись/verify, PoW verify (runtime).  
- Trusted skip PoW подключён через Address Book.  
- Не реализовано: полный набор ошибок SPEC‑140, hard caps enforcement в pipeline, единая политика ACK.DUPLICATE/REJECTED во всех ветках.

**Подзадачи:**  
- JIRA‑01.1: Полный набор ошибок SPEC‑140 (ERR_TTL_EXPIRED, ERR_DEDUP, ERR_SIG_REQUIRED/INVALID, ERR_POW_REQUIRED/INVALID).  
- JIRA‑01.2: Hard caps enforcement (max_msg_bytes/max_payload_bytes) в обработке Envelope.  
- JIRA‑01.3: Единая политика ACK (OK/DUPLICATE/REJECTED) для всех веток (в т.ч. service response).  
- JIRA‑01.4: Unit‑тесты pipeline (TTL, dedup, sig required/invalid, pow required/invalid).

### JIRA‑02: Address Book (полная модель + импорт/экспорт)
**Описание:** Реализовать полный Address Book по SPEC‑120, включая детерминированное разрешение alias, импорт/экспорт bundle как Content Node.  
**Статус:** Done  
**DoR:**  
- JSON формат Address Book зафиксирован.  
- Базовые CLI команды add/list есть.  
**DoD:**  
- Реализованы conflict resolution правила.  
- Импорт/экспорт bundle.addressbook.v1 (SPEC‑200).  
**AC:**  
- Тесты на разрешение конфликтов и истечение `expires_at_ms`.  
- Отказ импорта от untrusted Identity.

**Фактический прогресс:**  
- Реализованы: JSON формат Book/Entry, load/save, trusted check, CLI add/list.  
- Реализованы: conflict resolution, expires_at_ms, import/export bundles (bundle.addressbook.v1), CLI import/export, тесты.

**Подзадачи:**  
- JIRA‑02.1: Реализация conflict resolution по SPEC‑120 (детерминированно).  
- JIRA‑02.2: Поддержка `expires_at_ms` для imported записей.  
- JIRA‑02.3: Импорт/экспорт `bundle.addressbook.v1` (SPEC‑200).  
- JIRA‑02.4: CLI команды import/export + тесты.

### JIRA‑03: QUIC transport + Hello + peer_id verification
**Описание:** Завершить transport: стабильный handshake, strict peer_id vs TLS cert, error handling.  
**Статус:** Done  
**DoR:**  
- QUIC listener и dialer есть.  
- Hello CBOR формат и ValidateHello описаны.  
**DoD:**  
- Полный набор ошибок SPEC‑110.  
- Таймауты/лимиты применяются.  
**AC:**  
- Тесты: mismatched peer_id, time skew, unsupported version.  
- Runtime переходит в degraded при clock_skew.

**Фактический прогресс:**  
- Реализовано: QUIC listen/dial, hello CBOR, peer_id vs TLS cert check, time skew check.  
- Реализовано: полный набор ошибок, ретраи/backoff, тесты, базовое net.* логирование.

**Подзадачи:**  
- JIRA‑03.1: Полный набор ошибок SPEC‑110 (ERR_HANDSHAKE_TIME_SKEW, ERR_PEER_ID_MISMATCH, ERR_UNSUPPORTED_VERSION, ERR_ADDR_INVALID).  
- JIRA‑03.2: Ретраи/backoff для исходящих соединений (SPEC‑100).  
- JIRA‑03.3: Тесты handshake (time skew, mismatch, unsupported).  
- JIRA‑03.4: Логирование событий net.* (SPEC‑420).

### JIRA‑04: Message Send/Receive (chat.msg.v1)
**Описание:** Реализовать полноценный обмен `chat.msg.v1` с ACK, логированием и состоянием доставки.  
**Статус:** Done  
**DoR:**  
- Envelope pipeline готов.  
- CLI `send` работает.  
**DoD:**  
- Send получает ACK.OK / ACK.DUPLICATE / ACK.REJECTED.  
- Консоль показывает status/error.  
**AC:**  
- Интеграционный тест: send → ACK.OK.  
- Отражение ошибок в CLI.

**Фактический прогресс:**  
- Реализовано: CLI `send --addr --to --text`, ожидание ACK.  
- Реализовано: delivery tracking, логирование статусов, интеграционный тест send→ACK OK.

**Подзадачи:**  
- JIRA‑04.1: Delivery state tracking (sent/acked/failed).  
- JIRA‑04.2: Логирование статусов отправки.  
- JIRA‑04.3: Интеграционные тесты send→ACK OK/REJECTED.

### JIRA‑05: Runtime lifecycle и degraded причины
**Описание:** Реализовать полный жизненный цикл runtime согласно SPEC‑100, включая причины degraded и health.  
**Статус:** Done  
**DoR:**  
- netmgr FSM есть.  
**DoD:**  
- health endpoint `/healthz` (SPEC‑420).  
- degraded причины: `clock_skew`, `transport_errors`, `low_peers`.  
**AC:**  
- Тест: degraded выставляется при clock_skew.  
- `/healthz` отражает статус.

**Фактический прогресс:**  
- Реализовано: FSM, выставление `transport_errors`.  
- Реализовано: `/healthz`, причина `low_peers` + тесты.  
- `clock_skew` отмечено как placeholder, требуются реальные события от handshake (зафиксировано в документации).

**Подзадачи:**  
- JIRA‑05.1: HTTP `/healthz` endpoint (SPEC‑420).  
- JIRA‑05.2: Причины degraded (`clock_skew`, `low_peers`) + метрики/логи.  
- JIRA‑05.3: Тесты degraded переходов.

---

## MVP‑1 (завершение)

### JIRA‑06: Observability v1
**Описание:** JSONL логи, базовые метрики и события NET.  
**Статус:** Done  
**DoR:** SPEC‑420 утверждён.  
**DoD:**  
- JSONL логирование с обязательными полями.  
- Метрики (минимум) доступны локально.  
**AC:**  
- Проверка формата логов на sample run.  

**Подзадачи:**  
- JIRA‑06.1: JSONL логгер (обязательные поля).  
- JIRA‑06.2: Локальный Prometheus endpoint (минимум метрик).  
- JIRA‑06.3: Retention policy (лог/pcap).

### JIRA‑07: CLI/TUI console (минимальный UX)
**Описание:** Консольный UX по SPEC‑400 с индикаторами trust/pow/net.  
**Статус:** Done  
**DoR:** Envelope pipeline готов.  
**DoD:**  
- Индикаторы trust/pow/net работают.  
- Ошибки отображаются пользователю.  
**AC:**  
- Демо‑сценарий: trusted vs untrusted.

**Фактический прогресс:**  
- Реализованы индикаторы trust/pow/net при `send`.  
- CLI показывает ACK и ошибки, добавлен вывод `/healthz` в `peer status`.

**Подзадачи:**  
- JIRA‑07.1: Индикаторы trust/pow/net.  
- JIRA‑07.2: UX ошибок ACK/REJECTED.  
- JIRA‑07.3: Команды статуса (peer status + health).

---

## MVP‑2 (после стабилизации ядра)

### JIRA‑08: Node Graph (CID/dag‑cbor)
**Описание:** Реализация Content Node по SPEC‑200 с проверкой CID/подписи/лимитов.  
**Статус:** Todo  
**DoR:** Envelope pipeline стабилен.  
**DoD:**  
- Структуры Node v1, canonical dag‑cbor.  
- Валидация CID и sig.  
**AC:**  
- Golden tests CID для известного Node.

**Подзадачи:**  
- JIRA‑08.1: Структуры Node v1 + dag‑cbor canonical.  
- JIRA‑08.2: Валидация CID + подписи + лимитов.  
- JIRA‑08.3: Golden tests CID.

### JIRA‑09: Providers + node.fetch.v1
**Описание:** Полная реализация provider hints и fetch по SPEC‑210.  
**Статус:** Todo  
**DoR:** Node Graph готов.  
**DoD:**  
- ProviderRecord, fetch, cache, selection strategy.  
**AC:**  
- Тест: fetch по provider list.

**Подзадачи:**  
- JIRA‑09.1: ProviderRecord + announce payload.  
- JIRA‑09.2: Selection strategy (recent/trusted/parallel).  
- JIRA‑09.3: Fetch cache + errors.

### JIRA‑10: Access Policy & Encryption
**Описание:** Реализовать encrypted nodes по SPEC‑220.  
**Статус:** Todo  
**DoR:** Node Graph готов.  
**DoD:**  
- `enc.node.v1`, recipients sorting, XChaCha20‑Poly1305.  
**AC:**  
- Тест: decrypt success/fail.

**Подзадачи:**  
- JIRA‑10.1: enc.node.v1 структура + recipients сортировка.  
- JIRA‑10.2: XChaCha20‑Poly1305 encrypt/decrypt.  
- JIRA‑10.3: Sealed key для recipients.

---

## MVP‑3 (сервисы/задачи/интеграции)

### JIRA‑11: Service Descriptor + Updates
**Описание:** Реализация service.descriptor.v1 и обновлений через announce (SPEC‑300).  
**Статус:** Todo  
**DoR:** Envelope pipeline + Node Graph.  
**DoD:**  
- Latest trusted descriptor logic.  
**AC:**  
- Тест: обновление descriptor по времени.

**Подзадачи:**  
- JIRA‑11.1: service.descriptor.v1 структура как Node.  
- JIRA‑11.2: Latest trusted descriptor logic.  
- JIRA‑11.3: service.announce.v1 обработка.

### JIRA‑12: Tasks lifecycle
**Описание:** task.request/accept/progress/result/fail/receipt (SPEC‑310).  
**Статус:** Todo  
**DoR:** Service model готов.  
**DoD:**  
- Идемпотентность по client_request_id.  
**AC:**  
- Тесты на повтор request.

**Подзадачи:**  
- JIRA‑12.1: task.request/accept/fail.  
- JIRA‑12.2: task.progress/result/receipt.  
- JIRA‑12.3: Идемпотентность client_request_id.

### JIRA‑13: AI chat profile (Tasks‑based)
**Описание:** ai.chat.v1 профиль поверх Tasks (SPEC‑330).  
**Статус:** Todo  
**DoR:** Tasks lifecycle готов.  
**DoD:**  
- ai.chat.input.v1 + transcript Node.  
**AC:**  
- E2E тест chat request/result.

**Подзадачи:**  
- JIRA‑13.1: ai.chat.input.v1 структура.  
- JIRA‑13.2: transcript Node + safety labels.  
- JIRA‑13.3: E2E тест поверх Tasks.

---

## Инструменты разработки

### JIRA‑14: Packet capture + replay
**Описание:** Реализация capture/replay по SPEC‑430 (off by default).  
**Статус:** Todo  
**DoR:** Envelope pipeline стабилен.  
**DoD:**  
- `run/pcap.jsonl` с owner‑only правами.  
- replay tool с sandbox‑default.  
**AC:**  
- Replay воспроизводит задержки.

**Подзадачи:**  
- JIRA‑14.1: pcap writer (JSONL + owner‑only).  
- JIRA‑14.2: replay tool (sandbox‑only).  
- JIRA‑14.3: CLI flags + docs.

### JIRA‑15: Simulator
**Описание:** Симулятор N peer с соблюдением лимитов и PoW.  
**Статус:** Todo  
**DoR:** Envelope pipeline готов.  
**DoD:**  
- метрики latency/drop/pow reject.  
**AC:**  
- Test run N=5.

**Подзадачи:**  
- JIRA‑15.1: in‑proc N peer runner.  
- JIRA‑15.2: traffic generator (chat/task/fetch).  
- JIRA‑15.3: metrics collector.

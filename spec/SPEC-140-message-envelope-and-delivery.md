# SPEC-140: Сообщения (Envelope) и доставка (Delivery)

**Статус:** Approved v1.0 (2026-02-02)  
**Зависимости:** SPEC-001, SPEC-002, SPEC-010  
**Назначение:** зафиксировать wire-формат сообщения overlay-сети и правила доставки (ack, retry, dedup, TTL).

---

## 1) Wire-формат (CBOR, фиксировано)

В профиле v1 (direct mode) любое overlay-сообщение **ДОЛЖНО** быть Envelope `envelope.v1` в CBOR.

В профиле v2 (privacy-first) прикладные сообщения **ДОЛЖНЫ** передаваться как `envelope.v2` (SPEC-550) **внутри** `garlic.msg.v1` (SPEC-520). Прямой передачи `envelope.v2` по transport нет.

### 1.1 Envelope.v1

Envelope — CBOR map:

* `v` = 1
* `msg_id` (string, UUIDv7)
* `type` (string, например: `demo.msg.v1`, `task.request.v1`)
* `from` (map):
  * `peer_id` (string, обязателен)
  * `identity_id` (string, optional)
* `to` (map, ровно одно поле):
  * `peer_id` (string) или
  * `service_id` (string) или
  * `channel_id` (string)
* `ts_ms` (int64)
* `ttl_ms` (int64)
* `refs` (array, optional) — ссылки на связанные объекты:
  * элементы: map `{ "kind": "msg"|"node"|"task", "id": "<string>" }`
* `pow` (map, optional) — PoW stamp `pow_v1` (см. SPEC-002)
* `payload` (bytes) — CBOR bytes of payload-объекта, зависящего от `type`
* `sig` (bytes, optional) — подпись Identity отправителя

### 1.1b Envelope.v2 (privacy-first)

`envelope.v2` определён в SPEC-550 и является прикладным контейнером без `from.peer_id`. Он используется только внутри туннельной доставки.

### 1.2 Подпись Envelope (фиксировано)

Если `from.identity_id` задан, то `sig` **ДОЛЖЕН** быть присутствующим.

Подписываемые данные:

1. взять Envelope без поля `sig`;
2. сериализовать в canonical CBOR;
3. подписать Ed25519 приватным ключом Identity.

Получатель **ДОЛЖЕН** проверить подпись, если `identity_id` присутствует.

---

## 2) Идентификаторы сообщений

`msg_id` **ДОЛЖЕН** быть UUIDv7 в строковой форме.

Реализация **МОЖЕТ** использовать внешнюю библиотеку UUIDv7 при условии соответствия формату.

---

## 2.1 Размеры сообщений (v1, фиксировано)

* `max_msg_bytes` по умолчанию = **256 KiB**.
* `max_payload_bytes` по умолчанию = **128 KiB**.

---

## 3) TTL и дедупликация (фиксировано)

* Сообщение валидно только в интервале: `ts_ms <= now_ms <= ts_ms + ttl_ms`.
* Узел **ДОЛЖЕН** поддерживать дедуп-таблицу по `msg_id` минимум на `max(ttl_ms, 10 минут)`.
* Повторное получение `msg_id` в окне дедупликации **ДОЛЖНО** приводить к `ACK.DUPLICATE` без повторной обработки payload.

---

## 3.1 Ordering (фиксировано)

Доставка сообщений **НЕ ГАРАНТИРУЕТ** порядок (ordering) ни внутри `channel_id`, ни между любыми двумя сообщениями. Если прикладному протоколу нужен порядок, он **ДОЛЖЕН** реализовывать его явно (sequence numbers + reordering) на уровне payload.

---

## 4) Подтверждения (ACK) и ретраи

### 4.1 ACK тип

ACK — это Envelope с `type = ack.v1`, payload:

* `v` = 1
* `ack_for_msg_id` (string)
* `status` (string: `OK` | `DUPLICATE` | `REJECTED`)
* `error_code` (string, optional)

### 4.2 Правило ретрая отправителя

Если отправитель не получил `ACK.OK|ACK.DUPLICATE` за `ack_timeout_ms` (фиксировано: 1500ms), он **ДОЛЖЕН**:

* повторить отправку до 3 раз;
* затем пометить доставку как `delivery.failed`.

---

## 5) Ошибки (минимум)

Набор кодов ошибок (строки):

* `ERR_TTL_EXPIRED`
* `ERR_DEDUP`
* `ERR_SIG_REQUIRED`
* `ERR_SIG_INVALID`
* `ERR_POW_REQUIRED`
* `ERR_POW_INVALID`
* `ERR_ID_REVOKED`
* `ERR_PAYLOAD_DECODE`
* `ERR_UNSUPPORTED_TYPE`

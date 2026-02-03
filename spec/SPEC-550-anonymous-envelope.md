# SPEC-550: Envelope v2 (анонимная доставка) и идентичности отправителя

**Статус:** Draft v2.0 (2026-02-02)  
**Зависимости:** SPEC-001, SPEC-002, SPEC-010, SPEC-140, SPEC-310, SPEC-520  
**Назначение:** зафиксировать `envelope.v2` — прикладной контейнер сообщений, предназначенный для передачи через туннели без раскрытия `peer_id` отправителя.

---

## 1) Инварианты (фиксировано)

1. `envelope.v2` используется как “внутренний” прикладной контейнер в garlic (SPEC-520).
2. `envelope.v2` **НЕ ДОЛЖЕН** содержать `from.peer_id`.
3. `envelope.v2` **ДОЛЖЕН** быть подписан, если задан `from.identity_id` (аналогично v1).
4. PoW применяется на границах (входящие операции) по SPEC-002; конкретная точка применения для v2:
   * на входе в Directory Service (`dir.query.v1`),
   * на входе в NetDB store операции (SPEC-510),
   * на входе в сервисы, помеченные как untrusted.

---

## 2) Wire-формат `envelope.v2` (CBOR, fixed)

CBOR map:

* `v` = 2
* `msg_id` (string, UUIDv7)
* `type` (string) — например `chat.msg.v1`, `task.request.v1`
* `from` (map):
  * `identity_id` (string, optional) — did:key
  * `service_id` (string, optional) — если отправитель хочет быть адресуемым как сервис (mailbox)
* `to` (map):
  * `service_id` (string) — обязательно
* `ts_ms` (int64)
* `ttl_ms` (int64)
* `reply_to` (map, optional):
  * `service_id` (string) — mailbox для ответов (SPEC-530)
* `refs` (array, optional) — как в v1
* `payload` (bytes) — CBOR bytes payload
* `sig` (bytes, optional) — подпись identity

Правила:

1. `to.service_id` обязателен (в v2 нет прямой доставки на peer/channel).
2. Если `from.identity_id` задан, `sig` обязателен и проверяется по did:key.
3. Если `from.service_id` задан, он **ДОЛЖЕН** соответствовать `from.identity_id` и имени сервиса отправителя (SPEC-001), иначе сообщение отклоняется (`ERR_FROM_SERVICE_MISMATCH`).

---

## 3) Подпись (фиксировано)

Подпись рассчитывается как в SPEC-140:

1. взять объект без поля `sig`;
2. сериализовать canonical CBOR;
3. подписать Ed25519 приватным ключом Identity `from.identity_id`.

---

## 4) Ошибки (минимум)

* `ERR_ENV2_TTL_EXPIRED`
* `ERR_ENV2_SIG_REQUIRED`
* `ERR_ENV2_SIG_INVALID`
* `ERR_FROM_SERVICE_MISMATCH`

---

## 5) Семантика доставки v2 (фиксировано)

1. В v2 **НЕТ** автоматического ACK на уровне сети (в отличие от v1 `ack.v1`): ACK значительно ухудшает приватность (корреляция по времени).
2. Если прикладному протоколу нужна подтверждаемая доставка/результат:
   * он **ДОЛЖЕН** реализовать это явно на уровне payload (например, через Tasks протокол `task.*`, SPEC-310).
3. Получатель `envelope.v2` **ДОЛЖЕН** вести дедупликацию по `msg_id` минимум на `max(ttl_ms, 10 минут)` (аналогично v1, SPEC-140).

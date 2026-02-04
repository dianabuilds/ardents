# SPEC-200: Модель Node Graph (Content Nodes)

**Статус:** Approved v1.0 (2026-02-02)  
**Зависимости:** SPEC-001, SPEC-010  
**Назначение:** зафиксировать структуру Content Node, типы узлов, связи и правила неизменяемости.

---

## 1) Базовые решения

* `node_id` — CIDv1 (`sha2-256`, `dag-cbor`) (см. SPEC-001).
* Node неизменяем.
* Любой Node **ДОЛЖЕН** иметь подпись владельца Identity (см. SPEC-001).

Для защиты от DoS через “валидный, но огромный контент” вводятся фиксированные лимиты v1:

* `MAX_NODE_BYTES = 1_048_576` (1 MiB) — максимальный размер сериализованного Node (dag-cbor bytes).
* `MAX_LINKS_PER_NODE = 256` — максимальное число ссылок в `links`.

---

## 2) Структура Node.v1

Node — CBOR map (в `dag-cbor`), обязательные поля:

* `v` = 1
* `type` (string, формат: `<domain>.<name>.v<major>`)
* `created_at_ms` (int64)
* `owner` (string, `identity_id`)
* `links` (array) — допускается пустой
* `body` (any) — тип зависит от `type`
* `policy` (map) — см. SPEC-220
* `sig` (bytes) — Ed25519 подпись владельца

### 2.1 Link.v1

Элемент `links` — map:

* `rel` (string)
* `node_id` (string, CIDv1)

`rel` **ДОЛЖЕН** быть lowercase ASCII и соответствовать `^[a-z][a-z0-9._-]{0,31}$`.

---

## 3) Типы Node v1 (минимальный набор)

### 3.1 `bundle.addressbook.v1`

`body`:

* `entries` (array of `entry.v1` из SPEC-120)

### 3.2 `service.descriptor.v1`

`body` — см. SPEC-300.

### 3.3 `task.result.v1`

`body` — см. SPEC-310.

### 3.4 `identity.revocation.v1`

`body`:

* `revoked_identity_id` (string)
* `reason` (string)
* `ts_ms` (int64)

---

## 4) Инварианты проверки Node

Получатель Node **ДОЛЖЕН**:

1. проверить лимиты: `len(node_bytes) <= MAX_NODE_BYTES` и `len(links) <= MAX_LINKS_PER_NODE`;
2. проверить, что bytes Node дают именно `node_id` (CID);
3. проверить подпись `sig` по `owner`;
4. проверить `policy` (см. SPEC-220).

Если любой шаг не проходит — Node **ДОЛЖЕН** быть отклонён.

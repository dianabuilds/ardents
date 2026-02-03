# SPEC-300: Модель сервисов и capabilities

**Статус:** Draft v2.0 (2026-02-02)  
**Зависимости:** SPEC-001, SPEC-010, SPEC-140, SPEC-200  
**Назначение:** зафиксировать, как описываются сервисы, как они объявляют возможности и как peer/клиент обнаруживает сервис (v1 direct mode и v2 privacy-first).

---

## 1) Service Descriptor как Content Node

Service descriptor **ДОЛЖЕН** быть Content Node (см. SPEC-200).

* В профиле v1 используется `service.descriptor.v1`.
* В профиле v2 используется `service.descriptor.v2` (без endpoints).

### 1.1 `service.descriptor.v1` (v1 direct mode)

`body`:

* `v` = 1
* `service_name` (string, формат из SPEC-001)
* `service_id` (string, вычисляется из `owner` + `service_name`, см. SPEC-001)
* `endpoints` (array of объектов):
  * `peer_id` (string)
  * `addrs` (array of string, формат из SPEC-110)
  * `priority` (uint, 0..100, больше = лучше)
* `capabilities` (array of объектов `capability.v1`)
* `limits` (map):
  * `max_concurrency` (uint)
  * `max_payload_bytes` (uint)

Descriptor **ДОЛЖЕН** быть подписан владельцем (`owner`).

### 1.2 `service.descriptor.v2` (privacy-first)

В v2 endpoints (peer/addrs) **НЕ ДОЛЖНЫ** быть частью service discovery.

`service.descriptor.v2` — Node типа `service.descriptor.v2` со следующим `body`:

* `v` = 2
* `owner_identity_id` (string, did:key)
* `service_name` (string, формат из SPEC-001)
* `service_id` (string, вычисляется из `owner_identity_id` + `service_name`, SPEC-001)
* `capabilities` (array of объектов `capability.v1`)
* `limits` (map):
  * `max_concurrency` (uint)
  * `max_payload_bytes` (uint)
* `resources` (map):
  * `cpu_cores` (uint)
  * `ram_mb` (uint)

Descriptor v2 **ДОЛЖЕН** быть подписан ключом `owner_identity_id` (см. правила подписи Node в SPEC-200).

---

## 2) Capability.v1 (фиксировано)

Capability — CBOR map:

* `v` = 1
* `job_type` (string, например `ai.chat.v1`, `node.fetch.v1`)
* `modes` (array of string)

---

## 3) Discovery сервисов (v1/v2)

### 3.1 Discovery v1 (фиксировано)

В v1 сервис считается “найденным”, если выполняется одно из условий:

1. `service_id` присутствует в локальной Address Book (alias → service_id); или
2. (deprecated) `service.announce.v1` удалён из текущей реализации; в v2 discovery выполняется через NetDB `service.head.v1`/`service.lease_set.v1`.

Глобального “поиска по capabilities” в v1 **НЕТ**.

### 3.2 Discovery v2 (privacy-first, фиксировано)

В v2:

1. Актуальный descriptor находится через NetDB запись `service.head.v1` (SPEC-510): это источник истины для `descriptor_cid`.
2. Доставка в сервис выполняется через NetDB запись `service.lease_set.v1` + туннели (SPEC-510/520/530).
3. Поиск по capabilities выполняется через Directory Service `dir.query.v1` (SPEC-530) и является прикладным сервисом, доверяемым локально (Address Book).

### 3.3 Хостинг сервисов v1 (фиксировано)

* В v1 допускается **минимальный runtime** сервисов (каркас + интерфейсы без продакшн-логики).
* Встроенный сервис `node.fetch.v1` **ДОЛЖЕН** присутствовать в v1 как базовая capability.

---

## 4) Обновление сервиса (фиксировано)

Service descriptor неизменяем (как Node), поэтому обновляемость достигается повторными анонсами:

* (deprecated) `service.announce.v1` не используется; публикация выполняется через NetDB записи (SPEC-510/530).

Правило “latest trusted descriptor” (фиксировано):

* получатель хранит по каждому `service_id` ровно один “текущий” descriptor;
* новый descriptor **ДОЛЖЕН** приниматься, если:
  1. Node валиден по SPEC-200 (CID+sig);
  2. `body.service_id` совпадает с вычисленным `service_id(owner, service_name)` по SPEC-001;
  3. `created_at_ms` больше, чем у текущего; при равенстве — лексикографически меньший `node_id` как стабилизатор.

Никаких дополнительных head-указателей/каналов обновлений в v1 **НЕТ**: источник истины — последняя валидная подпись владельца, доставленная через анонсы и/или trusted Address Book.

### 4.1 Обновление v2 (privacy-first)

В v2 источником истины являются NetDB записи (SPEC-510/530):

* `service.head.v1` — указывает на текущий `descriptor_cid`
* `service.lease_set.v1` — указывает на текущие leases

Сервис **ДОЛЖЕН** пере-публиковать эти записи не реже, чем раз в `lease_ttl_ms/2` (по умолчанию 5 минут).

---

## 5) Ошибки (минимум)

* `ERR_SERVICE_DESCRIPTOR_INVALID`
* `ERR_SERVICE_ID_MISMATCH`
* `ERR_SERVICE_CAPABILITY_UNSUPPORTED`

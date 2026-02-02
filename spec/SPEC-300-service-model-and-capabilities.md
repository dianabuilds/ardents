# SPEC-300: Модель сервисов и capabilities

**Статус:** Approved v1.0 (2026-02-02)  
**Зависимости:** SPEC-001, SPEC-010, SPEC-140, SPEC-200  
**Назначение:** зафиксировать, как описываются сервисы, как они объявляют возможности и как peer/клиент обнаруживает сервис.

---

## 1) Service Descriptor как Content Node

Service descriptor **ДОЛЖЕН** быть Node типа `service.descriptor.v1` (см. SPEC-200).

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

---

## 2) Capability.v1 (фиксировано)

Capability — CBOR map:

* `v` = 1
* `job_type` (string, например `ai.chat.v1`, `node.fetch.v1`)
* `modes` (array of string)

---

## 3) Discovery сервисов v1 (фиксировано)

В v1 сервис считается “найденным”, если выполняется одно из условий:

1. `service_id` присутствует в локальной Address Book (alias → service_id); или
2. peer получил `service.announce.v1` (см. SPEC-110) и descriptor прошёл проверку подписи.

Глобального “поиска по capabilities” в v1 **НЕТ**.

### 3.2 Хостинг сервисов v1 (фиксировано)

* В v1 допускается **минимальный runtime** сервисов (каркас + интерфейсы без продакшн-логики).
* Встроенный сервис `node.fetch.v1` **ДОЛЖЕН** присутствовать в v1 как базовая capability.

---

## 3.1 Обновление сервиса (критично, фиксировано)

Service descriptor неизменяем (как Node), поэтому обновляемость достигается повторными анонсами:

* peer, который хостит сервис, **ДОЛЖЕН** периодически (не реже 1 раза в 60 секунд) отправлять `service.announce.v1` с `node_id` актуального `service.descriptor.v1`;
* при изменении конфигурации/версии сервиса peer **ДОЛЖЕН** немедленно отправить новый `service.announce.v1`.

Правило “latest trusted descriptor” (фиксировано):

* получатель хранит по каждому `service_id` ровно один “текущий” descriptor;
* новый descriptor **ДОЛЖЕН** приниматься, если:
  1. Node валиден по SPEC-200 (CID+sig);
  2. `body.service_id` совпадает с вычисленным `service_id(owner, service_name)` по SPEC-001;
  3. `created_at_ms` больше, чем у текущего; при равенстве — лексикографически меньший `node_id` как стабилизатор.

Никаких дополнительных head-указателей/каналов обновлений в v1 **НЕТ**: источник истины — последняя валидная подпись владельца, доставленная через анонсы и/или trusted Address Book.

---

## 4) Ошибки (минимум)

* `ERR_SERVICE_DESCRIPTOR_INVALID`
* `ERR_SERVICE_ID_MISMATCH`
* `ERR_SERVICE_CAPABILITY_UNSUPPORTED`

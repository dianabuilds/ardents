# SPEC-120: Address Book и Naming

**Статус:** Approved v1.0 (2026-02-02)  
**Зависимости:** SPEC-001, SPEC-002, SPEC-010  
**Назначение:** зафиксировать локальную адресную книгу: формат, источники, уровни доверия и детерминированные правила разрешения конфликтов.

---

## 1) Scope / Out-of-scope

### Scope

* Локальная адресная книга (файл на диске).
* Импорты/экспорты в виде подписанных бандлов (как Content Nodes).
* Модель доверия: кто считается trusted.

### Out-of-scope

* Глобальный “реестр имён”.
* Автоматический поиск “по имени” по сети.

---

## 2) Формат локальной Address Book (фиксировано)

Файл: `data/addressbook.json` (UTF-8, JSON).

Address Book является частью trust boundary: это локальный источник решений “кому доверять” и “что автоматически принимать/исполнять”.

Корневой объект:

* `v` = 1
* `updated_at_ms` (int64)
* `entries` (array)

### 2.1 Запись `entry.v1`

Каждая запись **ДОЛЖНА** иметь:

* `alias` (string)
* `target_type` (string: `identity` | `peer` | `service` | `node` | `channel`)
* `target_id` (string)
* `source` (string: `self` | `imported`)
* `trust` (string: `trusted` | `untrusted`)
* `note` (string, optional)
* `created_at_ms` (int64)

Опционально:

* `expires_at_ms` (int64, optional) — срок действия записи; рекомендуется для `source=imported`.

### 2.2 Ограничения на `alias`

`alias`:

* **ДОЛЖЕН** быть lowercase.
* **ДОЛЖЕН** соответствовать: `^[a-z0-9][a-z0-9._-]{0,62}[a-z0-9]$` (2..64 символа).

---

## 3) Доверие (фиксировано)

* `trust=trusted` означает: сообщения/объекты, подписанные этой Identity (или принадлежащие Service этой Identity), считаются доверенными для целей:
  * пропуска PoW (см. SPEC-002);
  * принятия service descriptors;
  * импорта address book bundles.
* `trust=untrusted` — все остальные.

`peer_id` **НИКОГДА** не является источником доверия (см. SPEC-001).

---

## 4) Конфликты и разрешение alias (детерминированно)

Если несколько записей имеют одинаковый `alias`, резолвер **ДОЛЖЕН** выбрать ровно одну по порядку:

1. `trust=trusted` предпочтительнее `untrusted`.
2. `source=self` предпочтительнее `imported`.
3. Больше `created_at_ms` предпочтительнее.
4. Лексикографически минимальный `target_id` (как стабилизатор).

Если после правил остаётся неоднозначность (не должно), резолвер **ДОЛЖЕН** вернуть ошибку `ERR_ALIAS_CONFLICT`.

---

## 5) Импорт/экспорт бандлов (фиксировано)

Address book bundle **ДОЛЖЕН** распространяться как Content Node типа `bundle.addressbook.v1` (см. SPEC-200):

* `body.entries` содержит список `entry.v1`;
* Node подписан Identity автора бандла;
* при импорте:
  * принимаются только бандлы, подписанные `trust=trusted` Identity;
  * импортированные записи помечаются `source=imported`.

---

## 6) Ошибки (минимум)

* `ERR_ALIAS_INVALID`
* `ERR_ALIAS_CONFLICT`
* `ERR_ADDRESSBOOK_SCHEMA`
* `ERR_IMPORT_UNTRUSTED_SOURCE`

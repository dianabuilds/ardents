# SPEC-310: Жизненный цикл задач (Tasks) и Receipts

**Статус:** Approved v1.0 (2026-02-02)  
**Зависимости:** SPEC-010, SPEC-140, SPEC-200, SPEC-220, SPEC-300  
**Назначение:** зафиксировать протокол выполнения задач (request/accept/progress/result/fail) и формат receipts.

---

## 1) Идентификаторы задач

`task_id` **ДОЛЖЕН** быть UUIDv7 (строка).

`client_request_id` **ДОЛЖЕН** быть UUIDv7 и использоваться для идемпотентности повторных отправок.

---

## 2) Сообщения Tasks (Envelope types)

### 2.1 `task.request.v1`

Payload:

* `v` = 1
* `task_id` (string)
* `client_request_id` (string)
* `job_type` (string)
* `input` (map) — job-специфичный ввод
* `ts_ms` (int64)

### 2.2 `task.accept.v1`

* `v` = 1
* `task_id` (string)
* `ts_ms` (int64)

### 2.3 `task.progress.v1`

* `v` = 1
* `task_id` (string)
* `pct` (uint, 0..100)
* `note` (string, optional)
* `ts_ms` (int64)

### 2.4 `task.result.v1` (Envelope)

* `v` = 1
* `task_id` (string)
* `result_node_id` (string, CIDv1) — ссылка на Content Node с результатом (см. раздел 3)
* `ts_ms` (int64)

### 2.5 `task.fail.v1`

* `v` = 1
* `task_id` (string)
* `error_code` (string)
* `error_message` (string, optional)
* `ts_ms` (int64)

### 2.6 `task.receipt.v1` (Envelope)

* `v` = 1
* `task_id` (string)
* `metrics` (map):
  * `duration_ms` (int64)
  * `cpu_ms` (int64, optional)
  * `bytes_in` (int64, optional)
  * `bytes_out` (int64, optional)
* `ts_ms` (int64)

---

## 3) Результаты как Content Nodes

Результат задачи **ДОЛЖЕН** быть сохранён как Node типа `task.result.v1` (Content Node).

`body`:

* `v` = 1
* `task_id` (string)
* `job_type` (string)
* `output` (any)

Политика доступа Node **ДОЛЖНА** соответствовать политике задачи/сервиса (см. SPEC-220).

---

## 4) Инварианты

* Сервис **ДОЛЖЕН** отвечать `task.accept` или `task.fail` не позднее 5 секунд после получения `task.request`.
* Сервис **ДОЛЖЕН** быть идемпотентен по `client_request_id` в окне 24 часов.

Идемпотентность и повторы (фиксировано):

* Если сервис получает повторный `task.request.v1` с тем же `client_request_id` и **идентичным** payload, он **ДОЛЖЕН** вернуть текущее состояние задачи (например, повторить `task.accept`, `task.progress`, `task.result` или `task.fail`).
* Если сервис получает `task.request.v1` с тем же `client_request_id`, но **разным** payload, он **ДОЛЖЕН** вернуть `task.fail.v1` с `error_code=ERR_TASK_REJECTED`.
* Если сервис получает `task.request.v1` с уже виденным `task_id`, он **ДОЛЖЕН** трактовать это как попытку конфликта идентичности и вернуть `task.fail.v1` с `error_code=ERR_TASK_REJECTED` (даже если `client_request_id` иной).

---

## 5) Ошибки (минимум)

* `ERR_TASK_UNSUPPORTED`
* `ERR_TASK_REJECTED`
* `ERR_TASK_TIMEOUT`

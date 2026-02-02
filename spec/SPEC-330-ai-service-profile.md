# SPEC-330: Профиль AI сервиса (ai.chat.v1)

**Статус:** Approved v1.0 (2026-02-02)  
**Зависимости:** SPEC-010, SPEC-300, SPEC-310, SPEC-220  
**Назначение:** зафиксировать AI чат как профиль поверх Tasks (единый lifecycle/receipts), плюс требования безопасности (safety).

---

## 1) Service

`service_name` **ДОЛЖНО** быть `ai.chat.v1`.

---

## 2) Выбранный вариант (фиксировано)

AI чат в v1 реализуется **Вариантом A**:

* AI = профиль поверх Tasks;
* запросы делаются через `task.request.v1` с `job_type=ai.chat.v1`;
* результаты возвращаются как `task.result.v1` (Envelope) со ссылкой на Content Node результата;
* receipts — общие по SPEC-310.

Отдельного “специального” AI протокола (с отдельными Envelope types) в v1 **НЕТ**.

---

## 3) Формат запроса (task.request.v1)

Используется `task.request.v1` из SPEC-310:

* `job_type` **ДОЛЖЕН** быть `ai.chat.v1`;
* `input` **ДОЛЖЕН** иметь структуру `ai.chat.input.v1` (см. ниже).

### 3.1 `ai.chat.input.v1`

`input` — map:

* `v` = 1
* `messages` (array):
  * элементы: map `{ "role": "system"|"user"|"assistant", "content": "<string>" }`
* `params` (map, optional):
  * `temperature` (float, optional)
  * `max_output_tokens` (uint, optional)
* `policy` (map):
  * `visibility` (`public` | `encrypted`)
  * `recipients` (array of `identity_id`, обязателен если `encrypted`)

`policy` — как в SPEC-220.

---

## 4) Формат результата (task.result.v1 + Content Node)

Сервис **ДОЛЖЕН** вернуть `task.result.v1` (Envelope) из SPEC-310, где:

* `result_node_id` указывает на Content Node типа `ai.chat.transcript.v1`.

### 4.1 Node `ai.chat.transcript.v1`

Node `type = ai.chat.transcript.v1`, `body`:

* `v` = 1
* `task_id` (string)
* `messages` (array) — итоговая переписка (как во входе)
* `safety` (map):
  * `v` = 1
  * `labels` (array of string)

Политика доступа Node **ДОЛЖНА** соответствовать `input.policy` из запроса.

---

## 5) Safety (фиксировано)

Сервис **ДОЛЖЕН**:

* возвращать `safety.labels` всегда (даже пустой список);
* не выполнять “инструменты” и не запускать код по умолчанию (любая tool-автоматизация требует отдельной SPEC).

---

## 6) Ошибки (минимум)

* `ERR_AI_POLICY_INVALID`
* `ERR_AI_INPUT_TOO_LARGE`
* `ERR_AI_REFUSED`

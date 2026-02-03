# SPEC-340: Web service profile (web.request.v1)

**Статус:** Draft v0.1 (2026-02-03)  
**Зависимости:** SPEC-010, SPEC-300, SPEC-310, SPEC-220, SPEC-320  
**Назначение:** зафиксировать минимальный профиль веб?сервиса поверх Tasks для интеграции внешних сайтов/приложений через локальный IPC?адаптер.

---

## 1) Service

`service_name` **ДОЛЖНО** быть `web.request.v1`.

---

## 2) Выбранный вариант (фиксировано)

Профиль `web.request.v1` реализуется **вариантом A**:

* запросы приходят как `task.request.v1` с `job_type=web.request.v1`;
* результат возвращается как `task.result.v1` со ссылкой на Content Node результата;
* receipts — общие по SPEC-310.

---

## 3) Формат запроса (task.request.v1)

Используется `task.request.v1` из SPEC-310:

* `job_type` **ДОЛЖЕН** быть `web.request.v1`;
* `input` **ДОЛЖЕН** иметь структуру `web.request.input.v1`.

### 3.1 `web.request.input.v1`

`input` — map:

* `v` = 1
* `method` (string, optional, default `GET`)
* `path` (string) — относительный путь (без scheme/host), допускается query
* `headers` (map<string,string>, optional)
* `body` (bytes, optional)
* `policy` (map, optional) — как в SPEC-220

Ограничения (фиксировано):

* `path` **НЕ ДОЛЖЕН** содержать scheme/host;
* тело запроса **ДОЛЖНО** укладываться в лимиты Content Node.

---

## 4) Формат результата (task.result.v1 + Content Node)

Сервис **ДОЛЖЕН** вернуть `task.result.v1` (Envelope) из SPEC-310, где:

* `result_node_id` указывает на Content Node типа `web.response.v1`.

### 4.1 Node `web.response.v1`

Node `type = web.response.v1`, `body`:

* `v` = 1
* `task_id` (string)
* `status` (uint)
* `headers` (map<string,string>, optional)
* `body` (bytes, optional)

`policy` Node **ДОЛЖНА** соответствовать `input.policy` из запроса. Если `input.policy` не задан — допускается `visibility=public`.

---

## 5) Безопасность

* Интеграция выполняется **ТОЛЬКО** через локальный IPC (SPEC-320).
* Запросы **НЕ ДОЛЖНЫ** выполнять абсолютные URL или произвольный SSRF.
* Upstream base URL MUST be loopback (localhost/127.0.0.1/::1).
* Hop?by?hop headers **ДОЛЖНЫ** игнорироваться.

---

## 6) Ошибки (минимум)

* `ERR_WEB_INPUT_INVALID`
* `ERR_WEB_INPUT_TOO_LARGE`
* `ERR_WEB_UPSTREAM_FAILED`
* `ERR_WEB_RESPONSE_INVALID`
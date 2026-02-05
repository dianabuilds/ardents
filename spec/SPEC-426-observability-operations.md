# SPEC-426: Наблюдаемость и эксплуатация (как устроено и как смотреть)

**Статус:** Draft v0.1 (2026-02-05)  
**Зависимости:** SPEC-010, SPEC-420, TECH-050  
**Назначение:** зафиксировать, как устроена наблюдаемость узла, где смотреть логи/метрики/health и как их интерпретировать.

---

## 1) Состав наблюдаемости

Узел **ДОЛЖЕН** предоставлять:

* **Логи** (JSON Lines)
* **Метрики** (Prometheus text format)
* **Health endpoint** (`/healthz`)
* **Support bundle** (по явной команде)

Packet capture (pcap) **МОЖЕТ** быть включён и считается чувствительным артефактом.

---

## 2) Логи

### 2.1 Формат и поля

Формат логов — JSON Lines (один JSON-объект на строку). Обязательные поля:

* `ts_ms` (int64, UTC)
* `level` (`debug`|`info`|`warn`|`error`)
* `component` (string)
* `event` (string)
* `peer_id` (optional)
* `msg_id` (optional)
* `error_code` (optional)

### 2.2 Куда пишутся

По умолчанию — stdout. Опционально: локальный файл (`observability.log_file`).

### 2.3 Безопасность логов

Логи **НЕ ДОЛЖНЫ** содержать приватные ключи, токены, plaintext приватных payloads и другие секреты (см. SPEC-420, TECH-000).

---

## 3) Метрики

### 3.1 Где смотреть

* Endpoint: `http://127.0.0.1:9090/` (loopback-only)
* Формат: Prometheus text format

### 3.2 Минимальные метрики

* `net_inbound_conns`, `net_outbound_conns`, `peers_connected`
* `msg_received_total{type}`, `msg_rejected_total{reason}`
* `pow_required_total`, `pow_invalid_total`, `clock_invalid_total`
* `task_request_total{job_type}`, `task_result_total{job_type}`
* `task_fail_total{code}`, `task_timeout_total`
* `ipc_errors_total{code}`, `ipc_timeout_total`
* `ack_latency_ms_bucket`, `ack_latency_ms_sum`, `ack_latency_ms_count`

### 3.3 Интерпретация

* `peers_connected=0` + `net.degraded` в логах = нет подключений (ожидаемо в локальном стенде).
* `task_fail_total{code}` и `ipc_errors_total{code}` — ключевые сигналы ошибок интеграций.
* `ack_latency_ms_bucket` — база для p50/p95/p99 на стороне мониторинга.

---

## 4) Health

### 4.1 Где смотреть

* Endpoint: `http://127.0.0.1:8081/healthz`
* Ответ JSON:
  * `status` (`ok`|`degraded`|`stopped`)
  * `peers_connected` (uint)

### 4.2 Интерпретация

* `status=ok` — сеть в online.
* `status=degraded` — есть причины деградации (см. логи `net.degraded`).

---

## 5) Support bundle

Команда: `peer support bundle`.

**Включает:** метаданные, `config`, `run/status.json`, лог tail (если включён), pcap метаданные.  
**Не включает:** токены, приватные ключи.

---

## 6) Как смотреть в Docker

* Health:
  * внутри контейнера: `wget -qO- http://127.0.0.1:8081/healthz`
* Метрики:
  * внутри контейнера: `wget -qO- http://127.0.0.1:9090 | head -n 20`

---

## 7) Минимальные операционные практики

* Health/metrics только на loopback.
* Логи не содержат секретов.
* IPC токен и ключи — owner-only.
* Любые изменения форматов логов/метрик **ДОЛЖНЫ** отражаться в SPEC-420 и TECH-050.

---

## 8) Совместимость

SPEC-426 не вводит новых wire-форматов и не меняет протоколы. Это операционная спецификация.

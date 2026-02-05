# OBS-001: Минимальные алерты и дашборды

**Статус:** Draft (2026-02-05)  
**Связано:** SPEC-420, SPEC-426, TECH-050  
**Назначение:** зафиксировать минимальный набор алертов и графиков для прод-наблюдаемости.

---

## 1) Алерты (минимум)

### 1.1 IPC/Tasks ошибки

**ALERT: ipc_errors_rate**  
**Условие:** `rate(ipc_errors_total[5m]) > 0`  
**Сигнал:** ошибки IPC.

**ALERT: task_fail_rate**  
**Условие:** `rate(task_fail_total[5m]) > 0`  
**Сигнал:** ошибки задач.

**ALERT: task_timeout_rate**  
**Условие:** `rate(task_timeout_total[5m]) > 0`  
**Сигнал:** таймауты задач.

### 1.2 Сеть

**ALERT: peers_zero**  
**Условие:** `peers_connected == 0` (дольше 5 минут)  
**Сигнал:** нет подключенных пиров.

**ALERT: msg_rejected_spike**  
**Условие:** `rate(msg_rejected_total[5m]) > 0` + рост относительно baseline  
**Сигнал:** рост отказов/abuse.

### 1.3 Clock/PoW

**ALERT: clock_invalid**  
**Условие:** `clock_invalid_total > 0`  
**Сигнал:** системные проблемы времени.

**ALERT: pow_invalid_spike**  
**Условие:** `rate(pow_invalid_total[5m]) > 0`  
**Сигнал:** некорректные PoW или злоупотребление.

---

## 2) Дашборды (минимум)

### 2.1 Сеть

* `net_inbound_conns`, `net_outbound_conns`
* `peers_connected`
* `msg_received_total{type}`, `msg_rejected_total{reason}`

### 2.2 Tasks/IPC

* `task_request_total{job_type}`
* `task_result_total{job_type}`
* `task_fail_total{code}`
* `task_timeout_total`
* `ipc_errors_total{code}`
* `ipc_timeout_total`

### 2.3 Latency

* `ack_latency_ms_bucket` (p50/p95/p99 на стороне мониторинга)

### 2.4 Clock/PoW

* `clock_invalid_total`
* `pow_required_total`, `pow_invalid_total`

---

## 3) Практика

* Алерты должны быть **actionable**, без шумных low-signal.
* Пороговые значения фиксируются оператором (зависят от масштаба сети).
* Для локального стенда допустим `peers_connected=0` без алерта.

---

## 4) Примеры (Prometheus)

```
# peers zero (5m)
min_over_time(peers_connected[5m]) == 0

# task fail rate
rate(task_fail_total[5m]) > 0

# ipc errors rate
rate(ipc_errors_total[5m]) > 0
```

# TECH-030: Динамическое тестирование системы (sim)

**Статус:** Done (2026-02-03)  
**Цель:** прогнать динамические сценарии через `cmd/sim` и зафиксировать результаты.

---

## 1) Окружение

- Go: 1.25.6
- Инструмент: `cmd/sim`

---

## 2) Сценарии и результаты

### Сценарий A — базовый (без потерь/ошибок PoW)

Команда:

```
go run ./cmd/sim -n 5 -duration 5s -rate 20 -seed 1 -drop-rate 0 -pow-invalid-rate 0
```

Результат (2026-02-03):

```
{
  "ack_ok": 100,
  "ack_rejected": 0,
  "ack_rejected_by": {},
  "delivered": 100,
  "drop_rate": 0,
  "dropped": 0,
  "latency_avg_ms": 1,
  "latency_p95_ms": 1,
  "pow_invalid": 0,
  "pow_reject_rate": 0,
  "pow_required": 0,
  "sent": 100,
  "traffic_by_type": {
    "node.fetch.v1": 13,
    "task.request.v1": 87
  }
}
```

### Сценарий B — потери 20%

Команда:

```
go run ./cmd/sim -n 5 -duration 5s -rate 20 -seed 2 -drop-rate 0.2 -pow-invalid-rate 0
```

Результат (2026-02-03):

```
{
  "ack_ok": 79,
  "ack_rejected": 0,
  "ack_rejected_by": {},
  "delivered": 79,
  "drop_rate": 0.21,
  "dropped": 21,
  "latency_avg_ms": 0,
  "latency_p95_ms": 0,
  "pow_invalid": 0,
  "pow_reject_rate": 0,
  "pow_required": 0,
  "sent": 100,
  "traffic_by_type": {
    "node.fetch.v1": 14,
    "task.request.v1": 86
  }
}
```

### Сценарий C — инъекция ошибок PoW (30%)

Команда:

```
go run ./cmd/sim -n 5 -duration 5s -rate 20 -seed 3 -drop-rate 0 -pow-invalid-rate 0.3
```

Результат (2026-02-03):

```
{
  "ack_ok": 71,
  "ack_rejected": 29,
  "ack_rejected_by": {
    "ERR_POW_INVALID": 16,
    "ERR_POW_REQUIRED": 13
  },
  "delivered": 100,
  "drop_rate": 0,
  "dropped": 0,
  "latency_avg_ms": 0,
  "latency_p95_ms": 0,
  "pow_invalid": 16,
  "pow_reject_rate": 0.29,
  "pow_required": 13,
  "sent": 100,
  "traffic_by_type": {
    "node.fetch.v1": 12,
    "task.request.v1": 88
  }
}
```

---

## 3) Выводы

- Базовый сценарий проходит без отказов (ACK.OK=100%).
- При `drop-rate=0.2` число доставок соответствует ожидаемым потерям.
- Инъекции PoW‑ошибок корректно переводятся в `ERR_POW_REQUIRED/ERR_POW_INVALID`.


---

## 4) V2 suite (privacy-first)

### Сценарий D — v2 dynamic suite

Команда:

```
go run ./cmd/sim -profile v2 -n 10 -seed 1
```

Ожидаемый выход:

```
{
  "checks": {
    "reseed_quorum": { "ok": true },
    "netdb_poisoning_reject": { "ok": true },
    "netdb_wire": { "ok": true },
    "dirquery_e2e": { "ok": true },
    "tunnel_rotate_padding": { "ok": true }
  },
  "latency_p95_ms": 0,
  "duration_ms": 0
}
```

При ошибке статус `ok=false` и указывается `error`.

Фактический прогон (2026-02-03):

```
{
  "checks": {
    "dirquery_e2e": {
      "ok": true
    },
    "netdb_poisoning_reject": {
      "ok": true
    },
    "netdb_wire": {
      "ok": true
    },
    "reseed_quorum": {
      "ok": true
    },
    "tunnel_rotate_padding": {
      "ok": true
    }
  },
  "latency_p95_ms": 1,
  "duration_ms": 53
}
```

---

## 5) CI checks (JIRA-32)

Единая команда для CI/локального прогона:

```
# Windows (PowerShell)
./scripts/ci/check.ps1

# Linux/macOS
./scripts/ci/check.sh
```

Параметры симуляции можно переопределить переменными среды:

```
SIM_PEERS=5 SIM_DURATION_SEC=5 SIM_RATE=20 ./scripts/ci/check.sh
```

Последний прогон `check.ps1`: 2026-02-03.

---

## 6) Нагрузочные и стабильностные тесты (JIRA-33)

### Сценарий E — load (N=50)

Команда:

```
./scripts/load/load.ps1
```

Результат (2026-02-03):

```
{
  "ack_ok": 489,
  "ack_rejected": 0,
  "ack_rejected_by": {},
  "delivered": 489,
  "drop_rate": 0,
  "dropped": 0,
  "latency_avg_ms": 1,
  "latency_p95_ms": 1,
  "pow_invalid": 0,
  "pow_reject_rate": 0,
  "pow_required": 0,
  "sent": 489,
  "traffic_by_type": {
    "node.fetch.v1": 77,
    "task.request.v1": 412
  }
}
```

---

## 7) Tunnel tests (JIRA-22)

### Test: 3-hop delivery + replay protection

* `internal/core/app/runtime/tunnel_integration_test.go`
* `internal/core/app/runtime/tunnel_replay_test.go`

Ожидаемое поведение:

* `tunnel.data.v1` проходит 3 hops (forward count = 3).
* replay seq не обрабатывается повторно (лог `ERR_TUNNEL_DATA_REPLAY`).

### Сценарий F — soak (30s)

Команда:

```
./scripts/load/soak.ps1
```

Результат (2026-02-03):

```
{
  "ack_ok": 300,
  "ack_rejected": 0,
  "ack_rejected_by": {},
  "delivered": 300,
  "drop_rate": 0,
  "dropped": 0,
  "latency_avg_ms": 1,
  "latency_p95_ms": 1,
  "pow_invalid": 0,
  "pow_reject_rate": 0,
  "pow_required": 0,
  "sent": 300,
  "traffic_by_type": {
    "node.fetch.v1": 41,
    "task.request.v1": 259
  }
}
```

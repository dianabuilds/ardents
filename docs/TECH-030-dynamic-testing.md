# TECH-030: Динамическое тестирование системы (sim)

**Статус:** Done (2026-02-02)  
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

Результат:

```
{
  "ack_ok": 100,
  "ack_rejected": 0,
  "ack_rejected_by": {},
  "delivered": 100,
  "drop_rate": 0,
  "dropped": 0,
  "latency_avg_ms": 0,
  "latency_p95_ms": 0,
  "pow_invalid": 0,
  "pow_reject_rate": 0,
  "pow_required": 0,
  "sent": 100,
  "traffic_by_type": {
    "chat.msg.v1": 54,
    "node.fetch.v1": 16,
    "task.request.v1": 30
  }
}
```

### Сценарий B — потери 20%

Команда:

```
go run ./cmd/sim -n 5 -duration 5s -rate 20 -seed 2 -drop-rate 0.2 -pow-invalid-rate 0
```

Результат:

```
{
  "ack_ok": 79,
  "ack_rejected": 0,
  "ack_rejected_by": {},
  "delivered": 79,
  "drop_rate": 0.21,
  "dropped": 21,
  "latency_avg_ms": 1,
  "latency_p95_ms": 1,
  "pow_invalid": 0,
  "pow_reject_rate": 0,
  "pow_required": 0,
  "sent": 100,
  "traffic_by_type": {
    "chat.msg.v1": 51,
    "node.fetch.v1": 17,
    "task.request.v1": 32
  }
}
```

### Сценарий C — инъекция ошибок PoW (30%)

Команда:

```
go run ./cmd/sim -n 5 -duration 5s -rate 20 -seed 3 -drop-rate 0 -pow-invalid-rate 0.3
```

Результат:

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
    "chat.msg.v1": 58,
    "node.fetch.v1": 16,
    "task.request.v1": 26
  }
}
```

---

## 3) Выводы

- Базовый сценарий проходит без отказов (ACK.OK=100%).
- При `drop-rate=0.2` число доставок соответствует ожидаемым потерям.
- Инъекции PoW‑ошибок корректно переводятся в `ERR_POW_REQUIRED/ERR_POW_INVALID`.


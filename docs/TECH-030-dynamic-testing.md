# TECH-030: Динамическое тестирование системы (sim)

**Статус:** Draft (2026-02-05)  
**Цель:** описать, как запускать динамические сценарии через `cmd/sim` и что проверять.

---

## 1) Окружение

* Go: 1.25.x
* Инструмент: `cmd/sim`

---

## 2) Минимальные сценарии

### 2.1 Базовый (без потерь/ошибок PoW)

Команда:

```
go run ./cmd/sim -n 5 -duration 5s -rate 20 -seed 1 -drop-rate 0 -pow-invalid-rate 0
```

**Проверить:**
* `ack_rejected` = 0
* `pow_invalid` = 0

### 2.2 Потери 20%

Команда:

```
go run ./cmd/sim -n 5 -duration 5s -rate 20 -seed 2 -drop-rate 0.2 -pow-invalid-rate 0
```

**Проверить:**
* `drop_rate` близок к 0.2
* `ack_rejected` = 0

### 2.3 Инъекция ошибок PoW (30%)

Команда:

```
go run ./cmd/sim -n 5 -duration 5s -rate 20 -seed 3 -drop-rate 0 -pow-invalid-rate 0.3
```

**Проверить:**
* `ack_rejected_by` содержит `ERR_POW_REQUIRED` и/или `ERR_POW_INVALID`

---

## 3) V2 suite (privacy-first)

Команда:

```
go run ./cmd/sim -profile v2 -n 10 -seed 1
```

**Проверить:**
* все `checks.*.ok == true`

---

## 4) CI / локальный прогон

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

---

## 5) Нагрузочные и стабильностные тесты

Нагрузочные сценарии описаны в `docs/LOAD-001-load-profiles.md`.

---

## 6) Замечания

* Результаты прогонов фиксируются в отдельных отчётах, не в TECH-документе.

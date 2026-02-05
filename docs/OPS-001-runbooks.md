# OPS-001: Runbooks (prod readiness)

**Статус:** Draft (2026-02-05)  
**Связано:** TECH-050, SPEC-425, SPEC-500  
**Назначение:** зафиксировать минимальные операционные runbooks для прод-эксплуатации.

---

## 1) Bootstrap / Первый запуск

**Цель:** запустить новый узел с чистого состояния.

### Шаги

1. Инициализация:
   * `peer init --home <path>`
2. Настройка конфигурации:
   * `config/node.json` (listen/advertise/limits/observability)
3. Запуск:
   * `peer start --home <path>`
4. Проверка:
   * `peer status --home <path>`
   * `/healthz` должен быть `ok` или `degraded` с понятной причиной

---

## 2) Reseed / Bootstrap обновление

**Цель:** восстановить сеть при отсутствии bootstrap peers.

### Шаги

1. Убедиться, что `reseed.enabled=true` в конфиге.
2. Указать `reseed.urls` и `reseed.authorities`.
3. Перезапустить узел.
4. В логах проверить `reseed.fetch.start` и `reseed.apply.ok`.

---

## 3) Incident Response (узел деградирует)

**Цель:** выявить причину деградации и восстановить работу.

### Чек-лист

* `peer status` → статус и `peers_connected`.
* Логи: `net.degraded`, `net.clock_invalid`, `net.bootstrap_failed`.
* Метрики: `msg_rejected_total`, `task_fail_total`, `ipc_errors_total`.
* Проверить доступность bootstrap peers.
* При необходимости — выполнить reseed.

---

## 4) Rollback / Откат версии

**Цель:** быстро вернуться к предыдущей версии.

### Шаги

1. Остановить сервис.
2. Восстановить предыдущий бинарник.
3. Восстановить backup (если миграции изменяли данные).
4. Запустить узел, проверить `/healthz` и `peer status`.

---

## 5) Обновление

**Цель:** обновить бинарник и применить миграции.

### Шаги

1. Остановить сервис.
2. Backup (см. TECH-050).
3. Обновить бинарник.
4. Запустить миграции:
   * `peer migrate --home <path>`
5. Запустить сервис.
6. Проверить `/healthz`.

---

## 6) Backup / Restore

**Цель:** сохранить/восстановить критичные данные.

### Что бэкапить

* `config/node.json`
* `data/identity/identity.key`
* `data/addressbook.json`
* `data/keys/peer.key`, `data/keys/peer.crt`
* `data/lkeys/` (если v2 сервисы)

### Восстановление

1. Восстановить файлы в тот же `ARDENTS_HOME`.
2. Запустить `peer`.
3. Проверить `/healthz`.

---

## 7) Поддержка

**Support bundle:**

* `peer support bundle`
* Не включает секреты/ключи.
* Использовать для передачи в поддержку.

---

## 8) Минимальные требования перед продом

* Health/metrics только loopback.
* IPC токен и ключи owner-only.
* Логи без секретов.
* Наличие алертов и load-профилей.

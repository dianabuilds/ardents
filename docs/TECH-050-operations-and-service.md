# TECH-050: Эксплуатация, директории и запуск (v1)

**Статус:** Draft (2026-02-02)  
**Назначение:** инструкция по запуску и эксплуатации узла (headless-first).

---

## 1) Целевой режим v1

* Основной режим: **сервер/узел (headless-first)**.
* Основной интерфейс: CLI + конфиги, минимум интерактива.
* Автозапуск после reboot: **не по умолчанию**, только по явному включению.
* Gateway: **выключен по умолчанию** (отдельный процесс, включение — явно).
* Логи: **stdout/journald по умолчанию** (JSONL). Запись в файл — опционально.

---

## 2) Директории и раскладка

### 2.1 Linux (XDG)

* Config: `${XDG_CONFIG_HOME:-~/.config}/ardents/`
* Data: `${XDG_DATA_HOME:-~/.local/share}/ardents/`
* State: `${XDG_STATE_HOME:-~/.local/state}/ardents/`
* Run: `${XDG_STATE_HOME:-~/.local/state}/ardents/run/`

### 2.2 Portable mode

Если задан `ARDENTS_HOME` или флаг `--home`:

* `<home>/config/`
* `<home>/data/`
* `<home>/run/`

### 2.3 Файлы v1

* config: `config/node.json`
* client config: `config/client.json`
* address book: `data/addressbook.json`
* identity: `data/identity/identity.key`
* transport keys: `data/keys/peer.key`, `data/keys/peer.crt`
* status: `run/status.json`
* gateway token: `run/peer.token`
* packet capture: `run/pcap.jsonl`

---

## 3) Запуск и управление (CLI)

### 3.1 Инициализация

* `peer init` — создаёт директории, config (если нет), identity, transport keys, address book.

Пример (portable):

```
peer init --home ./ardents-home
```

**Client config (для режима без `addr`):**  
создаётся отдельным шагом, вручную или утилитой (см. SPEC-460).  
Файл: `config/client.json`.

Шаблон: `docs/examples/client.json`.

### 3.2 Запуск

* `peer start` — запускает узел, пишет status в `run/status.json`, логи в stdout.
* `peer status` — показывает status и `/healthz` (если включён).

Пример (portable):

```
peer start --home ./ardents-home
peer status --home ./ardents-home
```

Опционально (локальный файл логов):

```
peer start --log.file log.jsonl
peer start --log.format json|text
```

### 3.3 Остановка

* Остановка процесса (signal/systemd).
* После остановки должен обновиться `run/status.json`.

---

## 3.4 Единая пользовательская строка (UFA)

Клиентские утилиты принимают **одну строку UFA** вместо набора `peer_id/service_id/owner_id`.  
Формат и порядок резолвинга определены в SPEC-415.

**Поддерживаемые варианты UFA:**

* alias из `data/addressbook.json`
* `node_id` (CIDv1, base32 lower)
* `service_id` (`svc_...`)
* `identity_id` (`did:key:...`) — преобразуется в `service_id` по `service_name`

**Примеры:**

```
webclient request --addr <host:port> --target <identity_id>
webclient request --addr <host:port> --target <service_id>
node get --target <alias>
node get --target <node_id>
```

Режим без явного `--addr` (требует `config/client.json`, см. SPEC-460):

```
webclient request --target <identity_id> --fetch-result
```

Если UFA не распознана — ошибка `ERR_UFA_UNSUPPORTED`.  
Если UFA не соответствует контексту клиента (например, `service_id` в Node Browser) — `ERR_UFA_TYPE_MISMATCH`.

---

## 3.5 Обнаружение узлов и сервисов: как сейчас и чего нет

### Что есть сейчас (v1)

1) **Bootstrap/Reseed (SPEC-500)**  
   Узел получает начальные адреса через reseed‑bundle от доверенных DA. Это старт сети без ручного ввода списка пиров.

1) **NetDB (SPEC-510)**  
   После входа узлы публикуют `router.info` и сервисные записи (`service.head.v1`, `service.lease_set.v1`).  
   Дальше доступность строится на TTL/обновлениях, без статических IP.

1) **Directory Service (SPEC-530)**  
   Поиск сервисов по возможностям/capabilities через `dir.query.v1`.  
   **По умолчанию внешние каталоги отключены** и используются только при явном включении локальной конфигурацией.

1) **Address Book bundles (SPEC-125/120)**  
   Локальные доверенные списки alias → target.  
   Не являются глобальным поиском; это trust‑политика и удобство.

### Чего нет (осознанно)

* **Глобального регистра доменных имён** с уникальностью “по всей сети”.  
* **Автоматического пополнения address book** из сети без trust‑политики.  
* **Поиска по узлам “вслепую”** без service_id/descriptor/NetDB.

### Практика в проде

* Ручной IP нужен только для старта (reseed).  
* Дальше сеть работает через NetDB/Directory и TTL‑обновления.

---

## 4) Инструкция по запуску на сервере (Linux)

### 4.1 Подготовка сервера

1) Создать системного пользователя и каталог данных:

    ```
    sudo useradd -r -m -d /var/lib/ardents -s /usr/sbin/nologin ardents
    sudo mkdir -p /var/lib/ardents
    sudo chown -R ardents:ardents /var/lib/ardents
    ```

1) Открыть UDP порт для QUIC (пример для 3840/udp):

    ```
    sudo ufw allow 3840/udp
    ```

1) Убедиться, что health/metrics доступны только локально
(по умолчанию `127.0.0.1` в конфиге).

### 4.2 Инициализация данных

```
sudo -u ardents peer init --home /var/lib/ardents
```

Проверить наличие файлов:

* `/var/lib/ardents/config/node.json`
* `/var/lib/ardents/data/identity/identity.key`
* `/var/lib/ardents/data/keys/peer.key`, `/var/lib/ardents/data/keys/peer.crt`
* `/var/lib/ardents/data/addressbook.json`

### 4.3 Настройка конфигов

Файл: `/var/lib/ardents/config/node.json`

Минимально проверить/настроить:

* `listen.quic_addr` — адрес/порт для входящих QUIC (например `"0.0.0.0:3840"`).
* `advertise.quic_addrs` — **публичные** адреса, которые peer публикует в NetDB (`router.info.v1`).
  Может отличаться от `listen.quic_addr` (NAT, Docker port mapping).
* `limits.*` — лимиты входящих/исходящих соединений, размер сообщений.
* `limits.handshake_rate_limit` и `limits.handshake_rate_window_ms` — ограничение частоты handshake (защита от всплесков).
* `observability.health_addr` и `observability.metrics_addr` — оставить `127.0.0.1:*`
  или включить reverse-proxy с ACL.

### 4.4 Запуск и проверка

```
sudo -u ardents peer start --home /var/lib/ardents
sudo -u ardents peer status --home /var/lib/ardents
```

`healthz` должен вернуть `status: ok|degraded`.

---

## 5) Автоматизация запуска

### 5.1 systemd (рекомендуется)

Сгенерировать unit:

```
peer systemd unit --mode=system --home /var/lib/ardents
```

Установить unit:

```
sudo peer install-service --mode=system --home /var/lib/ardents
sudo systemctl enable --now ardents.service
```

Проверка:

```
systemctl status ardents.service
```

### 5.2 Скрипт запуска

Пример wrapper:

```
#!/usr/bin/env sh
set -eu
export ARDENTS_HOME=/var/lib/ardents
exec /usr/local/bin/peer start --home "$ARDENTS_HOME"
```

---

## 6) Gateway (опционально)

По умолчанию gateway не запускается.

Чтобы включить:

* `peer start --enable-gateway`
* затем `gateway --home <тот же home>` (или `gateway --token <path>`)

---

## 7) Support bundle (диагностика без секретов)

Команда:

* `peer support bundle`

Включает:

* метаданные (OS/arch/go version/build info)
* снимок путей (`meta/paths.json`)
* `config/node.json`
* `run/status.json` (если есть)
* tail файла логов (если включён `observability.log_file`)
* метаданные packet capture (`run/pcap.meta.json`), если существует `run/pcap.jsonl`
* `data/addressbook.meta.json` по умолчанию, или полный `data/addressbook.json` по флагу `--include-addressbook`

Не включается:

* `run/peer.token`
* приватные ключи identity и transport keys (`data/identity/*`, `data/keys/*`)

---

## 8) Обновление и откат

### 8.1 Обновление

1) Остановить сервис.
1) Сделать backup (см. раздел 9).
1) Обновить бинарник `peer` и связанные утилиты.
1) Запустить сервис.
1) Проверить `peer status` и `/healthz`.

### 8.2 Откат

1) Остановить сервис.
1) Восстановить предыдущий бинарник `peer`.
1) Восстановить backup (если менялись данные).
1) Запустить сервис и проверить `/healthz`.

---

## 9) Backup / Restore

### 9.1 Что бэкапить

* `config/node.json`
* `data/identity/identity.key`
* `data/addressbook.json`
* `data/keys/peer.key`, `data/keys/peer.crt`
* `data/lkeys/` (если используются v2 сервисы)

### 9.2 Восстановление

1) Восстановить те же пути под `ARDENTS_HOME` или XDG.
1) Вернуть файлы из backup.
1) Запустить `peer` и проверить `/healthz`.

---

## 10) Наблюдаемость (минимум)

### 10.1 Логи

* Формат: JSON Lines.
* Ошибки на внешних границах — `ERR_*`.
* Логи не должны содержать секретов.

### 10.2 Метрики (минимум)

* Состояние сети: `net_inbound_conns`, `net_outbound_conns`, `peers_connected`.
* Ошибки IPC/Tasks: `ipc_errors_total`, `task_fail_total{code}`.
* Таймауты: `task_timeout_total`, `ipc_timeout_total`.
* Латентность: `ack_latency_ms_bucket` (p50/p95 считаются мониторингом).

---

## 11) Безопасность (минимум)

* IPC только локальный, токен обязателен.
* Файлы с ключами и токенами — owner-only.
* Gateway включается только явно.

---

## 12) Docker: локальный стенд для интеграций

Файлы:

* `Dockerfile`
* `docker/docker-compose.yml`
* `docker/ardents-home/config/node.json`
* `docker/static/index.html`

### 12.1 Сборка и запуск

Из корня репозитория:

```
cd docker
docker compose build
docker compose up -d
```

Посмотреть логи peer (нужен `identity_id`):

```
docker compose logs peer
```

`identity_id` печатается в выводе `peer init`.

**Усиленные настройки контейнера (server-like):**

* контейнеры запускаются **не от root** (UID/GID 10001);
* `read_only: true`;
* `cap_drop: ALL`;
* `security_opt: no-new-privileges:true`;
* `tmpfs: /tmp, /run`;
* healthcheck по `/healthz`.

### 12.2 Проверка интеграции: статический контент

```
docker compose run --rm webclient request \
  --addr peer:3840 \
  --target <identity_id|service_id|alias> \
  --path /
```

Ожидаемо: ACK OK и `result_node_id` в выводе.

### 12.3 Проверка интеграции: выполнение задачи + fetch результата

```
docker compose run --rm webclient request \
  --addr peer:3840 \
  --target <identity_id|service_id|alias> \
  --path / \
  --fetch-result
```

Ожидаемо: вывод `web.response.v1` с HTML‑текстом страницы.

### 12.4 Остановка

```
docker compose down
```

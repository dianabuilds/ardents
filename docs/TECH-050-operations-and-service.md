# TECH-050: Эксплуатация, директории и запуск (v1)

**Статус:** Draft (2026-02-02)  
**Назначение:** зафиксировать целевую схему “удобного продукта” для v1 (headless-first): где лежат файлы, как запускать и как оформить daemon/service.

---

## 1) Целевой режим v1

* Primary user v1: **server/node (headless-first)**.
* Основной интерфейс: CLI + конфиги, минимум интерактива.
* Автозапуск после reboot: **НЕ по умолчанию**, только по явному действию пользователя (systemd unit).
* Gateway: **выключен по умолчанию** (отдельный процесс, включение — явным действием).
* Логи: **stdout/journald по умолчанию** (структурированные JSON lines). Запись логов в файл — опциональная настройка/флаг.

---

## 2) Директории и раскладка (XDG/portable)

### 2.1 Linux default (XDG)

* Config: `${XDG_CONFIG_HOME:-~/.config}/ardents/`
* Data: `${XDG_DATA_HOME:-~/.local/share}/ardents/`
* State: `${XDG_STATE_HOME:-~/.local/state}/ardents/`
* Run: `${XDG_STATE_HOME:-~/.local/state}/ardents/run/`

### 2.2 Portable mode

Если задан `ARDENTS_HOME` или флаг `--home`, то используется portable раскладка:

* `<home>/config/`
* `<home>/data/`
* `<home>/run/`

### 2.3 Файлы v1

* config: `node.json` (в `config/`)
* address book: `addressbook.json` (в `data/`)
* identity: `data/identity/identity.key`
* transport keys: `data/keys/peer.key`, `data/keys/peer.crt`
* status: `run/status.json`
* gateway token: `run/peer.token`
* packet capture: `run/pcap.jsonl`

---

## 3) Запуск/настройка (CLI)

### 3.1 Инициализация

* `peer init` — создаёт директории, config (если нет), identity, transport keys и address book.

Portable пример:

* `peer init --home ./ardents-home`

### 3.2 Запуск

* `peer start` — запускает узел, пишет status в `run/status.json`, пишет логи в stdout.
* `peer status` — показывает status и `/healthz` (если включён).

Portable пример:

* `peer start --home ./ardents-home`

Опционально (локальный файл логов):

* `peer start --log.file log.jsonl` (пишет в `<run>/log.jsonl` для текущего home/XDG)
* `peer start --log.format json|text`

### 3.3 Gateway (опционально)

По умолчанию gateway не запускается.

Чтобы подготовить token (owner-only) и явно включить gateway-интеграцию:

* `peer start --enable-gateway ...`
* затем `gateway --home <same>` (или `gateway --token <path>`)

---

## 5) Support bundle (диагностика без секретов)

Для поддержки и воспроизводимости багов есть команда:

* `peer support bundle`

Что попадает в ZIP:

* метаданные (OS/arch/go version/build info)
* снимок путей (`meta/paths.json`)
* `config/node.json` (редактирование “секретов” зарезервировано; в v1 секретов в конфиге нет)
* `run/status.json` (если есть)
* tail файла логов (если включён `observability.log_file`)
* метаданные packet capture (`run/pcap.meta.json`), если существует `run/pcap.jsonl` (raw payload не включается)
* `data/addressbook.meta.json` по умолчанию, или полный `data/addressbook.json` по флагу `--include-addressbook` (поле `note` редактируется)

Не попадает (всегда):

* `run/peer.token`
* приватные ключи identity и transport keys (`data/identity/*`, `data/keys/*`)

---

## 4) systemd units (Linux)

### 4.1 Генерация unit файла

`peer systemd unit --mode=user` печатает unit в stdout.

Для system-wide (обычно server/VPS) рекомендуется portable home:

* `peer systemd unit --mode=system --home /var/lib/ardents`

### 4.1.1 Установка unit файла (удобная команда)

* `peer install-service --mode=user` — пишет unit в `${XDG_CONFIG_HOME:-~/.config}/systemd/user/ardents.service`.
* `peer install-service --mode=system --home /var/lib/ardents` — пытается записать unit в `/etc/systemd/system/ardents.service` (обычно требует root).

### 4.2 Установка/включение

Проект не включает автозапуск по умолчанию. Пользователь сам решает:

* `systemctl --user enable --now ardents.service` (user service)
* `sudo systemctl enable --now ardents.service` (system service)

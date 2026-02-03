# SPEC-000: Обзор системы и карта спецификаций

**Проект:** распределённая overlay-сеть для сервисов и контента, ориентированная на приватность и анонимную доставку (Tor/I2P-like), с адресной книгой, задачами и интеграциями.
**Статус:** Draft v2.0 (2026-02-02) — v1 профиль остаётся совместимым режимом “direct mode”.
**Цель документа:** дать общую модель системы, словарь, компоненты, жизненные сценарии и карту спецификаций.

## 1) Цели и принципы

### 1.1 Цели продукта

1. **Внутренняя сеть сервисов и контента** поверх обычного интернета: узлы, адреса, каналы, маршруты, оффлайн-режим.
2. **Простая прикладная ценность**: обмен контентом/состоянием + выполнение задач (в т.ч. AI) через единый транспорт.
3. **Приватность и анонимность по умолчанию (v2):** туннели, directory/reseed, NetDB, минимизация метаданных.
4. **Наблюдаемость без утечек:** диагностика обязательна, но не должна deanonymize пользователей и сервисы.

### 1.2 Профили протокола (фиксировано)

Система имеет два профиля, чтобы исключить двусмысленность поведения и требований:

1. **Profile v1: direct mode (совместимость/разработка)**  
   * discovery: только config/address book (SPEC-110, раздел 1)
   * доставка: `envelope.v1` напрямую/через простые relays (SPEC-130 v1)
   * direct-mode app features (`chat.msg.v1`, `service.announce.v1`) удалены из текущей реализации
   * приватность: ограниченная (SPEC-002)
2. **Profile v2: privacy-first (основной)**  
   * bootstrap: directory authorities +reseed (SPEC-500)
   * NetDB (DHT) и записи сети (SPEC-510)
   * туннели и garlic delivery (SPEC-520)
   * прикладная доставка: `envelope.v2` внутри garlic (SPEC-550)
   * анонимные сервисы и поиск по capabilities через directory service (SPEC-530)
   * модель угроз v2: SPEC-540

### 1.3 Негативные цели (что НЕ делаем как продукт)
* Глобально согласованное состояние данных (“везде одно и то же состояние в один момент времени”).
* Глобальный поиск “как Google”.
* Децентрализованный финальный консенсус “всё всегда истинно”.
* Монетизация/токены как обязательное ядро.

### 1.4 Принципы

* **Разделение сущностей:**

  * `Node` (контент/объект) ≠ `Peer` (участник сети) ≠ `Identity` (ключи/доверие) ≠ `Service` (исполнитель/провайдер).
* **Transport-first:** сеть должна быть полезной без UI, UI лишь представление.
* **Envelope-based протокол:** единый формат сообщения + типизированные payload’ы.
* **Наблюдаемость встроена:** трассировка маршрутов, причины отказов, диагностические события.
* **Ordering не является свойством транспорта:** доставка не гарантирует порядок сообщений, если это явно не добавлено на уровне прикладного протокола.

---

## 2) Глоссарий (минимальный)

* **Peer**: сетевой участник (процесс/узел сети), имеющий `peer_id` и транспортные возможности.
* **Identity**: криптографическая сущность (ключи), используемая для подписей и доверия.
* **Service**: обработчик запросов/задач, привязанный к identity/peer.
* **Node (Content Node)**: адресуемый объект контента/состояния в графе (`node_id`), может иметь версии, ссылки, политику доступа.
* **Channel**: логический поток сообщений/событий (тема), который может быть чат-лентой, очередью задач, стримом состояния.
* **Address Book**: локальная/доверенная таблица alias → destination (service_id/node_id), включая источники и доверие.
* **Route / Tunnel**: путь доставки сообщений через hops.
* **Envelope**: стандартная “обёртка” сообщения сети.
* **Capability**: декларация возможностей сервиса/пира (какие job_type принимает, лимиты, политика).

---

## 3) Высокоуровневая архитектура

### 3.1 Слои системы (модель “луковицы”)

1. **Core Runtime**

   * криптография, ключи, подписи, идентификаторы
2. **Transport & Routing**

   * p2p соединения, NAT traversal/relay, маршрутизация/туннели
3. **Messaging**

   * envelope, очереди, доставка, подтверждения, дедупликация
4. **Naming & Discovery**

   * адресная книга, каталоги, discovery, capability search
5. **Data Plane**

   * контент-узлы (node graph), репликация, политика доступа
6. **Service Plane**

   * задачи, воркеры, интеграции (AI, индексация, тест-раннеры)
7. **Clients**

   * чат-консоль, task UI, браузер графа, админ/диагностика

### 3.2 Компоненты (как отдельные спеки/модули)

* **NET**: управление сетью, состояния (on/off), подключения, peer sessions
* **DISCOVERY**: поиск узлов/пиров/сервисов
* **NAMING**: address book, alias, источники, конфликт-резолв
* **ROUTING**: маршруты/туннели, политики, метрики
* **MSG**: envelope, доставка, ack, store-and-forward
* **NODEGRAPH**: контент-узлы, версии, ссылки, policies
* **TASKS**: job lifecycle, очереди задач, результаты, receipts
* **SERVICES**: capability registry, service adapters
* **INTEGRATIONS**: IPC-адаптеры, внешние протоколы
* **CLIENTS**: UX представления + диагностика
* **OBS**: логирование, трассировка, события, health

---

## 4) Целевая схема взаимодействий

### 4.1 Идентификаторы (развести всё по полочкам)

* `peer_id`: идентификатор сетевого процесса (вычисляется из транспортного ключа; не источник доверия)
* `identity_id`: криптографическая идентичность `did:key` (подпись, доверие, владение)
* `service_id`: детерминированный адрес сервиса (из `identity_id` + `service_name`)
* `node_id`: адрес контента (CIDv1, content-addressed)
* `channel_id`: детерминированный идентификатор потока сообщений

> Правило: **NodeID отвечает “что”, PeerID отвечает “где/как достать”, ServiceID отвечает “кому отправить запрос”.**

### 4.2 Envelope (единая обёртка)

Минимальная форма (расширяемая):

* `msg_id`
* `type` (chat | task.request | task.result | state.update | signal …)
* `from`:
  * v1: `peer_id` + optional `identity_id` (SPEC-140)
  * v2: optional `identity_id` + optional `service_id` (SPEC-550)
* `to`:
  * v1: `peer_id` | `service_id` | `channel_id` (SPEC-140)
  * v2: `service_id` (SPEC-550)
* `ts_ms`, `ttl_ms`
* `refs` (ссылки на связанные msg/node/task)
* `sig` (подпись payload/envelope частей)

### 4.3 Основные потоки (end-to-end)

#### A) “Консоль/чат” как базовый UX сети (deprecated)

1. user выбирает alias или peer
2. клиент резолвит через address book
3. direct-mode chat удалён из текущей реализации
4. доставка через routing (прямо или через hops)
5. отображение и диагностика (latency, hops, trust)

#### B) Контентный граф (узлы как адрес контента)

1. publish node: `node.create`/`state.publish`
2. announce providers: `node.provide`
3. fetch node: `node.fetch` + store-and-forward
4. verify signature/policy
5. render links → переходы по графу

#### C) Задачи (в том числе AI) как сервисы

1. discover service по capability `job_type=ai.chat`
2. `task.request` → сервису
3. сервис отвечает `task.accept` (+progress optional)
4. `task.result` (ссылка на artifacts/nodes)
5. `task.receipt` (метрики, стоимость, доказательства)

---

## 5) Границы MVP (чтобы не умереть)

**MVP-1 (скелет сети):**

* NET: старт/стоп, соединение peer↔peer, сессии
* NAMING: локальный address book alias→peer/service
* MSG: envelope, доставка, ack, дедуп
* CLIENT: чат-консоль + диагностика (пинги, трассировка)
* OBS: события сети (peer connected/disconnected)

**MVP-2 (контент/граф):**

* NODEGRAPH: node publish/fetch, ссылки, версии v0
* PROVIDERS: кто хранит node_id
* CLIENT: viewer графа + переходы

**MVP-3 (задачи/интеграции):**

* TASKS: lifecycle
* SERVICES: capability registry
* INTEGRATIONS: IPC-адаптер (local) как первый внешний адаптер

### 5.1 Фиксация границ v1

* **v1 = строго MVP-1** из раздела выше.
* Всё из MVP-2/MVP-3 **НЕ ВХОДИТ В v1** и может присутствовать только как типы/интерфейсы без реализации.

### 5.2 Проектные ограничения v1 (зафиксировано)

* Репозиторий: open-source, лицензия MIT.
* Go: **не ниже 1.25** (dev-версия может быть 1.25.6), без жёсткого pin toolchain.
* `go.mod` module: `github.com/<org>/ardents`.
* Целевые платформы v1: **Windows + Linux**, архитектура **amd64**.
* `legacy/` — reference-only, не часть реализации v1 и может быть удалён при исчезновении нужды.
* Релизы v1: `CHANGELOG` не обязателен; SemVer вводится при появлении внешних пользователей.

---

## 6) Карта спецификаций (Spec Map)

### Уровень 0: обзор и терминология

* **SPEC-000** System Overview & Spec Map (этот документ)
* SPEC-010 Спецификационный стиль и конвенции
* SPEC-001 Identity и идентификаторы
* SPEC-002 Модель угроз и доверия (включая анти-спам)

### Уровень 1: сеть и сообщения

* SPEC-100 Network Manager (NET)
* SPEC-110 Peer Discovery Рё Handshake
* SPEC-120 Address Book Рё Naming
* SPEC-130 Routing Рё Relays (v1 direct mode)
* SPEC-140 Сообщения (Envelope v1) и доставка (direct)

### Уровень 1b: privacy-first транспорт (v2)

* SPEC-500 Directory Authorities & Reseed (bootstrap)
* SPEC-510 NetDB (DHT) и записи сети
* SPEC-520 Туннели и garlic/onion маршрутизация
* SPEC-530 Анонимные сервисы и directory-поиск
* SPEC-540 Модель угроз privacy-first
* SPEC-550 Envelope v2 (анонимная доставка)

### Уровень 2: контент и состояние

* SPEC-200 Модель Node Graph (Content Nodes)
* SPEC-210 Репликация контента и Providers
* SPEC-220 Политики доступа и шифрование

### Уровень 3: сервисы, задачи, интеграции

* SPEC-300 Модель сервисов и capabilities
* SPEC-310 Жизненный цикл задач и receipts
* SPEC-320 Интеграционные шлюзы (Integration Gateways)
* SPEC-330 Профиль AI сервиса (ai.chat.v1)
* SPEC-340 Web service profile (web.request.v1)

### Уровень 4: клиенты и наблюдаемость

* SPEC-400 Клиент Console/Chat (deprecated)
* SPEC-410 Клиент Node Browser
* SPEC-420 Диагностика и наблюдаемость
* SPEC-430 Инструменты администрирования и разработки

### Соответствие SPEC → файл

| SPEC     | Файл                                                 |
|----------|------------------------------------------------------|
| SPEC-000 | `spec/SPEC-000-system-overview.md`                   |
| SPEC-001 | `spec/SPEC-001-identity-and-identifiers.md`          |
| SPEC-002 | `spec/SPEC-002-threat-and-trust-model.md`            |
| SPEC-010 | `spec/SPEC-010-spec-style-and-conventions.md`        |
| SPEC-100 | `spec/SPEC-100-network-manager.md`                   |
| SPEC-110 | `spec/SPEC-110-peer-discovery-and-handshake.md`      |
| SPEC-120 | `spec/SPEC-120-address-book-and-naming.md`           |
| SPEC-130 | `spec/SPEC-130-routing-and-relays.md`                |
| SPEC-140 | `spec/SPEC-140-message-envelope-and-delivery.md`     |
| SPEC-200 | `spec/SPEC-200-node-graph-model.md`                  |
| SPEC-210 | `spec/SPEC-210-content-replication-and-providers.md` |
| SPEC-220 | `spec/SPEC-220-access-policy-and-encryption.md`      |
| SPEC-300 | `spec/SPEC-300-service-model-and-capabilities.md`    |
| SPEC-310 | `spec/SPEC-310-tasks-lifecycle-and-receipts.md`      |
| SPEC-320 | `spec/SPEC-320-integration-gateways.md`              |
| SPEC-330 | `spec/SPEC-330-ai-service-profile.md`                |
| SPEC-340 | `spec/SPEC-340-web-service-profile.md`               |
| SPEC-400 | `spec/SPEC-400-client-console-chat.md` (deprecated)  |
| SPEC-410 | `spec/SPEC-410-client-node-browser.md`               |
| SPEC-420 | `spec/SPEC-420-diagnostics-and-observability.md`     |
| SPEC-430 | `spec/SPEC-430-admin-and-dev-tools.md`               |
| SPEC-500 | `spec/SPEC-500-directory-authorities-and-reseed.md`  |
| SPEC-510 | `spec/SPEC-510-netdb-and-records.md`                 |
| SPEC-520 | `spec/SPEC-520-tunnels-and-garlic-routing.md`        |
| SPEC-530 | `spec/SPEC-530-anonymous-services-and-directory.md`  |
| SPEC-540 | `spec/SPEC-540-privacy-threat-model.md`              |
| SPEC-550 | `spec/SPEC-550-anonymous-envelope.md`                |

---

## 7) Definition of Done для “пакета спек”

Спеки считаются пригодными к реализации, когда для каждой:

* есть scope + out-of-scope
* перечислены состояния/события
* есть API/сообщения (минимум wire format)
* описаны ошибки и восстановления
* описаны метрики/логирование
* есть 2–3 основных user stories и 1–2 edge cases

Дополнение для MVP-1 acceptance:

* handshake success/fail
* send → ACK OK
* dedup работает
* PoW reject
* degraded NET state
* console shows error
* CI v1 (минимум): build + unit tests

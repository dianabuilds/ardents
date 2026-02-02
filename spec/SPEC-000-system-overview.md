# SPEC-000: Обзор системы и карта спецификаций

**Проект:** распределённая overlay-сеть для сервисов и контента (i2p-like), с адресной книгой, каналами/задачами и интеграциями.
**Статус:** Approved v1.0 (2026-02-02)
**Цель документа:** дать общую модель системы, словарь, компоненты, жизненные сценарии и карту спецификаций.

## 1) Цели и принципы

### 1.1 Цели продукта

1. **Внутренняя сеть сервисов и контента** поверх обычного интернета: узлы, адреса, каналы, маршруты, оффлайн-режим.
2. **Простая прикладная ценность**: обмен контентом/состоянием + выполнение задач (в т.ч. ИИ) через единый транспорт.
3. **Устойчивость и приватность как режимы**, без фанатизма: вначале наблюдаемость и дебаг важнее “идеальной анонимности”, но архитектура не должна её исключать.

### 1.2 Негативные цели (что НЕ делаем в MVP)

* Полноценная “анонимность уровня Tor/i2p” как верифицируемое утверждение.
* Глобально согласованное состояние данных (“везде одно и то же состояние в один момент времени”).
* Глобальный поиск “как Google”.
* Децентрализованный финальный консенсус “всё всегда истинно”.
* Монетизация/токены как обязательное ядро.

### 1.3 Принципы

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

   * задачи, воркеры, интеграции (ИИ, индексация, тест-раннеры)
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
* **INTEGRATIONS**: bridge gateways, внешние протоколы
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
* `from` (`peer_id` + optional `identity_id`)
* `to` (`peer_id` | `service_id` | `channel_id`)
* `ts_ms`, `ttl_ms`
* `refs` (ссылки на связанные msg/node/task)
* `route` (optional: hops metadata)
* `sig` (подпись payload/envelope частей)

### 4.3 Основные потоки (end-to-end)

#### A) “Консоль/чат” как базовый UX сети

1. user выбирает alias или peer
2. клиент резолвит через address book
3. отправляет `type=chat` в нужный destination
4. доставка через routing (прямо или через hops)
5. отображение и диагностика (latency, hops, trust)

#### B) Контентный граф (узлы как адрес контента)

1. publish node: `node.create`/`state.publish`
2. announce providers: `node.provide`
3. fetch node: `node.fetch` + store-and-forward
4. verify signature/policy
5. render links → переходы по графу

#### C) Задачи (в том числе ИИ) как сервисы

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
* INTEGRATIONS: bridge gateway (HTTP/gRPC) как первый внешний адаптер

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
* SPEC-110 Peer Discovery и Handshake
* SPEC-120 Address Book и Naming
* SPEC-130 Routing и Relays (туннели)
* SPEC-140 Сообщения (Envelope) и доставка (Delivery)

### Уровень 2: контент и состояние

* SPEC-200 Модель Node Graph (Content Nodes)
* SPEC-210 Репликация контента и Providers
* SPEC-220 Политики доступа и шифрование

### Уровень 3: сервисы, задачи, интеграции

* SPEC-300 Модель сервисов и capabilities
* SPEC-310 Жизненный цикл задач и receipts
* SPEC-320 Интеграционные шлюзы (Integration Gateways)
* SPEC-330 Профиль AI сервиса (ai.chat.v1)

### Уровень 4: клиенты и наблюдаемость

* SPEC-400 Клиент Console/Chat
* SPEC-410 Клиент Node Browser
* SPEC-420 Диагностика и наблюдаемость
* SPEC-430 Инструменты администрирования и разработки

### Соответствие SPEC → файл (v1)

| SPEC | Файл |
| --- | --- |
| SPEC-000 | `spec/SPEC-000-system-overview.md` |
| SPEC-001 | `spec/SPEC-001-identity-and-identifiers.md` |
| SPEC-002 | `spec/SPEC-002-threat-and-trust-model.md` |
| SPEC-010 | `spec/SPEC-010-spec-style-and-conventions.md` |
| SPEC-100 | `spec/SPEC-100-network-manager.md` |
| SPEC-110 | `spec/SPEC-110-peer-discovery-and-handshake.md` |
| SPEC-120 | `spec/SPEC-120-address-book-and-naming.md` |
| SPEC-130 | `spec/SPEC-130-routing-and-relays.md` |
| SPEC-140 | `spec/SPEC-140-message-envelope-and-delivery.md` |
| SPEC-200 | `spec/SPEC-200-node-graph-model.md` |
| SPEC-210 | `spec/SPEC-210-content-replication-and-providers.md` |
| SPEC-220 | `spec/SPEC-220-access-policy-and-encryption.md` |
| SPEC-300 | `spec/SPEC-300-service-model-and-capabilities.md` |
| SPEC-310 | `spec/SPEC-310-tasks-lifecycle-and-receipts.md` |
| SPEC-320 | `spec/SPEC-320-integration-gateways.md` |
| SPEC-330 | `spec/SPEC-330-ai-service-profile.md` |
| SPEC-400 | `spec/SPEC-400-client-console-chat.md` |
| SPEC-410 | `spec/SPEC-410-client-node-browser.md` |
| SPEC-420 | `spec/SPEC-420-diagnostics-and-observability.md` |
| SPEC-430 | `spec/SPEC-430-admin-and-dev-tools.md` |

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

# SPEC-460: Client Bootstrap и авто‑резолв без addr

**Статус:** Draft (2026-02-04)  
**Зависимости:** SPEC-001, SPEC-010, SPEC-120, SPEC-126, SPEC-415, SPEC-500, SPEC-510, SPEC-530  
**Назначение:** зафиксировать клиентский режим, при котором запросы выполняются без явного `addr` за счёт bootstrap/Directory/NetDB.

---

## 1) Scope / Out-of-scope

### Scope

* Формат и размещение `client.json`.
* Bootstrap‑пиры для входа в сеть.
* Авто‑резолв: `target` → `service_id` → доступный gateway.
* Политика доверия для источников.
* Ошибки.

### Out-of-scope

* Глобальный реестр доменных имён.
* Ручной режим с `--addr` (остаётся в SPEC-340/410).

---

## 2) Размещение и формат конфигурации

Файл: `config/client.json` (рядом с `config/node.json`).

Корневой объект:

* `v` (uint) = 1
* `bootstrap_peers` (array) — список начальных пиров
* `trusted_identities` (array) — список trusted identity (did:key)
* `refresh_ms` (int64) — период обновления bootstrap/каталогов (минимум 60_000)
* `reseed` (map) — внешний recovery‑канал (см. раздел 5)
* `limits` (map) — ограничения discovery cache (см. раздел 6.3)

### 2.1 Формат `bootstrap_peers`

Каждый элемент **ДОЛЖЕН** содержать:

* `peer_id` (string)
* `addrs` (array[string]) — список QUIC‑адресов

И **МОЖЕТ** содержать:

* `identity_id` (string, did:key) — owner identity данного peer (для прямого выбора gateway по `target=identity_id`).

---

## 3) Модель хранения: trust vs discovery

Клиент **ДОЛЖЕН** разделять:

* **Address Book** (`data/addressbook.json`, SPEC-120) — trust boundary, НЕ авто‑пополняется из сети.
* **Discovery cache** (локальный кэш обнаружения) — авто‑пополняется, всегда `untrusted`, всегда с TTL и лимитами.

Discovery cache **НЕ ДОЛЖЕН** влиять на trust‑политику и **НЕ ДОЛЖЕН** автоматически создавать алиасы/домены.

---

## 4) Нормативные требования

1) Клиент **ДОЛЖЕН** поддерживать режим без `addr` при наличии `client.json`.  
2) В режиме без `addr` клиент **ДОЛЖЕН**:
   * выполнить bootstrap через `bootstrap_peers`;
   * получить актуальные записи из NetDB/Directory;
   * выбрать доступный gateway для доставки запроса.
3) В режиме без `addr` доверие **ДОЛЖНО** определяться локально (`trusted_identities` + Address Book).  
4) Если нет `client.json` или `bootstrap_peers`, клиент **ДОЛЖЕН** вернуть `ERR_CLIENT_BOOTSTRAP_REQUIRED`.  
5) Если Directory/NetDB недоступны, клиент **ДОЛЖЕН** вернуть `ERR_CLIENT_DISCOVERY_UNAVAILABLE`.
6) Если все `bootstrap_peers` недоступны и discovery cache не содержит рабочих кандидатов, клиент **ДОЛЖЕН** перейти к внешнему recovery (reseed) по разделу 5.  
7) Клиент **НЕ ДОЛЖЕН** принимать произвольные списки узлов из сети как источник bootstrap, кроме источников, перечисленных в разделах 5 и 6.  

---

## 5) Внешний recovery‑канал (reseed)

Если клиент не может войти в сеть через `bootstrap_peers` и discovery cache, он **ДОЛЖЕН** выполнить внешний reseed.

`client.json.reseed` **ДОЛЖЕН** содержать:

* `enabled` (bool)
* `network_id` (string)
* `urls` (array[string]) — HTTPS endpoints с reseed bundle
* `authorities` (array[string]) — pinned `identity_id` DA

Правила:

1) Reseed выполняется по HTTPS и **ДОЛЖЕН** проверяться по кворуму подписей DA, как в SPEC-500.  
2) При успешном reseed клиент **ДОЛЖЕН** обновить bootstrap‑кандидаты и повторить bootstrap.  
3) Клиент **ДОЛЖЕН** применять backoff и rate‑limit на внешний reseed, чтобы не перегружать endpoints.  

---

## 6) Discovery cache: источники, TTL и анти‑спам

Discovery cache **ДОЛЖЕН** хранить записи вида:

* `peer_id`, `addr`
* `last_ok_ms`, `last_fail_ms`
* `fail_count`, `cooldown_until_ms`
* `expires_at_ms`
* `source` ∈ {`bootstrap`, `reseed`, `netdb`, `directory`, `handshake_hint`}

Источники, которые **РАЗРЕШЕНО** добавлять в discovery cache:

1) `bootstrap_peers` и результат reseed (раздел 5).  
2) NetDB записи `router.info` (SPEC-510) после валидации подписи и TTL.  
3) Directory‑результаты (SPEC-530) после валидации подписи результата и локальной trust‑политики.  
4) Handshake hint (`router_info` из handshake) — только как `handshake_hint` и только с коротким TTL.  

Анти‑спам требования:

* Клиент **ДОЛЖЕН** ограничивать размер discovery cache (`limits.max_peers`).  
* Клиент **ДОЛЖЕН** rate‑limit добавление новых записей (`limits.add_rate_limit` за `limits.add_rate_window_ms`).  
* Клиент **ДОЛЖЕН** применять backoff/cooldown для недоступных адресов.  
* Клиент **ДОЛЖЕН** дедуплицировать записи по `(peer_id, addr)`.

### 6.3 Параметры `limits` (фиксировано)

`client.json.limits` **ДОЛЖЕН** содержать:

* `max_peers` (uint) — максимум записей в discovery cache. **Default:** 512.  
* `add_rate_limit` (uint) — максимум добавлений новых записей за окно. **Default:** 50.  
* `add_rate_window_ms` (int64) — окно rate‑limit. **Default:** 10_000.  
* `cooldown_base_ms` (int64) — базовая задержка перед повтором после ошибки. **Default:** 2_000.  
* `cooldown_max_ms` (int64) — максимум cooldown. **Default:** 60_000.  
* `handshake_hint_ttl_ms` (int64) — TTL для `source=handshake_hint`. **Default:** 10_000.  

Если `limits` отсутствует, клиент **ДОЛЖЕН** применять значения по умолчанию.

---

## 7) Авто‑резолв (обобщённый алгоритм)

1) Разобрать `target` по SPEC-415 и получить `service_id`.
2) Выполнить bootstrap (если сеть не инициализирована).
3) Получить `service.head.v1` и `service.lease_set.v1` через NetDB.
4) Выбрать `gateway_peer_id` по доступности и TTL.
5) Доставить запрос через выбранный gateway.

Правило оптимизации UX: если исходный `target` был `identity_id` и среди `bootstrap_peers` есть запись с совпадающим `identity_id`, клиент **ДОЛЖЕН** сначала попытаться выбрать этот peer как gateway.

---

## 8) Ошибки (минимум)

* `ERR_CLIENT_BOOTSTRAP_REQUIRED`
* `ERR_CLIENT_DISCOVERY_UNAVAILABLE`
* `ERR_CLIENT_NO_GATEWAY_AVAILABLE`

---

## 9) Наблюдаемость

События (минимум):

* `client.bootstrap.started`
* `client.bootstrap.finished`
* `client.discovery.started`
* `client.discovery.failed`
* `client.gateway.selected`
* `client.reseed.started`
* `client.reseed.finished`

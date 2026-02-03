# SPEC-500: Directory Authorities & Reseed (privacy-first)

**Статус:** Draft v2.0 (2026-02-02)  
**Зависимости:** SPEC-001, SPEC-002, SPEC-010, SPEC-110, SPEC-420  
**Назначение:** зафиксировать tor-like модель доверенных directory authorities, формат reseed bundle и правила входа узла в сеть без предварительного знания пиров.

---

## 0) Термины

* **Directory Authority (DA)** — доверенный identity, подписывающий “снимок входа в сеть” (reseed bundle) и параметры сети.
* **Reseed** — процедура первичного получения seed RouterInfo и network params по внешнему каналу (HTTPS) и/или через overlay после входа.
* **Seed Router** — router/peer, который можно использовать для первичного подключения к сети и дальнейшего участия в NetDB (см. SPEC-510).

---

## 1) Модель доверия (фиксировано)

1. В сети существует фиксированный набор DA: **N = 5**.
2. Узел/клиент **ДОЛЖЕН** иметь pinned публичные ключи (identity_id) всех 5 DA (в дистрибутиве) либо получить их из доверенного локального источника (Address Book, “self/trusted”).
3. Для принятия reseed bundle узел **ДОЛЖЕН** собрать кворум подписей **M = 3 из 5**.
4. Подпись 2 из 5 **НЕДОСТАТОЧНА**.
5. DA **НЕ ЯВЛЯЮТСЯ** источником “доверия к сервисам”: они дают только “точки входа” (seed routers) и сетевые параметры. Доверие к сервисам определяется локально (Address Book + криптовалидация объектов).

---

## 2) Каналы reseed (фиксировано)

### 2.1 Внешний reseed (обязателен)

Узел, не имеющий ни одного известного router, **ДОЛЖЕН** выполнить внешний reseed:

* download по HTTPS (TLS) reseed bundle с URL из конфигурации;
* проверка кворума DA;
* извлечение seed RouterInfo и network params.

### 2.2 Overlay reseed (опционально, после входа)

После успешного входа в сеть узел **МОЖЕТ** обновлять seed через overlay-запрос к DA-обслуживаемому directory-сервису. Этот механизм не заменяет внешний reseed “нулевого дня”.

---

## 3) Формат reseed bundle (фиксировано)

### 3.1 Транспорт

* Reseed bundle доставляется как файл `reseed.bundle.v1` с Content-Type `application/cbor`.
* Тело ответа **ДОЛЖНО** быть canonical CBOR (SPEC-010).

### 3.2 Объект `reseed.bundle.v1`

CBOR map:

* `v` = 1
* `network_id` (string) — идентификатор сети (фиксированная строка, например `ardents.mainnet`)
* `issued_at_ms` (int64)
* `expires_at_ms` (int64) — TTL бандла; **ДОЛЖЕН** быть ≤ 24 часа
* `params` (map) — network params v1 (см. раздел 4)
* `routers` (array) — список `router.info.v1` объектов (см. SPEC-510)
* `signatures` (array) — подписи DA (см. 3.3)

### 3.3 Подписи DA (фиксировано)

Каждая подпись — CBOR map:

* `v` = 1
* `authority_identity_id` (string, did:key)
* `sig` (bytes) — Ed25519 подпись

Правило подписи:

* DA подписывает canonical CBOR от bundle **без поля** `signatures`.

Проверка:

* Узел **ДОЛЖЕН** верифицировать каждую подпись по `authority_identity_id`.
* Узел **ДОЛЖЕН** принять bundle только при наличии ≥3 валидных подписей от различных DA из pinned списка.

---

## 4) Network params (фиксировано)

`params` (CBOR map) **ДОЛЖЕН** содержать:

* `protocol_major` (uint) — **2**
* `protocol_minor` (uint) — ≥0
* `netdb` (map):
  * `k` (uint) — **20** (bucket size)
  * `alpha` (uint) — **3** (parallelism)
  * `replication` (uint) — **20**
  * `record_max_ttl_ms` (int64) — **3600_000**
* `tunnels` (map):
  * `hop_count_default` (uint) — **3**
  * `hop_count_min` (uint) — **2**
  * `hop_count_max` (uint) — **5**
  * `rotation_ms` (int64) — **600_000**
  * `lease_ttl_ms` (int64) — **600_000**
  * `padding_policy` (string) — **"basic.v1"** (см. SPEC-520)
* `anti_abuse` (map):
  * `pow_default_difficulty` (uint) — рекомендуемая сложность stamp для untrusted
  * `rate_limits` (map) — лимиты на netdb/tunnel operations (см. SPEC-002)

Любые отсутствующие поля **ДОЛЖНЫ** приводить к отказу применения bundle (ERR_RESEED_PARAMS_INVALID).

---

## 5) Правила применения reseed bundle (фиксировано)

После успешной проверки:

1. Узел **ДОЛЖЕН** сохранить `params` как активные сетевые параметры до истечения `expires_at_ms`.
2. Узел **ДОЛЖЕН** поместить объекты `routers` в локальную NetDB (как initial cache) с TTL, ограниченным `expires_at_ms - now_ms`.
3. Узел **ДОЛЖЕН** попытаться подключиться минимум к 3 различным seed routers (параллельно, с backoff).
4. Если не удалось подключиться ни к одному seed router, узел **ДОЛЖЕН** перейти в состояние `degraded` с причиной `no_bootstrap` и повторить reseed по backoff.

---

## 6) Ошибки (минимум)

* `ERR_RESEED_FETCH_FAILED`
* `ERR_RESEED_SIGNATURE_INVALID`
* `ERR_RESEED_QUORUM_NOT_REACHED`
* `ERR_RESEED_EXPIRED`
* `ERR_RESEED_PARAMS_INVALID`


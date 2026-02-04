# SPEC-110: Peer Discovery и Handshake

**Статус:** Draft v2.0 (2026-02-02)  
**Зависимости:** SPEC-001, SPEC-002, SPEC-010, SPEC-100  
**Назначение:** определить, как peer находит других peer и как устанавливается защищённая сессия с согласованием параметров.

---

## 1) Discovery v1 (фиксировано)

В v1 discovery **ДОЛЖЕН** быть только:

1. **Static bootstrap list** из `config/node.json` (см. SPEC-100).
2. **Address Book** записи типа `peer` (см. SPEC-120).

Никаких DHT/мультикаста/авто-сканирования в v1 **НЕТ**.
NAT traversal/relay discovery в v1 **НЕ ТРЕБУЕТСЯ**: подключение только по явным адресам (config/address book).

## 1.1 Discovery v2 (privacy-first, фиксировано)

В v2 discovery/bootstrapping выполняется так:

1. Узел **ДОЛЖЕН** выполнить reseed по SPEC-500 (quorum 3/5) и получить initial `router.info.v1` набор.
2. Узел **ДОЛЖЕН** подключиться минимум к 3 различным seed routers и выполнить первичное заполнение NetDB (SPEC-510):
   * `netdb.find_node.v1` / `netdb.find_value.v1` по ключевым диапазонам,
   * storage/validation записей по правилам SPEC-510.
3. Address Book (SPEC-120) остаётся локальным источником доверия и **ДОЛЖЕН** поддерживать “пинning” конкретных peer/service identities, но **НЕ является** механизмом глобального discovery.

---

## 2) Транспорт (фиксировано)

* Основной транспорт v1: **QUIC поверх UDP**.
* Шифрование/аутентификация транспорта: **TLS 1.3** в режиме взаимной аутентификации (mTLS).

Сертификат/ключ TLS:

* **ДОЛЖЕН** быть self-signed и привязан к транспортному ключу peer (Ed25519).
* Проверка peer:
  * узел **ДОЛЖЕН** вычислить `peer_id` из публичного ключа сертификата по SPEC-001;
  * `peer_id` **ДОЛЖЕН** совпасть с `peer_id`, заявленным в сообщении `hello`.

Хранение ключей/сертификатов (v1):

* транспортный ключ и TLS-сертификат **ДОЛЖНЫ** храниться в `data/keys/` в формате PEM.

Ротация транспортного ключа:

* в v1 транспортный ключ считается **стабильным**; ротация (смена `peer_id`) **НЕ ТРЕБУЕТСЯ**.

---

## 3) Адреса (фиксировано)

Формат адреса peer в текстовой форме:

* `quic://<host>:<port>`

Где:

* `<host>` — IP или DNS.
* `<port>` — 1..65535.

Другие схемы в v1 **НЕ ДОПУСКАЮТСЯ**.

---

## 4) Handshake протокол уровня приложения

После установления защищённого QUIC-соединения стороны обмениваются `hello` сообщениями (в CBOR).

### 4.1 Сообщение `hello.v1`

Поле `hello` — CBOR map:

* `v` = 1
* `peer_id` (string)
* `ts_ms` (int64)
* `nonce` (bytes, длина 16)
* `pow_difficulty` (uint) — требуемая сложность PoW для недоверенных входящих сообщений (см. SPEC-002)
* `max_msg_bytes` (uint) — лимит на входящие сообщения
* `capabilities_digest` (bytes, SHA-256) — хэш списка capabilities, которые peer объявляет (см. SPEC-300)

* `router_info` (bytes, optional) - canonical CBOR bytes of `router.info.v1` record (incl `sig`, see SPEC-510).
* `router_infos` (array[bytes], optional) - additional `router.info.v1` records (each incl `sig`, see SPEC-510).
  Rule: receiver MUST validate each record via SPEC-510, MUST treat as short-lived hint (SPEC-460),
  and MUST enforce limits: max 16 records per hello; max 16 KiB per record.

### 4.1b Сообщение `hello.v2` (privacy-first)

`hello.v2` используется между роутерами в профиле v2. Формат:

* `v` = 2
* `protocol_major` (uint) — **2**
* `protocol_minor` (uint)
* `peer_id` (string)
* `ts_ms` (int64)
* `nonce` (bytes, длина 16)
* `pow_difficulty` (uint) — как в v1
* `max_msg_bytes` (uint)
* `router_info` (bytes, optional) — canonical CBOR bytes записи `router.info.v1` (включая `sig`, см. SPEC-510)

Правило:

* если `router_info` присутствует, получатель **ДОЛЖЕН** валидировать его по SPEC-510 и **ДОЛЖЕН** использовать как “ускорение” bootstrap (без ожидания DHT репликации).

### 4.2 Правила проверки `hello`

Получатель `hello` **ДОЛЖЕН**:

1. проверить `ts_ms` в допустимом окне (±5 минут);
2. проверить `peer_id` по транспортному сертификату (см. раздел 2);
3. применить лимиты `max_msg_bytes` как верхнюю границу для входящих сообщений от этого peer (локальные лимиты важнее удалённых).

---

## 4.3 Определение `capabilities_digest` (фиксировано)

`capabilities_digest` вычисляется как:

1. взять список job_type строк, которые peer готов объявлять/обслуживать (объединение всех локально хостимых service capabilities);
2. отсортировать список лексикографически (UTF-8 bytes);
3. сериализовать в canonical CBOR как массив строк;
4. вычислить `SHA-256` от получившихся bytes.

Назначение:

* если у peer изменился `capabilities_digest`, удалённый узел **СЛЕДУЕТ** инициировать повторный запрос/приём `service.announce.v1` и обновить локальные дескрипторы.

---

## 5) Объявления сервисов на peer

В профиле v2 `service.announce.v1` **НЕ ДОЛЖЕН** использоваться (он раскрывает endpoints). В v2 discovery сервисов выполняется через NetDB `service.head.v1`/`service.lease_set.v1` (SPEC-510/530).

Peer **МОЖЕТ** после `hello` отправить `service.announce.v1`:

* сообщение **ДОЛЖНО** содержать ссылку (`node_id`) на `service.descriptor.v1` (см. SPEC-300);
* descriptor **ДОЛЖЕН** быть подписан Identity владельца;
* peer **НЕ ДОЛЖЕН** считаться “доверенным” только из-за этого объявления.
* текущая реализация `service.announce.v1` **НЕ ПОДДЕРЖИВАЕТ** и возвращает `ERR_UNSUPPORTED_TYPE`.

### 5.1 `service.announce.v1` (фиксировано)

`service.announce.v1` — это Envelope (SPEC-140) с payload:

* `v` = 1
* `service_id` (string)
* `descriptor_node_id` (string, CIDv1)
* `ts_ms` (int64)
* `ttl_ms` (int64, рекомендуется 120_000)

Получатель **ДОЛЖЕН**:

* выполнить fetch `descriptor_node_id` (SPEC-210) и валидацию descriptor (SPEC-200/300);
* обновлять “current descriptor” по правилам SPEC-300.

---

## 6) Ошибки (минимум)

* `ERR_HANDSHAKE_TIME_SKEW`
* `ERR_PEER_ID_MISMATCH`
* `ERR_UNSUPPORTED_VERSION`
* `ERR_ADDR_INVALID`

# SPEC-510: NetDB (DHT) и записи сети (privacy-first)

**Статус:** Draft v2.0 (2026-02-02)  
**Зависимости:** SPEC-001, SPEC-010, SPEC-110, SPEC-420, SPEC-500  
**Назначение:** зафиксировать модель NetDB (Kademlia-подобная DHT), типы записей (RouterInfo/LeaseSet/ServiceHead) и правила их подписи/валидации.

---

## 0) Термины

* **NetDB** — распределённая база сведений о роутерах и точках входа в сервисы.
* **Record** — подписанный CBOR-объект с TTL, хранимый/реплицируемый в NetDB.
* **Router** — peer-узел, участвующий в маршрутизации (имеет `peer_id` и транспортные адреса).
* **Lease** — “арендованный” вход в inbound-туннель сервиса на ограниченное время.

---

## 1) Инварианты (фиксировано)

1. NetDB реализует Kademlia-подобную DHT с параметрами из `reseed.bundle.v1.params.netdb` (SPEC-500).
2. Любая запись NetDB **ДОЛЖНА** быть:
   * canonical CBOR (SPEC-010),
   * подписана (см. раздел 3),
   * с явным `expires_at_ms`,
   * ограничена по размеру (см. 1.4).
3. Узел **НЕ ДОЛЖЕН** принимать/хранить/ретранслировать запись, если:
   * подпись невалидна,
   * `expires_at_ms <= now_ms`,
   * `expires_at_ms - now_ms` превышает `record_max_ttl_ms` из параметров сети (SPEC-500),
   * нарушены схемы ID (SPEC-001) или инварианты типа записи.
4. Лимиты (фиксировано):
   * `MAX_NETDB_RECORD_BYTES = 64 KiB`
   * `MAX_LEASES_PER_LEASESET = 16`
   * `MAX_ADDRS_PER_ROUTER = 8`

---

## 2) Ключи хранения в DHT (fixed)

NetDB хранит значения по 32-byte ключу `dht_key`, который детерминированно выводится **из адреса записи**, а не из содержимого (иначе запись невозможно запросить без предварительного знания байтов).

Кодирование (фиксировано):

* `dht_key = sha2-256(type_utf8 || 0x00 || address_utf8)`

Где:

* `type_utf8` — строка типа записи: `router.info.v1` | `service.lease_set.v1` | `service.head.v1`
* `address_utf8`:
  * для `router.info.v1` — `peer_id`
  * для `service.lease_set.v1` — `service_id`
  * для `service.head.v1` — `service_id`

Инвариант:

* Узел, получив record, **ДОЛЖЕН** вычислить ожидаемый `dht_key` по правилу выше и хранить record именно под этим ключом.

---

## 3) Типы записей (фиксировано)

### 3.1 RouterInfo: `router.info.v1`

Назначение: объявление роутера (peer) и его параметров.

CBOR map:

* `v` = 1
* `peer_id` (string) — по SPEC-001
* `transport_pub` (bytes) — Ed25519 public key транспорта (32 bytes)
* `onion_pub` (bytes) — X25519 public key для туннельного рукопожатия (32 bytes)
* `addrs` (array[string]) — список адресов (например `quic://host:port`)
* `caps` (map):
  * `relay` (bool) — участвует ли узел в маршрутизации (для v2: **true**)
  * `netdb` (bool) — обслуживает ли DHT-запросы (для v2: **true**)
* `published_at_ms` (int64)
* `expires_at_ms` (int64)
* `sig` (bytes) — Ed25519 подпись (см. ниже)

Правила валидации:

1. `peer_id` **ДОЛЖЕН** совпадать с вычисленным по `transport_pub` (SPEC-001).
2. `addrs` **ДОЛЖНЫ** быть уникальны, количество `<= MAX_ADDRS_PER_ROUTER`.
3. `caps.relay=true` и `caps.netdb=true` — фиксировано для v2 (узел, не готовый быть роутером, не является router).
4. `expires_at_ms - published_at_ms` **ДОЛЖНО** быть `<= record_max_ttl_ms`.

Подпись:

* Подписывается canonical CBOR от объекта **без** поля `sig`.
* Подписывает **транспортный** приватный ключ Ed25519, соответствующий `transport_pub`.
* Получатель **ДОЛЖЕН** верифицировать подпись по `transport_pub`.

---

### 3.2 Service LeaseSet: `service.lease_set.v1`

Назначение: объявление “точек входа” в inbound-туннели сервиса (I2P-like).

CBOR map:

* `v` = 1
* `service_id` (string) — по SPEC-001
* `owner_identity_id` (string) — did:key (SPEC-001)
* `service_name` (string) — для воспроизводимой проверки `service_id` (SPEC-001)
* `enc_pub` (bytes) — X25519 public key сервиса для end-to-end (32 bytes)
* `leases` (array[map]) — список lease:
  * `gateway_peer_id` (string) — peer_id роутера-входа
  * `tunnel_id` (bytes) — 16 bytes opaque id (случайный)
  * `expires_at_ms` (int64) — срок аренды
* `published_at_ms` (int64)
* `expires_at_ms` (int64) — общий TTL записи
* `sig` (bytes) — Ed25519 подпись

Правила валидации:

1. `service_id` **ДОЛЖЕН** совпадать с вычисленным по `owner_identity_id` и `service_name` (SPEC-001).
2. `leases.length` **ДОЛЖНА** быть `1..MAX_LEASES_PER_LEASESET`.
3. Каждый `gateway_peer_id` **ДОЛЖЕН** существовать в локальной NetDB как валидный `router.info.v1` (или быть запрошен по DHT).
4. `tunnel_id` **ДОЛЖЕН** быть длины ровно 16 bytes.
5. `expires_at_ms` LeaseSet **ДОЛЖЕН** быть `<= min(leases[*].expires_at_ms)` (не дольше любого lease).
6. `expires_at_ms - published_at_ms` **ДОЛЖНО** быть `<= record_max_ttl_ms`.

Подпись:

* Подписывается canonical CBOR от объекта **без** поля `sig`.
* Подписывает приватный ключ `owner_identity_id` (Ed25519).
* Получатель **ДОЛЖЕН** верифицировать подпись по публичному ключу, извлечённому из `owner_identity_id` (did:key).

---

### 3.3 Service Head pointer: `service.head.v1`

Назначение: обновляемый указатель на текущий descriptor сервиса (обновляемость без “заморозки” discovery).

CBOR map:

* `v` = 1
* `service_id` (string)
* `owner_identity_id` (string) — did:key
* `service_name` (string)
* `descriptor_cid` (string) — CIDv1 (SPEC-200)
* `published_at_ms` (int64)
* `expires_at_ms` (int64)
* `sig` (bytes) — Ed25519 подпись

Правила валидации:

1. `service_id` **ДОЛЖЕН** совпадать с вычисленным по `owner_identity_id` и `service_name` (SPEC-001).
2. `descriptor_cid` **ДОЛЖЕН** быть CIDv1 строки.
3. `expires_at_ms - published_at_ms` **ДОЛЖНО** быть `<= record_max_ttl_ms`.

Подпись:

* Аналогично LeaseSet: подпись ключом `owner_identity_id` по canonical CBOR объекта без `sig`.

---

## 4) DHT операции (wire, фиксировано)

NetDB операции передаются как Envelope `envelope.v1` (SPEC-140) **между роутерами** (`to.peer_id`) с `type`:

* `netdb.find_node.v1`
* `netdb.find_value.v1`
* `netdb.store.v1`
* `netdb.reply.v1`

Payload всех netdb сообщений — canonical CBOR map.

### 4.1 `netdb.find_node.v1`

* `v`=1
* `key` (bytes) — 32 bytes, `dht_key` keyspace (сырой `sha2-256` digest)
* `want` (string) — `"routers"`

Ответ `netdb.reply.v1` содержит массив ближайших `peer_id` (см. 4.4).

### 4.2 `netdb.find_value.v1`

* `v`=1
* `key` (bytes) — 32 bytes, `dht_key` keyspace (сырой `sha2-256` digest)

Ответ:
* если значение найдено — `netdb.reply.v1` с `value` (bytes) = canonical CBOR record;
* иначе — `netdb.reply.v1` с `nodes` (array) ближайших.

### 4.3 `netdb.store.v1`

* `v`=1
* `value` (bytes) — canonical CBOR record (включая `sig`)

Получатель **ДОЛЖЕН**:
1. валидировать record (раздел 1/3);
2. вычислить ожидаемый `dht_key` (раздел 2) из `(type, address)` записи;
3. сохранить/обновить запись, если она новее (см. 4.5);
4. ответить `netdb.reply.v1` с `status`.

### 4.4 `netdb.reply.v1`

* `v`=1
* `status` (string) — `OK` | `REJECTED` | `NOT_FOUND`
* `error_code` (string, optional)
* `nodes` (array[string], optional) — список `peer_id` (не более `k`)
* `value` (bytes, optional) — canonical record bytes

Ровно одно из `nodes`/`value` **ДОЛЖНО** присутствовать при `status=OK`.

### 4.5 Правило “новее/лучше” (фиксировано)

Для записей одного типа и “адреса”:

* RouterInfo: адрес = `peer_id`
* LeaseSet: адрес = `service_id`
* ServiceHead: адрес = `service_id`

Узел **ДОЛЖЕН** предпочитать запись с большим `published_at_ms`.  
При равенстве **ДОЛЖЕН** предпочитать запись с большим `expires_at_ms`.

---

## 5) Anti-poisoning (фиксировано)

1. Узел **ДОЛЖЕН** rate-limit netdb операции по `peer_id` (лимиты из SPEC-500.params.anti_abuse.rate_limits).
2. Узел **ДОЛЖЕН** применять PoW к `netdb.store.v1` от untrusted источников по правилам SPEC-002 (anti-spam).
3. Узел **ДОЛЖЕН** иметь локальный “quarantine cache” для новых RouterInfo и **НЕ ДОЛЖЕН** немедленно использовать их для построения туннелей, пока:
   * не установлено транспортное соединение хотя бы один раз, или
   * RouterInfo не встречен от >=2 независимых пиров (netdb replies).

---

## 6) Ошибки (минимум)

* `ERR_NETDB_BAD_RECORD`
* `ERR_NETDB_SIG_INVALID`
* `ERR_NETDB_EXPIRED`
* `ERR_NETDB_TOO_LARGE`
* `ERR_NETDB_NOT_AUTHORIZED`
* `ERR_NETDB_POW_REQUIRED`
* `ERR_NETDB_POW_INVALID`

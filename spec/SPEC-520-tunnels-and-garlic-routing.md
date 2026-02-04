# SPEC-520: Туннели и garlic/onion маршрутизация (privacy-first)

**Статус:** Draft v2.0 (2026-02-02)  
**Зависимости:** SPEC-001, SPEC-010, SPEC-110, SPEC-140, SPEC-420, SPEC-500, SPEC-510  
**Назначение:** зафиксировать туннельную маршрутизацию (hop=3), протокол построения/ротации туннелей, формат туннельных сообщений и обязательную базовую “шумовую” политику.

---

## 1) Инварианты v2 (фиксировано)

1. В privacy-first профиле любые прикладные сообщения (`task.*`, `node.fetch.v1`) **ДОЛЖНЫ** доставляться через туннели.
2. Длина туннеля по умолчанию: `hop_count_default=3` (SPEC-500). Конфигурация **ДОЛЖНА** позволять менять `hop_count` в диапазоне `[2..5]`.
3. Туннели **ДОЛЖНЫ** ротироваться с периодом `rotation_ms=600_000` и иметь leases `lease_ttl_ms=600_000` (SPEC-500).
4. Узел **ДОЛЖЕН** поддерживать:
   * минимум 1 outbound-туннель,
   * минимум 1 inbound-туннель “для ответов” (если узел является клиентом/сервисом).
5. Transport-level адреса/peer_id **НЕ ДОЛЖНЫ** попадать в прикладной payload, предназначенный для удалённого сервиса/клиента, если это не требуется протоколом (анти-deanonymization).

---

## 2) Криптопримитивы (фиксировано)

* DH: X25519
* KDF: HKDF-SHA256
* AEAD hop-to-hop: XChaCha20-Poly1305
* E2E (garlic до сервиса): XChaCha20-Poly1305

---

## 3) Построение туннеля (fixed)

### 3.1 Выбор маршрута

Инициатор туннеля **ДОЛЖЕН** выбирать hops из валидных `router.info.v1` (SPEC-510) по правилам:

1. Все hops **ДОЛЖНЫ** иметь разные `peer_id`.
2. Первый hop (entry) **НЕ ДОЛЖЕН** быть локальным узлом.
3. Узел **НЕ ДОЛЖЕН** выбирать hop, который:
   * отсутствует в quarantine/verified списке (SPEC-510 anti-poisoning),
   * находится в локальном ban-листе.

### 3.2 Идентификатор туннеля

`tunnel_id` — 16 bytes случайных данных, уникальных в пределах `peer_id`+`ttl`.

### 3.3 Wire: `tunnel.build.v1`

`tunnel.build.v1` передаётся как Envelope `envelope.v1` (SPEC-140) по прямому transport между соседними hops (`to.peer_id`).

Payload CBOR map:

* `v`=1
* `direction` (string) — `"inbound"` | `"outbound"`
* `tunnel_id` (bytes[16])
* `hop_index` (uint) — индекс текущего hop (0..L-1)
* `ephemeral_pub` (bytes[32]) — X25519 ephemeral pub инициатора для данного hop
* `record` (bytes) — hop record, зашифрованный под `router.info.onion_pub` данного hop

Hop record (до шифрования) — CBOR map:

* `v`=1
* `next_peer_id` (string, optional) — отсутствует на последнем hop
* `next_tunnel_id` (bytes[16], optional) — id для следующего hop segment
* `expires_at_ms` (int64)
* `flags` (map):
  * `is_gateway` (bool)

Шифрование record:

1. `ss = X25519(ephemeral_priv, hop.onion_pub)`
2. `key = HKDF-SHA256(ss, salt="ardents.tunnel.build.v1", info=peer_id||tunnel_id)`
3. `aead = XChaCha20-Poly1305(key)`
4. `record = aead.Seal(nonce=zero24, aad=canonical_cbor(header_aad), plaintext=canonical_cbor(record_map))`

`header_aad` (CBOR map, фиксировано) = объект со следующими полями:

* `v`
* `direction`
* `tunnel_id`
* `hop_index`
* `ephemeral_pub`

Т.е. `header_aad` — это payload `tunnel.build.v1` **без** поля `record`.

Правило: `nonce` = 24 нулевых байта, поскольку `key` уже уникален на контекст (peer_id+tunnel_id+ss). Реализация **НЕ ДОЛЖНА** переиспользовать `key` для другого контекста.

### 3.4 Ответ: `tunnel.build.reply.v1`

Ответ передаётся как Envelope `envelope.v1` (`to.peer_id` инициатора).

Payload:

* `v`=1
* `tunnel_id` (bytes[16])
* `hop_index` (uint)
* `status` (string) — `OK` | `REJECTED`
* `error_code` (string, optional)

---

## 4) Передача данных (fixed)

### 4.1 Wire: `tunnel.data.v1`

`tunnel.data.v1` передаётся hop-to-hop как Envelope `envelope.v1` между соседними роутерами.

Payload:

* `v`=1
* `tunnel_id` (bytes[16])
* `seq` (uint64) — монотонный счётчик в рамках `(peer_id, tunnel_id)` (для replay protection)
* `ct` (bytes) — hop-to-hop ciphertext

`ct` шифруется XChaCha20-Poly1305 ключом hop-сегмента, полученным из build-рукопожатия.

Plaintext внутри `ct` (CBOR map):

* `v`=1
* `kind` (string) — `"forward"` | `"deliver"` | `"padding"`
* `next_tunnel_id` (bytes[16], optional) — для `"forward"`
* `inner` (bytes, optional) — для `"forward"`: следующий `tunnel.data.v1` (CBOR bytes)  
* `garlic` (bytes, optional) — для `"deliver"`: `garlic.msg.v1` (см. 4.2)

Ровно одно из `inner`/`garlic` **ДОЛЖНО** быть присутствующим в зависимости от `kind`.

### 4.2 `garlic.msg.v1` (end-to-end)

`garlic.msg.v1` — это end-to-end зашифрованный контейнер, доставляемый до сервиса по inbound-туннелю (lease).

Wire (bytes) внутри `tunnel.data.v1.plaintext.garlic`:

CBOR map:

* `v`=1
* `to_service_id` (string)
* `ephemeral_pub` (bytes[32]) — X25519 ephemeral pub отправителя
* `ct` (bytes) — ciphertext

Шифрование:

1. Отправитель находит `service.lease_set.v1` для `to_service_id` (SPEC-510).
2. `ss = X25519(ephemeral_priv, lease_set.enc_pub)`
3. `key = HKDF-SHA256(ss, salt="ardents.garlic.v1", info=to_service_id)`
4. `ct = XChaCha20-Poly1305(key).Seal(nonce=zero24, aad=canonical_cbor(header), plaintext=canonical_cbor(inner))`

`header` = map `{v,to_service_id,ephemeral_pub}`.

Inner (plaintext) — CBOR map:

* `v`=1
* `expires_at_ms` (int64)
* `cloves` (array[map]) — список “зубчиков”:
  * `kind` = `"envelope"`
  * `envelope` (bytes) — `envelope.v2` bytes (см. SPEC-550)

Узел-сервис **ДОЛЖЕН** отклонить garlic, если `expires_at_ms < now_ms`.

---

## 5) Padding policy `basic.v1` (фиксировано)

Цель: минимально усложнить traffic analysis без “протокольной магии”.

1. Любой роутер **ДОЛЖЕН** принимать `tunnel.data.v1` с `kind="padding"` и обрабатывать его как no-op.
2. Инициатор туннеля **ДОЛЖЕН** отправлять padding, если в туннеле нет реального трафика:
   * каждые `2000ms ± 500ms` (джиттер),
   * размер ciphertext **ДОЛЖЕН** быть выровнен в ближайший bucket из: `512, 1024, 2048, 4096, 8192, 16384, 32768`.
3. Роутер **НЕ ДОЛЖЕН** различать padding и данные на уровне transport логов (логировать можно только счётчики, без содержимого).

---

## 6) Ошибки (минимум)

* `ERR_TUNNEL_BUILD_REJECTED`
* `ERR_TUNNEL_BUILD_DECODE`
* `ERR_TUNNEL_BUILD_CRYPTO`
* `ERR_TUNNEL_DATA_REPLAY`
* `ERR_TUNNEL_DATA_DECODE`
* `ERR_GARLIC_EXPIRED`
* `ERR_GARLIC_DECRYPT`

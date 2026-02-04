# SPEC-530: Анонимные сервисы, публикация и directory-поиск (privacy-first)

**Статус:** Draft v2.0 (2026-02-02)  
**Зависимости:** SPEC-001, SPEC-010, SPEC-110, SPEC-120, SPEC-200, SPEC-210, SPEC-300, SPEC-310, SPEC-420, SPEC-500, SPEC-510, SPEC-520, SPEC-550  
**Назначение:** зафиксировать, как сервисы публикуются и обнаруживаются в privacy-first сети, как доставляются запросы/ответы без раскрытия адресов сервис-хоста.

---

## 1) Инварианты (фиксировано)

1. В privacy-first профиле сервис **НЕ ДОЛЖЕН** рекламировать транспортные адреса (`quic://...`) как часть service discovery.
2. Вход в сервис выполняется только через:
   * NetDB LeaseSet (`service.lease_set.v1`, SPEC-510),
   * туннельную доставку (`SPEC-520`).
3. Обновляемость discovery сервиса **ДОЛЖНА** обеспечиваться NetDB указателем `service.head.v1` (SPEC-510).

---

## 2) Публикация сервиса (fixed)

Сервисный хост (узел, который запускает сервис) **ДОЛЖЕН**:

1. Иметь Identity владельца (did:key, SPEC-001).
2. Сформировать Service Descriptor Node `service.descriptor.v2` по SPEC-300 и получить `descriptor_cid`.
3. Публиковать `service.head.v1` в NetDB:
   * `descriptor_cid` = текущий CID,
   * `published_at_ms` = now,
   * `expires_at_ms` = `now + 3600_000` (1 час).
4. Поддерживать хотя бы один inbound-туннель для сервиса и публиковать `service.lease_set.v1`:
   * `leases[*].expires_at_ms` = `now + lease_ttl_ms` (SPEC-500),
   * `expires_at_ms` LeaseSet = `min(leases[*].expires_at_ms)`.
5. Пере-публиковать `service.head.v1` и `service.lease_set.v1` не реже, чем раз в `lease_ttl_ms/2` (то есть раз в 5 минут по умолчанию).

---

## 3) Разрешение сервиса (service resolution, fixed)

Клиент, имея `service_id`, **ДОЛЖЕН**:

1. Получить `service.lease_set.v1` из NetDB (SPEC-510) и проверить подпись.
2. Получить `service.head.v1` из NetDB и проверить подпись.
3. Получить `service.descriptor.v2` по `descriptor_cid` через `node.fetch.v1` (SPEC-300/210) либо из локального кэша.

Если какой-либо шаг не удался, клиент **ДОЛЖЕН** откатиться на повторный запрос NetDB с backoff, не превышая:

* `alpha` параллельных запросов (SPEC-500),
* `replication` попыток по разным узлам.

---

## 4) Доставка запросов/ответов (fixed)

### 4.1 Запрос к сервису

1. Клиент выбирает один lease из `service.lease_set.v1`:
   * предпочтение lease с максимальным `expires_at_ms`,
   * `gateway_peer_id` не должен быть забанен/неверифицирован.
2. Клиент формирует `envelope.v2` (SPEC-550) с:
   * `to.service_id = <target service_id>`,
   * `reply_to.service_id = <client mailbox service_id>` (см. 4.2),
   * payload по типу запроса (например `task.request.v1`).
3. Клиент упаковывает `envelope.v2` в `garlic.msg.v1` (SPEC-520) и доставляет через свой outbound-туннель на `gateway_peer_id` указанного lease.

### 4.2 Ответ сервиса

Для получения ответов клиент **ДОЛЖЕН** иметь собственный “mailbox” сервис:

* `service_name` = `client.mailbox.v1`
* `service_id` вычисляется по SPEC-001 из identity клиента и имени сервиса.

Клиент **ДОЛЖЕН** публиковать LeaseSet этого mailbox сервиса в NetDB аналогично любому сервису.

Сервис, формируя ответ, **ДОЛЖЕН**:

1. взять `reply_to.service_id` из запроса;
2. разрешить LeaseSet клиента;
3. доставить ответ (обычный `envelope.v2`) через туннели.

---

## 5) Directory-поиск по возможностям (fixed)

Поиск сервисов “по мощностям/capabilities” реализуется не DHT-перебором, а отдельным сервисом **Directory Service**:

* `service_name` = `dir.query.v1`
* Сервис предоставляет query API поверх Tasks (SPEC-310) и возвращает **подписанные** результаты.
* Доверие к Directory Service определяется локально (Address Book, trusted identities).

**Ограничение:** Directory Service **ДОЛЖЕН** использоваться только если он явно подключён в локальной конфигурации. По умолчанию обращения к внешним каталогам отключены.

Directory Service **ДОЛЖЕН** индексировать только:

* Service descriptors, подписанные владельцами (SPEC-300),
* И с валидным `service.head.v1` в NetDB (SPEC-510).

Directory Service **ДОЛЖЕН** применять rate‑limit на входящие запросы `dir.query.v1`. При превышении лимита сервис **ДОЛЖЕН** вернуть `task.fail.v1` с `error_code=ERR_DIR_RATE_LIMITED`.

### 5.1 Протокол `dir.query.v1` (Tasks, fixed)

Directory Service принимает задачи `task.request.v1` (SPEC-310), где:

* `job_type` = `dir.query.v1`
* `input` (CBOR map):
  * `v` = 1
  * `query` (map):
    * `service_name_prefix` (string, optional) — например `ai.`; если отсутствует, фильтр не применяется
    * `requires` (array[string], optional) — список обязательных capability ключей (из service.descriptor)
    * `min_resources` (map, optional):
      * `cpu_cores` (uint, optional)
      * `ram_mb` (uint, optional)
  * `limit` (uint) — максимум результатов, **фиксировано:** `<= 50`

Directory Service **ДОЛЖЕН** вернуть `task.result.v1`, где `result_node_id` указывает на Content Node:

* `node.type` = `dir.query.result.v1`
* `body` (CBOR map):
  * `v` = 1
  * `query_hash` (bytes[32]) — `sha2-256(canonical_cbor(input))`
  * `issued_at_ms` (int64)
  * `expires_at_ms` (int64) — `<= issued_at_ms + 60_000`
  * `results` (array[map]):
    * `service_id` (string)
    * `owner_identity_id` (string)
    * `service_name` (string)
    * `capabilities_digest` (bytes[32]) — как в SPEC-110 (digest descriptor)
    * `score` (int) — чем больше, тем выше в выдаче (детерминированный скоринг)
* Подпись Node (и CID) обеспечивает неизменяемость и проверяемость результата (SPEC-200).

Детерминированный скоринг (фиксировано):

1. Базовый score = 0
2. +100 если `owner_identity_id` trusted в Address Book клиента
3. +10 за каждый совпавший `requires` capability
4. + (min(100, cpu_cores)) + (min(100, ram_mb/1024)) при наличии `min_resources` (если данные заявлены в descriptor)

Directory Service **ДОЛЖЕН** сортировать выдачу по `score desc`, затем по `service_id asc`.

---

## 6) Ошибки (минимум)

* `ERR_SVC_RESOLVE_LEASESET`
* `ERR_SVC_RESOLVE_HEAD`
* `ERR_SVC_DESCRIPTOR_FETCH`
* `ERR_SVC_DELIVERY_FAILED`
* `ERR_DIR_UNTRUSTED`
* `ERR_DIR_RATE_LIMITED`

# SPEC-210: Репликация контента и Providers

**Статус:** Approved v1.0 (2026-02-02)  
**Зависимости:** SPEC-001, SPEC-010, SPEC-140, SPEC-200  
**Назначение:** определить, как узлы объявляют хранение Content Nodes и как выполняется fetch.

---

## 1) Модель providers v1

Provider — peer, который хранит `node_id` и **ДОЛЖЕН** уметь отдать bytes Node по запросу.

Provider-объявления **НЕ ЯВЛЯЮТСЯ** источником доверия к данным: доверие возникает только после проверки CID и подписи Node.

---

## 2) ProviderRecord.v1

ProviderRecord — payload для `provider.announce.v1` (Envelope, см. SPEC-140):

* `v` = 1
* `node_id` (string, CIDv1)
* `provider_peer_id` (string)
* `ts_ms` (int64)
* `ttl_ms` (int64)

ProviderRecord используется как “hint”. Даже если record ложный, это не нарушает целостность контента.

---

## 2.1 `provider.announce.v1` (фиксировано)

`provider.announce.v1` — это Envelope (SPEC-140), где `payload` — ProviderRecord.v1 (CBOR).

---

## 3) Fetch протокол v1 (фиксировано)

### 3.1 `node.fetch.v1`

Запрос:

* `v` = 1
* `node_id` (string)

Ответ:

* `v` = 1
* `node_bytes` (bytes) — canonical `dag-cbor` bytes Node

### 3.2 Проверка

Получатель `node_bytes` **ДОЛЖЕН**:

1. вычислить CID и сравнить с запрошенным `node_id`;
2. выполнить проверку Node по SPEC-200.

---

## 4) Кэширование

Peer **ДОЛЖЕН** кэшировать успешно проверенные Node по `node_id`.

---

## 4.1 Выбор provider (минимальная стратегия v1)

При наличии нескольких provider для одного `node_id` узел **ДОЛЖЕН** использовать следующую стратегию:

1. **Лимит параллелизма:** одновременно пытаться fetch не более чем у 3 provider.
2. **Предпочтение recent:** provider с более свежим `ts_ms` ProviderRecord предпочтительнее.
3. **Предпочтение trusted:** если provider `peer_id` присутствует в Address Book с `trust=trusted`, он предпочтительнее.
4. **Учет успешности:** provider, у которых в текущей сессии были успешные fetch, предпочтительнее.

Если fetch у provider не удался, узел **ДОЛЖЕН** попробовать следующего кандидата (в пределах лимитов и TTL).

---

## 5) Ошибки (минимум)

* `ERR_NODE_NOT_FOUND`
* `ERR_NODE_CID_MISMATCH`
* `ERR_NODE_SIG_INVALID`
* `ERR_NODE_POLICY_DENY`

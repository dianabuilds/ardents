# SPEC-220: Политики доступа и шифрование

**Статус:** Approved v1.0 (2026-02-02)  
**Зависимости:** SPEC-001, SPEC-010, SPEC-200  
**Назначение:** зафиксировать, как ограничивается доступ к Node и как шифруется приватный контент.

---

## 1) Policy.v1 (фиксировано)

`policy` — CBOR map:

* `v` = 1
* `visibility` (string: `public` | `encrypted`)

Других режимов в v1 **НЕТ**.

---

## 2) Public nodes

Для `visibility=public` поля Node (включая `links` и `body`) доступны всем.

---

## 2.1 Навигация графа (фиксировано)

`enc.node.v1` **НЕ УЧАСТВУЕТ** в графовой навигации без расшифровки: до успешной расшифровки внутреннего `PrivateNodePayload.v1` невозможно видеть `links`, и клиент **НЕ ДОЛЖЕН** “угадывать”/эмулировать ссылки.

---

## 3) Encrypted nodes (фиксировано)

Для `visibility=encrypted` Node **ДОЛЖЕН** иметь:

* `type = enc.node.v1`
* `links` **ДОЛЖЕН** быть пустым массивом `[]`
* `body` — объект `EncryptedBody.v1` (см. ниже)

### 3.1 EncryptedBody.v1

`EncryptedBody` — CBOR map:

* `v` = 1
* `alg` = `xchacha20poly1305`
* `recipients` (array)
* `ciphertext` (bytes)
* `nonce` (bytes, длина 24)

#### Recipient.v1

Элемент `recipients` — CBOR map:

* `identity_id` (string)
* `sealed_key` (bytes) — “запечатанный” content key для конкретного получателя

Канонизация recipients (фиксировано):

* перед сериализацией `EncryptedBody` массив `recipients` **ДОЛЖЕН** быть отсортирован лексикографически по `identity_id` (UTF-8 bytes).

### 3.2 Ключи и шифрование

* Content key: случайные 32 bytes.
* Шифрование `ciphertext`:
  * plaintext — canonical `dag-cbor` объекта `PrivateNodePayload.v1`
  * алгоритм — XChaCha20-Poly1305

`PrivateNodePayload.v1` (внутри ciphertext):

* `v` = 1
* `type` (string) — исходный “внутренний” тип (например `doc.note.v1`)
* `links` (array of Link.v1)
* `body` (any)

Запечатывание content key:

* `sealed_key` строится как sealed-box к получателю (Identity) с использованием X25519, полученного из Ed25519 публичного ключа (см. SPEC-130).

---

## 4) Ошибки (минимум)

* `ERR_ENC_UNSUPPORTED`
* `ERR_ENC_NO_RECIPIENT`
* `ERR_ENC_DECRYPT_FAILED`

# SPEC-125: Address Book Bundles Workflow (v1)

**Статус:** Draft (2026-02-04)  
**Зависимости:** SPEC-120, SPEC-200, SPEC-420  
**Назначение:** описать управляемый workflow импорта/экспорта address book bundles без введения глобального реестра.

---

## 1) Scope / Out-of-scope

### Scope

* Импорт/экспорт bundles `bundle.addressbook.v1` (SPEC-120/200).
* Требования к доверенным источникам и политике применения.
* UX-поток в CLI/клиентах (минимум).
* Наблюдаемость: события и ошибки.

### Out-of-scope

* Глобальный реестр имён/поиск по сети.
* Автоматическая агрегация без подтверждения пользователя.

---

## 2) Термины

* **Bundle** — Content Node типа `bundle.addressbook.v1`.
* **Source identity** — identity, подписавшая bundle.
* **Trusted identity** — identity, помеченная как trusted в локальном address book.

---

## 3) Требования (нормативные)

1) Клиент **НЕ ДОЛЖЕН** импортировать bundle, если `source identity` не является trusted.  
2) Клиент **ДОЛЖЕН** валидировать подпись bundle и тип `bundle.addressbook.v1`.  
3) Клиент **ДОЛЖЕН** записывать импортированные записи с `source=imported`.  
4) Клиент **ДОЛЖЕН** поддерживать `expires_at_ms` для imported записей.  
5) Клиент **НЕ ДОЛЖЕН** автоматически включать глобальный поиск/реестр.

---

## 4) Workflow импорта (минимум)

1) Пользователь выбирает bundle (файл/Node).  
2) Клиент проверяет подпись и trusted identity.  
3) Клиент применяет записи в локальный address book.  
4) Клиент фиксирует событие `addressbook.imported` с метаданными:
   * `source_identity`
   * `entries_added`
   * `entries_skipped`

---

## 5) Workflow экспорта (минимум)

1) Пользователь выбирает набор записей (по умолчанию все `source=self`).  
2) Клиент формирует `bundle.addressbook.v1` и подписывает его identity.  
3) Клиент сохраняет bundle в файл или публикует как Node.

---

## 6) Ошибки (минимум)

* `ERR_ADDRESSBOOK_BUNDLE_INVALID`
* `ERR_IMPORT_UNTRUSTED_SOURCE`
* `ERR_ALIAS_CONFLICT`

---

## 7) Наблюдаемость

* События:
  * `addressbook.imported`
  * `addressbook.exported`
* Логи должны содержать `source_identity` и количество примененных/пропущенных записей.

---

## 8) Совместимость

* SPEC совместим с SPEC-120/200 и не вводит изменений wire-форматов.


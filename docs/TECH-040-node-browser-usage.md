# TECH-040: Node Browser (CLI) — usage

**Статус:** Done (2026-02-02)  
**Цель:** практические сценарии использования `cmd/node` по SPEC‑410.

---

## 0) Что делает `cmd/node`

`cmd/node` — минимальный CLI для просмотра Content Nodes:

* выполняет `fetch` ноды по `node_id` (через runtime/transport);
* проверяет CID/подпись;
* выводит поля узла и ссылки;
* умеет раскрывать историю (`prev`/`supersedes`);
* может попытаться расшифровать `enc.node.v1` (если есть ключ).

По умолчанию используется `config/node.json` (создаётся при первом запуске).

---

## 1) Просмотр ноды

```
go run ./cmd/node get --id <cid>
```

По умолчанию `cmd/node` использует XDG/дефолтные директории узла. Для portable режима:

```
go run ./cmd/node get --home ./ardents-home --id <cid>
```

Выводит:

* `type`, `owner`, `created_at_ms`, `policy`
* `links`, `prev`, `supersedes`
* `verify_status` (ok/invalid)

---

## 2) Просмотр истории (prev/supersedes)

```
go run ./cmd/node get --id <cid> --history-depth 5
```

История выводится в поле `history` (BFS по `prev`/`supersedes`, глубина ограничена).

---

## 3) Encrypted nodes

```
go run ./cmd/node get --id <cid> --decrypt
```

Если есть ключ получателя (локальная identity), `encrypted=false`, иначе остаётся `encrypted=true`.

---

## 4) Типовые use‑cases

1) **Диагностика валидности узла**
   * проверить `verify_status` и `owner`;
   * быстро определить “битые” или подменённые узлы.

2) **Аудит версий контента**
   * `--history-depth` показывает цепочку `prev`/`supersedes`;
   * удобно для проверки корректного обновления ноды.

3) **Проверка доступности зашифрованного контента**
   * `--decrypt` показывает, может ли локальная identity открыть `enc.node.v1`;
   * если расшифровка невозможна — это нормальный результат (policy.visibility=encrypted).

4) **Разбор проблем с fetch**
   * если нода не находится или verify_status=invalid — это сигнал к проверке providers/маршрута.

---

## 5) Замечания по ограничениям

* `cmd/node` не изменяет граф и не публикует ноды — только чтение/проверка.
* Если сеть недоступна, `fetch` может завершиться ошибкой (см. вывод `error:`).

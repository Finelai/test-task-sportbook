# Mini Sportsbook Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Запустить мини-спортсбук, где один игрок пополняет условный баланс, видит обновления линии и ставит по зафиксированной цене.

**Architecture:** Три независимых Go-процесса с отдельными БД и ролями PostgreSQL. Ставки используют gRPC для подтверждения предложения и управления резервом, а Vue SPA получает HTTP/JSON и SSE через Nginx.

**Tech Stack:** Go, net/http, grpc-go, Protobuf, pgx, sqlc, Goose, PostgreSQL, Vue 3, TypeScript, Vite, Tailwind CSS, Nginx, Docker Compose, OpenAPI и Swagger UI, Grafana, Prometheus, Lighthouse CI.

**Spec:** [Согласованное ТЗ](../specs/2026-11-25-mini-sportsbook-design.md)

## Global Constraints

- Один фиксированный серверный `PlayerID`, начальный баланс 0. Условные деньги с двумя дробными знаками. Суммы в хранилище и gRPC — `int64` в минимальных единицах, суммы в HTTP — десятичные строки. Коэффициенты внутри сервисов — целые сотые, в HTTP — десятичные строки.
- Два независимых источника с равными весами, 8 матчей, П1 / Х / П2, оверраунд 5% до округления. Предложение действует 60 секунд при абсолютной разнице `≤ 1,00`, пока рынок открыт. Одно активное предложение на игрока и исход.
- Обработка ставки сохраняет этап перед межсервисным вызовом. Ставка становится принятой после подтверждённого списания зарезервированной суммы.
- Vue SPA без SSR, FSD, Tailwind. `docker compose up` запускает стенд. Полная сборка измеряет FCP, LCP и размер начальных JS/CSS, сохраняет отчёт в БД и артефакте.
- Соблюдать DDD-границы, TDD и интеграционные проверки. Каждый пункт с тестами выполняется в цикле RED → GREEN → REFACTOR. В CI ошибки и предупреждения линтинга блокируют сборку.
- Рабочая папка сейчас без Git. Перед первым коммитом исполнитель инициализирует репозиторий, если к тому времени доступного Git-репозитория ещё нет.

## Review Focus

| Сбой или граничный ввод | Ожидаемый результат | Задача |
|---|---|---|
| Ответ на списание потерян, процесс Ставок перезапущен | Одна принятая ставка и одно списание | 5 |
| Два запроса на выдачу предложения и одновременное изменение цены | Одно активное предложение, решение по заявке сохраняется атомарно | 3 |
| Новый снимок появляется в момент подключения SSE | Клиент получает снимок и все более новые версии | 3 |
| Два резерва претендуют на один остаток, повтор первого приходит после пополнения | Один резерв, прежний отказ по второму `BetID` | 4 |
| Сборка повторно сохраняет отчёт либо не получает валидный LCP | Одна запись на сборку, неверный отчёт останавливает сборку | 7 |

## Карта работ

| Путь | Ответственность |
|---|---|
| `contracts/openapi/v1.yaml`, `contracts/proto/*/v1` | Публичный HTTP/SSE, внутренние gRPC-контракты |
| `services/{line,wallet,bets}/cmd/server`, `internal/{domain,app,postgres,httpapi,rpc}` внутри сервисов | Запуск, домен, прикладные сценарии и адаптеры |
| `services/{line,wallet,bets}/migrations`, `services/{line,wallet,bets}/queries` | Схемы и запросы под отдельными ролями |
| `apps/web/src/{app,pages,features,entities,shared}` | FSD-слои SPA |
| `infra/{nginx,postgres,prometheus,grafana}`, `compose.yaml` | Запуск и наблюдаемость стенда |
| `tools/perf`, `apps/web/e2e`, `.github/workflows/check.yml` | Отчёт сборки, пользовательские тесты и CI |

## Порядок применения Superpowers

`superpowers:using-superpowers` и `superpowers:brainstorming` применены при подготовке ТЗ, `superpowers:writing-plans` — при составлении этого плана. Режим исполнения задаёт `superpowers:executing-plans` или `superpowers:subagent-driven-development`. Перед поведением применить `superpowers:test-driven-development`, перед заявлением результата и коммитом — `superpowers:verification-before-completion`. После задач 5 и 8 провести проверку по `superpowers:requesting-code-review`. При сбое использовать `superpowers:systematic-debugging`, при работе с замечаниями — `superpowers:receiving-code-review`.

Задачи 3 и 4 независимы после задачи 2. При выборе параллельного режима к ним применим `superpowers:dispatching-parallel-agents`. Если для работы нужна изоляция, применить `superpowers:using-git-worktrees`. При интеграции готовой ветки применить `superpowers:finishing-a-development-branch`.

## Задача 1. Каркас и проверки

**Файлы:** `go.mod`, `.golangci.yml`, `Makefile`, `compose.yaml`, `infra/postgres/init.sh`, `apps/web/{package.json,tsconfig.json,eslint.config.js,.prettierrc.json,vite.config.ts}`, `.github/workflows/check.yml`, `tools/checks/architecture_test.go`.

**Выход:** цели `make lint`, `make generate`, `make test`, `make test-integration` и `make test-e2e`. Последнюю подключить при появлении набора в задаче 6. `make test-integration` запускает тесты с тегом `integration` и Testcontainers.

- [x] Добавить проверки границ. Импорт исходников одного сервиса из другого и импорт адаптера из домена отклоняются. Во фронтенде импорт вверх по FSD и импорт чужого слайса мимо `index.ts` отклоняются. Проверить правила на положительных и отрицательных фикстурах.
- [x] Запустить проверки на отрицательных фикстурах до включения правил. Ожидание — тест падает, потому что запрещённый импорт пока проходит.
- [x] Настроить один Go-модуль, `gofmt`, `golangci-lint` с `errcheck`, `govet`, `staticcheck`, `ineffassign` и `unused`. Зафиксировать версии инструментов и способ их запуска.
- [x] Настроить Vue/Vite, Tailwind, TypeScript `strict`, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, `vue-tsc --noEmit`, ESLint с типовой информацией и `--max-warnings=0`, Prettier `--check`.
- [x] Поднять PostgreSQL с базами `line`, `wallet`, `bets`, `build_metrics` и отдельными ролями. Проверить, что роль сервиса подключается к своей базе и получает отказ при подключении к чужой. Отозвать право `CONNECT` через `PUBLIC`. Для БД настроить healthcheck Compose.
- [x] Запустить `make lint`, `make test` и `docker compose config -q`. Ожидание — код 0, отрицательные фикстуры отклонены предусмотренным правилом.
- [x] Создать коммит `chore: scaffold sportsbook workspace`.

## Задача 2. Контракты и Swagger

**Файлы:** `contracts/openapi/v1.yaml`, `contracts/proto/line/v1/line.proto`, `contracts/proto/wallet/v1/wallet.proto`, `contracts/contract_test.go`, `infra/nginx/default.conf`, `compose.yaml`.

**Выход:** HTTP использует префикс `/api/v1`. POST на пополнение и ставку принимает заголовок `Idempotency-Key`. Даты в HTTP — UTC RFC 3339. Swagger UI доступен на `/docs/`, YAML — на `/docs/openapi.yaml`. У каждого сервиса есть внутренние `/healthz`, `/readyz` и впоследствии `/metrics`. Готовность требует успешных миграций и доступности собственной БД.

| Маршрут | Вход и успешный результат |
|---|---|
| `GET /line`, `GET /line/events` | Снимок `{version,matches}`. SSE выдаёт `snapshot`, затем `match_updated` с `{version,match}`. Матч включает `matchId`, названия команд, `open` и три котировки с `outcome` и `odds` |
| `POST /line/offers` | `{matchId,outcome}` → 200 `{offerId,matchId,outcome,odds,expiresAt}`. Исходы — `HOME`, `DRAW`, `AWAY` |
| `POST /bets`, `GET /bets/{betId}` | POST `{offerId,odds,amount}` → 202. GET → 200. Оба возвращают `{betId,status,amount,odds}`; у отказа есть `reason` |
| `GET /wallet`, `POST /wallet/credits` | GET → 200 `{available,reserved}`. POST `{amount}` → 201 `{operationId,available,reserved}` |

Общие ошибки `{code,message}`. Коды — `invalid_request` (400), `not_found` (404), `idempotency_conflict` и `market_closed` (409), `unavailable` (503). Отказы ставки в `reason` — `offer_expired`, `odds_changed`, `offer_unavailable`, `offer_mismatch`, `market_closed`, `insufficient_funds`. Промежуточная недоступность не переводит ставку в `rejected`.

Внутренние операции:

- `LineService.ConfirmOffer(ConfirmOfferRequest) → ConfirmOfferResponse`. Вход содержит `bet_id`, `player_id`, `offer_id`, `expected_odds_hundredths`. Успех содержит матч, исход и подтверждённые сотые. Бизнес-отказ содержит причину из списка для Линии. Сетевые и временные ошибки передаются статусом gRPC.
- `WalletService.Reserve(ReserveRequest) → ReserveResponse` с `bet_id`, `player_id` и `amount_minor`. Результат — `RESERVED` или `INSUFFICIENT_FUNDS`. `Capture(ReserveKey) → CaptureResponse` и `Release(ReserveKey) → ReleaseResponse`, где `ReserveKey` содержит `bet_id`, `player_id`. Успех — `CAPTURED` или `RELEASED` соответственно. Конфликт параметров или противоположное конечное состояние — типизированный отказ.

- [ ] Добавить `TestPublishedContracts`. Проверить список путей, коды ответов, обязательные поля, `text/event-stream` и четыре gRPC-операции. Запуск до описания контрактов должен показать их отсутствие.
- [ ] Описать API, добавить генерацию Protobuf и TypeScript-типов OpenAPI. CI повторяет генерацию и проверяет отсутствие изменений в сгенерированных файлах.
- [ ] Настроить маршруты Nginx и Swagger UI из локальных ресурсов Compose. Для SSE отключить буферизацию и кэширование. Оставить backend-порты внутри Compose.
- [ ] Запустить `go test ./contracts -run TestPublishedContracts -count=1`, `make generate` и `docker compose config -q`.
- [ ] Создать коммит `feat: define public and internal API contracts`.

## Задача 3. Линия, SSE и гарантия цены

**Файлы:** `services/line/internal/domain/pricing.go`, `services/line/internal/app/{line.go,offers.go}`, `services/line/internal/httpapi/events.go`, `services/line/internal/rpc/confirm_offer.go`, `services/line/migrations/001_line.sql`, `services/line/queries/line.sql`. Тесты рядом с кодом и в `services/line/integration/line_test.go`.

**Выход:** `Price(sources [2][3]float64) (Odds1X2, error)`, HTTP и `ConfirmOffer` из задачи 2. Два отдельных генератора обновляют котировки раз в 5 секунд. Источник считается просроченным через 15 секунд. Для тестов доступны управляемые часы и фиксированные seed.

- [ ] Добавить тесты `TestPrice` и `TestStaleSourceClosesMarket`. Для двух наборов `[4,4,4]` получить три округлённых `2,86` и сумму 1,05 до округления с допуском `1e-9`. Изменение любого источника меняет цену. После потери источника рынок закрывается на границе 15 секунд и открывается после восстановления обоих.
- [ ] Добавить `TestOfferBoundaries`, `TestOfferIssuedOnce` и `TestConfirmOnce`. Проверить `2,10 → 3,10` и `2,10 → 1,10`, отклонить отклонение `1,01` в обе стороны, истечение ровно через 60 секунд, закрытый рынок, подмену цены и другого игрока. Параллельные выдачи получают один `OfferID`. Другой `BetID` получает отказ по использованному предложению, исходный повтор возвращает прежний результат после истечения срока.
- [ ] Добавить `TestSubscribeWithoutGap`. При обновлении вокруг момента подписки события клиентов образуют непрерывную последовательность после снимка. После переподключения клиент получает полный актуальный снимок.
- [ ] Запустить `go test ./services/line/... -tags=integration -count=1` и зафиксировать RED.
- [ ] Реализовать расчёт, генераторы и сохранение опубликованных цен. Проверку срока и актуальной цены, выдачу и подтверждение упорядочить транзакционной блокировкой рынка и записи предложения. Хранить решение и отпечаток запроса по `BetID`. При повторе проверять отпечаток и возвращать сохранённое решение.
- [ ] Реализовать SSE с согласованным снимком и подпиской. Каждое опубликованное обновление матча получает следующий номер версии. При переполнении очереди закрывать соединение, чтобы клиент восстановился по полному снимку.
- [ ] Повторить тесты, запустить `go test -race ./services/line/...` и `make lint`. Ожидание — код 0.
- [ ] Создать коммит `feat: publish line and guarantee offered odds`.

## Задача 4. Кошелёк и события резерва

**Файлы:** `services/wallet/internal/domain/wallet.go`, `services/wallet/internal/app/operations.go`, `services/wallet/internal/httpapi/wallet.go`, `services/wallet/internal/rpc/reserves.go`, `services/wallet/migrations/001_wallet.sql`, `services/wallet/queries/wallet.sql`, `services/wallet/integration/wallet_test.go`.

**Выход:** операции `Credit`, `Reserve`, `Capture`, `Release` и чтение остатков. Исходное состояние кошелька — `available=0`, `reserved=0`. gRPC и HTTP следуют задаче 2.

- [ ] Добавить `TestCreditIdempotent`, `TestInvalidAmount` и `TestConcurrentReserve`. Два POST `10.00` с одним ключом дают ровно одно зачисление. С тем же ключом `11.00` даёт 409. `0`, `-1`, `1.001` и переполнение отклоняются. Из остатка `10.00` две параллельные операции по `7.00` дают один резерв. После пополнения повтор отказавшего `BetID` возвращает первоначальный отказ.
- [ ] Добавить `TestReservationFinalState`. Два параллельных вызова Capture и Release дают ровно один конечный переход. Повтор победившего вызова возвращает прежний успех. Журнал содержит одну операцию завершения, доступный и зарезервированный остатки неотрицательны.
- [ ] Запустить `go test ./services/wallet/... -tags=integration -count=1` и зафиксировать RED.
- [ ] Реализовать разбор суммы целыми числами, атомарное изменение остатков с блокировкой записи кошелька, журнал и ключи повторов. Хранить и бизнес-отказы резервирования, проверять неизменность параметров по `BetID`. Проверить переполнение суммы доступного и зарезервированного остатков при зачислении.
- [ ] Повторить интеграционные тесты и `make lint`.
- [ ] Создать коммит `feat: implement wallet credits and reservations`.

## Задача 5. Приём ставки и восстановление

**Файлы:** `services/bets/internal/domain/bet.go`, `services/bets/internal/app/{submit.go,worker.go}`, `services/bets/internal/httpapi/bets.go`, `services/bets/internal/rpc/clients.go`, `services/bets/migrations/001_bets.sql`, `services/bets/queries/bets.sql`, `services/bets/integration/process_test.go`.

**Вход:** реальные gRPC-адаптеры задач 3 и 4. **Выход:** HTTP POST/GET из задачи 2. Внутренние этапы `confirm_offer`, `reserve`, `capture`. Этапы до результата отображаются как `pending`.

- [ ] Добавить `TestBetIdempotency` и `TestRejectWithoutCharge`. Повтор одного запроса возвращает прежний `betId`, изменение суммы или предложения даёт 409. Устаревшее предложение отклоняется до обращения к Кошельку. При недостатке средств состояние `rejected` и списаний нет.
- [ ] Добавить `TestRecoverAfterCapture`. На реальном PostgreSQL и с реальными gRPC-серверами оборвать ответ сразу после фиксации Capture. Перезапустить обработчик, повторить заявку и проверить тот же `betId`, статус `accepted`, исходную цену предложения и ровно одно списание. Аналогично проверить потерю ответа на ConfirmOffer и Reserve.
- [ ] Запустить `go test ./services/bets/... -tags=integration -count=1` и зафиксировать RED.
- [ ] Реализовать постановку заявки в БД и worker с сохранёнными этапами. Для получения работы использовать аренду строки на 15 секунд, дедлайн gRPC 2 секунды и условную запись результата с токеном аренды. При временной ошибке повторять тот же вызов через 1 секунду с экспоненциальным увеличением до 30 секунд. При исчерпании аренды подхватывать работу с последнего сохранённого этапа.
- [ ] Перед приёмом фиксировать подтверждённую цену. Записывать `accepted` только после Capture. Для пользовательского сценария после резерва выполняется Capture. Release остаётся внутренней операцией, проверенной в задаче 4.
- [ ] Повторить интеграционные тесты. Провести `superpowers:requesting-code-review` по переходам состояний, блокировкам и границам идемпотентности.
- [ ] Создать коммит `feat: recover bet placement across services`.

## Задача 6. SPA и сквозные сценарии

**Файлы:** `apps/web/src/pages/sportsbook/ui/SportsbookPage.vue`, `apps/web/src/entities/{line,bet,wallet}/index.ts`, `apps/web/src/features/{select-outcome,place-bet,top-up-wallet}/index.ts`, `apps/web/src/shared/api/http.ts`, `apps/web/e2e/sportsbook.spec.ts`, `apps/web/playwright.config.ts`.

**Вход:** контракт задачи 2 и сервисы задач 3–5. **Выход:** один экран с линией, формой ставки, состоянием соединения, кошельком и условным пополнением.

- [ ] Добавить тесты компонентов. Поток меняет витрину, сохраняет выбранную цену в форме и заменяет снимок после переподключения. Отказ цены требует отдельного подтверждения. Повтор после неопределённой ошибки использует тот же ключ. После принятой ставки и зачисления интерфейс перечитывает баланс.
- [ ] Добавить сценарии Playwright `tops up and places a bet` и `reconnects and requests a new offer`. Первый зачисляет `10.00`, ставит `3.00` и ожидает `accepted` и `7.00`. Второй проверяет состояния разрыва и нового предложения на управляемой тестовой линии.
- [ ] Запустить `npm --prefix apps/web run test:unit` до реализации, зафиксировать RED.
- [ ] Реализовать экран с Tailwind и Vue composables. Обновлять `pending` через GET ставки раз в секунду, прекращать опрос по конечному статусу. При сетевой ошибке показать повтор и сохранить исходную сумму, цену и ключ операции. В тестовом стенде управлять часами и генераторами через тестовую конфигурацию, без служебного маршрута в пользовательском API.
- [ ] Запустить `make lint`, `npm --prefix apps/web run test:unit` и `make test-e2e`. Ожидание — оба сквозных сценария проходят на Compose.
- [ ] Создать коммит `feat: build sportsbook SPA`.

## Задача 7. Метрики сервисов и отчёт сборки

**Файлы:** `infra/prometheus/prometheus.yml`, `infra/grafana/{provisioning,dashboards}`, `tools/perf/{report.mjs,report.test.mjs,lighthouserc.cjs}`, `infra/postgres/002_build_metrics.sql`, `Makefile`, `compose.yaml`.

**Выход:** `make build` собирает production SPA, поднимает стенд с детерминированными данными, ждёт готовности HTTP и БД, запускает Lighthouse CI и записывает отчёт. `make import-perf-report REPORT=...` повторно загружает артефакт в настроенную PostgreSQL.

- [ ] Добавить `TestMetricsExposure` в интеграционные наборы трёх сервисов. Проверить HTTP/gRPC счётчики и длительность на каждом поддерживаемом транспорте. Для Линии проверить SSE-подключения и свежесть источников, для Ставок — число и максимальный возраст ожидающих ставок, для Кошелька — результаты резервов и пополнений. Проверить отсутствие `request_id` и `bet_id` среди меток.
- [ ] Добавить тесты `report.test.mjs`. Статические импорты учитываются один раз, динамический чанк исключается, пустой FCP или LCP проваливает шаг. Три значения `[120,300,180]` дают медиану `180`. Повтор импорта по одному `build_id` оставляет одну строку в PostgreSQL.
- [ ] Запустить `node --test tools/perf/report.test.mjs` и `go test ./services/... -tags=integration -run TestMetricsExposure -count=1`. Зафиксировать RED.
- [ ] Подключить Prometheus и структурированные логи с передачей `request_id` и `bet_id` через gRPC metadata. Настроить в Grafana дашборды сервисов и сборок. Для PostgreSQL создать отдельную роль Grafana с правом чтения отчётов.
- [ ] Реализовать отчёт `{buildId,revision,recordedAt,profile,fcpMs,lcpMs,initialJsBytes,initialCssBytes,initialJsGzipBytes,initialCssGzipBytes}`. Включить manifest Vite, пройти граф статических импортов от входов HTML, посчитать байты файлов и gzip с фиксированным уровнем 9. Для FCP и LCP брать отдельно медиану трёх холодных запусков Lighthouse в фиксированном мобильном профиле.
- [ ] Записать JSON-артефакт и выполнить upsert по `build_id`. Локально использовать постоянный том. В CI передавать настроенную БД истории или временную БД задания, и публиковать JSON-артефакт для последующего импорта. Ошибка измерения или записи возвращает ненулевой код.
- [ ] Запустить `make build`. Проверить запись в PostgreSQL по `build_id`, совпадение всех значений с артефактом, ответы Prometheus и наличие данных в дашбордах Grafana через API.
- [ ] Создать коммит `feat: publish runtime and build metrics`.

## Задача 8. Приёмка и README

**Файлы:** `README.md`, `.github/workflows/check.yml`, `Makefile`, `compose.yaml`.

- [ ] Запустить `docker compose up --build --wait` на чистом отдельном проекте Compose. Проверить SPA, здоровье трёх сервисов, наличие Swagger UI и доступность Grafana.
- [ ] Прогнать `make lint`, `make test`, `make test-integration`, `make test-e2e` и `make build`. Проверить, что CI вызывает тот же набор команд, сохраняет отчёт и отклоняет расхождение сгенерированных контрактов.
- [ ] Описать запуск, устройство линии и ставки, условные деньги, проверку метрик, оставшиеся ограничения и фактический вклад пользователя и ИИ. Указать фактически затраченное время. Ссылка на репозиторий берётся с согласованного remote после публикации, если она отдельно разрешена.
- [ ] Провести итоговое ревью по `superpowers:requesting-code-review`. Сверить результаты с шестью критериями приёмки ТЗ.
- [ ] Создать коммит `docs: document sportsbook operation and acceptance`.

## Решения уровня реализации

План задаёт один корневой Go-модуль, npm и GitHub Actions. Выбраны интервалы источников 5 и 15 секунд, опрос ставки раз в секунду, параметры аренды и повторов в задаче 5. Эти параметры должны быть конфигурируемыми на тестовом стенде. Версии зависимостей и образов, точные имена метрик и оформление дашбордов фиксируются в соответствующих задачах.

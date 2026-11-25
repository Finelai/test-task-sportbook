# Wallet

Wallet предоставляет `GET /api/v1/wallet` и `POST /api/v1/wallet/credits`. HTTP использует фиксированного игрока `player-1`; при первом чтении кошелёк создаётся с нулевыми остатками. Данные хранятся в собственной базе PostgreSQL.

Для ручной проверки запустите отдельный проект Compose с новым томом:

```sh
docker compose -p wallet-manual --profile wallet-check up --build -d
```

Другие профили, занимающие порт 8080, необходимо остановить. Откройте [Swagger UI](http://localhost:8080/docs/) и выполните `GET /api/v1/wallet` через **Try it out → Execute**. Ожидаемый ответ: `available: "0.00"`, `reserved: "0.00"`. Повторите запрос после `docker compose -p wallet-manual --profile wallet-check restart wallet`. `GET /api/v1/line` должен возвращать восемь матчей реального сервиса Line. Ставки пока направляются в контрактную заглушку.

Сохраните проект и том для следующих этапов. Для повторной проверки с чистыми данными используйте другое имя проекта Compose; не удаляйте существующий том.

Проверки:

```sh
go test ./services/wallet/...
go test -tags=integration ./services/wallet/... -count=1
docker compose --profile wallet-check config -q
```

Интеграционные тесты требуют доступного Docker.

## Проверка пополнения

Пересоберите существующий проект `wallet-manual` той же командой `docker compose -p wallet-manual --profile wallet-check up --build -d`. В Swagger выполните `POST /api/v1/wallet/credits` с заголовком `Idempotency-Key: manual-credit-1` и телом `{"amount":"10.00"}`. Повтор запроса возвращает тот же `operationId` и баланс `10.00`. Повтор с суммой `11.00` возвращает `409`, а запрос с новым ключом и суммой `1.001` — `400`. Проверьте баланс и восемь матчей Line, затем перезапустите Wallet и убедитесь, что баланс сохранился. Маршрут пополнения направлен в Wallet, ставки остаются на контрактной заглушке.

## Проверка резервирования

Пересоберите существующий стенд командой `docker compose -p wallet-manual --profile wallet-check up --build -d`. После пополнения на `10.00` выполните:

```sh
grpcurl -plaintext -import-path contracts/proto -proto wallet/v1/wallet.proto \
  -d '{"betId":"manual-bet-1","playerId":"player-1","amountMinor":"300"}' \
  localhost:9093 wallet.v1.WalletService/Reserve
```

Ожидается `RESERVED`. Повтор вызова сохраняет результат, а баланс в Swagger равен `available: "7.00"`, `reserved: "3.00"`. Повтор с `amountMinor: "400"` возвращает `PARAMETER_CONFLICT`. Сохраните стенд и резерв для следующего этапа. gRPC доступен на `127.0.0.1:9093` только в профиле `wallet-check`.

# Wallet

На первом этапе Wallet предоставляет `GET /api/v1/wallet`. HTTP использует фиксированного игрока `player-1`; при первом чтении кошелёк создаётся с нулевыми остатками. Данные хранятся в собственной базе PostgreSQL.

Для ручной проверки запустите отдельный проект Compose с новым томом:

```sh
docker compose -p wallet-manual --profile wallet-check up --build -d
```

Другие профили, занимающие порт 8080, необходимо остановить. Откройте [Swagger UI](http://localhost:8080/docs/) и выполните `GET /api/v1/wallet` через **Try it out → Execute**. Ожидаемый ответ: `available: "0.00"`, `reserved: "0.00"`. Повторите запрос после `docker compose -p wallet-manual --profile wallet-check restart wallet`. `GET /api/v1/line` должен возвращать восемь матчей реального сервиса Line. Пополнение и ставки пока направляются в контрактную заглушку.

Сохраните проект и том для следующих этапов. Для повторной проверки с чистыми данными используйте другое имя проекта Compose; не удаляйте существующий том.

Проверки:

```sh
go test ./services/wallet/...
go test -tags=integration ./services/wallet/... -count=1
docker compose --profile wallet-check config -q
```

Интеграционные тесты требуют доступного Docker.

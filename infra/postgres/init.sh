#!/bin/sh
set -eu

for database in line wallet bets build_metrics; do
    case "$database" in
        line) password=${LINE_DB_PASSWORD:-line} ;;
        wallet) password=${WALLET_DB_PASSWORD:-wallet} ;;
        bets) password=${BETS_DB_PASSWORD:-bets} ;;
        build_metrics) password=${BUILD_METRICS_DB_PASSWORD:-build_metrics} ;;
    esac

    SPORTSBOOK_DB_PASSWORD="$password" psql -X --set=ON_ERROR_STOP=1 \
        --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
        --set=database="$database" <<'SQL'
\getenv password SPORTSBOOK_DB_PASSWORD
CREATE ROLE :"database" LOGIN PASSWORD :'password';
CREATE DATABASE :"database" OWNER :"database";
REVOKE CONNECT ON DATABASE :"database" FROM PUBLIC;
SQL
done

psql -X --set=ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<'SQL'
REVOKE CONNECT ON DATABASE postgres, template0, template1 FROM PUBLIC;
SQL

#!/bin/sh
# Compose service "init": prepares the database and exits. Safe to run on every `docker compose up`:
# Goose applies only missing migrations, the catalog scripts are no-ops when the catalog matches,
# HR sync keeps existing accounts and passwords, the demo seed runs only on a database without progress.
set -eu

: "${DB_HOST:?}" "${DB_PORT:?}" "${DB_USER:?}" "${DB_PASSWORD:?}" "${DB_NAME:?}"
dsn="host=${DB_HOST} port=${DB_PORT} user=${DB_USER} password=${DB_PASSWORD} dbname=${DB_NAME} sslmode=${DB_SSLMODE:-disable}"

echo "[init] migrations"
goose -dir /app/migrations postgres "$dsn" up

echo "[init] checklist catalog"
init-catalog

echo "[init] Mock HR sync"
if [ -n "${DEMO_PASSWORD:-}" ]; then
  sync-hr -snapshot /app/data/corporate_employees.json
else
  echo "[init] DEMO_PASSWORD is empty: accounts are not created. Set DEMO_PASSWORD in .env or run sync-hr with -credentials." >&2
fi

if [ "${DEMO_SEED:-1}" = "1" ] && [ -n "${DEMO_PASSWORD:-}" ]; then
  echo "[init] demo seed"
  seed-demo
fi
echo "[init] done"

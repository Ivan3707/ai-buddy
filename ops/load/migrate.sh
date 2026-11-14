#!/bin/sh
set -eu

actual_db=$(psql -X -A -t -v ON_ERROR_STOP=1 -c 'SELECT current_database()')
if [ "$actual_db" != 'ai_buddy_load' ]; then
  echo "Refusing migrations: connected to unexpected database: $actual_db" >&2
  exit 1
fi

head=$(find /migrations -maxdepth 1 -name '[0-9]*_*.sql' -type f | sed 's|.*/||; s|_.*||' | sort -n | tail -1)
if [ -z "$head" ]; then
  echo 'No Goose migration files found' >&2
  exit 1
fi

history=$(psql -X -A -t -v ON_ERROR_STOP=1 -c "SELECT to_regclass('public.goose_db_version') IS NOT NULL")
if [ "$history" = 't' ]; then
  applied=$(psql -X -A -t -v ON_ERROR_STOP=1 -c 'SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied')
  if [ "$applied" -eq 9 ]; then
    compatible=$(psql -X -A -t -v ON_ERROR_STOP=1 -c "SELECT to_regclass('public.checklist_feedback') IS NOT NULL AND to_regclass('public.corporate_access_requests') IS NOT NULL AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='users' AND column_name='corporate_id')")
    if [ "$compatible" != 't' ]; then
      echo 'Legacy Goose v9 schema fingerprint does not match; refusing migration' >&2
      exit 1
    fi
    echo 'Existing load database uses verified legacy Goose v9 schema; no migration applied'
    exit 0
  fi
  if [ "$applied" -ne 0 ] && [ "$applied" -ne 1 ] && [ "$applied" -ne 2 ]; then
    echo "Existing load database uses unsupported Goose version $applied; no migration applied" >&2
    exit 1
  fi
fi

goose -dir /migrations postgres "host=$PGHOST port=$PGPORT user=$PGUSER password=$PGPASSWORD dbname=$PGDATABASE sslmode=disable" up

applied=$(psql -X -A -t -v ON_ERROR_STOP=1 -c 'SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied')
head_number=$(printf '%s' "$head" | sed 's/^0*//')
head_number=${head_number:-0}
if [ "$head_number" -ne "$applied" ]; then
  echo "Migration head $head differs from applied version $applied" >&2
  exit 1
fi
echo "Database $actual_db is at Goose head $applied"

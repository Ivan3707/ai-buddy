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

goose -dir /migrations postgres "host=$PGHOST port=$PGPORT user=$PGUSER password=$PGPASSWORD dbname=$PGDATABASE sslmode=disable" up

applied=$(psql -X -A -t -v ON_ERROR_STOP=1 -c 'SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied')
head_number=$(printf '%s' "$head" | sed 's/^0*//')
head_number=${head_number:-0}
if [ "$head_number" -ne "$applied" ]; then
  echo "Migration head $head differs from applied version $applied" >&2
  exit 1
fi
echo "Database $actual_db is at Goose head $applied"

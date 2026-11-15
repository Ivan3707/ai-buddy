#!/usr/bin/env bash
# Run backend integration tests against a disposable PostgreSQL, never the Compose application volume.
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
container="ai-buddy-verify-${BASHPID}-${RANDOM}"
password="ai_buddy_verify_${RANDOM}${RANDOM}"
created=0

cleanup() {
  if (( created )); then
    docker stop "$container" >/dev/null
  fi
}
trap cleanup EXIT

for program in docker goose go; do
  if ! command -v "$program" >/dev/null 2>&1; then
    printf 'Required command is unavailable: %s\n' "$program" >&2
    exit 1
  fi
done

docker run --rm -d --name "$container" \
  --tmpfs /var/lib/postgresql/data:rw,size=256m \
  -e "POSTGRES_PASSWORD=$password" \
  -e POSTGRES_DB=ai_buddy_verify_backend \
  -p 127.0.0.1::5432 postgres:16-alpine >/dev/null
created=1

ready=0
for (( attempt=0; attempt<30; attempt++ )); do
  if docker exec "$container" pg_isready -U postgres -d ai_buddy_verify_backend >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done
if (( ! ready )); then
  printf 'Temporary PostgreSQL did not become ready\n' >&2
  exit 1
fi

binding="$(docker port "$container" 5432/tcp)"
port="${binding##*:}"
if [[ ! "$port" =~ ^[0-9]+$ ]]; then
  printf 'Invalid temporary PostgreSQL port\n' >&2
  exit 1
fi

docker exec "$container" psql -U postgres -d postgres -v ON_ERROR_STOP=1 \
  -c 'CREATE DATABASE ai_buddy_verify_catalog' >/dev/null

export TEST_DATABASE_URL="postgres://postgres:${password}@127.0.0.1:${port}/ai_buddy_verify_backend?sslmode=disable"
export TEST_CATALOG_DATABASE_URL="postgres://postgres:${password}@127.0.0.1:${port}/ai_buddy_verify_catalog?sslmode=disable"
export RUN_E2E=1
unset VERIFY_DATABASE_CONNECTION

goose -dir "$repo_root/apps/backend/migrations" postgres "$TEST_DATABASE_URL" up
goose -dir "$repo_root/apps/backend/migrations" postgres "$TEST_CATALOG_DATABASE_URL" up

cd "$repo_root/apps/backend"
go test -count=1 -p 1 ./...

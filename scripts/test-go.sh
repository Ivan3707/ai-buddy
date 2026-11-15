#!/bin/sh
# Go backend: unit + PostgreSQL integration + opt-in E2E with statement coverage over all packages.
# Run through Docker: docker compose -f compose.test.yaml run --rm go
set -eu
out=/repo/coverage/go
mkdir -p "$out"
export PATH="$PATH:$(go env GOPATH)/bin"
command -v goose >/dev/null 2>&1 || go install github.com/pressly/goose/v3/cmd/goose@v3.24.1
goose -dir migrations postgres "$TEST_DATABASE_URL" up >/dev/null
goose -dir migrations postgres "$TEST_CATALOG_DATABASE_URL" up >/dev/null
go vet ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile="$out/coverage.out" ./...
go tool cover -html="$out/coverage.out" -o "$out/index.html"
total=$(go tool cover -func="$out/coverage.out" | awk '/^total:/ {sub("%","",$3); print $3}')
echo "Go statement coverage: ${total}%"
awk -v t="$total" 'BEGIN { exit (t + 0 >= 80) ? 0 : 1 }' || { echo "Go coverage is below 80%" >&2; exit 1; }

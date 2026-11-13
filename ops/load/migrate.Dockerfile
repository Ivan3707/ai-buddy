FROM golang:1.26-alpine AS build
RUN CGO_ENABLED=0 go install github.com/pressly/goose/v3/cmd/goose@v3.24.1

FROM postgres:16-alpine
COPY --from=build /go/bin/goose /usr/local/bin/goose
COPY apps/backend/migrations /migrations
COPY ops/load/migrate.sh /usr/local/bin/load-migrate
RUN chmod +x /usr/local/bin/load-migrate
ENTRYPOINT ["load-migrate"]

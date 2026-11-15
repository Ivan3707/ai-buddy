FROM golang:1.26-alpine AS build
WORKDIR /src
COPY apps/backend/ .
RUN CGO_ENABLED=0 go build -o /server ./cmd/api

# One-shot database initialization for Compose: Goose migrations, checklist catalog, Mock HR sync and the
# optional demo seed. Reviewers need only Docker: no Go, Goose or PowerShell on the host.
FROM build AS init-build
RUN CGO_ENABLED=0 go build -o /out/init-catalog ./cmd/init-catalog \
 && CGO_ENABLED=0 go build -o /out/sync-hr ./cmd/sync-hr \
 && CGO_ENABLED=0 go build -o /out/seed-demo ./cmd/seed-demo \
 && CGO_ENABLED=0 GOBIN=/out go install github.com/pressly/goose/v3/cmd/goose@v3.24.1

FROM alpine:3.22 AS init
WORKDIR /app
COPY --from=init-build /out/ /usr/local/bin/
COPY apps/backend/migrations /app/migrations
COPY apps/backend/scripts /app/scripts
COPY data/corporate_employees.json /app/data/corporate_employees.json
COPY apps/backend/docker-init.sh /app/docker-init.sh
USER 10001
ENTRYPOINT ["/bin/sh", "/app/docker-init.sh"]

FROM alpine:3.22
WORKDIR /app
COPY --from=build /server /app/server
COPY data /data
ENV DATA_DIR=/data AI_URL=http://ai:8001
USER 10001
EXPOSE 8080
CMD ["/app/server"]

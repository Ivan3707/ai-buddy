FROM golang:1.26-alpine AS build
WORKDIR /src
COPY apps/backend/ .
RUN CGO_ENABLED=0 go build -o /seed-load ./cmd/seed-load
RUN CGO_ENABLED=0 go build -o /cleanup-load ./cmd/cleanup-load

FROM alpine:3.22
COPY --from=build /seed-load /usr/local/bin/seed-load
COPY --from=build /cleanup-load /usr/local/bin/cleanup-load
COPY apps/backend/migrations /migrations
ENTRYPOINT ["/usr/local/bin/seed-load"]

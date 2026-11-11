FROM golang:1.26-alpine AS build
WORKDIR /src
COPY apps/backend/ .
RUN CGO_ENABLED=0 go build -o /server .
FROM alpine:3.22
WORKDIR /app
COPY --from=build /server /app/server
COPY data /data
ENV DATA_DIR=/data AI_URL=http://ai:8001
USER 10001
EXPOSE 8080
CMD ["/app/server"]
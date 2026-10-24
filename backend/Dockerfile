# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS builder
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN go build -trimpath -ldflags='-s -w' -o /out/api ./cmd/api \
 && go build -trimpath -ldflags='-s -w' -o /out/migrate ./cmd/migrate \
 && go build -trimpath -ldflags='-s -w' -o /out/admin ./cmd/admin

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S app -G app
COPY --from=builder /out/api /usr/local/bin/api
COPY --from=builder /out/migrate /usr/local/bin/migrate
COPY --from=builder /out/admin /usr/local/bin/admin
USER app
EXPOSE 8080
# Readiness: the API answers and PostgreSQL is reachable (busybox wget).
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/ready || exit 1
ENTRYPOINT ["/usr/local/bin/api"]

FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /api ./cmd/api
FROM alpine:3.21
RUN addgroup -S app && adduser -S app -G app
USER app
COPY --from=builder /api /api
EXPOSE 8080
ENTRYPOINT ["/api"]

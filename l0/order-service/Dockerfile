FROM golang:1.24-alpine AS builder

WORKDIR /app

# Dependencies
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Source
COPY cmd/ ./cmd/
COPY internal ./internal/

# Build
RUN CGO_ENABLED=0 GOOS=linux go build  \
    -ldflags="-s -w"                   \
    -trimpath                          \
    -o /order-service                  \
    ./cmd/service

# Run
FROM alpine:latest

RUN apk --no-cache add ca-certificates wget
RUN adduser -D -u 1000 appuser

WORKDIR /app

COPY --from=builder /order-service .
COPY web/ ./web/

USER appuser
EXPOSE 8081

CMD ["./order-service"]

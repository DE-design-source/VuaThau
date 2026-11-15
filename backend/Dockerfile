# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.27-alpine AS builder

WORKDIR /src

# Cache module downloads in their own layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0 \
    GOOS=linux

RUN go build -trimpath -ldflags "-s -w" -o /out/api ./cmd/api

# ---- Runtime stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S -G app -H -s /sbin/nologin app

WORKDIR /app

COPY --from=builder /out/api /app/api

ENV APP_ENV=production \
    PORT=8080

USER app:app

EXPOSE 8080

ENTRYPOINT ["/app/api"]

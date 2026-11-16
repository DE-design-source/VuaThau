# Image chạy API Go + web quản lý. Binary build sẵn (GOOS=linux CGO_ENABLED=0), web là frontend/dist.
# Dựng: xem deploy/server/README.md (deploy.ps1 build rồi chép bin/ và web/ vào đây).
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 app \
    && adduser -S -u 10001 -G app -H -s /sbin/nologin app

WORKDIR /app
COPY bin/api bin/ntctl /app/
COPY web/ /app/web/
COPY migrations/ /app/migrations/
# File chép từ Windows (scp) có thể mang quyền 700: cho mọi user đọc được web + migration
RUN chmod -R a+rX /app/web /app/migrations && chmod 755 /app/api /app/ntctl

ENV APP_ENV=production \
    PORT=8080 \
    DATA_DIR=/data \
    STATIC_DIR=/app/web \
    TZ=Asia/Ho_Chi_Minh

USER app:app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD wget -qO- http://127.0.0.1:8080/api/v1/config >/dev/null || exit 1
ENTRYPOINT ["/app/api"]

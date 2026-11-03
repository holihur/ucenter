# ── Frontend build ────────────────────────────────────────────────────────────
FROM node:22-alpine AS web
RUN corepack enable
WORKDIR /app/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# ── Go build ──────────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS server
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -tags nomsgpack -trimpath -ldflags="-s -w" -o /out/ucenter ./cmd/server \
 && CGO_ENABLED=0 go build -tags nomsgpack -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed \
 && CGO_ENABLED=0 go build -tags nomsgpack -trimpath -ldflags="-s -w" -o /out/purge-tenants ./cmd/purge-tenants

# ── Runtime ───────────────────────────────────────────────────────────────────
FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
 && addgroup -S -g 10001 ucenter && adduser -S -D -H -u 10001 -G ucenter ucenter
WORKDIR /app
COPY --from=server /out/ucenter /out/seed /out/purge-tenants ./
COPY --from=web /app/web/dist ./web/dist
USER ucenter
EXPOSE 8080
ENV ADDR=:8080 \
    WEB_DIR=/app/web/dist \
    MONGO_URI=mongodb://mongo:27017 \
    MONGO_DB=ucenter \
    BASE_URL=http://localhost:8080
ENTRYPOINT ["./ucenter"]

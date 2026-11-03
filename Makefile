ifneq (,$(wildcard ./.env))
include .env
export
endif

ADDR ?= :8080
BASE_URL ?= http://localhost:8080
MONGO_URI ?= mongodb://127.0.0.1:27017
MONGO_DB ?= ucenter

.PHONY: help install seed dev dev-server dev-web build build-web build-server run test smoke docker-up docker-seed clean

help:
	@echo "UCenter targets:"
	@echo "  make install      Install frontend dependencies"
	@echo "  make seed         Create demo tenants, users and OIDC clients"
	@echo "  make dev-server   Run the Go API on $(ADDR)"
	@echo "  make dev-web      Run the Vite dev server on :5173"
	@echo "  make build        Build the SPA and Go binaries"
	@echo "  make run          Build and serve the SPA + API from one process"
	@echo "  make test         Run Go tests and TypeScript checks"
	@echo "  make smoke        Run the OIDC end-to-end smoke test"
	@echo "  make mcp-smoke    Run the MCP end-to-end smoke test"
	@echo "  make email-smoke  Run the email invite/reset/verify/MFA smoke test"
	@echo "  make scim-smoke   Run the SCIM 2.0 contract smoke test"
	@echo "  make tenant-smoke Run the tenant lifecycle smoke test"
	@echo "  make webhook-smoke Run the webhook retry/signature smoke test"
	@echo "  make docker-up    Start MongoDB + UCenter with Docker Compose"
	@echo "  make docker-seed  Seed the Docker deployment"

install:
	cd web && pnpm install

seed:
	cd server && MONGO_URI=$(MONGO_URI) MONGO_DB=$(MONGO_DB) BASE_URL=$(BASE_URL) go run ./cmd/seed

dev-server:
	cd server && DEV=true ADDR=$(ADDR) BASE_URL=$(BASE_URL) MONGO_URI=$(MONGO_URI) MONGO_DB=$(MONGO_DB) go run ./cmd/server

dev-web:
	cd web && VITE_API_TARGET=$(BASE_URL) pnpm dev

build-web:
	cd web && pnpm build

build-server:
	cd server && mkdir -p bin && go build -tags nomsgpack -trimpath -o bin/ucenter ./cmd/server && go build -tags nomsgpack -trimpath -o bin/seed ./cmd/seed

build: build-web build-server

run: build
	cd server && WEB_DIR=../web/dist ADDR=$(ADDR) BASE_URL=$(BASE_URL) MONGO_URI=$(MONGO_URI) MONGO_DB=$(MONGO_DB) ./bin/ucenter

test:
	cd server && go test -tags nomsgpack ./...
	cd web && pnpm typecheck

smoke:
	BASE_URL=$(BASE_URL) ./scripts/oidc-smoke.sh

mcp-smoke:
	BASE_URL=$(BASE_URL) ./scripts/mcp-smoke.sh

email-smoke:
	BASE_URL=$(BASE_URL) MONGO_URI=$(MONGO_URI) MONGO_DB=$(MONGO_DB) ./scripts/email-smoke.sh

scim-smoke:
	BASE_URL=$(BASE_URL) ./scripts/scim-smoke.sh

tenant-smoke:
	BASE_URL=$(BASE_URL) ./scripts/tenant-smoke.sh

webhook-smoke:
	BASE_URL=$(BASE_URL) WEBHOOK_ALLOW_PRIVATE=true ./scripts/webhook-smoke.sh

docker-up:
	docker compose up -d --build

docker-seed:
	docker compose run --rm --entrypoint ./seed ucenter

clean:
	rm -rf web/dist server/bin

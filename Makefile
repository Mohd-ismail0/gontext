# Context Fabric — common developer and operator targets.
# Requires: Go 1.26+, Docker, docker compose plugin.

MODULE      := github.com/xsama/context-fabric
BIN         := bin/context-fabric
CF_BIN      := bin/cf
IMAGE       := ghcr.io/mohd-ismail0/gontext
VERSION     := $(shell cat VERSION 2>/dev/null || echo dev)
IMAGE_TAG   := $(IMAGE):$(VERSION)
COMPOSE_DIR := deploy/compose
COMPOSE     := docker compose -f $(COMPOSE_DIR)/docker-compose.yml --env-file $(COMPOSE_DIR)/.env
COMPOSE_STARTER := docker compose \
	-f $(COMPOSE_DIR)/docker-compose.yml \
	-f $(COMPOSE_DIR)/docker-compose.starter.yaml \
	--env-file $(COMPOSE_DIR)/.env.starter
GOFLAGS     := -trimpath
LDFLAGS     := -s -w -X main.buildVersion=$(VERSION)

.PHONY: all build test lint docker-build compose-up compose-starter-up compose-starter-down \
	compose-starter-preflight compose-starter-smoke migrate doctor clean backup restore release-check

all: build

build:
	@mkdir -p bin
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BIN) ./cmd/context-fabric
	@if [ -d ./cmd/cf ]; then CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(CF_BIN) ./cmd/cf; fi

test:
	# Avoid `go test -o <dir> ./...` — shared package basenames clash (retrieval.test).
	go test -count=1 ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed; running go vet"; \
		go vet ./...; \
	fi

docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg TARGETOS=linux \
		--build-arg TARGETARCH=$${GOARCH:-amd64} \
		-t $(IMAGE_TAG) \
		-t $(IMAGE):latest \
		.

compose-up:
	@test -f $(COMPOSE_DIR)/.env || cp $(COMPOSE_DIR)/.env.example $(COMPOSE_DIR)/.env
	$(COMPOSE) up --build -d

compose-starter-up:
	@test -f $(COMPOSE_DIR)/.env.starter || cp $(COMPOSE_DIR)/.env.starter.example $(COMPOSE_DIR)/.env.starter
	bash scripts/compose-starter-preflight.sh
	$(COMPOSE_STARTER) up --build -d

compose-starter-down:
	$(COMPOSE_STARTER) down

compose-starter-preflight:
	bash scripts/compose-starter-preflight.sh

compose-starter-smoke:
	bash scripts/compose-smoke.sh

migrate:
	@test -f $(COMPOSE_DIR)/.env || cp $(COMPOSE_DIR)/.env.example $(COMPOSE_DIR)/.env
	$(COMPOSE) run --rm migrate

doctor:
	@test -f $(COMPOSE_DIR)/.env || cp $(COMPOSE_DIR)/.env.example $(COMPOSE_DIR)/.env
	$(COMPOSE) run --rm --no-deps serve doctor

backup:
	bash scripts/backup.sh $(OUT_DIR)

restore:
	@test -n "$(IN_DIR)" || (echo "usage: make restore IN_DIR=<backup-dir>" >&2; exit 2)
	bash scripts/restore.sh $(IN_DIR)

release-check: lint test
	bash scripts/validate-contracts.sh
	bash scripts/compose-starter-preflight.sh
	@if command -v helm >/dev/null 2>&1; then helm lint deploy/helm/context-fabric; else echo "helm not installed; skipping helm lint"; fi
	@echo "release-check: ok (version $(VERSION))"

clean:
	rm -rf bin

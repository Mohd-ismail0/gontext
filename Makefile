# Context Fabric — common developer and operator targets.
# Requires: Go 1.26+, Docker, docker compose plugin.

MODULE      := github.com/xsama/context-fabric
BIN         := bin/context-fabric
CF_BIN      := bin/cf
IMAGE       := ghcr.io/xsama/context-fabric
IMAGE_TAG   := $(IMAGE):$(or $(VERSION),dev)
COMPOSE_DIR := deploy/compose
COMPOSE     := docker compose -f $(COMPOSE_DIR)/docker-compose.yml --env-file $(COMPOSE_DIR)/.env
GOFLAGS     := -trimpath

.PHONY: all build test lint docker-build compose-up migrate doctor clean

all: build

build:
	@mkdir -p bin
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(BIN) ./cmd/context-fabric
	@if [ -d ./cmd/cf ]; then CGO_ENABLED=0 go build $(GOFLAGS) -o $(CF_BIN) ./cmd/cf; fi

test:
	go test ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed; running go vet"; \
		go vet ./...; \
	fi

docker-build:
	docker build \
		--build-arg TARGETOS=linux \
		--build-arg TARGETARCH=$${GOARCH:-amd64} \
		-t $(IMAGE_TAG) \
		-t $(IMAGE):latest \
		.

compose-up:
	@test -f $(COMPOSE_DIR)/.env || cp $(COMPOSE_DIR)/.env.example $(COMPOSE_DIR)/.env
	$(COMPOSE) up --build -d

migrate:
	@test -f $(COMPOSE_DIR)/.env || cp $(COMPOSE_DIR)/.env.example $(COMPOSE_DIR)/.env
	$(COMPOSE) run --rm migrate

doctor:
	@test -f $(COMPOSE_DIR)/.env || cp $(COMPOSE_DIR)/.env.example $(COMPOSE_DIR)/.env
	$(COMPOSE) run --rm --entrypoint /usr/local/bin/context-fabric serve doctor

clean:
	rm -rf bin

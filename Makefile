# tinyproxy + Prometheus exporter -- developer shortcuts.
#
# Run from the repository root.

SHELL := /bin/sh

GO          ?= go
MODULE_DIR  := exporter
IMAGE_TAG   ?= local
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REVISION    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
TARGET_OS   ?= linux
TARGET_ARCH ?= amd64

# Build time network overrides. Set these when dl-cdn.alpinelinux.org or
# proxy.golang.org are unreachable from the build host, for example:
#   make images APK_MIRROR=https://mirrors.aliyun.com/alpine \
#               GOPROXY=https://mirrors.aliyun.com/goproxy,direct
APK_MIRROR  ?=
GOPROXY     ?= https://proxy.golang.org,direct

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: fmt
fmt: ## Format the Go sources.
	cd $(MODULE_DIR) && $(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet.
	cd $(MODULE_DIR) && $(GO) vet ./...

.PHONY: test
test: ## Run the unit tests with the race detector.
	cd $(MODULE_DIR) && $(GO) test -race -count=1 ./...

.PHONY: build
build: ## Compile every package.
	cd $(MODULE_DIR) && $(GO) build ./...

.PHONY: binary-linux
binary-linux: ## Cross compile the static binary used by Dockerfile.slim.
	cd $(MODULE_DIR) && CGO_ENABLED=0 GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) $(GO) build \
		-trimpath \
		-ldflags "-s -w -X main.version=$(VERSION) -X main.revision=$(REVISION)" \
		-o bin/tinyproxy-exporter-$(TARGET_OS)-$(TARGET_ARCH) .

.PHONY: run
run: ## Run the exporter locally (expects TINYPROXY_* environment variables).
	cd $(MODULE_DIR) && $(GO) run .

.PHONY: images
images: ## Build both images with the standard multi stage Dockerfiles.
	docker build --build-arg APK_MIRROR=$(APK_MIRROR) \
		-t tinyproxy:$(IMAGE_TAG) ./tinyproxy
	docker build --build-arg APK_MIRROR=$(APK_MIRROR) --build-arg GOPROXY=$(GOPROXY) \
		--build-arg VERSION=$(VERSION) --build-arg REVISION=$(REVISION) \
		-t tinyproxy-exporter:$(IMAGE_TAG) ./exporter

.PHONY: images-offline
images-offline: binary-linux ## Build the exporter image from a locally compiled binary.
	docker build -f exporter/Dockerfile.slim \
		--build-arg TARGETOS=$(TARGET_OS) --build-arg TARGETARCH=$(TARGET_ARCH) \
		-t tinyproxy-exporter:$(IMAGE_TAG) ./exporter

.PHONY: up
up: ## Run the production stack (GHCR `latest` images).
	docker compose up -d

.PHONY: up-dev
up-dev: ## Run the development stack (build images locally).
	docker compose -f docker-compose.dev.yml up -d --build

.PHONY: down
down: ## Stop the production stack.
	docker compose down

.PHONY: down-dev
down-dev: ## Stop the development stack.
	docker compose -f docker-compose.dev.yml down

.PHONY: logs
logs: ## Follow the production stack logs.
	docker compose logs -f

.PHONY: logs-dev
logs-dev: ## Follow the development stack logs.
	docker compose -f docker-compose.dev.yml logs -f

.PHONY: clean
clean: ## Remove local build artifacts.
	rm -rf $(MODULE_DIR)/bin

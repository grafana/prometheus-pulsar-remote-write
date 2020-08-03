GIT_BRANCH := $(shell git branch --show-current)
GIT_SHA    := $(shell git rev-parse --short HEAD)

BUILD_IMAGE := jdbgrafana/prometheus-pulsar-remote-write-build-image

# from https://suva.sh/posts/well-documented-makefiles/
.PHONY: help
help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build go binary
	CGO_ENABLED=0 GOOS=linux go build -installsuffix cgo -o prometheus-pulsar-remote-write

.PHONY: test
test: ## Run all tests
	go test -race ./...

.PHONY: bench
bench: ## Run all benchmarks
	go test -bench . ./...

.PHONY: lint
lint: ## Lint
	golangci-lint run ./...

.PHONY: image
image: ## Build docker image
	docker build -t grafana/prometheus-pulsar-remote-write .

.drone/drone.yml: .drone/drone.jsonnet ## Update the CI configuration file
	drone jsonnet --target $@ --format --stream --source $<\
		--extVar BUILD_IMAGE=$(BUILD_IMAGE):7ee8ff6

prometheus-pulsar-remote-write.sha256: prometheus-pulsar-remote-write ## Produce SHA256 checksum for the go binary
	sha256sum $< | cut -b -64 > $@

build-image/.uptodate: build-image/Dockerfile .git/refs/heads/$(GIT_BRANCH) ## Build docker image used in CI builds
	docker build -t $(BUILD_IMAGE):$(GIT_SHA) build-image
	touch $@

build-image/.published: build-image/.uptodate ## Publish docker image used in CI builds
	docker push $(BUILD_IMAGE):$(GIT_SHA)

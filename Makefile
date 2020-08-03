# This makefile provider some wrapper around bazel targets

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
	drone jsonnet --target $@ --format --stream --source $<

prometheus-pulsar-remote-write.sha256: prometheus-pulsar-remote-write ## Produce SHA256 checksum for the go binary
	sha256sum $< | cut -b -64 > $@

GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
GIT_SHA    := $(shell git rev-parse --short HEAD)

BUILD_IMAGE := grafana/prometheus-pulsar-remote-write-build-image

README_COMMAND := go build && { ./prometheus-pulsar-remote-write help && ./prometheus-pulsar-remote-write help produce && ./prometheus-pulsar-remote-write help consume ; } 2>&1

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

.PHONY: integration
integration: ## Run integration suite
	go test -v ./integration/...

.PHONY: bench
bench: ## Run all benchmarks
	go test -bench . ./...

.PHONY: lint
lint: ## Lint
	golangci-lint run ./...

.PHONY: verify-readme
verify-readme: ## Ensure the README.md is update to date
	$(README_COMMAND) | go run hack/update-readme.go

.PHONY: update-readme
update-readme: ## Update the README.md is update to date
	$(README_COMMAND) | go run hack/update-readme.go --update

.PHONY: image
image: ## Build docker image
	docker build -t grafana/prometheus-pulsar-remote-write .

.drone/drone.yml: .drone/drone.jsonnet Makefile ## Update the CI configuration file
	drone jsonnet --target $@ --format --stream --source $<\
		--extVar BUILD_IMAGE=$(BUILD_IMAGE):c1b1dc1

BIN_SUFFIXES := linux-amd64 linux-arm64 darwin-amd64 windows-amd64.exe
BINARIES     := $(patsubst %, dist/prometheus-pulsar-remote-write-%, $(BIN_SUFFIXES))
SHAS        := $(patsubst %, %.sha256, $(BINARIES))

dist: ## Make the dist directory
	mkdir -p dist

binaries: $(BINARIES)

$(BINARIES): | dist ## Cross compile go binaries
	CGO_ENABLED=0 gox -output dist/{{.Dir}}-{{.OS}}-{{.Arch}} $$(echo $(BIN_SUFFIXES) | sed -E -e 's/.exe//' -e 's/([a-z]+)-([a-z0-9]+)/-osarch=\1\/\2/g')
shas: $(SHAS) | dist ## Produce SHA256 checksums for all go binaries

%.sha256: % ## Produce a SHA256 checksum for a file
	sha256sum $< | cut -b -64 > $@

build-image/.uptodate: build-image/Dockerfile .git/refs/heads/$(GIT_BRANCH) ## Build docker image used in CI builds
	docker build -t $(BUILD_IMAGE):$(GIT_SHA) build-image
	touch $@

build-image/.published: build-image/.uptodate ## Publish docker image used in CI builds
	docker push $(BUILD_IMAGE):$(GIT_SHA)
	touch $@

.PHONY: clean
clean:
	rm -rfv dist

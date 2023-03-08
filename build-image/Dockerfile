FROM debian:stretch-slim as builder-base
RUN apt-get update && apt-get install -y curl

FROM builder-base as drone-cli-builder
ARG DRONE_CLI_VERSION=1.2.1
ARG DRONE_CLI_CHECKSUM=b74d0eeb878a8845787d6d1acaf8f2480d6a4acd2b7daaaaacbaafc443c8797e
RUN curl -L https://github.com/drone/drone-cli/releases/download/v${DRONE_CLI_VERSION}/drone_linux_amd64.tar.gz | tar zx
RUN echo "${DRONE_CLI_CHECKSUM} drone" | sha256sum -c - || (printf "wanted: %s\n   got: %s\n" "${DRONE_CLI_CHECKSUM}" "$(sha256sum drone)"; exit 1)
RUN install -t /usr/local/bin drone

FROM builder-base as golangci-lint-builder
ARG GOLANGCI_LINT_VERSION=1.51.2
ARG GOLANGCI_LINT_CHECKSUM=b5032ef462fd6a837dc57882d89927f868fbb3e75d3ec63f02413ef3e973be1f
RUN curl -L https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz |tar zx
RUN echo "${GOLANGCI_LINT_CHECKSUM} golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64/golangci-lint" | sha256sum -c - || (printf "wanted: %s\n   got: %s\n" "${GOLANGCI_LINT_CHECKSUM}" "$(sha256sum golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64/golangci-lint)"; exit 1)
RUN install -t /usr/local/bin golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64/golangci-lint

FROM golang:1.20.2-bullseye as gox-builder
ARG GOX_VERSION=1.0.1
ARG GOX_CHECKSUM=b2ad2e4dd784a9a15d39d01b6259b3f8ac5059d35f2db64531d3738deb93abe9
RUN curl -L https://github.com/mitchellh/gox/archive/v${GOX_VERSION}.tar.gz | tar zx
RUN cd gox-"${GOX_VERSION}" && go build
RUN echo "${GOX_CHECKSUM} gox-${GOX_VERSION}/gox" | sha256sum -c - || (printf "wanted: %s\n   got: %s\n" "${GOX_CHECKSUM}" "$(sha256sum gox-${GOX_VERSION}/gox)"; exit 1)
RUN install -t /usr/local/bin gox-"${GOX_VERSION}"/gox

FROM golang:1.20.2-bullseye
COPY --from=drone-cli-builder /usr/local/bin/drone /usr/local/bin
COPY --from=golangci-lint-builder /usr/local/bin/golangci-lint /usr/local/bin
COPY --from=gox-builder /usr/local/bin/gox /usr/local/bin

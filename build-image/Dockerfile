FROM debian:stretch-slim as builder-base
RUN apt-get update && apt-get install -y curl

FROM builder-base as drone-cli-builder
ARG DRONE_CLI_VERSION=1.2.1
RUN curl -L https://github.com/drone/drone-cli/releases/download/v${DRONE_CLI_VERSION}/drone_linux_amd64.tar.gz | tar zx && install -t /usr/local/bin drone

FROM builder-base as golangci-lint-builder
ARG GOLANG_CI_LINT_VERSION=1.30.0
RUN curl -L https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v${GOLANG_CI_LINT_VERSION}

FROM golang:1.14-stretch as gox-builder
ARG GOX_VERSION=1.0.1
RUN git clone https://github.com/mitchellh/gox
RUN cd gox && go build

FROM golang:1.14-stretch
COPY --from=drone-cli-builder /usr/local/bin/drone /usr/local/bin
COPY --from=golangci-lint-builder /bin/golangci-lint /usr/local/bin
COPY --from=gox-builder /go/gox/gox /usr/local/bin

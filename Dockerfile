FROM golang:1.20.2

# fetch dependencies
WORKDIR /app/
COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY pkg pkg/
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o prometheus-pulsar-remote-write .

FROM alpine:3.17
COPY --from=0 /app/prometheus-pulsar-remote-write /usr/local/bin/prometheus-pulsar-remote-write
USER nobody
ENTRYPOINT ["prometheus-pulsar-remote-write"]

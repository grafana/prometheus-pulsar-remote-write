# prometheus-pulsar-remote-write

A [Prometheus] remote_write adapter for [Pulsar], based on
[remote-storage-adapter][prometheus/remote-storage-adapter] and inspried by
[liangyuanpeng/prometheus-pulsar-adapter].

## Configuration

Prometheus needs to have a `remote_write` url configured, pointing to the
`/write` endpoint of the host and port where the prometheus-pulsar-remote-write
service is running. For example:

```
remote_write:
  - url: "http://prometheus-pulsar-remote-write:9201/write"
```

## Usage

```
usage: prometheus-pulsar-remote-write [<flags>]

Pulsar Remote storage adapter for Prometheus

Flags:
  -h, --help                     Show context-sensitive help (also try
                                 --help-long and --help-man).
      --send-timeout=30s         The timeout to use when sending samples to the
                                 remote storage.
      --web.listen-address=":9201"  
                                 Address to listen on for web endpoints.
      --web.telemetry-path="/metrics"  
                                 Path under which to expose metrics.
      --web.write-path="/write"  Path under which to receive remote_write
                                 requests.
      --replica-label=__replica__ ...  
                                 External label to identify replicas. Can be
                                 specified multiple times.
      --pulsar.url=""            The URL of the remote Pulsar server to send
                                 samples to. Example:
                                 pulsar://pulsar-proxy:6650. None, if empty.
      --pulsar.connection-timeout=30s  
                                 The timeout to use when connection to the
                                 remote Pulsar server.
      --pulsar.serializer="json"  
                                 Specifies the serialization format
                                 
                                 json: JSON default format as defined by
                                 github.com/prometheus/common/model
                                 
                                 {
                                 
                                   "metric": {
                                     "__name__": "foo",
                                     "labelfoo": "label-bar"
                                   },
                                   "value": [
                                     0,
                                     "456"
                                   ]
                                 
                                 }
                                 
                                 json-compat: JSON compat provides compatability
                                 with
                                 github.com/liangyuanpeng/prometheus-pulsar-adapter
                                 
                                 {
                                 
                                   "name": "foo",
                                   "labels": {
                                     "__name__": "foo",
                                     "labelfoo": "label-bar"
                                   },
                                   "value": "456",
                                   "timestamp": "1970-01-01T00:00:00Z"
                                 
                                 }
                                 
                                 avro-json-compat[=<path to schema>]: AVRO-JSON
                                 which can optionally read a custom schema
                                 
                                 Default schema: {
                                 
                                   "namespace": "io.prometheus",
                                   "type": "record",
                                   "name": "Metric",
                                   "doc:": "A basic schema for representing Prometheus metrics",
                                   "fields": [
                                     {
                                       "name": "timestamp",
                                       "type": "string"
                                     },
                                     {
                                       "name": "value",
                                       "type": "string"
                                     },
                                     {
                                       "name": "name",
                                       "type": "string"
                                     },
                                     {
                                       "name": "labels",
                                       "type": {
                                         "type": "map",
                                         "values": "string"
                                       }
                                     },
                                     {
                                       "name": "tenant_id",
                                       "type": "string",
                                       "default": ""
                                     }
                                   ]
                                 
                                 }
      --pulsar.topic="metrics"   The Pulsar topic to publish the metrics on
      --log.level=info           Only log messages with the given severity or
                                 above. One of: [debug, info, warn, error]
      --log.format=logfmt        Output format of log messages. One of: [logfmt,
                                 json]
```

## Development

### Integration tests

There are some integration tests, which are only run if there is a the
TEST_PULSAR_URL environment variable set.

```
docker run -it \
  -p 6650:6650 \
  -p 8080:8080 \
  --mount source=pulsardata,target=/pulsar/data \
  --mount source=pulsarconf,target=/pulsar/conf \
  apachepulsar/pulsar:2.6.0 \
  bin/pulsar standalone

export TEST_PULSAR_URL=pulsar://127.0.0.1:6650
go test -race ./...
```


[Prometheus]:https://prometheus.io/
[Pulsar]:https://pulsar.apache.org/
[Prometheus/remote-storage-adapter]:https://github.com/prometheus/prometheus/tree/v2.20.0/documentation/examples/remote_storage/remote_storage_adapter
[liangyuanpeng/prometheus-pulsar-adapter]:https://github.com/liangyuanpeng/prometheus-pulsar-adapter

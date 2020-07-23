module github.com/grafana/prometheus-pulsar-remote-write

go 1.14

require (
	github.com/apache/pulsar-client-go v0.0.0-00010101000000-000000000000
	github.com/go-kit/kit v0.10.0
	github.com/gogo/protobuf v1.3.1
	github.com/golang/snappy v0.0.1
	github.com/linkedin/goavro v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/common v0.10.0
	github.com/prometheus/prometheus v2.5.0+incompatible
	github.com/stretchr/testify v1.4.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/linkedin/goavro.v1 v1.0.5 // indirect
)

// This avoids some depreaction warning
replace github.com/golang/protobuf => github.com/golang/protobuf v1.3.5

// TODO: Move back to upstream once those two PRs are merged:
//
// Fixes go.mod issue with invalid version
// https://github.com/apache/pulsar-client-go/pull/330
//
// Fix producer goroutine leak
// https://github.com/apache/pulsar-client-go/pull/331
replace github.com/apache/pulsar-client-go => github.com/simonswine/pulsar-client-go v0.1.2-0.20200723122534-1ea6107fea8b

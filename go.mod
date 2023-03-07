module github.com/grafana/prometheus-pulsar-remote-write

go 1.14

require (
	github.com/apache/pulsar-client-go v0.9.0
	github.com/go-kit/log v0.2.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/snappy v0.0.4
	github.com/hexops/gotextdiff v1.0.3
	github.com/linkedin/goavro/v2 v2.9.8
	github.com/prometheus/client_golang v1.14.0
	github.com/prometheus/common v0.39.0
	github.com/prometheus/prometheus v0.42.0
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.8.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
)

// Exclude grpc v1.30.0 because of breaking changes. See #7621.
exclude (
	// Exclude grpc v1.30.0 because of breaking changes. See #7621.
	github.com/grpc-ecosystem/grpc-gateway v1.14.7
	google.golang.org/api v0.30.0
)

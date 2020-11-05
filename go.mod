module github.com/grafana/prometheus-pulsar-remote-write

go 1.14

require (
	github.com/apache/pulsar-client-go v0.2.0
	github.com/go-kit/kit v0.10.0
	github.com/gogo/protobuf v1.3.1
	github.com/golang/snappy v0.0.2
	github.com/linkedin/goavro v2.1.0+incompatible
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/common v0.14.0
	github.com/prometheus/prometheus v1.8.2-0.20201015110737-0a7fdd3b7696
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.5.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/linkedin/goavro.v1 v1.0.5 // indirect
)

// Exclude grpc v1.30.0 because of breaking changes. See #7621.
exclude (
	// Exclude grpc v1.30.0 because of breaking changes. See #7621.
	github.com/grpc-ecosystem/grpc-gateway v1.14.7
	google.golang.org/api v0.30.0
)

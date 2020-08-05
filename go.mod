module github.com/grafana/prometheus-pulsar-remote-write

go 1.14

require (
	github.com/Azure/go-autorest/autorest v0.11.2 // indirect
	github.com/apache/pulsar-client-go v0.1.2-0.20200729045024-c0cba320e933
	github.com/go-kit/kit v0.10.0
	github.com/gogo/protobuf v1.3.1
	github.com/golang/snappy v0.0.1
	github.com/gophercloud/gophercloud v0.12.0 // indirect
	github.com/linkedin/goavro v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/common v0.10.0
	github.com/prometheus/prometheus v1.8.2-0.20200724121523-657ba532e42f
	github.com/sirupsen/logrus v1.4.2
	github.com/stretchr/testify v1.5.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/linkedin/goavro.v1 v1.0.5 // indirect
	k8s.io/client-go v0.18.6 // indirect
)

// Exclude grpc v1.30.0 because of breaking changes. See #7621.
exclude google.golang.org/grpc v1.30.0

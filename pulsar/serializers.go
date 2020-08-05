package pulsar

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"time"

	"github.com/linkedin/goavro"
	"github.com/prometheus/common/model"
)

type Serializer interface {
	Marshal(*Sample) ([]byte, error)
}

// JSONSerializer represents the sample in the upstream model
type JSONSerializer struct {
}

func NewSample(s *model.Sample) *Sample {
	return &Sample{
		Value: model.SamplePair{
			Timestamp: s.Timestamp,
			Value:     s.Value,
		},
		Metric: s.Metric,
	}
}

type Sample struct {
	Value    model.SamplePair `json:"value"`
	Metric   model.Metric     `json:"metric,omitempty"`
	TenantID string           `json:"tenant_id,omitempty"`
}

func (s *Sample) jsonCompat() map[string]interface{} {
	data := map[string]interface{}{
		"timestamp": s.Value.Timestamp.Time().UTC().Format(time.RFC3339Nano),
		"value":     s.Value.Value.String(),
		"name":      string(s.Metric["__name__"]),
		"labels":    s.Metric,
	}
	if s.TenantID != "" {
		data["tenant_id"] = s.TenantID
	}
	return data
}

func (*JSONSerializer) Marshal(s *Sample) ([]byte, error) {
	return json.Marshal(s)
}

func NewJSONSerializer() *JSONSerializer {
	return &JSONSerializer{}
}

// JSONCompatSerializer represents the sample in the upstream model
type JSONCompatSerializer struct {
}

func (*JSONCompatSerializer) Marshal(s *Sample) ([]byte, error) {
	return json.Marshal(s.jsonCompat())
}

func NewJSONCompatSerializer() *JSONCompatSerializer {
	return &JSONCompatSerializer{}
}

const AvroJSONDefaultSchema = `{
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
`

// AvroJSONSerializer represents a metrics serializer that writes Avro-JSON
type AvroJSONSerializer struct {
	codec *goavro.Codec
}

func (a *AvroJSONSerializer) Marshal(s *Sample) ([]byte, error) {
	labels := make(map[string]string, len(s.Metric))
	for k, l := range s.Metric {
		labels[string(k)] = string(l)
	}
	data := s.jsonCompat()
	data["labels"] = labels
	return a.codec.TextualFromNative(nil, data)
}

func NewAvroJSONSerializer(r io.Reader) (*AvroJSONSerializer, error) {
	var schema string
	if r == nil {
		schema = AvroJSONDefaultSchema
	} else {
		schemaBytes, err := ioutil.ReadAll(r)
		if err != nil {
			return nil, err
		}
		schema = string(schemaBytes)
	}

	codec, err := goavro.NewCodec(schema)
	if err != nil {
		return nil, err
	}

	return &AvroJSONSerializer{
		codec: codec,
	}, nil
}

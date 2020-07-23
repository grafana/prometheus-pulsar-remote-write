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
	Marshal(*model.Sample) ([]byte, error)
}

// JSONSerializer represents the sample in the upstream model
type JSONSerializer struct {
}

func (*JSONSerializer) Marshal(s *model.Sample) ([]byte, error) {
	return s.MarshalJSON()
}

func NewJSONSerializer() *JSONSerializer {
	return &JSONSerializer{}
}

func jsonCompat(s *model.Sample) map[string]interface{} {
	return map[string]interface{}{
		"timestamp": s.Timestamp.Time().UTC().Format(time.RFC3339Nano),
		"value":     s.Value.String(),
		"name":      string(s.Metric["__name__"]),
		"labels":    s.Metric,
	}
}

// JSONCompatSerializer represents the sample in the upstream model
type JSONCompatSerializer struct {
}

func (*JSONCompatSerializer) Marshal(s *model.Sample) ([]byte, error) {
	return json.Marshal(jsonCompat(s))
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
    }
  ]
}`

// AvroJSONSerializer represents a metrics serializer that writes Avro-JSON
type AvroJSONSerializer struct {
	codec *goavro.Codec
}

func (a *AvroJSONSerializer) Marshal(s *model.Sample) ([]byte, error) {
	labels := make(map[string]string, len(s.Metric))
	for k, l := range s.Metric {
		labels[string(k)] = string(l)
	}
	data := jsonCompat(s)
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

package pulsar

import (
	"math"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
)

func newSampleNormal() *Sample {
	return NewSample(&model.Sample{
		Metric: model.Metric{
			model.MetricNameLabel: "foo",
			"labelfoo":            "label-bar",
		},
		Value:     456,
		Timestamp: 0,
	})
}

func newSampleInf() *Sample {
	return NewSample(&model.Sample{
		Metric: model.Metric{
			model.MetricNameLabel: "foo",
			"labelfoo":            "label-bar",
		},
		Value:     model.SampleValue(math.Inf(1)),
		Timestamp: 10001,
	})
}

func newSampleNormalTenant() *Sample {
	s := newSampleNormal()
	s.TenantID = "fake"
	return s
}

func TestSerializeToJSON(t *testing.T) {
	serializer := NewJSONSerializer()

	for _, tc := range []struct {
		input    *Sample
		expected []byte
	}{
		{
			newSampleNormal(),
			[]byte(`{"value":[0,"456"],"metric":{"__name__":"foo","labelfoo":"label-bar"}}`),
		},
		{
			newSampleInf(),
			[]byte(`{"value":[10.001,"+Inf"],"metric":{"__name__":"foo","labelfoo":"label-bar"}}`),
		},
		{
			newSampleNormalTenant(),
			[]byte(`{"value":[0,"456"],"metric":{"__name__":"foo","labelfoo":"label-bar"},"tenant_id":"fake"}`),
		},
	} {
		actual, err := serializer.Marshal(tc.input)
		assert.Nil(t, err)
		assert.JSONEqf(t, string(tc.expected), string(actual), "wrong json serialization found")
	}
}

func BenchmarkSerializeToJSON(b *testing.B) {
	serializer := NewJSONSerializer()
	sample := newSampleNormal()
	for n := 0; n < b.N; n++ {
		_, _ = serializer.Marshal(sample)
	}
}

func TestSerializeToJSONCompat(t *testing.T) {
	serializer := NewJSONCompatSerializer()

	for _, tc := range []struct {
		input    *Sample
		expected []byte
	}{
		{
			newSampleNormal(),
			[]byte(`{"value":"456","timestamp":"1970-01-01T00:00:00Z","name":"foo","labels":{"__name__":"foo","labelfoo":"label-bar"}}`),
		},
		{
			newSampleInf(),
			[]byte(`{"value":"+Inf","timestamp":"1970-01-01T00:00:10.001Z","name":"foo","labels":{"__name__":"foo","labelfoo":"label-bar"}}`),
		},
		{
			newSampleNormalTenant(),
			[]byte(`{"value":"456","timestamp":"1970-01-01T00:00:00Z","name":"foo","labels":{"__name__":"foo","labelfoo":"label-bar"},"tenant_id":"fake"}`),
		},
	} {
		actual, err := serializer.Marshal(tc.input)
		assert.Nil(t, err)
		assert.JSONEqf(t, string(tc.expected), string(actual), "wrong json serialization found")
	}
}

func BenchmarkSerializeToJSONCompat(b *testing.B) {
	serializer := NewJSONCompatSerializer()
	sample := newSampleNormal()
	for n := 0; n < b.N; n++ {
		_, _ = serializer.Marshal(sample)
	}
}

func TestSerializeToAvro(t *testing.T) {
	serializer, err := NewAvroJSONSerializer(nil)
	assert.Nil(t, err)

	for _, tc := range []struct {
		input    *Sample
		expected []byte
	}{
		{
			newSampleNormal(),
			[]byte(`{"value":"456","timestamp":"1970-01-01T00:00:00Z","name":"foo","labels":{"__name__":"foo","labelfoo":"label-bar"},"tenant_id":""}`),
		},
		{
			newSampleInf(),
			[]byte(`{"value":"+Inf","timestamp":"1970-01-01T00:00:10.001Z","name":"foo","labels":{"__name__":"foo","labelfoo":"label-bar"},"tenant_id":""}`),
		},
		{
			newSampleNormalTenant(),
			[]byte(`{"value":"456","timestamp":"1970-01-01T00:00:00Z","name":"foo","labels":{"__name__":"foo","labelfoo":"label-bar"},"tenant_id":"fake"}`),
		},
	} {
		actual, err := serializer.Marshal(tc.input)
		assert.Nil(t, err)
		assert.JSONEqf(t, string(tc.expected), string(actual), "wrong json serialization found")
	}
}

func BenchmarkSerializeToAvroJSON(b *testing.B) {
	serializer, _ := NewAvroJSONSerializer(nil)
	sample := newSampleNormal()
	for n := 0; n < b.N; n++ {
		_, _ = serializer.Marshal(sample)
	}
}

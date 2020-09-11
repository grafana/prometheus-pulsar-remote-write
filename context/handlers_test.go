package context_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	mcontext "github.com/grafana/prometheus-pulsar-remote-write/context"
)

type fakeClock struct {
	Time *time.Time
}

func (f *fakeClock) Now() time.Time {
	return *f.Time
}

func TestMaxConnectionAgeHandler(t *testing.T) {
	t1 := time.Unix(1577873472, 0)
	t2 := t1.Add(500 * time.Millisecond)
	t3 := t1.Add(1001 * time.Millisecond)
	fclock := &fakeClock{Time: &t1}
	mcontext.WithClock(fclock)

	// set start time
	ctx := mcontext.ContextWithConnectionStartTime(
		context.Background(),
		t1,
	)

	// test context getter
	tFromContext, ok := mcontext.ConnectionStartTimeFromContext(ctx)
	assert.Equal(t, t1, tFromContext)
	assert.True(t, ok)

	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
	})

	h := mcontext.MaxConnectionAgeHandler(mux, time.Second)

	req, err := http.NewRequestWithContext(ctx, "GET", "/test", nil)
	if err != nil {
		t.Fatal(err)
	}

	// first request at the same time
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "", rr.Header().Get("Connection"))

	// advance time half a second
	fclock.Time = &t2
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "", rr.Header().Get("Connection"))

	// advance time half a second and a bit
	fclock.Time = &t3
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "close", rr.Header().Get("Connection"))

}

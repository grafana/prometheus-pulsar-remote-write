package integration

import (
	"flag"
	"math/rand"
	"net"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/grafana/prometheus-pulsar-remote-write/pkg/app"
)

type Clocker interface {
	Now() time.Time
}

var clock Clocker = &testClock{}

type testClock struct {
	t time.Time
}

// every call to now will add another second to the time
func (c *testClock) Now() time.Time {
	if c.t.IsZero() {
		c.t = time.Unix(1588462000, 0)
	} else {
		c.t = c.t.Add(time.Second)
	}
	return c.t
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

var letters = []rune("abcdefghijklmnopqrstuvwxyz")

// return a random string
func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

const envTestPulsarURL = "TEST_PULSAR_URL"

func skipWithoutPulsar(t *testing.T) {
	if os.Getenv(envTestPulsarURL) == "" {
		t.Skipf("Integration tests skipped as not pulsar URL provided in environment variable %s.", envTestPulsarURL)
	}
}

func getRandomFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Get a new instance of the application configured for testing
func getNewTestApp() *app.App {
	// Fresh registry so we can be sure of the number of metrics registered during a test.
	// In production use, nil is passed to indicate the global default registry should be used.
	a := app.New(prometheus.NewRegistry())
	// lower delay to run tests faster
	a.WithConsumeBatchMaxDelay(200 * time.Millisecond)
	return a
}

func TestMain(m *testing.M) {
	flag.Parse()
	// reduce verbosity of logrus which is used by the pulsar golang library
	if !testing.Verbose() {
		logrus.SetLevel(logrus.WarnLevel)
	}
	os.Exit(m.Run())
}

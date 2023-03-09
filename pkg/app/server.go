package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	mcontext "github.com/grafana/prometheus-pulsar-remote-write/pkg/context"
)

type server struct {
	server *http.Server
	mux    *http.ServeMux
	logger log.Logger
}

func (s *server) run(ctx context.Context) error {
	// create error channel to receive from listening to the server
	errCh := make(chan error)

	// start server
	go func() {
		errCh <- s.server.ListenAndServe()
	}()

	_ = level.Info(s.logger).Log("msg", "Server starting to listen", "address", s.server.Addr)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		// first asking http to shut down and wait for it up to 10 seconds
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		errShutdown := s.server.Shutdown(ctx)

		if errShutdown != nil {
			return fmt.Errorf("failed to stop server: %w", errShutdown)
		}
	}

	_ = level.Info(s.logger).Log("msg", "Successfully stopped server")
	return nil
}

func (a *App) newServer() *server {
	srv := &http.Server{
		Addr: a.cfg.listenAddr,
	}

	// Implement maximum connection age, this avoids long running remote_write
	// connections creating an unbalanced share of load
	if a.cfg.maxConnectionAge.Seconds() > 0 {
		_ = level.Debug(a.logger).Log("msg", "Setup a max connection age for HTTP connections", "duration", a.cfg.maxConnectionAge)
		srv.IdleTimeout = a.cfg.maxConnectionAge
		srv.ConnContext = func(ctx context.Context, _ net.Conn) context.Context {
			return mcontext.ContextWithConnectionStartTime(ctx, time.Now())
		}
	}

	mux := http.NewServeMux()

	// setup metrics endpoint
	mux.Handle(a.cfg.telemetryPath, promhttp.InstrumentMetricHandler(
		a.registry, promhttp.HandlerFor(a.gatherer, promhttp.HandlerOpts{}),
	))

	// setup ready check
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			_ = level.Warn(a.logger).Log("msg", "Error replying to health check")
		}
	})

	// enable pprof endpoint if enabled by cmdline
	if !a.cfg.disablePprof {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	srv.Handler = mcontext.MaxConnectionAgeHandler(mux, a.cfg.maxConnectionAge)

	return &server{
		server: srv,
		mux:    mux,
		logger: a.logger,
	}
}

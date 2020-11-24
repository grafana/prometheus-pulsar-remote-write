package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	promlogflag "github.com/prometheus/common/promlog/flag"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

type App struct {
	app    *kingpin.Application
	logger log.Logger
	cfg    *config

	registry prometheus.Registerer
	gatherer prometheus.Gatherer

	cmdProduce *produceCommand
	cmdConsume *consumeCommand
}

func (a *App) WithConsumeBatchMaxDelay(d time.Duration) {
	a.cmdConsume.batchMaxDelay = d
}

type flagger interface {
	Flag(name, help string) *kingpin.FlagClause
}

type config struct {
	// generic options
	listenAddr       string
	telemetryPath    string
	promlogConfig    promlog.Config
	disablePprof     bool
	maxConnectionAge time.Duration
}

func New() *App {
	app := kingpin.New(filepath.Base(os.Args[0]), "Pulsar Remote storage adapter for Prometheus")
	app.HelpFlag.Short('h')

	cfg := &config{
		promlogConfig: promlog.Config{},
	}

	promlogflag.AddFlags(app, &cfg.promlogConfig)
	app.Flag("web.listen-address", "Address to listen on for web endpoints.").
		Default(":9201").StringVar(&cfg.listenAddr)
	app.Flag("web.telemetry-path", "Path under which to expose metrics.").
		Default("/metrics").StringVar(&cfg.telemetryPath)
	app.Flag("web.disable-pprof", "Disable the pprof tracing/debugging endpoints under /debug/pprof.").
		Default("false").BoolVar(&cfg.disablePprof)
	app.Flag("web.max-connection-age", "If set this limits the maximum lifetime of persistent HTTP connections.").
		Default("0s").DurationVar(&cfg.maxConnectionAge)

	a := &App{
		app:      app,
		cfg:      cfg,
		registry: prometheus.DefaultRegisterer,
		gatherer: prometheus.DefaultGatherer,
	}

	a.cmdProduce = newProduceCommand(a)
	a.cmdConsume = newConsumeCommand(a)

	return a
}

func (a *App) signalHandler(ctx context.Context) (context.Context, func()) {
	// trap Ctrl+C and call cancel on the context
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, func() {
		signal.Stop(c)
		cancel()
	}

}

// RunError signals no to display the error, as it already had been logged. The exit code should still be set though
type RunError struct {
	error
}

func (a *App) Run(ctx context.Context, args ...string) error {
	cmd, err := a.app.Parse(args)
	if err != nil {
		a.app.Usage(args)
		return fmt.Errorf("error parsing commandline arguments: %w", err)
	}

	// setup logger
	a.logger = promlog.New(&a.cfg.promlogConfig)

	// reduce verbosity of logrus which is used by the pulsar golang library
	if a.cfg.promlogConfig.Level.String() != "debug" {
		logrus.SetLevel(logrus.WarnLevel)
	}

	switch cmd {
	case a.cmdProduce.name:
		err = a.cmdProduce.run(ctx)
	case a.cmdConsume.name:
		err = a.cmdConsume.run(ctx)
	default:
		return fmt.Errorf("error unknown command %s", cmd)
	}

	if err != nil {
		_ = level.Error(a.logger).Log("command", cmd, "error", err)
		return RunError{err}
	}

	return nil
}

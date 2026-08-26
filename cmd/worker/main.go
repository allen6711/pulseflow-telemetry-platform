// Command worker runs the PulseFlow telemetry processing service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/allen6711/pulseflow-telemetry-platform/internal/config"
	"github.com/allen6711/pulseflow-telemetry-platform/internal/health"
	"github.com/allen6711/pulseflow-telemetry-platform/internal/httpserver"
	"github.com/allen6711/pulseflow-telemetry-platform/internal/lifecycle"
	"github.com/allen6711/pulseflow-telemetry-platform/internal/logging"
	"github.com/allen6711/pulseflow-telemetry-platform/internal/observability"
)

// Build metadata, injected at link time.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// healthcheck lets the container healthcheck probe this process without curl,
// which the distroless runtime image does not carry.
var healthcheck = flag.Bool("healthcheck", false,
	"probe this process's own health endpoint and exit 0 when healthy")

func run() error {
	flag.Parse()

	cfg, err := config.Load(config.WorkerDefaults)
	if err != nil {
		// The logger's own level comes from the configuration that just failed,
		// so this event is emitted through a default handler at stderr. The
		// process stops here: no listener opens, no client is constructed, and
		// no partially initialized state survives (FR-017).
		reportConfigFailure(err)
		return err
	}
	if cfg.Version == "dev" && version != "dev" {
		cfg.Version = version
	}

	if *healthcheck {
		return httpserver.SelfCheck(cfg.HTTPPort, health.ReadyPath, 3*time.Second)
	}

	log := logging.New(os.Stdout, logging.Options{
		ServiceName: cfg.ServiceName,
		Version:     cfg.Version,
		Level:       cfg.LogLevel,
	})

	// go-redis logs connection-pool diagnostics through its own package-level
	// logger; route them into slog so every line stays parseable (FR-024).
	redis.SetLogger(logging.NewRedisLogger(log))

	registry := observability.NewRegistry(observability.BuildInfo{Version: cfg.Version, Commit: commit})

	srv := httpserver.New(httpserver.Options{
		Port:     cfg.HTTPPort,
		Logger:   log,
		Registry: registry,
		Metrics:  observability.NewHTTPMetrics(registry),
	})
	srv.Handle(health.LivePath, health.LivenessHandler(cfg.ServiceName, cfg.Version))

	// Dependency clients are constructed lazily by the health checkers, so an
	// unreachable dependency never aborts startup (FR-037). The service starts,
	// reports not-ready, and becomes ready on its own once dependencies appear.
	kafkaChecker := health.NewKafkaChecker(cfg.KafkaBrokers)
	clickHouseChecker := health.NewClickHouseChecker(health.ClickHouseConfig{
		Addr:     cfg.ClickHouseAddr,
		Database: cfg.ClickHouseDatabase,
		User:     cfg.ClickHouseUser,
		Password: cfg.ClickHousePassword,
	})
	redisChecker := health.NewRedisChecker(health.RedisConfig{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
	})
	readiness := health.NewAggregator(health.AggregatorOptions{
		Service:  cfg.ServiceName,
		Version:  cfg.Version,
		Checkers: []health.Checker{kafkaChecker, clickHouseChecker, redisChecker},
		Timeout:  cfg.HealthCheckTimeout,
		CacheTTL: cfg.HealthCacheTTL,
		Logger:   log,
		Metrics:  observability.NewDependencyMetrics(registry),
	})
	srv.Handle(health.ReadyPath, health.ReadinessHandler(readiness))

	ctx, second, stop := lifecycle.Signals(context.Background())
	defer stop()

	// A signal that arrives while we were still wiring things up must not leave
	// a half-initialized process behind (FR-022).
	if lifecycle.Aborted(ctx) {
		log.Info("startup_aborted")
		return nil
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	log.InfoContext(ctx, "service_started",
		slog.Int("port", cfg.HTTPPort),
		slog.String("environment", cfg.Environment),
		slog.Any("config", cfg),
	)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	// Order is the contract: readiness first so traffic drains, then the
	// server drains in-flight requests, then clients close.
	return lifecycle.Shutdown(
		lifecycle.Options{
			GracePeriod: cfg.ShutdownGracePeriod,
			Logger:      log,
			Second:      second,
			Signal:      "SIGTERM",
		},
		lifecycle.Step{Name: "readiness_gate", Fn: func(context.Context) error {
			readiness.BeginShutdown()
			return nil
		}},
		lifecycle.Step{Name: "http_server", Fn: srv.Shutdown},
		lifecycle.Step{Name: "dependency_clients", Fn: func(context.Context) error {
			return errors.Join(
				kafkaChecker.Close(),
				clickHouseChecker.Close(),
				redisChecker.Close(),
			)
		}},
	)
}

// reportConfigFailure emits config_validation_failed with one entry per failing
// setting, then leaves the caller to exit non-zero.
func reportConfigFailure(err error) {
	log := logging.New(os.Stderr, logging.Options{
		ServiceName: "pulseflow-worker",
		Version:     version,
		Level:       "error",
	})

	var invalid *config.ValidationError
	if errors.As(err, &invalid) {
		log.Error("config_validation_failed",
			slog.Any("errors", invalid.Messages()),
			slog.Any("fields", invalid.Fields()),
		)
		return
	}
	log.Error("config_validation_failed", slog.String("error", err.Error()))
}

// Command tinyproxy-exporter exposes tinyproxy run-time data as Prometheus
// metrics.
//
// It observes tinyproxy through two independent channels:
//
//   - the StatHost statistics page, fetched through the proxy itself;
//   - the tinyproxy log file, followed incrementally.
//
// Both can be disabled individually; at least one has to stay enabled.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/zzhkhyl/tinyproxy-exporter/collector"
	"github.com/zzhkhyl/tinyproxy-exporter/config"
	"github.com/zzhkhyl/tinyproxy-exporter/source"
)

// Overridden at build time with -ldflags "-X main.version=... -X main.revision=...".
var (
	version   = "dev"
	revision  = "unknown"
	buildDate = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "tinyproxy-exporter: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: parseLogLevel(cfg.LogLevel)}))
	slog.SetDefault(logger)

	build := collector.BuildInfo{Version: version, Revision: revision, GoVersion: runtime.Version()}
	logger.Info("starting tinyproxy exporter",
		"version", version,
		"revision", revision,
		"build_date", buildDate,
		"listen_address", cfg.ListenAddress,
		"metrics_path", cfg.MetricsPath,
		"stats_enabled", cfg.StatsEnabled,
		"log_enabled", cfg.LogEnabled,
		"traffic_enabled", cfg.TrafficEnabled(),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var (
		wg      sync.WaitGroup
		stats   *source.StatsScraper
		logs    *source.LogTailer
		traffic *source.TrafficReader
	)

	if cfg.StatsEnabled {
		stats, err = source.NewStatsScraper(cfg.StatsURL(), cfg.ProxyURL, cfg.StatsInterval, cfg.StatsTimeout, logger)
		if err != nil {
			return fmt.Errorf("statistics page scraper: %w", err)
		}
		logger.Info("statistics page scraping enabled",
			"url", stats.URL(), "proxy", cfg.ProxyURL, "interval", stats.Interval().String())
	}

	if cfg.LogEnabled {
		logs, err = source.NewLogTailer(cfg.LogPath, cfg.LogPollInterval, cfg.LogFromStart,
			source.ParseOptions{StatsHost: cfg.StatsHost, MaxHosts: cfg.MaxHosts}, logger)
		if err != nil {
			return fmt.Errorf("log tailer: %w", err)
		}
		logger.Info("log tailing enabled", "path", logs.Path(), "from_start", cfg.LogFromStart)
	}

	if cfg.TrafficEnabled() {
		traffic, err = source.NewTrafficReader(cfg.TrafficInterface, cfg.TrafficInterval, logger)
		if err != nil {
			return fmt.Errorf("traffic reader: %w", err)
		}
		logger.Info("interface traffic collection enabled",
			"interface", traffic.Interface(), "interval", cfg.TrafficInterval.String())
	}

	if stats == nil && logs == nil && traffic == nil {
		return errors.New("every data source is disabled, nothing to export")
	}

	start := func(name string, run func(context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("data source started", "source", name)
			run(ctx)
			logger.Info("data source stopped", "source", name)
		}()
	}
	if stats != nil {
		start("stats", stats.Run)
	}
	if logs != nil {
		start("log", logs.Run)
	}
	if traffic != nil {
		start("traffic", traffic.Run)
	}

	// Convert to interfaces explicitly: assigning a nil *source.T would yield a
	// non-nil interface holding a nil pointer, which the collector cannot
	// distinguish from an enabled source.
	var (
		statsSource   collector.StatsSource
		logSource     collector.LogSource
		trafficSource collector.TrafficSource
	)
	if stats != nil {
		statsSource = stats
	}
	if logs != nil {
		logSource = logs
	}
	if traffic != nil {
		trafficSource = traffic
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collector.New(statsSource, logSource, trafficSource, build),
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	mux := http.NewServeMux()
	mux.Handle(cfg.MetricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
		ErrorHandling:     promhttp.ContinueOnError,
	}))
	mux.HandleFunc("/healthz", healthzHandler(func() []namedSource {
		return collectSources(stats, logs, traffic)
	}))
	if cfg.MetricsPath != "/" {
		mux.HandleFunc("/", indexHandler(cfg.MetricsPath))
	}

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "address", cfg.ListenAddress)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		stop()
		wg.Wait()
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown requested, draining")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown failed", "err", err)
	}

	wg.Wait()
	logger.Info("stopped")
	return nil
}

// namedSource couples a source name with its health for reporting.
type namedSource struct {
	name string
	h    source.HealthSnapshot
}

func collectSources(stats *source.StatsScraper, logs *source.LogTailer, traffic *source.TrafficReader) []namedSource {
	var out []namedSource
	if stats != nil {
		out = append(out, namedSource{"stats", stats.Health()})
	}
	if logs != nil {
		out = append(out, namedSource{"log", logs.Health()})
	}
	if traffic != nil {
		out = append(out, namedSource{"traffic", traffic.Health()})
	}
	return out
}

type healthSource struct {
	Name        string `json:"name"`
	Up          bool   `json:"up"`
	LastSuccess string `json:"last_success,omitempty"`
	LastError   string `json:"last_error,omitempty"`
	DurationMS  int64  `json:"duration_ms"`
	Failures    uint64 `json:"failures"`
	Attempts    uint64 `json:"attempts"`
}

type healthReport struct {
	Status   string         `json:"status"`
	Version  string         `json:"version"`
	Revision string         `json:"revision"`
	Sources  []healthSource `json:"sources"`
}

// healthzHandler reports the state of every enabled data source.
//
// It answers 200 while at least one source is healthy and 503 when none is, so
// a container health check notices that monitoring is blind.
func healthzHandler(sources func() []namedSource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := sources()

		report := healthReport{Status: "ok", Version: version, Revision: revision}
		anyUp := len(list) == 0
		for _, s := range list {
			entry := healthSource{
				Name:       s.name,
				Up:         s.h.Up,
				LastError:  s.h.LastError,
				DurationMS: s.h.Duration.Milliseconds(),
				Failures:   s.h.Failures,
				Attempts:   s.h.Attempts,
			}
			if !s.h.LastSuccess.IsZero() {
				entry.LastSuccess = s.h.LastSuccess.UTC().Format(time.RFC3339)
			}
			if s.h.Up {
				anyUp = true
			}
			report.Sources = append(report.Sources, entry)
		}

		status := http.StatusOK
		if !anyUp {
			report.Status = "degraded"
			status = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	}
}

func indexHandler(metricsPath string) http.HandlerFunc {
	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>tinyproxy exporter</title></head>
<body>
<h1>tinyproxy exporter</h1>
<p>Prometheus metrics: <a href="%[1]s">%[1]s</a></p>
<p>Health: <a href="/healthz">/healthz</a></p>
<p>Version %[2]s (revision %[3]s, built %[4]s)</p>
</body>
</html>
`, metricsPath, version, revision, buildDate)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}
}

func parseLogLevel(name string) slog.Level {
	switch name {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

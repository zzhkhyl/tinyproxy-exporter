package config

import (
	"strings"
	"testing"
	"time"
)

// allKeys lists every environment variable the exporter reads. Setting them to
// an empty value makes the loader fall back to its defaults, so tests are
// immune to whatever the developer has exported.
var allKeys = []string{
	"EXPORTER_LISTEN_ADDRESS",
	"EXPORTER_METRICS_PATH",
	"LOG_LEVEL",
	"TINYPROXY_STATS_ENABLED",
	"TINYPROXY_PROXY_URL",
	"TINYPROXY_STATS_HOST",
	"TINYPROXY_STATS_INTERVAL",
	"TINYPROXY_STATS_TIMEOUT",
	"TINYPROXY_LOG_ENABLED",
	"TINYPROXY_LOG_PATH",
	"TINYPROXY_LOG_POLL_INTERVAL",
	"TINYPROXY_LOG_FROM_START",
	"TINYPROXY_LOG_MAX_HOSTS",
	"TINYPROXY_TRAFFIC_INTERFACE",
	"TINYPROXY_TRAFFIC_INTERVAL",
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range allKeys {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ListenAddress != DefaultListenAddress {
		t.Errorf("listen address = %q, want %q", cfg.ListenAddress, DefaultListenAddress)
	}
	if cfg.MetricsPath != DefaultMetricsPath {
		t.Errorf("metrics path = %q, want %q", cfg.MetricsPath, DefaultMetricsPath)
	}
	if !cfg.StatsEnabled || !cfg.LogEnabled {
		t.Errorf("stats and log sources should be enabled by default: %+v", cfg)
	}
	if cfg.StatsURL() != "http://tinyproxy.stats/" {
		t.Errorf("stats url = %q", cfg.StatsURL())
	}
	if cfg.StatsInterval != DefaultStatsInterval || cfg.StatsTimeout != DefaultStatsTimeout {
		t.Errorf("stats timings = %s/%s", cfg.StatsInterval, cfg.StatsTimeout)
	}
	if cfg.LogFromStart {
		t.Error("log should be followed from the end by default")
	}
	if cfg.MaxHosts != DefaultMaxHosts {
		t.Errorf("max hosts = %d, want %d", cfg.MaxHosts, DefaultMaxHosts)
	}
	if cfg.TrafficEnabled() {
		t.Error("traffic collection must be off by default")
	}
}

func TestLoadOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("EXPORTER_LISTEN_ADDRESS", "127.0.0.1:9999")
	t.Setenv("EXPORTER_METRICS_PATH", "/prom")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("TINYPROXY_PROXY_URL", "http://proxy.internal:3128")
	t.Setenv("TINYPROXY_STATS_HOST", "stats.local")
	t.Setenv("TINYPROXY_STATS_INTERVAL", "45s")
	t.Setenv("TINYPROXY_STATS_TIMEOUT", "2s")
	t.Setenv("TINYPROXY_STATS_ENABLED", "false")
	t.Setenv("TINYPROXY_LOG_ENABLED", "false")
	t.Setenv("TINYPROXY_LOG_PATH", "/tmp/tinyproxy.log")
	t.Setenv("TINYPROXY_LOG_FROM_START", "true")
	t.Setenv("TINYPROXY_LOG_POLL_INTERVAL", "250ms")
	t.Setenv("TINYPROXY_LOG_MAX_HOSTS", "17")
	t.Setenv("TINYPROXY_TRAFFIC_INTERFACE", "eth0")
	t.Setenv("TINYPROXY_TRAFFIC_INTERVAL", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ListenAddress != "127.0.0.1:9999" || cfg.MetricsPath != "/prom" || cfg.LogLevel != "debug" {
		t.Errorf("http settings not applied: %+v", cfg)
	}
	if cfg.StatsEnabled || cfg.LogEnabled {
		t.Error("sources should be disabled")
	}
	if cfg.StatsURL() != "http://stats.local/" {
		t.Errorf("stats url = %q", cfg.StatsURL())
	}
	if cfg.StatsInterval != 45*time.Second || cfg.StatsTimeout != 2*time.Second {
		t.Errorf("stats timings = %s/%s", cfg.StatsInterval, cfg.StatsTimeout)
	}
	if !cfg.LogFromStart || cfg.LogPollInterval != 250*time.Millisecond || cfg.MaxHosts != 17 {
		t.Errorf("log settings not applied: %+v", cfg)
	}
	if !cfg.TrafficEnabled() || cfg.TrafficInterval != 30*time.Second {
		t.Errorf("traffic settings not applied: %+v", cfg)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "bad duration",
			env:  map[string]string{"TINYPROXY_STATS_INTERVAL": "soon"},
			want: "TINYPROXY_STATS_INTERVAL",
		},
		{
			name: "bad boolean",
			env:  map[string]string{"TINYPROXY_LOG_ENABLED": "maybe"},
			want: "TINYPROXY_LOG_ENABLED",
		},
		{
			name: "bad max hosts",
			env:  map[string]string{"TINYPROXY_LOG_MAX_HOSTS": "0"},
			want: "TINYPROXY_LOG_MAX_HOSTS",
		},
		{
			name: "proxy url without scheme",
			env:  map[string]string{"TINYPROXY_PROXY_URL": "tinyproxy:8888"},
			want: "TINYPROXY_PROXY_URL",
		},
		{
			name: "metrics path without slash",
			env:  map[string]string{"EXPORTER_METRICS_PATH": "metrics"},
			want: "EXPORTER_METRICS_PATH",
		},
		{
			name: "unknown log level",
			env:  map[string]string{"LOG_LEVEL": "verbose"},
			want: "LOG_LEVEL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			_, err := Load()
			if err == nil {
				t.Fatalf("expected an error mentioning %s", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %s", err, tc.want)
			}
		})
	}
}

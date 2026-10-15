// Package config loads the exporter configuration from the environment.
//
// Everything is configured through environment variables so the exporter can be
// driven declaratively from docker-compose or Kubernetes manifests.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Defaults used when the matching environment variable is unset.
const (
	DefaultListenAddress   = ":9119"
	DefaultMetricsPath     = "/metrics"
	DefaultLogLevel        = "info"
	DefaultProxyURL        = "http://tinyproxy:8888"
	DefaultStatsHost       = "tinyproxy.stats"
	DefaultStatsInterval   = 15 * time.Second
	DefaultStatsTimeout    = 5 * time.Second
	DefaultLogPath         = "/var/log/tinyproxy/tinyproxy.log"
	DefaultLogPollInterval = time.Second
	DefaultMaxHosts        = 500
	DefaultTrafficInterval = 15 * time.Second
)

// Config is the fully resolved exporter configuration.
type Config struct {
	// HTTP server.
	ListenAddress string
	MetricsPath   string
	LogLevel      string

	// StatHost statistics page (fetched through the proxy).
	StatsEnabled  bool
	ProxyURL      string
	StatsHost     string
	StatsInterval time.Duration
	StatsTimeout  time.Duration

	// Tinyproxy log file tailing.
	LogEnabled      bool
	LogPath         string
	LogPollInterval time.Duration
	LogFromStart    bool
	MaxHosts        int

	// Optional container level traffic counters (/proc/net/dev).
	TrafficInterface string
	TrafficInterval  time.Duration
}

// StatsURL is the URL requested through the proxy to obtain the statistics
// page. Tinyproxy intercepts requests whose Host header equals StatHost, so the
// host part of this URL is what makes the trick work.
func (c *Config) StatsURL() string {
	return "http://" + c.StatsHost + "/"
}

// TrafficEnabled reports whether interface level traffic collection is on.
func (c *Config) TrafficEnabled() bool { return c.TrafficInterface != "" }

// Load reads and validates the configuration from the process environment.
func Load() (*Config, error) {
	c := &Config{}
	p := &parser{}

	c.ListenAddress = envString("EXPORTER_LISTEN_ADDRESS", DefaultListenAddress)
	c.MetricsPath = envString("EXPORTER_METRICS_PATH", DefaultMetricsPath)
	c.LogLevel = strings.ToLower(envString("LOG_LEVEL", DefaultLogLevel))

	c.StatsEnabled = p.boolean("TINYPROXY_STATS_ENABLED", true)
	c.ProxyURL = envString("TINYPROXY_PROXY_URL", DefaultProxyURL)
	c.StatsHost = envString("TINYPROXY_STATS_HOST", DefaultStatsHost)
	c.StatsInterval = p.duration("TINYPROXY_STATS_INTERVAL", DefaultStatsInterval)
	c.StatsTimeout = p.duration("TINYPROXY_STATS_TIMEOUT", DefaultStatsTimeout)

	c.LogEnabled = p.boolean("TINYPROXY_LOG_ENABLED", true)
	c.LogPath = envString("TINYPROXY_LOG_PATH", DefaultLogPath)
	c.LogPollInterval = p.duration("TINYPROXY_LOG_POLL_INTERVAL", DefaultLogPollInterval)
	c.LogFromStart = p.boolean("TINYPROXY_LOG_FROM_START", false)
	c.MaxHosts = p.positiveInt("TINYPROXY_LOG_MAX_HOSTS", DefaultMaxHosts)

	c.TrafficInterface = strings.TrimSpace(envString("TINYPROXY_TRAFFIC_INTERFACE", ""))
	c.TrafficInterval = p.duration("TINYPROXY_TRAFFIC_INTERVAL", DefaultTrafficInterval)

	errs := p.errs()
	if err := c.validate(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return c, nil
}

func (c *Config) validate() error {
	var errs []error

	if strings.TrimSpace(c.ListenAddress) == "" {
		errs = append(errs, errors.New("EXPORTER_LISTEN_ADDRESS must not be empty"))
	}
	if !strings.HasPrefix(c.MetricsPath, "/") {
		errs = append(errs, fmt.Errorf("EXPORTER_METRICS_PATH must start with %q, got %q", "/", c.MetricsPath))
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "warning", "error":
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL must be one of debug|info|warn|error, got %q", c.LogLevel))
	}

	if c.StatsEnabled {
		u, err := url.Parse(c.ProxyURL)
		if err != nil {
			errs = append(errs, fmt.Errorf("TINYPROXY_PROXY_URL is not a valid URL: %w", err))
		} else {
			if u.Scheme != "http" && u.Scheme != "https" {
				errs = append(errs, fmt.Errorf("TINYPROXY_PROXY_URL must use http or https, got %q", u.Scheme))
			}
			if u.Host == "" {
				errs = append(errs, errors.New("TINYPROXY_PROXY_URL must contain a host:port"))
			}
		}
		if strings.TrimSpace(c.StatsHost) == "" {
			errs = append(errs, errors.New("TINYPROXY_STATS_HOST must not be empty"))
		}
		if c.StatsTimeout <= 0 {
			errs = append(errs, errors.New("TINYPROXY_STATS_TIMEOUT must be greater than zero"))
		}
		if c.StatsInterval <= 0 {
			errs = append(errs, errors.New("TINYPROXY_STATS_INTERVAL must be greater than zero"))
		}
	}

	if c.LogEnabled {
		if strings.TrimSpace(c.LogPath) == "" {
			errs = append(errs, errors.New("TINYPROXY_LOG_PATH must not be empty"))
		}
		if c.LogPollInterval <= 0 {
			errs = append(errs, errors.New("TINYPROXY_LOG_POLL_INTERVAL must be greater than zero"))
		}
	}

	return errors.Join(errs...)
}

// parser accumulates conversion failures so that a single run reports every
// misconfigured variable at once.
type parser struct {
	errsList []error
}

func (p *parser) errs() []error { return p.errsList }

func (p *parser) fail(key, raw, want string) {
	p.errsList = append(p.errsList, fmt.Errorf("invalid value %q for %s: want %s", raw, key, want))
}

func (p *parser) boolean(key string, def bool) bool {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return def
	}
	v, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		p.fail(key, raw, "a boolean such as true|false|1|0")
		return def
	}
	return v
}

func (p *parser) duration(key string, def time.Duration) time.Duration {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return def
	}
	v, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		p.fail(key, raw, "a duration such as 15s or 2m")
		return def
	}
	return v
}

func (p *parser) positiveInt(key string, def int) int {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return def
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		p.fail(key, raw, "a positive integer")
		return def
	}
	return v
}

func envString(key, def string) string {
	if raw, ok := os.LookupEnv(key); ok && strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	return def
}

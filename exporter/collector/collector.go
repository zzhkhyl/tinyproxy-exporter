// Package collector exposes everything the source package observes as
// Prometheus metrics.
//
// Semantics worth knowing:
//
//   - tinyproxy_up is 1 as soon as *one* enabled data source is healthy, i.e.
//     the proxy is observable. Use tinyproxy_exporter_all_sources_up and
//     tinyproxy_exporter_source_up for the granular view.
//
//   - The statistics page counters (requests, bad, denied, refused) are
//     cumulative per tinyproxy process and reset when it restarts. The source
//     package compensates for those resets, so they are exported as counters.
//     `connections_open` is a gauge because it goes up and down.
//
//   - tinyproxy_stats_value{metric="..."} mirrors every numeric label found on
//     the statistics page, so a future tinyproxy release cannot silently blind
//     the exporter.
//
//   - tinyproxy has no byte counters at all; see tinyproxy_traffic_* for the
//     optional interface level fallback.
package collector

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/zzhkhyl/tinyproxy-exporter/source"
)

const namespace = "tinyproxy"

// BuildInfo describes the exporter build.
type BuildInfo struct {
	Version   string
	Revision  string
	GoVersion string
}

// StatsSource provides statistics page observations.
type StatsSource interface {
	Latest() source.StatsScrape
	Health() source.HealthSnapshot
}

// LogSource provides log derived observations.
type LogSource interface {
	Counters() source.LogCounters
	Health() source.HealthSnapshot
}

// TrafficSource provides interface level byte counters.
type TrafficSource interface {
	Latest() source.TrafficCounters
	Health() source.HealthSnapshot
}

// Collector implements prometheus.Collector.
type Collector struct {
	stats   StatsSource
	logs    LogSource
	traffic TrafficSource
	build   BuildInfo

	descs []*prometheus.Desc

	up           *prometheus.Desc
	allSourcesUp *prometheus.Desc
	sourceUp     *prometheus.Desc
	sourceLastOK *prometheus.Desc
	sourceTime   *prometheus.Desc
	sourceFails  *prometheus.Desc
	buildInfo    *prometheus.Desc

	connectionsOpen    *prometheus.Desc
	requestsTotal      *prometheus.Desc
	badConnections     *prometheus.Desc
	deniedConnections  *prometheus.Desc
	refusedConnections *prometheus.Desc
	stathostInfo       *prometheus.Desc
	statsValue         *prometheus.Desc

	logLines          *prometheus.Desc
	logReadBytes      *prometheus.Desc
	logParseErrors    *prometheus.Desc
	logUnknown        *prometheus.Desc
	logClientConns    *prometheus.Desc
	logEstablished    *prometheus.Desc
	logClosed         *prometheus.Desc
	logBadRequests    *prometheus.Desc
	logStathost       *prometheus.Desc
	logRequestEntity  *prometheus.Desc
	logRequests       *prometheus.Desc
	logRequestsByHost *prometheus.Desc
	logDenied         *prometheus.Desc
	logErrors         *prometheus.Desc

	trafficReceived *prometheus.Desc
	trafficSent     *prometheus.Desc
}

// New builds the collector. Any of stats/logs/traffic may be nil, in which case
// the corresponding metrics are simply not exported.
//
// Callers must pass an untyped nil (not a nil *source.StatsScraper) to disable a
// source.
func New(stats StatsSource, logs LogSource, traffic TrafficSource, build BuildInfo) *Collector {
	c := &Collector{
		stats:   stats,
		logs:    logs,
		traffic: traffic,
		build:   build,

		up: prometheus.NewDesc(
			namespace+"_up",
			"1 when tinyproxy is observable through at least one enabled data source.",
			nil, nil),
		allSourcesUp: prometheus.NewDesc(
			namespace+"_exporter_all_sources_up",
			"1 when every enabled data source succeeded on its most recent attempt.",
			nil, nil),
		sourceUp: prometheus.NewDesc(
			namespace+"_exporter_source_up",
			"1 when the data source succeeded on its most recent attempt.",
			[]string{"source"}, nil),
		sourceLastOK: prometheus.NewDesc(
			namespace+"_exporter_source_last_success_timestamp_seconds",
			"Unix timestamp of the most recent successful read of the data source.",
			[]string{"source"}, nil),
		sourceTime: prometheus.NewDesc(
			namespace+"_exporter_source_scrape_duration_seconds",
			"Duration of the most recent read of the data source.",
			[]string{"source"}, nil),
		sourceFails: prometheus.NewDesc(
			namespace+"_exporter_source_scrape_failures_total",
			"Number of failed reads of the data source.",
			[]string{"source"}, nil),
		buildInfo: prometheus.NewDesc(
			namespace+"_exporter_build_info",
			"Build information of the exporter.",
			[]string{"version", "revision", "goversion"}, nil),

		connectionsOpen: prometheus.NewDesc(
			namespace+"_connections_open",
			"Number of currently open connections (tinyproxy statistics page: num_open).",
			nil, nil),
		requestsTotal: prometheus.NewDesc(
			namespace+"_requests_total",
			"Total number of requests accepted (tinyproxy statistics page: num_reqs, incremented per accepted connection).",
			nil, nil),
		badConnections: prometheus.NewDesc(
			namespace+"_bad_connections_total",
			"Total number of bad connections (tinyproxy statistics page: num_badcons).",
			nil, nil),
		deniedConnections: prometheus.NewDesc(
			namespace+"_denied_connections_total",
			"Total number of denied connections (tinyproxy statistics page: num_denied).",
			nil, nil),
		refusedConnections: prometheus.NewDesc(
			namespace+"_refused_connections_total",
			"Total number of connections refused due to high load (tinyproxy statistics page: num_refused).",
			nil, nil),
		stathostInfo: prometheus.NewDesc(
			namespace+"_stathost_info",
			"Build information reported by the tinyproxy statistics page.",
			[]string{"version", "package"}, nil),
		statsValue: prometheus.NewDesc(
			namespace+"_stats_value",
			"Raw numeric values found on the tinyproxy statistics page, mirroring every field verbatim.",
			[]string{"metric"}, nil),

		logLines: prometheus.NewDesc(
			namespace+"_log_lines_total",
			"Number of tinyproxy log lines read by the exporter.",
			nil, nil),
		logReadBytes: prometheus.NewDesc(
			namespace+"_log_read_bytes_total",
			"Number of bytes read from the tinyproxy log file.",
			nil, nil),
		logParseErrors: prometheus.NewDesc(
			namespace+"_log_parse_errors_total",
			"Number of log lines that did not match tinyproxy's log line format.",
			nil, nil),
		logUnknown: prometheus.NewDesc(
			namespace+"_log_unrecognized_lines_total",
			"Number of well formed log lines whose message has no classification rule yet.",
			nil, nil),
		logClientConns: prometheus.NewDesc(
			namespace+"_log_client_connections_total",
			"Number of accepted client connections, from the log. Includes statistics page requests, matching tinyproxy's own STAT_OPEN counter; use tinyproxy_log_requests_total for proxied requests only.",
			nil, nil),
		logEstablished: prometheus.NewDesc(
			namespace+"_log_established_connections_total",
			"Number of upstream connections established, from the log.",
			nil, nil),
		logClosed: prometheus.NewDesc(
			namespace+"_log_closed_connections_total",
			"Number of client connections closed, from the log.",
			nil, nil),
		logBadRequests: prometheus.NewDesc(
			namespace+"_log_bad_requests_total",
			"Number of rejected requests (bad request, unknown method), from the log.",
			nil, nil),
		logStathost: prometheus.NewDesc(
			namespace+"_log_stathost_requests_total",
			"Number of statistics page requests observed in the log (the exporter's own scrapes when it is the only client).",
			nil, nil),
		logRequestEntity: prometheus.NewDesc(
			namespace+"_log_request_entity_bytes_total",
			"Bytes of request bodies relayed, summed from 'Read request entity of N bytes' log lines.",
			nil, nil),
		logRequests: prometheus.NewDesc(
			namespace+"_log_requests_total",
			"Number of proxied requests by HTTP method, from the log.",
			[]string{"method"}, nil),
		logRequestsByHost: prometheus.NewDesc(
			namespace+"_log_requests_by_host_total",
			"Number of proxied requests by target host, from the log.",
			[]string{"host"}, nil),
		logDenied: prometheus.NewDesc(
			namespace+"_log_denied_total",
			"Number of refusals by reason (acl, filter, auth, connect_port, max_clients), from the log.",
			[]string{"reason"}, nil),
		logErrors: prometheus.NewDesc(
			namespace+"_log_errors_total",
			"Number of internal failures by kind, from the log.",
			[]string{"type"}, nil),

		trafficReceived: prometheus.NewDesc(
			namespace+"_traffic_received_bytes_total",
			"Interface bytes received. Reflects tinyproxy traffic only when the exporter shares its network namespace.",
			[]string{"interface"}, nil),
		trafficSent: prometheus.NewDesc(
			namespace+"_traffic_sent_bytes_total",
			"Interface bytes sent. Reflects tinyproxy traffic only when the exporter shares its network namespace.",
			[]string{"interface"}, nil),
	}

	c.descs = []*prometheus.Desc{
		c.up, c.allSourcesUp, c.sourceUp, c.sourceLastOK, c.sourceTime, c.sourceFails, c.buildInfo,
		c.connectionsOpen, c.requestsTotal, c.badConnections, c.deniedConnections,
		c.refusedConnections, c.stathostInfo, c.statsValue,
		c.logLines, c.logReadBytes, c.logParseErrors, c.logUnknown, c.logEstablished,
		c.logClientConns,
		c.logClosed, c.logBadRequests, c.logStathost, c.logRequestEntity,
		c.logRequests, c.logRequestsByHost, c.logDenied, c.logErrors,
		c.trafficReceived, c.trafficSent,
	}
	return c
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range c.descs {
		ch <- d
	}
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	metrics := make([]prometheus.Metric, 0, 64)
	add := func(d *prometheus.Desc, vt prometheus.ValueType, v float64, labels ...string) {
		metrics = append(metrics, prometheus.MustNewConstMetric(d, vt, v, labels...))
	}

	c.collectHealth(add)
	c.collectStats(add)
	c.collectLogs(add)
	c.collectTraffic(add)

	add(c.buildInfo, prometheus.GaugeValue, 1, c.build.Version, c.build.Revision, c.build.GoVersion)

	for _, m := range metrics {
		ch <- m
	}
}

type namedHealth struct {
	name string
	h    source.HealthSnapshot
}

func (c *Collector) enabledSources() []namedHealth {
	var out []namedHealth
	if c.stats != nil {
		out = append(out, namedHealth{"stats", c.stats.Health()})
	}
	if c.logs != nil {
		out = append(out, namedHealth{"log", c.logs.Health()})
	}
	if c.traffic != nil {
		out = append(out, namedHealth{"traffic", c.traffic.Health()})
	}
	return out
}

func (c *Collector) collectHealth(add func(*prometheus.Desc, prometheus.ValueType, float64, ...string)) {
	sources := c.enabledSources()

	anyUp := len(sources) == 0
	allUp := true
	for _, s := range sources {
		if s.h.Up {
			anyUp = true
		} else {
			allUp = false
		}
	}

	up := 0.0
	if anyUp {
		up = 1
	}
	all := 0.0
	if allUp {
		all = 1
	}
	add(c.up, prometheus.GaugeValue, up)
	add(c.allSourcesUp, prometheus.GaugeValue, all)

	for _, s := range sources {
		healthy := 0.0
		if s.h.Up {
			healthy = 1
		}
		add(c.sourceUp, prometheus.GaugeValue, healthy, s.name)
		add(c.sourceTime, prometheus.GaugeValue, s.h.Duration.Seconds(), s.name)
		add(c.sourceFails, prometheus.CounterValue, float64(s.h.Failures), s.name)
		if !s.h.LastSuccess.IsZero() {
			add(c.sourceLastOK, prometheus.GaugeValue, float64(s.h.LastSuccess.Unix()), s.name)
		}
	}
}

func (c *Collector) collectStats(add func(*prometheus.Desc, prometheus.ValueType, float64, ...string)) {
	if c.stats == nil {
		return
	}
	scrape := c.stats.Latest()
	if scrape.Result == nil {
		return
	}

	fields := scrape.Result.Fields
	if v, ok := fields[source.FieldOpenConnections]; ok {
		add(c.connectionsOpen, prometheus.GaugeValue, v)
	}
	counters := scrape.Counters
	if v, ok := counters[source.FieldRequestsTotal]; ok {
		add(c.requestsTotal, prometheus.CounterValue, v)
	}
	if v, ok := counters[source.FieldBadConnections]; ok {
		add(c.badConnections, prometheus.CounterValue, v)
	}
	if v, ok := counters[source.FieldDeniedConnections]; ok {
		add(c.deniedConnections, prometheus.CounterValue, v)
	}
	if v, ok := counters[source.FieldRefusedConnections]; ok {
		add(c.refusedConnections, prometheus.CounterValue, v)
	}

	for label, v := range scrape.Result.Raw {
		add(c.statsValue, prometheus.GaugeValue, v, label)
	}

	if version, ok := scrape.Result.Text[source.TextFieldVersion]; ok {
		add(c.stathostInfo, prometheus.GaugeValue, 1, version, scrape.Result.Text[source.TextFieldPackage])
	}
}

func (c *Collector) collectLogs(add func(*prometheus.Desc, prometheus.ValueType, float64, ...string)) {
	if c.logs == nil {
		return
	}
	lc := c.logs.Counters()

	add(c.logLines, prometheus.CounterValue, float64(lc.Lines))
	add(c.logReadBytes, prometheus.CounterValue, float64(lc.ReadBytes))
	add(c.logParseErrors, prometheus.CounterValue, float64(lc.ParseErrors))
	add(c.logUnknown, prometheus.CounterValue, float64(lc.Unknown))
	add(c.logClientConns, prometheus.CounterValue, float64(lc.ClientConnections))
	add(c.logEstablished, prometheus.CounterValue, float64(lc.Established))
	add(c.logClosed, prometheus.CounterValue, float64(lc.Closed))
	add(c.logBadRequests, prometheus.CounterValue, float64(lc.BadRequests))
	add(c.logStathost, prometheus.CounterValue, float64(lc.StathostRequests))
	add(c.logRequestEntity, prometheus.CounterValue, float64(lc.RequestEntityBytes))

	for method, v := range lc.Requests {
		add(c.logRequests, prometheus.CounterValue, float64(v), method)
	}
	for host, v := range lc.RequestsByHost {
		add(c.logRequestsByHost, prometheus.CounterValue, float64(v), host)
	}
	for reason, v := range lc.Denied {
		add(c.logDenied, prometheus.CounterValue, float64(v), reason)
	}
	for kind, v := range lc.Errors {
		add(c.logErrors, prometheus.CounterValue, float64(v), kind)
	}
}

func (c *Collector) collectTraffic(add func(*prometheus.Desc, prometheus.ValueType, float64, ...string)) {
	if c.traffic == nil {
		return
	}
	tc := c.traffic.Latest()
	if !tc.Seen() {
		return
	}
	add(c.trafficReceived, prometheus.CounterValue, tc.Received, tc.Interface)
	add(c.trafficSent, prometheus.CounterValue, tc.Sent, tc.Interface)
}

package collector

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/zzhkhyl/tinyproxy-exporter/source"
)

type fakeStats struct {
	scrape source.StatsScrape
	health source.HealthSnapshot
}

func (f fakeStats) Latest() source.StatsScrape    { return f.scrape }
func (f fakeStats) Health() source.HealthSnapshot { return f.health }

type fakeLog struct {
	counters source.LogCounters
	health   source.HealthSnapshot
}

func (f fakeLog) Counters() source.LogCounters  { return f.counters }
func (f fakeLog) Health() source.HealthSnapshot { return f.health }

type fakeTraffic struct {
	counters source.TrafficCounters
	health   source.HealthSnapshot
}

func (f fakeTraffic) Latest() source.TrafficCounters { return f.counters }
func (f fakeTraffic) Health() source.HealthSnapshot  { return f.health }

func healthy() source.HealthSnapshot {
	return source.HealthSnapshot{Up: true, LastSuccess: time.Now(), Duration: 5 * time.Millisecond, Failures: 1, Attempts: 2}
}

func gather(t *testing.T, c *Collector) map[string]*dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := make(map[string]*dto.MetricFamily, len(families))
	for _, f := range families {
		out[f.GetName()] = f
	}
	return out
}

func scalar(t *testing.T, families map[string]*dto.MetricFamily, name string) float64 {
	t.Helper()
	f, ok := families[name]
	if !ok {
		t.Fatalf("metric %s is missing", name)
	}
	metrics := f.GetMetric()
	if len(metrics) != 1 {
		t.Fatalf("metric %s has %d samples, want 1", name, len(metrics))
	}
	return value(metrics[0])
}

func labeled(t *testing.T, families map[string]*dto.MetricFamily, name string, want map[string]string) float64 {
	t.Helper()
	f, ok := families[name]
	if !ok {
		t.Fatalf("metric %s is missing", name)
	}
	for _, m := range f.GetMetric() {
		if hasLabels(m, want) {
			return value(m)
		}
	}
	t.Fatalf("metric %s has no sample with labels %v", name, want)
	return 0
}

func hasLabels(m *dto.Metric, want map[string]string) bool {
	got := make(map[string]string, len(m.GetLabel()))
	for _, l := range m.GetLabel() {
		got[l.GetName()] = l.GetValue()
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

func value(m *dto.Metric) float64 {
	switch {
	case m.Gauge != nil:
		return m.Gauge.GetValue()
	case m.Counter != nil:
		return m.Counter.GetValue()
	case m.Untyped != nil:
		return m.Untyped.GetValue()
	}
	return 0
}

func TestCollectorExportsEveryChannel(t *testing.T) {
	stats := fakeStats{
		scrape: source.StatsScrape{
			Result: &source.StatsResult{
				Fields:  map[string]float64{source.FieldOpenConnections: 3},
				Text:    map[string]string{source.TextFieldVersion: "1.11.3", source.TextFieldPackage: "tinyproxy"},
				Raw:     map[string]float64{"open_connections": 3},
				RawText: map[string]string{},
			},
			Counters: map[string]float64{
				source.FieldRequestsTotal:     142,
				source.FieldBadConnections:    0,
				source.FieldDeniedConnections: 2,
			},
			Duration: 3 * time.Millisecond,
			At:       time.Now(),
		},
		health: healthy(),
	}

	logs := fakeLog{
		counters: source.LogCounters{
			Lines:              12,
			ReadBytes:          900,
			ParseErrors:        1,
			Unknown:            2,
			Requests:           map[string]uint64{"GET": 1, "CONNECT": 1},
			RequestsByHost:     map[string]uint64{"example.com": 2},
			ClientConnections:  7,
			Established:        1,
			Closed:             1,
			Denied:             map[string]uint64{"acl": 1, "max_clients": 3},
			BadRequests:        1,
			Errors:             map[string]uint64{"upstream_connect": 1},
			StathostRequests:   4,
			RequestEntityBytes: 2048,
		},
		health: healthy(),
	}

	traffic := fakeTraffic{
		counters: source.TrafficCounters{Interface: "eth0", Received: 1000, Sent: 2000, At: time.Now()},
		health:   healthy(),
	}

	families := gather(t, New(stats, logs, traffic, BuildInfo{Version: "1.2.3", Revision: "abc", GoVersion: "go1.26"}))

	// Aggregated health.
	if got := scalar(t, families, "tinyproxy_up"); got != 1 {
		t.Errorf("tinyproxy_up = %v, want 1", got)
	}
	if got := scalar(t, families, "tinyproxy_exporter_all_sources_up"); got != 1 {
		t.Errorf("all_sources_up = %v, want 1", got)
	}
	for _, src := range []string{"stats", "log", "traffic"} {
		if got := labeled(t, families, "tinyproxy_exporter_source_up", map[string]string{"source": src}); got != 1 {
			t.Errorf("source_up{source=%s} = %v, want 1", src, got)
		}
	}

	// Statistics page channel.
	if got := scalar(t, families, "tinyproxy_connections_open"); got != 3 {
		t.Errorf("connections_open = %v, want 3", got)
	}
	if got := scalar(t, families, "tinyproxy_requests_total"); got != 142 {
		t.Errorf("requests_total = %v, want 142", got)
	}
	if got := scalar(t, families, "tinyproxy_bad_connections_total"); got != 0 {
		t.Errorf("bad_connections_total = %v, want 0", got)
	}
	if got := scalar(t, families, "tinyproxy_denied_connections_total"); got != 2 {
		t.Errorf("denied_connections_total = %v, want 2", got)
	}
	if _, ok := families["tinyproxy_refused_connections_total"]; ok {
		t.Error("refused_connections_total must be absent when the page does not report it")
	}
	if got := labeled(t, families, "tinyproxy_stats_value", map[string]string{"metric": "open_connections"}); got != 3 {
		t.Errorf("stats_value passthrough = %v, want 3", got)
	}
	if got := labeled(t, families, "tinyproxy_stathost_info",
		map[string]string{"version": "1.11.3", "package": "tinyproxy"}); got != 1 {
		t.Errorf("stathost_info = %v, want 1", got)
	}

	// Log channel.
	if got := scalar(t, families, "tinyproxy_log_lines_total"); got != 12 {
		t.Errorf("log_lines_total = %v, want 12", got)
	}
	if got := scalar(t, families, "tinyproxy_log_parse_errors_total"); got != 1 {
		t.Errorf("log_parse_errors_total = %v, want 1", got)
	}
	if got := scalar(t, families, "tinyproxy_log_unrecognized_lines_total"); got != 2 {
		t.Errorf("log_unrecognized_lines_total = %v, want 2", got)
	}
	if got := labeled(t, families, "tinyproxy_log_requests_total", map[string]string{"method": "GET"}); got != 1 {
		t.Errorf("log_requests_total{method=GET} = %v, want 1", got)
	}
	if got := labeled(t, families, "tinyproxy_log_requests_by_host_total", map[string]string{"host": "example.com"}); got != 2 {
		t.Errorf("log_requests_by_host_total = %v, want 2", got)
	}
	if got := scalar(t, families, "tinyproxy_log_client_connections_total"); got != 7 {
		t.Errorf("log_client_connections_total = %v, want 7", got)
	}
	if got := labeled(t, families, "tinyproxy_log_denied_total", map[string]string{"reason": "max_clients"}); got != 3 {
		t.Errorf("log_denied_total{reason=max_clients} = %v, want 3", got)
	}
	if got := labeled(t, families, "tinyproxy_log_errors_total", map[string]string{"type": "upstream_connect"}); got != 1 {
		t.Errorf("log_errors_total = %v, want 1", got)
	}
	if got := scalar(t, families, "tinyproxy_log_request_entity_bytes_total"); got != 2048 {
		t.Errorf("log_request_entity_bytes_total = %v, want 2048", got)
	}
	if got := scalar(t, families, "tinyproxy_log_stathost_requests_total"); got != 4 {
		t.Errorf("log_stathost_requests_total = %v, want 4", got)
	}

	// Traffic channel.
	if got := labeled(t, families, "tinyproxy_traffic_received_bytes_total", map[string]string{"interface": "eth0"}); got != 1000 {
		t.Errorf("traffic received = %v, want 1000", got)
	}
	if got := labeled(t, families, "tinyproxy_traffic_sent_bytes_total", map[string]string{"interface": "eth0"}); got != 2000 {
		t.Errorf("traffic sent = %v, want 2000", got)
	}

	// Build info.
	if got := labeled(t, families, "tinyproxy_exporter_build_info",
		map[string]string{"version": "1.2.3", "revision": "abc", "goversion": "go1.26"}); got != 1 {
		t.Errorf("build_info = %v, want 1", got)
	}
}

func TestCollectorReportsDownSources(t *testing.T) {
	down := source.HealthSnapshot{Up: false, LastError: "connection refused", Failures: 3, Attempts: 3}

	families := gather(t, New(
		fakeStats{health: down},
		fakeLog{counters: source.LogCounters{}, health: down},
		nil,
		BuildInfo{Version: "test"},
	))

	if got := scalar(t, families, "tinyproxy_up"); got != 0 {
		t.Errorf("tinyproxy_up = %v, want 0", got)
	}
	if got := scalar(t, families, "tinyproxy_exporter_all_sources_up"); got != 0 {
		t.Errorf("all_sources_up = %v, want 0", got)
	}
	if got := labeled(t, families, "tinyproxy_exporter_source_scrape_failures_total", map[string]string{"source": "log"}); got != 3 {
		t.Errorf("scrape failures = %v, want 3", got)
	}
	if _, ok := families["tinyproxy_exporter_source_last_success_timestamp_seconds"]; ok {
		t.Error("a source that never succeeded must not publish a last success timestamp")
	}
	if _, ok := families["tinyproxy_traffic_received_bytes_total"]; ok {
		t.Error("a disabled traffic reader must not be exported")
	}
}

func TestCollectorWithoutAnySourceIsUp(t *testing.T) {
	// The exporter itself is alive; there is simply nothing to observe.
	families := gather(t, New(nil, nil, nil, BuildInfo{Version: "test"}))
	if got := scalar(t, families, "tinyproxy_up"); got != 1 {
		t.Errorf("tinyproxy_up = %v, want 1", got)
	}
	if _, ok := families["tinyproxy_log_lines_total"]; ok {
		t.Error("log metrics must be absent when log tailing is disabled")
	}
}

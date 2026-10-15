package source

import (
	"testing"
)

// Lines below are byte-accurate copies of what tinyproxy 1.11.3 writes, using
// the envelope from src/log.c:176-182 and the format strings from src/reqs.c,
// src/conns.c, src/child.c and src/acl.c.
const sampleLog = `CONNECT   Oct 09 15:45:00.123 [1234]: Request (file descriptor 6): GET http://example.com/index.html HTTP/1.1
CONNECT   Oct 09 15:45:00.200 [1234]: Established connection to host "example.com" using file descriptor 8.
INFO      Oct 09 15:45:01.000 [1234]: Closed connection between local client (fd:6) and remote client (fd:8)
NOTICE    Oct 09 15:45:02.000 [1234]: Unauthorized connection from "10.0.0.5".
NOTICE    Oct 09 15:45:03.000 [1234]: Request for the stathost.
CONNECT   Oct 09 15:45:03.100 [1234]: Request (file descriptor 7): GET http://tinyproxy.stats/ HTTP/1.1
INFO      Oct 09 15:45:04.000 [1234]: Read request entity of 2048 bytes
CONNECT   Oct 09 15:45:05.000 [1234]: Request (file descriptor 9): CONNECT example.org:443 HTTP/1.1
ERROR     Oct 09 15:45:06.000 [1234]: process_request: Bad Request on file descriptor 9
WARNING   Oct 09 15:45:07.000 [1234]: Maximum number of connections reached. Refusing new connections.
NOTICE    Oct 09 15:45:08.000 [1234]: Proxying refused on filtered domain "ads.example.net"
INFO      Oct 09 15:45:09.000 [1234]: Failed auth attempt (file descriptor: 10), ip 10.0.0.9
`

func parseAll(t *testing.T, log string, opts ParseOptions) LogCounters {
	t.Helper()
	counters := newLogCounters()
	for _, line := range splitLines(log) {
		ParseLogLine(line, &counters, opts)
	}
	return counters
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func TestParseLogLineClassifiesRealTinyproxyMessages(t *testing.T) {
	c := parseAll(t, sampleLog, ParseOptions{StatsHost: "tinyproxy.stats", MaxHosts: 100})

	if c.Lines != 12 {
		t.Fatalf("lines = %d, want 12", c.Lines)
	}
	if c.ParseErrors != 0 {
		t.Fatalf("parse errors = %d, want 0", c.ParseErrors)
	}
	if c.Unknown != 0 {
		t.Fatalf("unrecognized lines = %d, want 0", c.Unknown)
	}

	if got := c.Requests["GET"]; got != 1 {
		t.Fatalf("GET requests = %d, want 1 (the stathost request must be excluded)", got)
	}
	if got := c.Requests["CONNECT"]; got != 1 {
		t.Fatalf("CONNECT requests = %d, want 1", got)
	}
	if got := c.RequestsByHost["example.com"]; got != 1 {
		t.Fatalf("example.com requests = %d, want 1 (%v)", got, c.RequestsByHost)
	}
	if got := c.RequestsByHost["example.org"]; got != 1 {
		t.Fatalf("example.org requests = %d, want 1 (%v)", got, c.RequestsByHost)
	}
	if _, ok := c.RequestsByHost["tinyproxy.stats"]; ok {
		t.Fatalf("the stathost must not appear as a proxied host: %v", c.RequestsByHost)
	}

	if c.StathostRequests != 1 {
		t.Fatalf("stathost requests = %d, want 1", c.StathostRequests)
	}
	if c.Established != 1 {
		t.Fatalf("established = %d, want 1", c.Established)
	}
	if c.Closed != 1 {
		t.Fatalf("closed = %d, want 1", c.Closed)
	}
	if c.BadRequests != 1 {
		t.Fatalf("bad requests = %d, want 1", c.BadRequests)
	}
	if c.RequestEntityBytes != 2048 {
		t.Fatalf("request entity bytes = %d, want 2048", c.RequestEntityBytes)
	}

	for _, reason := range []string{"acl", "max_clients", "filter", "auth"} {
		if got := c.Denied[reason]; got != 1 {
			t.Fatalf("denied[%s] = %d, want 1 (%v)", reason, got, c.Denied)
		}
	}
}

func TestParseLogLineCountsMethods(t *testing.T) {
	log := `CONNECT   Oct 09 15:45:00.000 [1]: Request (file descriptor 4): POST http://api.example.com/v1 HTTP/1.1
CONNECT   Oct 09 15:45:00.100 [1]: Request (file descriptor 4): HEAD http://api.example.com/v1 HTTP/1.1
CONNECT   Oct 09 15:45:00.200 [1]: Request (file descriptor 4): POST http://api.example.com/v1 HTTP/1.1
`
	c := parseAll(t, log, ParseOptions{StatsHost: "tinyproxy.stats", MaxHosts: 100})
	if c.Requests["POST"] != 2 || c.Requests["HEAD"] != 1 {
		t.Fatalf("method counters = %v, want POST=2 HEAD=1", c.Requests)
	}
	if c.RequestsByHost["api.example.com"] != 3 {
		t.Fatalf("host counter = %v, want 3", c.RequestsByHost)
	}
}

func TestParseLogLineHandlesOriginFormRequests(t *testing.T) {
	// Reverse / transparent proxying logs an origin form target.
	log := "CONNECT   Oct 09 15:45:00.000 [1]: Request (file descriptor 4): GET /healthz HTTP/1.1\n"
	c := parseAll(t, log, ParseOptions{MaxHosts: 100})
	if c.Requests["GET"] != 1 {
		t.Fatalf("origin form request not counted: %v", c.Requests)
	}
	if len(c.RequestsByHost) != 0 {
		t.Fatalf("origin form request must not invent a host: %v", c.RequestsByHost)
	}
}

func TestParseLogLineBoundsHostCardinality(t *testing.T) {
	log := `CONNECT   Oct 09 15:45:00.000 [1]: Request (file descriptor 4): GET http://a.example/ HTTP/1.1
CONNECT   Oct 09 15:45:00.100 [1]: Request (file descriptor 4): GET http://b.example/ HTTP/1.1
CONNECT   Oct 09 15:45:00.200 [1]: Request (file descriptor 4): GET http://c.example/ HTTP/1.1
CONNECT   Oct 09 15:45:00.300 [1]: Request (file descriptor 4): GET http://a.example/ HTTP/1.1
`
	c := parseAll(t, log, ParseOptions{MaxHosts: 2})
	if len(c.RequestsByHost) != 3 {
		t.Fatalf("expected 2 hosts plus the overflow bucket, got %v", c.RequestsByHost)
	}
	if got := c.RequestsByHost[overflowHostLabel]; got != 1 {
		t.Fatalf("overflow bucket = %d, want 1 (%v)", got, c.RequestsByHost)
	}
	if got := c.RequestsByHost["a.example"]; got != 2 {
		t.Fatalf("known host must keep being counted: %v", c.RequestsByHost)
	}
}

func TestParseLogLineRejectsMalformedLines(t *testing.T) {
	log := `this is not a tinyproxy log line
CONNECT   Oct 09 : truncated [1234]: Request (file descriptor 6): GET http://example.com/ HTTP/1.1
`
	c := parseAll(t, log, ParseOptions{})
	if c.ParseErrors != 2 {
		t.Fatalf("parse errors = %d, want 2", c.ParseErrors)
	}
	if c.Lines != 2 {
		t.Fatalf("lines = %d, want 2", c.Lines)
	}
}

func TestParseLogLineCountsWellFormedButUnknownMessages(t *testing.T) {
	log := "INFO      Oct 09 15:45:00.000 [1]: A brand new tinyproxy message nobody has seen yet.\n"
	c := parseAll(t, log, ParseOptions{})
	if c.Unknown != 1 {
		t.Fatalf("unrecognized = %d, want 1", c.Unknown)
	}
	if c.ParseErrors != 0 {
		t.Fatalf("a well formed line must not be a parse error: %d", c.ParseErrors)
	}
}

func TestParseLogLineRecognisesBenignNoiseWithoutCountingItAsUnknown(t *testing.T) {
	// Verbatim from a live tinyproxy 1.11.2 container.
	log := `INFO      Oct 09 08:11:07.012 [1]: No upstream proxy for tinyproxy-exporter
INFO      Oct 09 08:11:07.013 [1]: opensock: opening connection to tinyproxy-exporter:9119
INFO      Oct 09 08:11:07.013 [1]: opensock: getaddrinfo returned for tinyproxy-exporter:9119
INFO      Oct 09 08:11:07.000 [1]: Now running as group "tinyproxy".
INFO      Oct 09 08:11:07.000 [1]: Setting the various signals.
INFO      Oct 09 08:11:07.000 [1]: Starting main loop. Accepting connections.
NOTICE    Oct 09 08:11:07.000 [1]: Initializing tinyproxy ...
`
	c := parseAll(t, log, ParseOptions{})
	if c.Unknown != 0 {
		t.Fatalf("benign messages must not be reported as unrecognized: %d", c.Unknown)
	}
	if c.Lines != 7 {
		t.Fatalf("lines = %d, want 7", c.Lines)
	}
}

func TestParseLogLineCountsClientConnections(t *testing.T) {
	// Both variants from src/reqs.c:1658-1659.
	log := `CONNECT   Oct 09 08:11:07.012 [1]: Connect (file descriptor 5): 127.0.0.1
CONNECT   Oct 09 08:11:07.017 [1]: Connect (file descriptor 6): 10.0.0.7 at [client.example]
`
	c := parseAll(t, log, ParseOptions{})
	if c.ClientConnections != 2 {
		t.Fatalf("client connections = %d, want 2", c.ClientConnections)
	}
	if c.Unknown != 0 {
		t.Fatalf("client connects are a known message: %d", c.Unknown)
	}
}

func TestParseLogLineKeepsUpstreamMissAsAnError(t *testing.T) {
	// "No upstream proxy for X" is informational, "No upstream proxy defined
	// for X." is a real warning about a configured upstream (src/reqs.c).
	log := `WARNING   Oct 09 15:45:00.000 [1]: No upstream proxy defined for example.com.
`
	c := parseAll(t, log, ParseOptions{})
	if c.Errors["upstream_missing"] != 1 {
		t.Fatalf("expected an upstream_missing error, got %v", c.Errors)
	}
	if c.Unknown != 0 {
		t.Fatalf("unrecognized = %d, want 0", c.Unknown)
	}
}

func TestParseLogLineCountsUpstreamErrors(t *testing.T) {
	log := `WARNING   Oct 09 15:45:00.000 [1]: Could not connect to upstream proxy.
WARNING   Oct 09 15:45:00.100 [1]: No upstream proxy defined for example.com.
INFO      Oct 09 15:45:00.200 [1]: handle_connection: Could not send CONNECT method greeting to client.
`
	c := parseAll(t, log, ParseOptions{})
	if c.Errors["upstream_connect"] != 1 || c.Errors["upstream_missing"] != 1 || c.Errors["send_connect_response"] != 1 {
		t.Fatalf("error counters = %v", c.Errors)
	}
}

func TestLogCountersCloneIsolatesMaps(t *testing.T) {
	c := newLogCounters()
	c.Requests["GET"] = 1
	clone := c.clone()
	clone.Requests["GET"] = 99
	if c.Requests["GET"] != 1 {
		t.Fatal("clone shares the underlying map")
	}
}

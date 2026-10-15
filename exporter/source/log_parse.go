package source

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Log line handling is derived from tinyproxy 1.11.3 sources.
//
// Envelope (src/log.c:176-182):
//
//	"%-9s %s.%03lu [%ld]: "  ->  "CONNECT   Oct 09 15:45:00.123 [1234]: <msg>"
//
// with a level string taken from src/log.c:34-44:
// CRITICAL, ERROR, WARNING, NOTICE, INFO, DEBUG, CONNECT.
//
// Messages are matched against the exact format strings used in src/reqs.c,
// src/conns.c, src/child.c and src/acl.c.
var (
	reLogEnvelope = regexp.MustCompile(`^([A-Z]+)\s+\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}\.\d{3}\s+\[\d+\]:\s?(.*)$`)

	reRequestLine = regexp.MustCompile(`^Request \(file descriptor \d+\):\s*(.*)$`)
	reEntityBytes = regexp.MustCompile(`^Read request entity of (\d+) bytes$`)
	reClosedConn  = regexp.MustCompile(`^Closed connection between local client \(fd:\d+\) and remote client \(fd:\d+\)$`)

	// src/reqs.c:1658-1659 has two variants:
	//   Connect (file descriptor %d): %s at [%s]
	//   Connect (file descriptor %d): %s
	reClientConnect = regexp.MustCompile(`^Connect \(file descriptor \d+\):`)
)

// benignPrefixes lists informational messages that are perfectly normal. They
// are recognised explicitly so that tinyproxy_log_unrecognized_lines_total stays
// a meaningful "tinyproxy changed something" signal instead of noise.
var benignPrefixes = []string{
	"opensock: ",             // src/sock.c
	"Added upstream ",        // src/upstream.c
	"Found upstream proxy ",  // src/upstream.c
	"No upstream proxy for ", // src/upstream.c (no upstream configured)
	"Not sending client headers to remote machine", // src/reqs.c
	"Now running as ",                            // src/main.c
	"Setting the various signals.",               // src/main.c
	"Starting main loop. Accepting connections.", // src/main.c
	"Initializing tinyproxy",                     // src/main.c
	"Reloading config file",                      // src/main.c
	"Shutting down.",                             // src/main.c
}

// overflowHostLabel absorbs request hosts beyond ParseOptions.MaxHosts so that
// a hostile or simply very diverse client population cannot explode the label
// cardinality of tinyproxy_log_requests_by_host_total.
const overflowHostLabel = "__other__"

// ParseOptions tunes log line interpretation.
type ParseOptions struct {
	// StatsHost is the configured StatHost. Requests for it are issued by the
	// exporter itself and are therefore not counted as proxied traffic.
	StatsHost string
	// MaxHosts caps the number of distinct host label values. Zero or negative
	// means unlimited.
	MaxHosts int
}

// LogCounters aggregates everything the exporter learns from the log file.
//
// All of these are cumulative for the lifetime of the exporter process: a log
// rotation or truncation does not reset them, which is exactly what a
// Prometheus counter needs.
type LogCounters struct {
	// Lines is the number of log lines read.
	Lines uint64
	// ParseErrors counts lines that did not match tinyproxy's log envelope.
	ParseErrors uint64
	// Unknown counts well formed lines whose message has no rule yet.
	Unknown uint64
	// ReadBytes counts bytes consumed from the log file.
	ReadBytes uint64

	// Requests counts requests by HTTP method (from "Request (file
	// descriptor N): <METHOD> <target> <PROTO>").
	Requests map[string]uint64
	// RequestsByHost counts requests by target host.
	RequestsByHost map[string]uint64
	// ClientConnections counts accepted client connections ("Connect (file
	// descriptor N): <ip>"), the log equivalent of tinyproxy's STAT_OPEN.
	ClientConnections uint64
	// Established counts "Established connection to ..." messages.
	Established uint64
	// Closed counts "Closed connection between local client ..." messages.
	Closed uint64
	// Denied counts refusals by reason: acl, filter, auth, connect_port,
	// max_clients.
	Denied map[string]uint64
	// BadRequests counts rejected requests.
	BadRequests uint64
	// Errors counts internal failures by kind.
	Errors map[string]uint64

	// StathostRequests counts statistics page requests (the exporter's own
	// scrapes, if it is the only client using the StatHost).
	StathostRequests uint64
	// RequestEntityBytes sums request bodies reported by
	// "Read request entity of N bytes".
	RequestEntityBytes uint64

	// LastLineAt is when a line was last processed.
	LastLineAtMillis int64
}

func newLogCounters() LogCounters {
	return LogCounters{
		Requests:       make(map[string]uint64),
		RequestsByHost: make(map[string]uint64),
		Denied:         make(map[string]uint64),
		Errors:         make(map[string]uint64),
	}
}

func (c LogCounters) clone() LogCounters {
	out := c
	out.Requests = cloneCounters(c.Requests)
	out.RequestsByHost = cloneCounters(c.RequestsByHost)
	out.Denied = cloneCounters(c.Denied)
	out.Errors = cloneCounters(c.Errors)
	return out
}

func cloneCounters(in map[string]uint64) map[string]uint64 {
	out := make(map[string]uint64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ParseLogLine classifies a single log line and folds it into c.
func ParseLogLine(line string, c *LogCounters, opts ParseOptions) {
	line = strings.TrimRight(line, "\r")
	if strings.TrimSpace(line) == "" {
		return
	}

	c.Lines++

	m := reLogEnvelope.FindStringSubmatch(line)
	if m == nil {
		c.ParseErrors++
		return
	}
	if !classifyLogMessage(strings.TrimSpace(m[2]), c, opts) {
		c.Unknown++
	}
}

func classifyLogMessage(msg string, c *LogCounters, opts ParseOptions) bool {
	if m := reRequestLine.FindStringSubmatch(msg); m != nil {
		return classifyRequestLine(m[1], c, opts)
	}
	if m := reEntityBytes.FindStringSubmatch(msg); m != nil {
		if n, err := strconv.ParseUint(m[1], 10, 64); err == nil {
			c.RequestEntityBytes += n
		}
		return true
	}
	if reClosedConn.MatchString(msg) {
		c.Closed++
		return true
	}
	if reClientConnect.MatchString(msg) {
		c.ClientConnections++
		return true
	}

	switch {
	case msg == "Request for the stathost.":
		// Informational: counted through the request line instead, otherwise
		// every scrape would be counted twice.
		return true
	case strings.HasPrefix(msg, "Established connection to "):
		c.Established++
		return true
	case strings.HasPrefix(msg, "Unauthorized connection from "):
		c.Denied["acl"]++
		return true
	case strings.HasPrefix(msg, "Proxying refused on filtered "):
		c.Denied["filter"]++
		return true
	case strings.HasPrefix(msg, "Failed auth attempt "):
		c.Denied["auth"]++
		return true
	case strings.HasPrefix(msg, "Refused CONNECT method on port "):
		c.Denied["connect_port"]++
		return true
	case strings.HasPrefix(msg, "Maximum number of connections reached."):
		c.Denied["max_clients"]++
		return true
	case strings.HasPrefix(msg, "process_request: Bad Request on file descriptor "),
		strings.HasPrefix(msg, "Unknown method ("):
		c.BadRequests++
		return true
	case strings.HasPrefix(msg, "read_request_line: Client "):
		c.Errors["client_closed"]++
		return true
	case strings.HasPrefix(msg, "handle_connection: Could not send CONNECT"):
		c.Errors["send_connect_response"]++
		return true
	case strings.HasPrefix(msg, "Could not connect to upstream proxy"):
		c.Errors["upstream_connect"]++
		return true
	case strings.HasPrefix(msg, "No upstream proxy defined for "):
		c.Errors["upstream_missing"]++
		return true
	case strings.HasPrefix(msg, "Could not retrieve all the headers from the client"):
		c.Errors["client_headers"]++
		return true
	case strings.HasPrefix(msg, "Could not retrieve all the headers from the remote server"):
		c.Errors["server_headers"]++
		return true
	}

	for _, prefix := range benignPrefixes {
		if strings.HasPrefix(msg, prefix) {
			return true
		}
	}
	return false
}

// classifyRequestLine handles "Request (file descriptor N): <request line>".
func classifyRequestLine(target string, c *LogCounters, opts ParseOptions) bool {
	method, host := splitRequestTarget(target)
	if method == "" {
		return false
	}

	if opts.StatsHost != "" && strings.EqualFold(host, opts.StatsHost) {
		c.StathostRequests++
		return true
	}

	c.Requests[method]++
	if host != "" {
		c.RequestsByHost[boundedHost(strings.ToLower(host), c.RequestsByHost, opts.MaxHosts)]++
	}
	return true
}

// splitRequestTarget extracts the method and, when present, the target host
// from a request line such as
//
//	GET http://example.com/index.html HTTP/1.1
//	CONNECT example.com:443 HTTP/1.1
//	GET /index.html HTTP/1.1          (transparent / reverse proxying)
func splitRequestTarget(target string) (method, host string) {
	fields := strings.Fields(target)
	if len(fields) == 0 {
		return "", ""
	}
	method = fields[0]
	if len(fields) < 2 {
		return method, ""
	}

	raw := fields[1]
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return method, u.Hostname()
	}
	if h, _, err := net.SplitHostPort(raw); err == nil {
		return method, h
	}
	return method, ""
}

// boundedHost keeps the host label set bounded.
func boundedHost(host string, seen map[string]uint64, max int) string {
	if _, ok := seen[host]; ok {
		return host
	}
	if max <= 0 || len(seen) < max {
		return host
	}
	return overflowHostLabel
}

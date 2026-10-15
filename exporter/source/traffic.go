package source

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// procNetDev is overridable in tests.
var procNetDev = "/proc/net/dev"

// TrafficCounters holds interface level byte counters.
//
// Tinyproxy itself tracks no bytes at all (src/stats.c has no byte field and
// nothing in src/ counts payload), so the only way to obtain traffic volumes is
// to read the network interface counters. When the exporter shares tinyproxy's
// network namespace (docker-compose `network_mode: "service:tinyproxy"`) these
// numbers describe the proxy's traffic; otherwise they describe the exporter's
// own namespace and are merely context.
type TrafficCounters struct {
	Interface string
	Received  float64
	Sent      float64
	At        time.Time
	Err       error
}

// Seen reports whether a successful read has happened.
func (c TrafficCounters) Seen() bool { return !c.At.IsZero() }

// TrafficReader polls /proc/net/dev for a single interface.
type TrafficReader struct {
	path     string
	iface    string
	interval time.Duration
	logger   *slog.Logger

	health Health

	mu     sync.Mutex
	latest TrafficCounters
	rx     resetTracker
	tx     resetTracker
}

// NewTrafficReader builds a reader for the given interface.
func NewTrafficReader(iface string, interval time.Duration, logger *slog.Logger) (*TrafficReader, error) {
	if strings.TrimSpace(iface) == "" {
		return nil, errors.New("traffic interface must not be empty")
	}
	if interval <= 0 {
		return nil, errors.New("traffic interval must be greater than zero")
	}
	return &TrafficReader{
		path:     procNetDev,
		iface:    iface,
		interval: interval,
		logger:   logger,
	}, nil
}

// Interface is the polled network interface.
func (r *TrafficReader) Interface() string { return r.iface }

// Run polls the interface until ctx is done.
func (r *TrafficReader) Run(ctx context.Context) {
	r.collect()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.collect()
		}
	}
}

func (r *TrafficReader) collect() {
	start := time.Now()
	data, err := os.ReadFile(r.path)
	if err == nil {
		var rx, tx uint64
		rx, tx, err = ParseProcNetDev(data, r.iface)
		if err == nil {
			r.mu.Lock()
			received := r.rx.accumulate(float64(rx))
			sent := r.tx.accumulate(float64(tx))
			r.latest = TrafficCounters{
				Interface: r.iface,
				Received:  received,
				Sent:      sent,
				At:        time.Now(),
			}
			r.mu.Unlock()
			r.health.MarkSuccess(time.Since(start))
			return
		}
	}

	r.health.MarkFailure(err, time.Since(start))
	r.logger.Warn("cannot read interface counters", "interface", r.iface, "path", r.path, "err", err)
}

// Latest returns the most recent counters.
func (r *TrafficReader) Latest() TrafficCounters {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latest
}

// Health reports the reachability of /proc/net/dev.
func (r *TrafficReader) Health() HealthSnapshot { return r.health.Snapshot() }

// ParseProcNetDev extracts the received/sent byte counters of one interface.
//
// /proc/net/dev line layout:
//
//	iface: rx_bytes rx_packets rx_errs rx_drop rx_fifo rx_frame rx_compressed
//	       rx_multicast tx_bytes tx_packets ...
func ParseProcNetDev(data []byte, iface string) (received, sent uint64, err error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	for scanner.Scan() {
		line := scanner.Text()
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		if strings.TrimSpace(line[:idx]) != iface {
			continue
		}

		fields := strings.Fields(line[idx+1:])
		if len(fields) < 9 {
			return 0, 0, fmt.Errorf("interface %s: expected at least 9 counters, got %d", iface, len(fields))
		}
		rx, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("interface %s: parse rx bytes: %w", iface, err)
		}
		tx, err := strconv.ParseUint(fields[8], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("interface %s: parse tx bytes: %w", iface, err)
		}
		return rx, tx, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("scan %s: %w", procNetDev, err)
	}
	return 0, 0, fmt.Errorf("interface %s not found in %s", iface, procNetDev)
}

package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// maxStatsBody caps how much of the statistics page we are willing to read.
const maxStatsBody = 1 << 20 // 1 MiB

// StatsScrape is one snapshot of the statistics page.
type StatsScrape struct {
	// Result holds the values as reported by tinyproxy.
	Result *StatsResult
	// Counters holds the monotonic, restart compensated values for the fields
	// listed in monotonicFields. It is a subset of Result.Fields.
	Counters map[string]float64
	// Duration is how long the HTTP round trip took.
	Duration time.Duration
	// At is when the scrape finished. Zero when it never succeeded.
	At time.Time
	// Err is the failure of the most recent attempt, if any. A successful
	// attempt clears it.
	Err error
}

// Seen reports whether a successful scrape has ever happened.
func (s StatsScrape) Seen() bool { return !s.At.IsZero() }

// StatsScraper polls the StatHost statistics page through the proxy.
//
// Tinyproxy answers requests whose Host header equals `StatHost` itself
// (src/reqs.c:409), so the request has to travel through the proxy: the host
// part of the URL is the trigger, and every scrape therefore appears in
// tinyproxy's own counters.
type StatsScraper struct {
	url      string
	interval time.Duration
	logger   *slog.Logger
	client   *http.Client

	health Health

	mu       sync.Mutex
	latest   StatsScrape
	trackers map[string]*resetTracker
}

// NewStatsScraper builds a scraper for statsURL (for example
// "http://tinyproxy.stats/") reached through proxyURL.
func NewStatsScraper(statsURL, proxyURL string, interval, timeout time.Duration, logger *slog.Logger) (*StatsScraper, error) {
	if statsURL == "" {
		return nil, errors.New("stats url must not be empty")
	}
	pURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("parse proxy url %q: %w", proxyURL, err)
	}
	if pURL.Host == "" {
		return nil, fmt.Errorf("proxy url %q has no host", proxyURL)
	}
	if interval <= 0 {
		return nil, errors.New("stats interval must be greater than zero")
	}
	if timeout <= 0 {
		return nil, errors.New("stats timeout must be greater than zero")
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyURL(pURL),
		DisableCompression:    true, // tinyproxy's page is tiny; avoid surprises
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: timeout,
		TLSHandshakeTimeout:   timeout,
		ExpectContinueTimeout: time.Second,
	}

	return &StatsScraper{
		url:      statsURL,
		interval: interval,
		logger:   logger,
		client:   &http.Client{Transport: transport, Timeout: timeout},
		trackers: make(map[string]*resetTracker),
	}, nil
}

// URL is the address that is fetched through the proxy.
func (s *StatsScraper) URL() string { return s.url }

// Interval is the polling period.
func (s *StatsScraper) Interval() time.Duration { return s.interval }

// Run scrapes immediately and then once per interval until ctx is done.
func (s *StatsScraper) Run(ctx context.Context) {
	s.Scrape(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Scrape(ctx)
		}
	}
}

// Scrape performs a single fetch.
func (s *StatsScraper) Scrape(ctx context.Context) {
	reqCtx, cancel := context.WithTimeout(ctx, s.client.Timeout)
	defer cancel()

	start := time.Now()
	body, err := s.fetch(reqCtx)
	elapsed := time.Since(start)
	if err != nil {
		s.health.MarkFailure(err, elapsed)
		s.mu.Lock()
		s.latest.Duration = elapsed
		s.latest.Err = err
		s.mu.Unlock()
		if !errors.Is(err, context.Canceled) {
			s.logger.Warn("statistics page scrape failed", "url", s.url, "err", err)
		}
		return
	}

	res := ParseStatsPage(body)

	s.mu.Lock()
	counters := make(map[string]float64, len(res.Fields))
	for field, raw := range res.Fields {
		if !monotonicFields[field] {
			continue
		}
		tracker, ok := s.trackers[field]
		if !ok {
			tracker = &resetTracker{}
			s.trackers[field] = tracker
		}
		counters[field] = tracker.accumulate(raw)
	}
	s.latest = StatsScrape{Result: res, Counters: counters, Duration: elapsed, At: time.Now()}
	s.mu.Unlock()

	s.health.MarkSuccess(elapsed)
	s.logger.Debug("statistics page scraped",
		"url", s.url, "duration", elapsed, "fields", len(res.Fields), "raw_fields", len(res.Raw))
}

func (s *StatsScraper) fetch(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/plain, text/html;q=0.9, */*;q=0.1")
	req.Header.Set("User-Agent", "tinyproxy-exporter")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxStatsBody))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s from %s", resp.Status, s.url)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxStatsBody))
	if err != nil {
		return nil, fmt.Errorf("read statistics page: %w", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("statistics page is empty")
	}
	return body, nil
}

// Latest returns the most recent scrape. The returned maps are never mutated
// after publication, so readers may use them freely.
func (s *StatsScraper) Latest() StatsScrape {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest
}

// Health reports the reachability of the statistics page.
func (s *StatsScraper) Health() HealthSnapshot { return s.health.Snapshot() }

// resetTracker turns a counter that resets when tinyproxy restarts into a
// monotonically increasing one.
type resetTracker struct {
	last   float64
	offset float64
	seen   bool
}

func (t *resetTracker) accumulate(v float64) float64 {
	if t.seen && v < t.last {
		// tinyproxy (re)started: keep the total monotonic.
		t.offset += t.last
	}
	t.last = v
	t.seen = true
	return v + t.offset
}

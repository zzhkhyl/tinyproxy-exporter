package source

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func appendToFile(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("append to %s: %v", path, err)
	}
}

const getRequestLine = "CONNECT   Oct 09 15:45:00.123 [1234]: Request (file descriptor 6): GET http://example.com/index.html HTTP/1.1\n"

func TestLogTailerSkipsPreExistingContentAndFollowsAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinyproxy.log")
	if err := os.WriteFile(path, []byte("INFO      Oct 09 15:44:00.000 [1]: Initializing tinyproxy ...\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tailer, err := NewLogTailer(path, 5*time.Millisecond, false,
		ParseOptions{StatsHost: "tinyproxy.stats", MaxHosts: 8}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tailer.Run(ctx)

	time.Sleep(60 * time.Millisecond)
	if got := tailer.Counters().Lines; got != 0 {
		t.Fatalf("pre-existing content must be skipped, counted %d lines", got)
	}
	if !tailer.Health().Up {
		t.Fatalf("expected a readable log file to be healthy: %+v", tailer.Health())
	}

	appendToFile(t, path, getRequestLine)

	waitFor(t, 3*time.Second, "the appended request to be counted", func() bool {
		return tailer.Counters().Requests["GET"] == 1
	})
}

func TestLogTailerWaitsForMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-yet.log")

	tailer, err := NewLogTailer(path, 5*time.Millisecond, true, ParseOptions{}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tailer.Run(ctx)

	time.Sleep(60 * time.Millisecond)
	if tailer.Health().Up {
		t.Fatal("a missing log file must not report a healthy source")
	}

	appendToFile(t, path, getRequestLine)

	waitFor(t, 3*time.Second, "the log file to appear", func() bool {
		return tailer.Counters().Requests["GET"] == 1 && tailer.Health().Up
	})
}

func TestLogTailerFollowsRotationAndTruncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinyproxy.log")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	tailer, err := NewLogTailer(path, 5*time.Millisecond, true, ParseOptions{MaxHosts: 8}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tailer.Run(ctx)

	appendToFile(t, path, getRequestLine)
	waitFor(t, 3*time.Second, "the first request", func() bool {
		return tailer.Counters().Requests["GET"] == 1
	})

	// logrotate with `create`: the inode changes.
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	appendToFile(t, path, "CONNECT   Oct 09 15:46:00.000 [1234]: Request (file descriptor 6): POST http://rotated.example/ HTTP/1.1\n")
	waitFor(t, 3*time.Second, "the post-rotation request", func() bool {
		return tailer.Counters().Requests["POST"] == 1
	})

	// logrotate with `copytruncate`: same inode, size shrinks.
	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	appendToFile(t, path, "CONNECT   Oct 09 15:47:00.000 [1234]: Request (file descriptor 6): HEAD http://truncated.example/ HTTP/1.1\n")
	waitFor(t, 3*time.Second, "the post-truncation request", func() bool {
		return tailer.Counters().Requests["HEAD"] == 1
	})

	// Counters are cumulative for the exporter's lifetime, they never reset.
	c := tailer.Counters()
	if c.Requests["GET"] != 1 || c.Requests["POST"] != 1 || c.Requests["HEAD"] != 1 {
		t.Fatalf("counters must survive rotation and truncation: %v", c.Requests)
	}
}

func TestLogTailerHandlesPartialTrailingLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinyproxy.log")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	tailer, err := NewLogTailer(path, 5*time.Millisecond, true, ParseOptions{MaxHosts: 8}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tailer.Run(ctx)

	// Write everything but the trailing newline: nothing may be counted yet.
	appendToFile(t, path, "CONNECT   Oct 09 15:45:00.123 [1234]: Request (file descriptor 6): GET http://half.example/ HTTP/1.1")
	time.Sleep(80 * time.Millisecond)
	if got := tailer.Counters(); got.Lines != 0 || got.Requests["GET"] != 0 {
		t.Fatalf("a partial line must not be consumed: %+v", got)
	}

	appendToFile(t, path, "\n")
	waitFor(t, 3*time.Second, "the completed line", func() bool {
		return tailer.Counters().Requests["GET"] == 1
	})
}

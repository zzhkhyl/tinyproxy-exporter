package source

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// maxLogLineBytes guards against a never terminated line.
	maxLogLineBytes = 64 * 1024
	// logReadBuffer is the size of the bufio reader used for the log file.
	logReadBuffer = 64 * 1024
)

// LogTailer incrementally follows tinyproxy's log file.
//
// tinyproxy appends to its log file and never rotates it itself (see
// create_file_safely() in src/utils.c, opened with O_APPEND), so the tailer has
// to cope with external rotation (inode change) and truncation (logrotate
// copytruncate) on its own.
type LogTailer struct {
	path         string
	pollInterval time.Duration
	fromStart    bool
	opts         ParseOptions
	logger       *slog.Logger

	health Health

	mu       sync.Mutex
	counters LogCounters

	// The fields below belong to the Run goroutine exclusively.
	file     *os.File
	reader   *bufio.Reader
	buf      string
	offset   int64
	started  bool
	lastWarn string
}

// NewLogTailer builds a tailer for path.
func NewLogTailer(path string, pollInterval time.Duration, fromStart bool, opts ParseOptions, logger *slog.Logger) (*LogTailer, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("log path must not be empty")
	}
	if pollInterval <= 0 {
		return nil, errors.New("log poll interval must be greater than zero")
	}
	return &LogTailer{
		path:         path,
		pollInterval: pollInterval,
		fromStart:    fromStart,
		opts:         opts,
		logger:       logger,
		counters:     newLogCounters(),
	}, nil
}

// Path is the followed log file.
func (t *LogTailer) Path() string { return t.path }

// Run follows the log file until ctx is done. It never gives up on a missing
// file: the exporter usually starts before (or at the same time as) tinyproxy.
func (t *LogTailer) Run(ctx context.Context) {
	defer t.closeFile()

	for ctx.Err() == nil {
		if err := t.ensureOpen(); err != nil {
			t.fail(err, 0)
			t.sleep(ctx)
			continue
		}

		start := time.Now()
		err := t.readAvailable()
		t.fail(err, time.Since(start))
		if err != nil {
			t.closeFile()
		}
		t.sleep(ctx)
	}
}

func (t *LogTailer) sleep(ctx context.Context) {
	timer := time.NewTimer(t.pollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// fail records an error, if any, rate limiting the warning message.
func (t *LogTailer) fail(err error, d time.Duration) {
	if err == nil {
		t.health.MarkSuccess(d)
		t.lastWarn = ""
		return
	}
	t.health.MarkFailure(err, d)

	msg := err.Error()
	if msg != t.lastWarn {
		t.lastWarn = msg
		t.logger.Warn("cannot read tinyproxy log", "path", t.path, "err", err)
	}
}

func (t *LogTailer) ensureOpen() error {
	if t.file != nil {
		return nil
	}

	f, err := os.Open(t.path)
	if err != nil {
		return err
	}

	seekTo := t.offset
	if !t.started {
		t.started = true
		if t.fromStart {
			seekTo = 0
		} else {
			end, err := f.Seek(0, io.SeekEnd)
			if err != nil {
				_ = f.Close()
				return err
			}
			seekTo = end
		}
	}
	if seekTo != 0 {
		if _, err := f.Seek(seekTo, io.SeekStart); err != nil {
			_ = f.Close()
			return err
		}
	}

	t.file = f
	t.reader = bufio.NewReaderSize(f, logReadBuffer)
	t.offset = seekTo
	t.buf = ""
	return nil
}

func (t *LogTailer) closeFile() {
	if t.file != nil {
		_ = t.file.Close()
	}
	t.file = nil
	t.reader = nil
}

// readAvailable consumes everything that is currently in the file.
func (t *LogTailer) readAvailable() error {
	for {
		chunk, err := t.reader.ReadString('\n')
		if chunk != "" {
			t.offset += int64(len(chunk))
			t.buf += chunk
			t.consumeLines()
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
	}
	return t.checkFileState()
}

// consumeLines emits every complete line currently held in the carry over
// buffer. A partial trailing line stays buffered until its newline arrives.
func (t *LogTailer) consumeLines() {
	for {
		i := strings.IndexByte(t.buf, '\n')
		if i < 0 {
			break
		}
		line := t.buf[:i]
		t.buf = t.buf[i+1:]

		t.mu.Lock()
		t.counters.ReadBytes += uint64(len(line)) + 1
		ParseLogLine(line, &t.counters, t.opts)
		t.counters.LastLineAtMillis = time.Now().UnixMilli()
		t.mu.Unlock()
	}

	if len(t.buf) > maxLogLineBytes {
		t.mu.Lock()
		t.counters.ParseErrors++
		t.mu.Unlock()
		t.logger.Warn("log file contains an unterminated line, dropping it",
			"path", t.path, "bytes", len(t.buf))
		t.buf = ""
	}
}

// checkFileState detects rotation and truncation.
func (t *LogTailer) checkFileState() error {
	current, err := t.file.Stat()
	if err != nil {
		return err
	}

	if current.Size() < t.offset {
		// Truncated in place: the old content is gone, so reading from the
		// beginning cannot double count.
		if _, err := t.file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		t.offset = 0
		t.buf = ""
		t.reader.Reset(t.file)
		t.logger.Info("log file was truncated, restarting from the beginning", "path", t.path)
		return nil
	}

	onDisk, err := os.Stat(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			t.logger.Info("log file disappeared, waiting for it to return", "path", t.path)
			t.closeFile()
			t.offset = 0
			return nil
		}
		return err
	}
	if !os.SameFile(current, onDisk) {
		t.logger.Info("log file was rotated, following the new file", "path", t.path)
		t.closeFile()
		t.offset = 0
		return nil
	}
	return nil
}

// Counters returns a deep copy of the accumulated counters.
func (t *LogTailer) Counters() LogCounters {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.counters.clone()
}

// Health reports whether the log file is currently readable.
func (t *LogTailer) Health() HealthSnapshot { return t.health.Snapshot() }

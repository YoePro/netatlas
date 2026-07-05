package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"netatlas/internal/config"
	"netatlas/internal/ingest"
)

var parseFailures = newParseFailureSampler()

func cooperateRuntime(cfg *config.Config) {
	switch cfg.RuntimeMode {
	case "low":
		time.Sleep(250 * time.Microsecond)
	case "medium":
		if atomic.AddUint64(&runtimeCooperateCounter, 1)%1000 == 0 {
			time.Sleep(time.Millisecond)
		}
	}
}

var runtimeCooperateCounter uint64

func startProgressReporter(ctx context.Context, cfg *config.Config, runStats *stats) func() {
	if cfg.LogMode == "quiet" || cfg.ProgressInterval <= 0 {
		return func() {}
	}

	done := make(chan struct{})
	finished := make(chan struct{})
	start := time.Now()
	go func() {
		defer close(finished)
		ticker := time.NewTicker(cfg.ProgressInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				printProgress(os.Stdout, runStats, start)
			}
		}
	}()

	return func() {
		close(done)
		<-finished
	}
}

func printProgress(w io.Writer, runStats *stats, start time.Time) {
	lines := atomic.LoadUint64(&runStats.lines)
	parsed := atomic.LoadUint64(&runStats.parsed)
	offset := atomic.LoadUint64(&runStats.currentOffset)
	size := atomic.LoadUint64(&runStats.inputSize)
	if size > 0 {
		eta := progressETA(start, offset, size)
		if eta > 0 {
			fmt.Fprintf(w, "Progress: lines=%d parsed=%d offset=%d/%d %.1f%% eta=%s\n", lines, parsed, offset, size, float64(offset)/float64(size)*100, eta.Round(time.Second))
			return
		}
		fmt.Fprintf(w, "Progress: lines=%d parsed=%d offset=%d/%d %.1f%%\n", lines, parsed, offset, size, float64(offset)/float64(size)*100)
		return
	}
	fmt.Fprintf(w, "Progress: lines=%d parsed=%d offset=%d\n", lines, parsed, offset)
}

func progressETA(start time.Time, offset, size uint64) time.Duration {
	if offset == 0 || offset >= size {
		return 0
	}
	elapsed := time.Since(start)
	if elapsed < 2*time.Second {
		return 0
	}
	bytesPerSecond := float64(offset) / elapsed.Seconds()
	if bytesPerSecond <= 0 {
		return 0
	}
	return time.Duration(float64(time.Second) * float64(size-offset) / bytesPerSecond)
}

type parseFailureSampler struct {
	mu      sync.Mutex
	seen    map[string]struct{}
	samples []string
}

func newParseFailureSampler() *parseFailureSampler {
	return &parseFailureSampler{seen: make(map[string]struct{})}
}

func recordParseFailure(cfg *config.Config, err error, line string) {
	if cfg.LogMode == "verbose" {
		log.Printf("parse failed: %v: %q", err, line)
	}
	if cfg.ParseFailureSamples <= 0 {
		return
	}
	parseFailures.add(cfg.ParseFailureSamples, fmt.Sprintf("%v: %s", err, line))
}

func (s *parseFailureSampler) add(limit int, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.samples) >= limit {
		return
	}
	if _, ok := s.seen[value]; ok {
		return
	}
	s.seen[value] = struct{}{}
	s.samples = append(s.samples, value)
}

func (s *parseFailureSampler) write(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.samples) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create parse failure directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open parse failure sample file: %w", err)
	}
	defer file.Close()
	for _, sample := range s.samples {
		if _, err := fmt.Fprintln(file, sample); err != nil {
			return fmt.Errorf("write parse failure sample: %w", err)
		}
	}
	s.samples = nil
	s.seen = make(map[string]struct{})
	return nil
}

func flushParseFailures(cfg *config.Config) error {
	if cfg.ParseFailureSamples <= 0 || cfg.ParseFailurePath == "" {
		return nil
	}
	return parseFailures.write(cfg.ParseFailurePath)
}

func acquireRunLock(cfg *config.Config) (*ingest.LockFile, error) {
	if cfg.DryRun && !cfg.DryRunUpdatesOffset {
		return nil, nil
	}
	return ingest.AcquireLock(cfg.OffsetStatePath + ".lock")
}

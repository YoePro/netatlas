package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"dnslog/internal/config"
	"dnslog/internal/ingest"
	"dnslog/internal/model"
	"dnslog/internal/parser"
	"dnslog/internal/store"
)

type parseResult struct {
	seq      uint64
	state    ingest.OffsetState
	event    model.DNSEvent
	hasEvent bool
}

type stats struct {
	lines          uint64
	parsed         uint64
	ignored        uint64
	parseFailures  uint64
	written        uint64
	writeFailures  uint64
	batchesWritten uint64
}

func main() {
	configPath := flag.String("config", "config.ini", "path to INI config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	eventStore, err := store.NewNeo4jStore(ctx, cfg)
	if err != nil {
		log.Fatalf("create store: %v", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := eventStore.Close(closeCtx); err != nil {
			log.Printf("close store: %v", err)
		}
	}()

	start := time.Now()
	runStats := &stats{}

	if err := run(ctx, cfg, eventStore, runStats); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("ingestion failed: %v", err)
	}

	fmt.Printf(
		"Done in %s. lines=%d parsed=%d ignored=%d parse_failures=%d written=%d batches=%d write_failures=%d dry_run=%t\n",
		time.Since(start).Round(time.Millisecond),
		atomic.LoadUint64(&runStats.lines),
		atomic.LoadUint64(&runStats.parsed),
		atomic.LoadUint64(&runStats.ignored),
		atomic.LoadUint64(&runStats.parseFailures),
		atomic.LoadUint64(&runStats.written),
		atomic.LoadUint64(&runStats.batchesWritten),
		atomic.LoadUint64(&runStats.writeFailures),
		cfg.DryRun,
	)
}

func run(ctx context.Context, cfg *config.Config, eventStore store.EventStore, runStats *stats) error {
	rawLines := make(chan ingest.RawLine, cfg.BatchSize*2)
	parseResults := make(chan parseResult, cfg.BatchSize*2)
	orderedResults := make(chan parseResult, cfg.BatchSize*2)
	writerErr := make(chan error, 1)
	offsetStore := ingest.NewOffsetStore(cfg.OffsetStatePath)

	var parserWg sync.WaitGroup
	for i := 0; i < cfg.WorkerCount; i++ {
		parserWg.Add(1)
		go parseWorker(ctx, rawLines, parseResults, parser.ServerMeta{
			Name: cfg.ServerName,
			Role: cfg.ServerRole,
		}, runStats, &parserWg)
	}

	var reorderWg sync.WaitGroup
	reorderWg.Add(1)
	go reorderResults(ctx, parseResults, orderedResults, &reorderWg)

	var writerWg sync.WaitGroup
	writerWg.Add(1)
	go batchWriter(ctx, orderedResults, eventStore, offsetStore, cfg, runStats, writerErr, &writerWg)

	readErr := ingest.ReadFile(ctx, cfg.LogFilePath, offsetStore, rawLines)
	close(rawLines)
	parserWg.Wait()
	close(parseResults)
	reorderWg.Wait()
	writerWg.Wait()

	select {
	case err := <-writerErr:
		if err != nil {
			return err
		}
	default:
	}

	return readErr
}

func parseWorker(
	ctx context.Context,
	rawLines <-chan ingest.RawLine,
	results chan<- parseResult,
	server parser.ServerMeta,
	runStats *stats,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-rawLines:
			if !ok {
				return
			}
			atomic.AddUint64(&runStats.lines, 1)
			result := parseResult{
				seq:   line.Seq,
				state: line.NextState,
			}
			event, err := parser.ParseLine(line.Text, server)
			if err != nil {
				if errors.Is(err, parser.ErrIgnored) {
					atomic.AddUint64(&runStats.ignored, 1)
				} else {
					atomic.AddUint64(&runStats.parseFailures, 1)
					log.Printf("parse failed: %v: %q", err, line.Text)
				}
			} else {
				result.event = event
				result.hasEvent = true
				atomic.AddUint64(&runStats.parsed, 1)
			}

			select {
			case <-ctx.Done():
				return
			case results <- result:
			}
		}
	}
}

func reorderResults(ctx context.Context, in <-chan parseResult, out chan<- parseResult, wg *sync.WaitGroup) {
	defer wg.Done()
	defer close(out)

	next := uint64(1)
	buffered := make(map[uint64]parseResult)

	emitReady := func() bool {
		for {
			result, ok := buffered[next]
			if !ok {
				return true
			}
			delete(buffered, next)
			select {
			case <-ctx.Done():
				return false
			case out <- result:
				next++
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-in:
			if !ok {
				if emitReady() && len(buffered) > 0 {
					log.Printf("dropping %d out-of-order parser results after input closed", len(buffered))
				}
				return
			}
			buffered[result.seq] = result
			if !emitReady() {
				return
			}
		}
	}
}

func batchWriter(
	ctx context.Context,
	results <-chan parseResult,
	eventStore store.EventStore,
	offsetStore *ingest.OffsetStore,
	cfg *config.Config,
	runStats *stats,
	errs chan<- error,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	ticker := time.NewTicker(cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]model.DNSEvent, 0, cfg.BatchSize)
	var pendingState ingest.OffsetState
	hasPendingState := false

	flush := func() bool {
		if len(batch) == 0 {
			if hasPendingState {
				if err := offsetStore.Save(pendingState); err != nil {
					errs <- err
					return false
				}
				hasPendingState = false
			}
			return true
		}
		if err := writeBatchWithRetry(ctx, eventStore, batch, cfg.MaxWriteRetries, cfg.RetryDelay); err != nil {
			atomic.AddUint64(&runStats.writeFailures, 1)
			errs <- err
			return false
		}
		if hasPendingState {
			if err := offsetStore.Save(pendingState); err != nil {
				errs <- err
				return false
			}
			hasPendingState = false
		}
		atomic.AddUint64(&runStats.written, uint64(len(batch)))
		atomic.AddUint64(&runStats.batchesWritten, 1)
		batch = batch[:0]
		return true
	}

	for {
		select {
		case <-ctx.Done():
			_ = flush()
			return
		case <-ticker.C:
			if !flush() {
				return
			}
		case result, ok := <-results:
			if !ok {
				_ = flush()
				return
			}
			pendingState = result.state
			hasPendingState = true
			if result.hasEvent {
				batch = append(batch, result.event)
			}
			if len(batch) >= cfg.BatchSize && !flush() {
				return
			}
		}
	}
}

func writeBatchWithRetry(
	ctx context.Context,
	eventStore store.EventStore,
	batch []model.DNSEvent,
	maxRetries int,
	retryDelay time.Duration,
) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := eventStore.WriteBatch(ctx, batch); err != nil {
			lastErr = err
			if attempt == maxRetries {
				break
			}
			log.Printf("write batch failed, retrying in %s: %v", retryDelay, err)
			if retryDelay <= 0 {
				continue
			}
			timer := time.NewTimer(retryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("write batch failed after %d retries: %w", maxRetries, lastErr)
}

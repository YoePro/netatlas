package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"dnslog/internal/analytics"
	"dnslog/internal/config"
	"dnslog/internal/ingest"
	"dnslog/internal/model"
	"dnslog/internal/ops"
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
	lines                      uint64
	parsed                     uint64
	ignored                    uint64
	notable                    uint64
	notableRCode               uint64
	notableSecurityDeniedCache uint64
	parseFailures              uint64
	skippedGenesis             uint64
	written                    uint64
	writeFailures              uint64
	batchesWritten             uint64
	currentOffset              uint64
	inputSize                  uint64
	ignoredBind                uint64
	ignoredConfig              uint64
	ignoredFiltered            uint64
	ignoredNetwork             uint64
	ignoredNotify              uint64
	ignoredRateLimit           uint64
	ignoredResolver            uint64
	ignoredSocket              uint64
	ignoredTimeout             uint64
	ignoredXferIn              uint64
	ignoredZoneload            uint64
}

func main() {
	if wantsHelp(os.Args[1:]) {
		printHelp(os.Stdout)
		return
	}
	if wantsQueries(os.Args[1:]) {
		if err := printOperationalQueries(os.Stdout, os.Args[1:]); err != nil {
			log.Fatalf("%v", err)
		}
		return
	}
	if wantsAnalytics(os.Args[1:]) {
		if err := printAnalyticsReports(os.Stdout, os.Args[1:]); err != nil {
			log.Fatalf("%v", err)
		}
		return
	}
	if wantsSystemCheck(os.Args[1:]) {
		printSystemCheck(os.Stdout)
		return
	}

	command, flagArgs := extractCommand(os.Args[1:])

	flag.Usage = func() {
		printHelp(flag.CommandLine.Output())
	}
	configPath := flag.String("config", "config.ini", "path to INI config file")
	inputPath := flag.String("input", "", "override configured log input file")
	offsetStatePath := flag.String("offset-state", "", "override offset state file")
	debugEnabled := flag.Bool("debug", false, "print debug logging")
	telemetryEnabled := flag.Bool("telemetry", false, "print runtime telemetry after ingestion")
	quietEnabled := flag.Bool("quiet", false, "suppress normal output")
	verboseEnabled := flag.Bool("verbose", false, "print verbose output")
	benchmarkWorkers := flag.String("benchmark-workers", "", "benchmark worker_count values, e.g. 1,2,4 or 1-4")
	if err := flag.CommandLine.Parse(flagArgs); err != nil {
		log.Fatalf("parse flags: %v", err)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	cfg.Debug = *debugEnabled
	if *quietEnabled && *verboseEnabled {
		log.Fatalf("-quiet and -verbose cannot be used together")
	}
	if *quietEnabled {
		cfg.LogMode = "quiet"
	}
	if *verboseEnabled {
		cfg.LogMode = "verbose"
		cfg.Debug = true
		*telemetryEnabled = true
	}
	inputOverridden := *inputPath != ""
	offsetStateOverridden := flagWasSet("offset-state")
	if inputOverridden {
		cfg.LogFilePath = *inputPath
	}
	if offsetStateOverridden {
		cfg.OffsetStatePath = *offsetStatePath
	} else if inputOverridden {
		preparedStatePath, err := prepareInputOffsetState(cfg.LogFilePath, cfg.OffsetStatePath, cfg.DryRun)
		if err != nil {
			log.Fatalf("prepare input offset state: %v", err)
		}
		cfg.OffsetStatePath = preparedStatePath
	}
	if cfg.OffsetStatePath == "" {
		log.Fatalf("offset state path must not be empty")
	}
	if cfg.LogMode != "quiet" {
		fmt.Printf("Input: %s\n", cfg.LogFilePath)
	}

	switch command {
	case "preflight":
		if err := runPreflight(context.Background(), os.Stdout, cfg); err != nil {
			log.Fatalf("preflight failed: %v", err)
		}
		return
	case "benchmark":
		if err := runBenchmark(context.Background(), os.Stdout, cfg, *benchmarkWorkers); err != nil {
			log.Fatalf("benchmark failed: %v", err)
		}
		return
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

	runStats := &stats{}
	var perf *perfMonitor
	if *telemetryEnabled {
		perf = startPerfMonitor()
	}

	runErr := run(ctx, cfg, eventStore, runStats)
	if err := flushParseFailures(cfg); err != nil && runErr == nil {
		runErr = err
	}
	var metrics performanceMetrics
	if perf != nil {
		metrics = perf.Stop(runStats, cfg.BatchSize)
	}
	if shouldPrintSummary(cfg, runStats, runErr) {
		printRunSummary(os.Stdout, runStats, cfg.DryRun, metrics, *telemetryEnabled)
	}

	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		log.Fatalf("ingestion failed: %v", runErr)
	}
}

func wantsHelp(args []string) bool {
	if len(args) != 1 {
		return false
	}
	return args[0] == "help" || args[0] == "-h" || args[0] == "--help"
}

func wantsQueries(args []string) bool {
	return len(args) > 0 && args[0] == "queries"
}

func wantsAnalytics(args []string) bool {
	return len(args) > 0 && args[0] == "analytics"
}

func wantsSystemCheck(args []string) bool {
	return len(args) > 0 && args[0] == "system-check"
}

func extractCommand(args []string) (string, []string) {
	if len(args) == 0 {
		return "", args
	}
	switch args[0] {
	case "benchmark", "preflight":
		return args[0], args[1:]
	default:
		return "", args
	}
}

func shouldPrintSummary(cfg *config.Config, runStats *stats, runErr error) bool {
	if cfg.LogMode != "quiet" {
		return true
	}
	return runErr != nil ||
		atomic.LoadUint64(&runStats.notable) > 0 ||
		atomic.LoadUint64(&runStats.parseFailures) > 0 ||
		atomic.LoadUint64(&runStats.writeFailures) > 0
}

func flagWasSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func printOperationalQueries(w io.Writer, args []string) error {
	if len(args) > 2 {
		return fmt.Errorf("usage: dnslog queries [name]")
	}
	name := ""
	if len(args) == 2 {
		name = args[1]
	}
	return ops.PrintQueries(w, name)
}

func printAnalyticsReports(w io.Writer, args []string) error {
	if len(args) > 2 {
		return fmt.Errorf("usage: dnslog analytics [name]")
	}
	name := ""
	if len(args) == 2 {
		name = args[1]
	}
	return analytics.PrintReports(w, name)
}

func run(ctx context.Context, cfg *config.Config, eventStore store.EventStore, runStats *stats) error {
	genesis, err := newGenesisFilter(cfg, time.Now())
	if err != nil {
		return err
	}
	if cfg.Debug && genesis.enabled {
		log.Printf("genesis filter enabled: after=%s", genesis.after.Format(time.RFC3339))
	}

	rawLines := make(chan ingest.RawLine, cfg.BatchSize*2)
	parseResults := make(chan parseResult, cfg.BatchSize*2)
	orderedResults := make(chan parseResult, cfg.BatchSize*2)
	writerErr := make(chan error, 1)
	offsetStore := ingest.NewOffsetStore(cfg.OffsetStatePath)
	if cfg.Debug {
		log.Printf("offset state path: %s", offsetStore.Path())
	}
	if !cfg.DryRun {
		if err := offsetStore.EnsureDir(); err != nil {
			return err
		}
	}
	lock, err := acquireRunLock(cfg)
	if err != nil {
		return err
	}
	if lock != nil {
		defer func() {
			if err := lock.Release(); err != nil {
				log.Printf("release lock: %v", err)
			}
		}()
	}

	progressDone := startProgressReporter(ctx, cfg, runStats)
	defer progressDone()

	var parserWg sync.WaitGroup
	for i := 0; i < cfg.WorkerCount; i++ {
		parserWg.Add(1)
		go parseWorker(ctx, rawLines, parseResults, parser.Options{
			Server: parser.ServerMeta{
				Name: cfg.ServerName,
				Role: cfg.ServerRole,
			},
			IgnoreReverseLookup: cfg.IgnoreReverseLookup,
			IgnoredDomains:      cfg.IgnoredDomains,
			LocalDomains:        cfg.LocalDomains,
		}, genesis, cfg, runStats, &parserWg)
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
	parserOptions parser.Options,
	genesis genesisFilter,
	cfg *config.Config,
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
			atomic.StoreUint64(&runStats.currentOffset, uint64(line.NextState.Offset))
			atomic.StoreUint64(&runStats.inputSize, uint64(line.NextState.Size))
			result := parseResult{
				seq:   line.Seq,
				state: line.NextState,
			}
			if genesis.shouldSkip(line.Text) {
				atomic.AddUint64(&runStats.skippedGenesis, 1)
				select {
				case <-ctx.Done():
					return
				case results <- result:
				}
				continue
			}
			event, err := parser.ParseLine(line.Text, parserOptions)
			if err != nil {
				if errors.Is(err, parser.ErrIgnored) {
					atomic.AddUint64(&runStats.ignored, 1)
					incrementIgnoredCategory(runStats, parser.IgnoredCategory(line.Text))
				} else if errors.Is(err, parser.ErrNotable) {
					atomic.AddUint64(&runStats.notable, 1)
					incrementNotableCategory(runStats, parser.NotableCategory(line.Text))
				} else {
					atomic.AddUint64(&runStats.parseFailures, 1)
					recordParseFailure(cfg, err, line.Text)
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
			cooperateRuntime(cfg)
		}
	}
}

func incrementNotableCategory(runStats *stats, category string) {
	switch category {
	case parser.NotableCategorySecurityDeniedCache:
		atomic.AddUint64(&runStats.notableSecurityDeniedCache, 1)
	default:
		atomic.AddUint64(&runStats.notableRCode, 1)
	}
}

func incrementIgnoredCategory(runStats *stats, category string) {
	switch category {
	case parser.IgnoreCategoryBindNoise:
		atomic.AddUint64(&runStats.ignoredBind, 1)
	case parser.IgnoreCategoryConfig:
		atomic.AddUint64(&runStats.ignoredConfig, 1)
	case parser.IgnoreCategoryNetwork:
		atomic.AddUint64(&runStats.ignoredNetwork, 1)
	case parser.IgnoreCategoryNotify:
		atomic.AddUint64(&runStats.ignoredNotify, 1)
	case parser.IgnoreCategoryRateLimit:
		atomic.AddUint64(&runStats.ignoredRateLimit, 1)
	case parser.IgnoreCategoryResolver:
		atomic.AddUint64(&runStats.ignoredResolver, 1)
	case parser.IgnoreCategorySocket:
		atomic.AddUint64(&runStats.ignoredSocket, 1)
	case parser.IgnoreCategoryTimeout:
		atomic.AddUint64(&runStats.ignoredTimeout, 1)
	case parser.IgnoreCategoryXferIn:
		atomic.AddUint64(&runStats.ignoredXferIn, 1)
	case parser.IgnoreCategoryZoneload:
		atomic.AddUint64(&runStats.ignoredZoneload, 1)
	default:
		atomic.AddUint64(&runStats.ignoredFiltered, 1)
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

	savePendingState := func() bool {
		if !hasPendingState {
			return true
		}
		if cfg.DryRun {
			if !cfg.DryRunUpdatesOffset {
				hasPendingState = false
				return true
			}
			if cfg.Debug {
				log.Printf("[dry-run] updating offset because dry_run_updates_offset=true")
			}
		}
		if err := offsetStore.Save(pendingState); err != nil {
			errs <- err
			return false
		}
		hasPendingState = false
		return true
	}

	flush := func(flushCtx context.Context) bool {
		if len(batch) == 0 {
			return savePendingState()
		}
		if err := writeBatchWithRetry(flushCtx, eventStore, batch, cfg.MaxWriteRetries, cfg.RetryDelay); err != nil {
			atomic.AddUint64(&runStats.writeFailures, 1)
			errs <- err
			return false
		}
		if !savePendingState() {
			return false
		}
		atomic.AddUint64(&runStats.written, uint64(len(batch)))
		atomic.AddUint64(&runStats.batchesWritten, 1)
		batch = batch[:0]
		return true
	}

	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = flush(flushCtx)
			cancel()
			return
		case <-ticker.C:
			if !flush(ctx) {
				return
			}
		case result, ok := <-results:
			if !ok {
				_ = flush(ctx)
				return
			}
			pendingState = result.state
			hasPendingState = true
			if result.hasEvent {
				batch = append(batch, result.event)
			}
			if len(batch) >= cfg.BatchSize && !flush(ctx) {
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

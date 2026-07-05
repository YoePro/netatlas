package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"dnslog/internal/config"
	"dnslog/internal/store"
)

func printSystemCheck(w io.Writer) {
	cpus := runtime.NumCPU()
	mem := detectMemoryBytes()
	workers := cpus - 1
	if workers < 1 {
		workers = 1
	}
	if workers > 4 {
		workers = 4
	}
	batchSize := 2000
	mode := "medium"
	if cpus <= 2 || (mem > 0 && mem < 1024*1024*1024) {
		batchSize = 1000
		mode = "low"
	}

	fmt.Fprintln(w, "Detected:")
	fmt.Fprintf(w, "  arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(w, "  cpu: %d\n", cpus)
	if mem > 0 {
		fmt.Fprintf(w, "  memory: %s\n", formatBytes(mem))
	} else {
		fmt.Fprintln(w, "  memory: unknown")
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Recommended:")
	fmt.Fprintf(w, "  worker_count = %d\n", workers)
	fmt.Fprintf(w, "  batch_size = %d\n", batchSize)
	fmt.Fprintf(w, "  runtime_mode = %s\n", mode)
	fmt.Fprintln(w, "  flush_interval = 5s")
}

func runBenchmark(ctx context.Context, w io.Writer, cfg *config.Config, workerSpec string) error {
	workerCounts, err := benchmarkWorkerCounts(workerSpec, cfg.WorkerCount)
	if err != nil {
		return err
	}

	for i, workers := range workerCounts {
		runCfg := *cfg
		runCfg.WorkerCount = workers
		runCfg.DryRun = true
		runCfg.DryRunUpdatesOffset = false
		runCfg.LogMode = "quiet"
		runCfg.Debug = false
		runCfg.ProgressInterval = 0
		runCfg.OffsetStatePath = filepath.Join(os.TempDir(), fmt.Sprintf("dnslog-benchmark-%d-%d.offset.json", os.Getpid(), i))
		_ = os.Remove(runCfg.OffsetStatePath)

		fmt.Fprintf(w, "Benchmark worker_count=%d\n", workers)
		eventStore, err := store.NewNeo4jStore(ctx, &runCfg)
		if err != nil {
			return err
		}

		runStats := &stats{}
		perf := startPerfMonitor()
		runErr := run(ctx, &runCfg, eventStore, runStats)
		metrics := perf.Stop(runStats, runCfg.BatchSize)
		closeErr := eventStore.Close(ctx)
		_ = os.Remove(runCfg.OffsetStatePath)
		printRunSummary(w, runStats, true, metrics, true)
		if closeErr != nil {
			return closeErr
		}
		if runErr != nil {
			return runErr
		}
		if i < len(workerCounts)-1 {
			fmt.Fprintln(w)
		}
	}

	return nil
}

func benchmarkWorkerCounts(spec string, configured int) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		if configured <= 0 {
			return nil, fmt.Errorf("worker_count must be greater than zero")
		}
		return []int{configured}, nil
	}
	if strings.Contains(spec, "-") && !strings.Contains(spec, ",") {
		parts := strings.Split(spec, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid benchmark worker range %q", spec)
		}
		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid benchmark worker range %q: %w", spec, err)
		}
		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid benchmark worker range %q: %w", spec, err)
		}
		if start <= 0 || end < start {
			return nil, fmt.Errorf("invalid benchmark worker range %q", spec)
		}
		counts := make([]int, 0, end-start+1)
		for value := start; value <= end; value++ {
			counts = append(counts, value)
		}
		return counts, nil
	}

	seen := make(map[int]struct{})
	counts := []int{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid benchmark worker value %q: %w", part, err)
		}
		if value <= 0 {
			return nil, fmt.Errorf("benchmark worker value must be greater than zero: %d", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		counts = append(counts, value)
	}
	if len(counts) == 0 {
		return nil, fmt.Errorf("benchmark worker list is empty")
	}
	return counts, nil
}

func runPreflight(ctx context.Context, w io.Writer, cfg *config.Config) error {
	fmt.Fprintln(w, "Preflight:")
	if err := checkReadableFile(cfg.LogFilePath); err != nil {
		return err
	}
	fmt.Fprintf(w, "  log_file: ok (%s)\n", cfg.LogFilePath)

	if err := checkOffsetWritable(cfg.OffsetStatePath); err != nil {
		return err
	}
	fmt.Fprintf(w, "  offset_state: ok (%s)\n", cfg.OffsetStatePath)

	if cfg.DryRun {
		fmt.Fprintln(w, "  neo4j: skipped (dry_run=true)")
		return nil
	}

	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	eventStore, err := store.NewNeo4jStore(checkCtx, cfg)
	if err != nil {
		return err
	}
	defer eventStore.Close(checkCtx)
	fmt.Fprintln(w, "  neo4j: ok")
	fmt.Fprintln(w, "  schema: ok")
	return nil
}

func checkReadableFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open log file %q: %w", path, err)
	}
	return file.Close()
}

func checkOffsetWritable(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create offset state directory %q: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, ".dnslog-preflight-*")
	if err != nil {
		return fmt.Errorf("create preflight temp file in %q: %w", dir, err)
	}
	name := temp.Name()
	if err := temp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("close preflight temp file: %w", err)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("remove preflight temp file: %w", err)
	}
	return nil
}

func detectMemoryBytes() uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	var kb uint64
	if _, err := fmt.Sscanf(string(data), "MemTotal: %d kB", &kb); err != nil {
		return 0
	}
	return kb * 1024
}

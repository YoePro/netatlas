package main

import (
	"fmt"
	"io"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
)

type performanceMetrics struct {
	Total            time.Duration
	Lines            uint64
	Parsed           uint64
	Written          uint64
	Batches          uint64
	BatchSize        int
	LinesPerSecond   float64
	ParsedPerSecond  float64
	WrittenPerSecond float64
	PeakAllocBytes   uint64
	CPUTime          time.Duration
	CPUUtilization   float64
	BatchEfficiency  float64
}

type perfMonitor struct {
	start     time.Time
	startCPU  time.Duration
	peakAlloc uint64
	stop      chan struct{}
	done      chan struct{}
}

func startPerfMonitor() *perfMonitor {
	monitor := &perfMonitor{
		start:    time.Now(),
		startCPU: cpuTime(),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	monitor.sampleMemory()

	go func() {
		defer close(monitor.done)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-monitor.stop:
				monitor.sampleMemory()
				return
			case <-ticker.C:
				monitor.sampleMemory()
			}
		}
	}()

	return monitor
}

func (m *perfMonitor) Stop(runStats *stats, batchSize int) performanceMetrics {
	close(m.stop)
	<-m.done

	total := time.Since(m.start)
	lines := atomic.LoadUint64(&runStats.lines)
	parsed := atomic.LoadUint64(&runStats.parsed)
	written := atomic.LoadUint64(&runStats.written)
	batches := atomic.LoadUint64(&runStats.batchesWritten)
	cpu := cpuTime() - m.startCPU

	seconds := total.Seconds()
	if seconds <= 0 {
		seconds = 1
	}

	metrics := performanceMetrics{
		Total:            total,
		Lines:            lines,
		Parsed:           parsed,
		Written:          written,
		Batches:          batches,
		BatchSize:        batchSize,
		LinesPerSecond:   float64(lines) / seconds,
		ParsedPerSecond:  float64(parsed) / seconds,
		WrittenPerSecond: float64(written) / seconds,
		PeakAllocBytes:   atomic.LoadUint64(&m.peakAlloc),
		CPUTime:          cpu,
		CPUUtilization:   cpu.Seconds() / seconds * 100,
	}

	if batches > 0 && batchSize > 0 {
		metrics.BatchEfficiency = float64(written) / float64(batches*uint64(batchSize)) * 100
	}

	return metrics
}

func (m *perfMonitor) sampleMemory() {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	for {
		current := atomic.LoadUint64(&m.peakAlloc)
		if mem.Alloc <= current {
			return
		}
		if atomic.CompareAndSwapUint64(&m.peakAlloc, current, mem.Alloc) {
			return
		}
	}
}

func printPerformanceBaseline(w io.Writer, metrics performanceMetrics) {
	fmt.Fprintln(w, "Performance baseline:")
	fmt.Fprintf(w, "  total_time=%s\n", metrics.Total.Round(time.Millisecond))
	fmt.Fprintf(w, "  parse_throughput=%.2f lines/sec\n", metrics.LinesPerSecond)
	fmt.Fprintf(w, "  parsed_event_throughput=%.2f events/sec\n", metrics.ParsedPerSecond)
	fmt.Fprintf(w, "  write_throughput=%.2f events/sec\n", metrics.WrittenPerSecond)
	fmt.Fprintf(w, "  peak_memory=%s\n", formatBytes(metrics.PeakAllocBytes))
	fmt.Fprintf(w, "  cpu_time=%s\n", metrics.CPUTime.Round(time.Millisecond))
	fmt.Fprintf(w, "  cpu_utilization=%.2f%%\n", metrics.CPUUtilization)
	fmt.Fprintf(w, "  batch_efficiency=%.2f%% (%d events / %d batches / batch_size=%d)\n",
		metrics.BatchEfficiency,
		metrics.Written,
		metrics.Batches,
		metrics.BatchSize,
	)
}

func cpuTime() time.Duration {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0
	}

	return timevalDuration(usage.Utime) + timevalDuration(usage.Stime)
}

func timevalDuration(value syscall.Timeval) time.Duration {
	return time.Duration(value.Sec)*time.Second + time.Duration(value.Usec)*time.Microsecond
}

func formatBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}

	div, exp := uint64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}

// Package metrics provides thread-safe counters, gauges, and a simple
// text-format exporter compatible with the Prometheus exposition format
// (Content-Type: text/plain; version=0.0.4).
//
// This is intentionally dependency-free — no Prometheus client library
// is vendored. The entire surface is ~200 lines so migrations are trivial.
package metrics

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Store is a goroutine-safe metrics registry.
type Store struct {
	mu       sync.Mutex
	counters map[string]int64
	gauges   map[string]int64
	latency  map[string]*latencyBucket
	started  time.Time
}

type latencyBucket struct {
	count int64
	total time.Duration
}

// New creates an empty Store.
func New() *Store {
	return &Store{
		counters: make(map[string]int64),
		gauges:   make(map[string]int64),
		latency:  make(map[string]*latencyBucket),
		started:  time.Now(),
	}
}

// Incr increments a counter by 1.
func (s *Store) Incr(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters[name]++
}

// Add increments a counter by delta (delta may be negative, but counters
// should monotonically increase in the Prometheus sense).
func (s *Store) Add(name string, delta int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters[name] += delta
}

// SetGauge sets a gauge to value.
func (s *Store) SetGauge(name string, value int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gauges[name] = value
}

// ObserveLatency records a single latency observation in the named bucket.
func (s *Store) ObserveLatency(name string, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.latency[name]
	if !ok {
		b = &latencyBucket{}
		s.latency[name] = b
	}
	b.count++
	b.total += d
}

// Snapshot returns a consistent snapshot of every metric.
func (s *Store) Snapshot() (counters, gauges map[string]int64, latencies map[string]struct{ Count, AvgMs float64 }) {
	s.mu.Lock()
	defer s.mu.Unlock()

	counters = make(map[string]int64, len(s.counters))
	for k, v := range s.counters {
		counters[k] = v
	}

	gauges = make(map[string]int64, len(s.gauges))
	for k, v := range s.gauges {
		gauges[k] = v
	}

	latencies = make(map[string]struct{ Count, AvgMs float64 }, len(s.latency))
	for k, v := range s.latency {
		avg := 0.0
		if v.count > 0 {
			avg = float64(v.total.Milliseconds()) / float64(v.count)
		}
		latencies[k] = struct{ Count, AvgMs float64 }{
			Count: float64(v.count),
			AvgMs: avg,
		}
	}
	return
}

// WritePrometheusText writes all metrics in Prometheus text format to w.
func (s *Store) WritePrometheusText(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, _ = fmt.Fprintf(w, "# HELP phenodag_uptime_seconds Uptime of the process in seconds.\n")
	_, _ = fmt.Fprintf(w, "# TYPE phenodag_uptime_seconds gauge\n")
	_, _ = fmt.Fprintf(w, "phenodag_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))

	for k, v := range s.counters {
		name := sanitise(k)
		_, _ = fmt.Fprintf(w, "# HELP %s Total count of %s events.\n", name, k)
		_, _ = fmt.Fprintf(w, "# TYPE %s counter\n", name)
		_, _ = fmt.Fprintf(w, "%s %d\n", name, v)
	}

	for k, v := range s.gauges {
		name := sanitise(k)
		_, _ = fmt.Fprintf(w, "# HELP %s Current gauge value for %s.\n", name, k)
		_, _ = fmt.Fprintf(w, "# TYPE %s gauge\n", name)
		_, _ = fmt.Fprintf(w, "%s %d\n", name, v)
	}

	for k, b := range s.latency {
		prefix := sanitise(k)
		if b.count > 0 {
			avg := float64(b.total.Milliseconds()) / float64(b.count)
			_, _ = fmt.Fprintf(w, "# HELP %s Latency observations for %s.\n", prefix, k)
			_, _ = fmt.Fprintf(w, "# TYPE %s summary\n", prefix)
			_, _ = fmt.Fprintf(w, "%s_count %d\n", prefix, b.count)
			_, _ = fmt.Fprintf(w, "%s_sum_ms %d\n", prefix, b.total.Milliseconds())
			_, _ = fmt.Fprintf(w, "%s_avg_ms %.1f\n", prefix, avg)
		}
	}
}

// sanitise replaces non-Prometheus characters with underscores.
func sanitise(name string) string {
	r := strings.NewReplacer(
		".", "_",
		"-", "_",
		" ", "_",
		"/", "_",
		":", "_",
	)
	return "phenodag_" + r.Replace(name)
}

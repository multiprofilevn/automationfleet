//go:build integration

package automationfleet_test

import (
	"sort"
	"sync"
	"testing"
	"time"
)

// latencyRecorder collects per-op durations across many benchmark iterations
// and reports p50/p95/max — a mean alone hides the tail that decides whether
// 9 native browsers can hold a business SLA (Non-functional requirement in
// phase-04-benchmark-throughput-native.md). Shape borrowed from chromekit's
// own stress harness (chromekit/stress_helpers_test.go latencyRecorder).
type latencyRecorder struct {
	mu   sync.Mutex
	data map[string][]time.Duration
	errs map[string]int64
}

func newLatencyRecorder() *latencyRecorder {
	return &latencyRecorder{data: map[string][]time.Duration{}, errs: map[string]int64{}}
}

func (r *latencyRecorder) record(op string, d time.Duration, err error) {
	r.mu.Lock()
	r.data[op] = append(r.data[op], d)
	if err != nil {
		r.errs[op]++
	}
	r.mu.Unlock()
}

type latStat struct {
	op            string
	n             int
	errs          int64
	p50, p95, max time.Duration
}

func (r *latencyRecorder) stats() []latStat {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]latStat, 0, len(r.data))
	for op, ds := range r.data {
		cp := append([]time.Duration(nil), ds...)
		sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
		out = append(out, latStat{
			op:   op,
			n:    len(cp),
			errs: r.errs[op],
			p50:  pct(cp, 0.50),
			p95:  pct(cp, 0.95),
			max:  cp[len(cp)-1],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].op < out[j].op })
	return out
}

// pct requires >= 20 samples per the phase's non-functional requirement to
// mean anything at p95 — callers should not trust this on fewer.
func pct(sorted []time.Duration, q float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(q * float64(len(sorted)-1))
	return sorted[idx]
}

// logPercentiles writes p50/p95/max per op to the benchmark log — visible
// with `go test -v`, captured verbatim in bench.out by the manual run command.
func logPercentiles(b *testing.B, r *latencyRecorder) {
	b.Helper()
	for _, s := range r.stats() {
		if s.n < 20 {
			b.Logf("%-28s n=%-5d (WARNING: <20 samples, see phase non-functional requirement)", s.op, s.n)
		}
		b.Logf("%-28s n=%-5d errs=%-3d p50=%-10s p95=%-10s max=%s",
			s.op, s.n, s.errs, s.p50, s.p95, s.max)
	}
}

package monitor

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type counters struct {
	mu sync.RWMutex
	m  map[string]*atomic.Int64
}

func newCounters() *counters {
	return &counters{m: map[string]*atomic.Int64{}}
}

func (c *counters) inc(labels string, delta int64) {
	c.mu.RLock()
	v, ok := c.m[labels]
	c.mu.RUnlock()
	if ok {
		v.Add(delta)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok = c.m[labels]
	if !ok {
		v = &atomic.Int64{}
		c.m[labels] = v
	}
	v.Add(delta)
}

func (c *counters) set(labels string, val int64) {
	c.mu.RLock()
	v, ok := c.m[labels]
	c.mu.RUnlock()
	if ok {
		v.Store(val)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok = c.m[labels]
	if !ok {
		v = &atomic.Int64{}
		c.m[labels] = v
	}
	v.Store(val)
}

func (c *counters) snapshot() map[string]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]int64, len(c.m))
	for k, v := range c.m {
		out[k] = v.Load()
	}
	return out
}

var histogramBuckets = []float64{50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000}

type histogram struct {
	mu      sync.RWMutex
	buckets map[string][]*atomic.Int64
	sums    map[string]*atomic.Int64
	counts  map[string]*atomic.Int64
}

func newHistogram() *histogram {
	return &histogram{
		buckets: map[string][]*atomic.Int64{},
		sums:    map[string]*atomic.Int64{},
		counts:  map[string]*atomic.Int64{},
	}
}

func (h *histogram) observe(labels string, valueMs float64) {
	h.initLabel(labels)
	h.sums[labels].Add(int64(valueMs * 1000))
	h.counts[labels].Add(1)
	for i, bound := range histogramBuckets {
		if valueMs <= bound {
			h.buckets[labels][i].Add(1)
		}
	}
	infIdx := len(histogramBuckets)
	h.buckets[labels][infIdx].Add(1)
}

func (h *histogram) initLabel(labels string) {
	h.mu.RLock()
	_, ok := h.sums[labels]
	h.mu.RUnlock()
	if ok {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.sums[labels]; ok {
		return
	}
	n := len(histogramBuckets) + 1
	buckets := make([]*atomic.Int64, n)
	for i := 0; i < n; i++ {
		buckets[i] = &atomic.Int64{}
	}
	h.buckets[labels] = buckets
	h.sums[labels] = &atomic.Int64{}
	h.counts[labels] = &atomic.Int64{}
}

type histogramSnap struct {
	buckets []int64
	sum     int64
	count   int64
}

func (h *histogram) snapshot() map[string]histogramSnap {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[string]histogramSnap, len(h.sums))
	for labels := range h.sums {
		buckets := h.buckets[labels]
		snap := histogramSnap{
			buckets: make([]int64, len(buckets)),
			sum:     h.sums[labels].Load(),
			count:   h.counts[labels].Load(),
		}
		for i, b := range buckets {
			snap.buckets[i] = b.Load()
		}
		out[labels] = snap
	}
	return out
}

type Registry struct {
	requestTotal              *counters
	requestDuration           *histogram
	tokensTotal               *counters
	emptyRetriesTotal         atomic.Int64
	accountSwitchRetriesTotal atomic.Int64
	accountPoolInUse          atomic.Int64
	accountPoolAvailable      atomic.Int64
	accountPoolWaiting        atomic.Int64
	accountPoolTotal          atomic.Int64
	alertEventsTotal          *counters
}

var global = &Registry{
	requestTotal:    newCounters(),
	requestDuration: newHistogram(),
	tokensTotal:     newCounters(),
	alertEventsTotal: newCounters(),
}

func RecordRequest(surface, model string, statusCode int, elapsedMs int64, promptTokens, completionTokens, reasoningTokens int) {
	statusLabel := fmt.Sprintf("%d", statusCode)
	if statusCode >= 200 && statusCode < 300 {
		statusLabel = "2xx"
	} else if statusCode >= 400 && statusCode < 500 {
		statusLabel = "4xx"
	} else if statusCode >= 500 {
		statusLabel = "5xx"
	}
	requestLabels := fmt.Sprintf(`surface="%s",model="%s",status="%s"`, surface, model, statusLabel)
	global.requestTotal.inc(requestLabels, 1)
	durationLabels := fmt.Sprintf(`surface="%s",model="%s"`, surface, model)
	global.requestDuration.observe(durationLabels, float64(elapsedMs))
	global.tokensTotal.inc(`type="prompt"`, int64(promptTokens))
	global.tokensTotal.inc(`type="completion"`, int64(completionTokens))
	global.tokensTotal.inc(`type="reasoning"`, int64(reasoningTokens))
}

func RecordEmptyRetry() {
	global.emptyRetriesTotal.Add(1)
}

func RecordAccountSwitchRetry() {
	global.accountSwitchRetriesTotal.Add(1)
}

func RecordAccountPool(inUse, available, waiting, total int) {
	global.accountPoolInUse.Store(int64(inUse))
	global.accountPoolAvailable.Store(int64(available))
	global.accountPoolWaiting.Store(int64(waiting))
	global.accountPoolTotal.Store(int64(total))
}

func RecordAlertEvent(severity, event string) {
	labels := fmt.Sprintf(`severity="%s",event="%s"`, severity, event)
	global.alertEventsTotal.inc(labels, 1)
}

func Serialize(sb *strings.Builder) {
	sb.WriteString("# HELP ds2api_requests_total Total number of API requests.\n")
	sb.WriteString("# TYPE ds2api_requests_total counter\n")
	writeCounters(sb, "ds2api_requests_total", global.requestTotal.snapshot())

	sb.WriteString("# HELP ds2api_request_duration_ms Request latency in milliseconds.\n")
	sb.WriteString("# TYPE ds2api_request_duration_ms histogram\n")
	writeHistogram(sb, "ds2api_request_duration_ms", global.requestDuration.snapshot())

	sb.WriteString("# HELP ds2api_tokens_total Total tokens processed.\n")
	sb.WriteString("# TYPE ds2api_tokens_total counter\n")
	writeCounters(sb, "ds2api_tokens_total", global.tokensTotal.snapshot())

	sb.WriteString("# HELP ds2api_empty_output_retries_total Number of empty-output retries.\n")
	sb.WriteString("# TYPE ds2api_empty_output_retries_total counter\n")
	fmt.Fprintf(sb, "ds2api_empty_output_retries_total %d\n", global.emptyRetriesTotal.Load())

	sb.WriteString("# HELP ds2api_account_switch_retries_total Number of account-switch retries after 429.\n")
	sb.WriteString("# TYPE ds2api_account_switch_retries_total counter\n")
	fmt.Fprintf(sb, "ds2api_account_switch_retries_total %d\n", global.accountSwitchRetriesTotal.Load())

	sb.WriteString("# HELP ds2api_account_pool_in_use Number of in-use account slots.\n")
	sb.WriteString("# TYPE ds2api_account_pool_in_use gauge\n")
	fmt.Fprintf(sb, "ds2api_account_pool_in_use %d\n", global.accountPoolInUse.Load())

	sb.WriteString("# HELP ds2api_account_pool_available Number of available accounts with free slots.\n")
	sb.WriteString("# TYPE ds2api_account_pool_available gauge\n")
	fmt.Fprintf(sb, "ds2api_account_pool_available %d\n", global.accountPoolAvailable.Load())

	sb.WriteString("# HELP ds2api_account_pool_waiting Number of requests waiting in queue.\n")
	sb.WriteString("# TYPE ds2api_account_pool_waiting gauge\n")
	fmt.Fprintf(sb, "ds2api_account_pool_waiting %d\n", global.accountPoolWaiting.Load())

	sb.WriteString("# HELP ds2api_account_pool_total Total number of configured accounts.\n")
	sb.WriteString("# TYPE ds2api_account_pool_total gauge\n")
	fmt.Fprintf(sb, "ds2api_account_pool_total %d\n", global.accountPoolTotal.Load())

	sb.WriteString("# HELP ds2api_alert_events_total Number of alert events fired.\n")
	sb.WriteString("# TYPE ds2api_alert_events_total counter\n")
	writeCounters(sb, "ds2api_alert_events_total", global.alertEventsTotal.snapshot())
}

func writeCounters(sb *strings.Builder, name string, m map[string]int64) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(sb, "%s{%s} %d\n", name, k, m[k])
	}
}

func writeHistogram(sb *strings.Builder, name string, m map[string]histogramSnap) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		snap := m[k]
		cumulative := int64(0)
		for i, bound := range histogramBuckets {
			cumulative += snap.buckets[i]
			fmt.Fprintf(sb, "%s_bucket{%s,le=\"%.0f\"} %d\n", name, k, bound, cumulative)
		}
		cumulative += snap.buckets[len(histogramBuckets)]
		fmt.Fprintf(sb, "%s_bucket{%s,le=\"+Inf\"} %d\n", name, k, cumulative)
		fmt.Fprintf(sb, "%s_sum{%s} %.3f\n", name, k, float64(snap.sum)/1000.0)
		fmt.Fprintf(sb, "%s_count{%s} %d\n", name, k, snap.count)
	}
}

func ResetForTesting() {
	global = &Registry{
		requestTotal:    newCounters(),
		requestDuration: newHistogram(),
		tokensTotal:     newCounters(),
		alertEventsTotal: newCounters(),
	}
}

func MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		var sb strings.Builder
		Serialize(&sb)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sb.String()))
	}
}

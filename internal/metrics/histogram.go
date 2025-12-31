package metrics

import (
	"sort"
	"sync"
	"time"
)

// Histogram tracks response time distribution
type Histogram struct {
	mu       sync.RWMutex
	values   []float64 // Response times in milliseconds
	maxSize  int       // Maximum number of values to keep
	min      float64
	max      float64
	sum      float64
	count    uint64
}

// NewHistogram creates a new histogram with a maximum size
func NewHistogram(maxSize int) *Histogram {
	return &Histogram{
		values:  make([]float64, 0, maxSize),
		maxSize: maxSize,
		min:     0,
		max:     0,
		sum:     0,
		count:   0,
	}
}

// Record adds a duration measurement to the histogram
func (h *Histogram) Record(duration time.Duration) {
	ms := float64(duration.Microseconds()) / 1000.0 // Convert to milliseconds

	h.mu.Lock()
	defer h.mu.Unlock()

	// Update min/max
	if h.count == 0 || ms < h.min {
		h.min = ms
	}
	if ms > h.max {
		h.max = ms
	}

	h.sum += ms
	h.count++

	// Add to values array, maintaining max size
	if len(h.values) < h.maxSize {
		h.values = append(h.values, ms)
	} else {
		// Circular buffer: replace oldest value
		h.values[int(h.count-1)%h.maxSize] = ms
	}
}

// Snapshot returns a snapshot of the current histogram statistics
type HistogramSnapshot struct {
	Min   float64 `json:"min_ms"`
	Max   float64 `json:"max_ms"`
	Avg   float64 `json:"avg_ms"`
	P50   float64 `json:"p50_ms"`
	P95   float64 `json:"p95_ms"`
	P99   float64 `json:"p99_ms"`
	Count uint64  `json:"count"`
}

// Snapshot returns a snapshot of the histogram statistics
func (h *Histogram) Snapshot() HistogramSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.count == 0 {
		return HistogramSnapshot{}
	}

	// Calculate average
	avg := h.sum / float64(h.count)

	// Calculate percentiles from the values array
	// Make a copy to avoid modifying the original
	valuesCopy := make([]float64, len(h.values))
	copy(valuesCopy, h.values)
	sort.Float64s(valuesCopy)

	p50 := percentile(valuesCopy, 0.50)
	p95 := percentile(valuesCopy, 0.95)
	p99 := percentile(valuesCopy, 0.99)

	return HistogramSnapshot{
		Min:   h.min,
		Max:   h.max,
		Avg:   avg,
		P50:   p50,
		P95:   p95,
		P99:   p99,
		Count: h.count,
	}
}

// percentile calculates the percentile from a sorted slice
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if len(sorted) == 1 {
		return sorted[0]
	}

	// Calculate index
	rank := p * float64(len(sorted)-1)
	lower := int(rank)
	upper := lower + 1

	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	// Linear interpolation
	weight := rank - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// Reset clears all histogram data
func (h *Histogram) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.values = make([]float64, 0, h.maxSize)
	h.min = 0
	h.max = 0
	h.sum = 0
	h.count = 0
}

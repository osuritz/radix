package metrics

import (
	"testing"
	"time"
)

func TestNewHistogram(t *testing.T) {
	h := NewHistogram(100)
	if h == nil {
		t.Fatal("NewHistogram returned nil")
	}

	if h.maxSize != 100 {
		t.Errorf("maxSize = %d, want 100", h.maxSize)
	}

	snapshot := h.Snapshot()
	if snapshot.Count != 0 {
		t.Errorf("initial count = %d, want 0", snapshot.Count)
	}
}

func TestHistogramRecord(t *testing.T) {
	h := NewHistogram(10)

	// Record some durations
	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}

	for _, d := range durations {
		h.Record(d)
	}

	snapshot := h.Snapshot()

	// Check count
	if snapshot.Count != 5 {
		t.Errorf("count = %d, want 5", snapshot.Count)
	}

	// Check min/max
	if snapshot.Min < 9.9 || snapshot.Min > 10.1 {
		t.Errorf("min = %.2f, want ~10", snapshot.Min)
	}

	if snapshot.Max < 49.9 || snapshot.Max > 50.1 {
		t.Errorf("max = %.2f, want ~50", snapshot.Max)
	}

	// Check average
	expectedAvg := 30.0
	if snapshot.Avg < expectedAvg-1 || snapshot.Avg > expectedAvg+1 {
		t.Errorf("avg = %.2f, want ~%.2f", snapshot.Avg, expectedAvg)
	}

	// Check median (p50)
	if snapshot.P50 < 29 || snapshot.P50 > 31 {
		t.Errorf("p50 = %.2f, want ~30", snapshot.P50)
	}
}

func TestHistogramPercentiles(t *testing.T) {
	h := NewHistogram(1000)

	// Record 100 values: 1ms, 2ms, 3ms, ..., 100ms
	for i := 1; i <= 100; i++ {
		h.Record(time.Duration(i) * time.Millisecond)
	}

	snapshot := h.Snapshot()

	// P50 should be around 50ms
	if snapshot.P50 < 49 || snapshot.P50 > 51 {
		t.Errorf("p50 = %.2f, want ~50", snapshot.P50)
	}

	// P95 should be around 95ms
	if snapshot.P95 < 94 || snapshot.P95 > 96 {
		t.Errorf("p95 = %.2f, want ~95", snapshot.P95)
	}

	// P99 should be around 99ms
	if snapshot.P99 < 98 || snapshot.P99 > 100 {
		t.Errorf("p99 = %.2f, want ~99", snapshot.P99)
	}
}

func TestHistogramCircularBuffer(t *testing.T) {
	h := NewHistogram(3) // Small buffer size

	// Record more values than buffer size
	for i := 1; i <= 10; i++ {
		h.Record(time.Duration(i) * time.Millisecond)
	}

	snapshot := h.Snapshot()

	// Count should reflect all recordings
	if snapshot.Count != 10 {
		t.Errorf("count = %d, want 10", snapshot.Count)
	}

	// Min should be from the first value
	if snapshot.Min < 0.9 || snapshot.Min > 1.1 {
		t.Errorf("min = %.2f, want ~1", snapshot.Min)
	}

	// Max should be from the last value
	if snapshot.Max < 9.9 || snapshot.Max > 10.1 {
		t.Errorf("max = %.2f, want ~10", snapshot.Max)
	}

	// Average should include all values
	expectedAvg := 5.5 // (1+2+3+...+10)/10
	if snapshot.Avg < expectedAvg-0.5 || snapshot.Avg > expectedAvg+0.5 {
		t.Errorf("avg = %.2f, want ~%.2f", snapshot.Avg, expectedAvg)
	}
}

func TestHistogramReset(t *testing.T) {
	h := NewHistogram(100)

	// Record some values
	h.Record(10 * time.Millisecond)
	h.Record(20 * time.Millisecond)
	h.Record(30 * time.Millisecond)

	// Reset
	h.Reset()

	snapshot := h.Snapshot()

	// Everything should be zero
	if snapshot.Count != 0 {
		t.Errorf("count after reset = %d, want 0", snapshot.Count)
	}

	if snapshot.Min != 0 {
		t.Errorf("min after reset = %.2f, want 0", snapshot.Min)
	}

	if snapshot.Max != 0 {
		t.Errorf("max after reset = %.2f, want 0", snapshot.Max)
	}
}

func TestPercentileFunction(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		p      float64
		want   float64
	}{
		{
			name:   "empty slice",
			values: []float64{},
			p:      0.5,
			want:   0,
		},
		{
			name:   "single value",
			values: []float64{42},
			p:      0.5,
			want:   42,
		},
		{
			name:   "median of odd count",
			values: []float64{1, 2, 3, 4, 5},
			p:      0.5,
			want:   3,
		},
		{
			name:   "median of even count",
			values: []float64{1, 2, 3, 4},
			p:      0.5,
			want:   2.5,
		},
		{
			name:   "p95",
			values: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:      0.95,
			want:   9.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := percentile(tt.values, tt.p)
			// Use tolerance for floating point comparison
			tolerance := 0.1
			if got < tt.want-tolerance || got > tt.want+tolerance {
				t.Errorf("percentile(%v, %.2f) = %.2f, want %.2f (±%.2f)",
					tt.values, tt.p, got, tt.want, tolerance)
			}
		})
	}
}

func TestHistogramConcurrency(t *testing.T) {
	h := NewHistogram(1000)

	// Record from multiple goroutines
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				h.Record(time.Millisecond)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	snapshot := h.Snapshot()

	// Should have 1000 recordings
	if snapshot.Count != 1000 {
		t.Errorf("count = %d, want 1000", snapshot.Count)
	}
}

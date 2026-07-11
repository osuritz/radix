package metrics

import (
	"net/http"
	"testing"
	"time"
)

// benchStatuses and benchMethods give RecordRequest a realistic spread of
// status codes and methods so the sync.Map counters see more than one key.
var (
	benchStatuses = []int{http.StatusOK, http.StatusCreated, http.StatusNotFound, http.StatusInternalServerError}
	benchMethods  = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}
)

func BenchmarkCollector_RecordRequest(b *testing.B) {
	c := NewCollector("bench", "test")
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		c.RecordRequest(benchStatuses[i%len(benchStatuses)], benchMethods[i%len(benchMethods)],
			5*time.Millisecond, 128, 1024)
		i++
	}
}

func BenchmarkCollector_RecordRequest_Parallel(b *testing.B) {
	c := NewCollector("bench", "test")
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.RecordRequest(benchStatuses[i%len(benchStatuses)], benchMethods[i%len(benchMethods)],
				5*time.Millisecond, 128, 1024)
			i++
		}
	})
}

func BenchmarkCollector_Snapshot(b *testing.B) {
	c := NewCollector("bench", "test")
	// Fill the response-time histogram to its capacity (10k samples) so the
	// snapshot's percentile computation runs at its steady-state cost.
	for i := 0; i < 10000; i++ {
		c.RecordRequest(benchStatuses[i%len(benchStatuses)], benchMethods[i%len(benchMethods)],
			time.Duration(i%50)*time.Millisecond, 128, 1024)
	}
	b.ReportAllocs()
	for b.Loop() {
		snapshot := c.Snapshot()
		if snapshot.Requests.Total == 0 {
			b.Fatal("snapshot recorded zero requests")
		}
	}
}

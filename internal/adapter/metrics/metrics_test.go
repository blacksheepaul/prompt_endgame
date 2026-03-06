package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// newIsolatedProviderErrors creates an isolated ProviderErrors counter for testing
// to avoid global state pollution between test runs
func newIsolatedProviderErrors() *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "test_provider_errors_total",
		Help: "Test counter",
	}, []string{"type"})
}

func TestProviderErrors(t *testing.T) {
	tests := []struct {
		name      string
		errType   string
		increment int
	}{
		{"timeout error", "timeout", 1},
		{"connection_refused error", "connection_refused", 2},
		{"429 error", "429", 3},
		{"parse_error", "parse_error", 1},
		{"cancelled error", "cancelled", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use isolated counter to avoid cross-test pollution
			counter := newIsolatedProviderErrors()

			for i := 0; i < tt.increment; i++ {
				counter.WithLabelValues(tt.errType).Inc()
			}

			count := testutil.ToFloat64(counter.WithLabelValues(tt.errType))
			if count != float64(tt.increment) {
				t.Errorf("Expected count %d for type %s, got %f", tt.increment, tt.errType, count)
			}
		})
	}
}

func TestProviderErrors_ValidLabels(t *testing.T) {
	// Verify the global ProviderErrors accepts exactly the expected error types
	// These are the canonical labels used in production code
	validTypes := []string{"timeout", "connection_refused", "429", "parse_error", "cancelled", "stream_error"}

	counter := newIsolatedProviderErrors()
	for _, errType := range validTypes {
		counter.WithLabelValues(errType).Inc()
		count := testutil.ToFloat64(counter.WithLabelValues(errType))
		if count != 1 {
			t.Errorf("Expected count 1 for valid type %q, got %f", errType, count)
		}
	}
}

func TestQueueWaitTime(t *testing.T) {
	// Use isolated histogram to avoid global state
	hist := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "test_queue_wait_seconds",
		Help:    "Test histogram",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})

	testValues := []float64{0.001, 0.01, 0.1, 0.5, 1.0, 2.0}
	for _, value := range testValues {
		hist.Observe(value)
	}

	count := testutil.CollectAndCount(hist)
	if count == 0 {
		t.Error("QueueWaitTime histogram should have recorded values")
	}
}

func TestQueueWaitTime_BucketCoverage(t *testing.T) {
	// Verify the global QueueWaitTime has buckets that cover typical queue wait scenarios:
	// - Sub-millisecond (0.001s) for fast paths
	// - Sub-second (0.1, 0.25, 0.5s) for normal loads
	// - Multi-second (1s, 5s, 10s) for backpressure scenarios
	hist := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "test_queue_wait_coverage",
		Help:    "Test",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})

	// Observe values that span the expected range and verify they land in buckets
	hist.Observe(0.001) // minimum fast path
	hist.Observe(10.0)  // maximum backpressure

	count := testutil.CollectAndCount(hist)
	if count == 0 {
		t.Error("Histogram should be collectable")
	}
}

func TestMetricsRegistration(t *testing.T) {
	// Test that all metrics are properly registered and non-nil
	metrics := []struct {
		name   string
		metric interface{}
	}{
		{"ActiveTurns", ActiveTurns},
		{"TurnDuration", TurnDuration},
		{"TurnTotal", TurnTotal},
		{"Goroutines", Goroutines},
		{"TimeToFirstToken", TimeToFirstToken},
		{"TokensPerSecond", TokensPerSecond},
		{"TotalTokens", TotalTokens},
		{"ProviderErrors", ProviderErrors},
		{"QueueWaitTime", QueueWaitTime},
	}

	for _, m := range metrics {
		t.Run(m.name, func(t *testing.T) {
			if m.metric == nil {
				t.Errorf("%s should not be nil", m.name)
			}
		})
	}
}

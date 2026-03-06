package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ActiveTurns tracks the number of currently running turns
	ActiveTurns = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "prompt_endgame_active_turns",
		Help: "Number of turns currently being processed",
	})

	// TurnDuration tracks the duration of turn processing
	TurnDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "prompt_endgame_turn_duration_seconds",
		Help:    "Duration of turn processing in seconds",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
	})

	// TurnTotal tracks the total number of turns processed
	TurnTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "prompt_endgame_turn_total",
		Help: "Total number of turns processed",
	}, []string{"status"})

	// Goroutines tracks the number of goroutines (for baseline observation)
	Goroutines = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "prompt_endgame_goroutines",
		Help: "Number of goroutines currently running",
	})

	// TimeToFirstToken tracks the time from turn start to first token received
	TimeToFirstToken = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "prompt_endgame_ttft_seconds",
		Help:    "Time to first token in seconds",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	})

	// TokensPerSecond tracks the token generation rate
	TokensPerSecond = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "prompt_endgame_tokens_per_second",
		Help:    "Tokens per second generation rate",
		Buckets: []float64{1, 5, 10, 20, 50, 100, 200, 500},
	})

	// TotalTokens tracks the total number of tokens generated
	TotalTokens = promauto.NewCounter(prometheus.CounterOpts{
		Name: "prompt_endgame_tokens_total",
		Help: "Total number of tokens generated",
	})

	// ProviderErrors tracks errors from LLM providers by error type
	// Labels: type (timeout, connection_refused, 429, parse_error, cancelled)
	ProviderErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "prompt_endgame_provider_errors_total",
		Help: "Total number of provider errors by type",
	}, []string{"type"})

	// QueueWaitTime tracks the time spent waiting in queue before processing
	QueueWaitTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "prompt_endgame_queue_wait_seconds",
		Help:    "Time spent waiting in queue before processing",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})
)

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
)

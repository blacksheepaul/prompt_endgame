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
)

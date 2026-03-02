# Observability

This directory contains monitoring and observability configurations for Prompt Endgame.

## Grafana Dashboard

Import `grafana/dashboards/prompt_endgame.json` into your Grafana instance.

### Panels

- **Active Turns**: Current number of turns being processed (thresholds: green < 10, yellow < 50, red >= 50)
- **Goroutines**: Current goroutine count (thresholds: green < 100, yellow < 500, red >= 500)
- **Turn Duration**: p50/p95/p99 latency percentiles over 5m windows
- **Turns by Status**: Turn completion rate by status (done/cancelled/error)

### Setup

1. Configure Prometheus to scrape `http://localhost:${PORT}/metrics`
2. Add Prometheus as a data source in Grafana
3. Import the dashboard JSON

## Prometheus

Example scrape configuration:

```yaml
scrape_configs:
    - job_name: "prompt_endgame"
      static_configs:
          - targets: ["localhost:${PORT}"]
```

## Metrics Reference

See `internal/adapter/metrics/metrics.go` for metric definitions.

### Available Metrics

- `prompt_endgame_active_turns` (Gauge): Currently running turns
- `prompt_endgame_turn_duration_seconds` (Histogram): Turn processing duration
- `prompt_endgame_turn_total` (Counter): Total turns by status
- `prompt_endgame_goroutines` (Gauge): Current goroutine count

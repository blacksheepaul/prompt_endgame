package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const baseURL = "http://localhost:10180"

// FakeLLMScenario defines configuration parameters for a Fake LLM test scenario
type FakeLLMScenario struct {
	Name                 string
	MaxConcurrent        int
	FixedDelayMs         int
	JitterMs             int
	SlowdownQPSThreshold int
	SlowdownFactor       float64
}

// predefinedScenarios maps scenario names to their configurations
var predefinedScenarios = map[string]FakeLLMScenario{
	"fast": {
		Name:          "fast",
		MaxConcurrent: 200,
		FixedDelayMs:  10,
		JitterMs:      5,
	},
	"slow": {
		Name:          "slow",
		MaxConcurrent: 10,
		FixedDelayMs:  500,
		JitterMs:      100,
	},
	"backpressure": {
		Name:                 "backpressure",
		MaxConcurrent:        5,
		FixedDelayMs:         100,
		JitterMs:             50,
		SlowdownQPSThreshold: 50,
		SlowdownFactor:       0.5,
	},
}

// TestResults holds aggregated load test results
type TestResults struct {
	TotalTurns   int
	SuccessTurns int
	FailedTurns  int
	TotalTokens  int64
	Throughput   float64 // turns/sec
	AvgLatency   time.Duration
	P50Latency   time.Duration
	P95Latency   time.Duration
	P99Latency   time.Duration
}

// MarkdownReport holds all data needed to generate a test report
type MarkdownReport struct {
	Title         string
	Timestamp     time.Time
	Scenario      string
	Concurrency   int
	Duration      time.Duration
	FakeLLMConfig FakeLLMScenario
	Results       TestResults
	OutputDir     string
}

// Generate produces a Markdown-formatted report string with actual field values
func (r MarkdownReport) Generate() string {
	successRate := 0.0
	failRate := 0.0
	if r.Results.TotalTurns > 0 {
		successRate = float64(r.Results.SuccessTurns) / float64(r.Results.TotalTurns) * 100
		failRate = float64(r.Results.FailedTurns) / float64(r.Results.TotalTurns) * 100
	}

	return fmt.Sprintf(`# %s

Generated: %s

## Test Configuration

| Parameter | Value |
|-----------|-------|
| Scenario | %s |
| Concurrency | %d rooms |
| Duration | %s |

## Fake LLM Configuration

| Parameter | Value |
|-----------|-------|
| Max Concurrent | %d |
| Fixed Delay | %dms |
| Jitter | %dms |
| Slowdown QPS Threshold | %d |
| Slowdown Factor | %.2f |

## Results Summary

| Metric | Value |
|--------|-------|
| Total Turns | %d |
| Success | %d (%.1f%%) |
| Failed | %d (%.1f%%) |
| Total Tokens | %d |
| Throughput | %.2f turns/sec |

## Latency Distribution

| Percentile | Latency |
|------------|---------|
| P50 | %s |
| P95 | %s |
| P99 | %s |

## Key Metrics

- Avg Latency: %s
- Throughput: %.2f turns/sec
- Total Tokens: %d
`,
		r.Title,
		r.Timestamp.Format("2006-01-02 15:04:05"),
		r.Scenario,
		r.Concurrency,
		r.Duration.String(),
		r.FakeLLMConfig.MaxConcurrent,
		r.FakeLLMConfig.FixedDelayMs,
		r.FakeLLMConfig.JitterMs,
		r.FakeLLMConfig.SlowdownQPSThreshold,
		r.FakeLLMConfig.SlowdownFactor,
		r.Results.TotalTurns,
		r.Results.SuccessTurns, successRate,
		r.Results.FailedTurns, failRate,
		r.Results.TotalTokens,
		r.Results.Throughput,
		r.Results.P50Latency.Round(time.Millisecond).String(),
		r.Results.P95Latency.Round(time.Millisecond).String(),
		r.Results.P99Latency.Round(time.Millisecond).String(),
		r.Results.AvgLatency.Round(time.Millisecond).String(),
		r.Results.Throughput,
		r.Results.TotalTokens,
	)
}

// WriteFile writes the report to a .md file inside outputDir and returns the file path.
func (r MarkdownReport) WriteFile(outputDir string) (string, error) {
	filename := fmt.Sprintf("report_%s_%s.md",
		r.Scenario,
		r.Timestamp.Format("20060102_150405"),
	)
	path := filepath.Join(outputDir, filename)
	if err := os.WriteFile(path, []byte(r.Generate()), 0644); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}
	return path, nil
}

// lookupScenario returns the FakeLLMScenario for the given name, or an error if unknown.
func lookupScenario(name string) (FakeLLMScenario, error) {
	s, ok := predefinedScenarios[name]
	if !ok {
		return FakeLLMScenario{}, fmt.Errorf("unknown scenario %q; available: fast, slow, backpressure", name)
	}
	return s, nil
}

// fakeLLMConfigPayload returns the JSON body for PATCH /admin/config.
func fakeLLMConfigPayload(s FakeLLMScenario) []byte {
	type payload struct {
		MaxConcurrent        int     `json:"max_concurrent"`
		FixedDelayMs         int     `json:"fixed_delay_ms"`
		JitterMs             int     `json:"jitter_ms"`
		SlowdownQPSThreshold int     `json:"slowdown_qps_threshold"`
		SlowdownFactor       float64 `json:"slowdown_factor"`
	}
	b, _ := json.Marshal(payload{
		MaxConcurrent:        s.MaxConcurrent,
		FixedDelayMs:         s.FixedDelayMs,
		JitterMs:             s.JitterMs,
		SlowdownQPSThreshold: s.SlowdownQPSThreshold,
		SlowdownFactor:       s.SlowdownFactor,
	})
	return b
}

// configureFakeLLM sends the scenario config to the Fake LLM admin API.
func configureFakeLLM(fakeLLMBase string, s FakeLLMScenario) error {
	url := fakeLLMBase + "/admin/config"
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(fakeLLMConfigPayload(s)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("PATCH %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PATCH %s: HTTP %d: %s", url, resp.StatusCode, string(body))
	}
	return nil
}

// resetFakeLLM resets the Fake LLM to a permissive default state.
func resetFakeLLM(fakeLLMBase string) error {
	return configureFakeLLM(fakeLLMBase, FakeLLMScenario{
		Name:          "reset",
		MaxConcurrent: 1000,
		FixedDelayMs:  0,
		JitterMs:      0,
	})
}

// MetricsSnapshot captures Prometheus metrics at a point in time
type MetricsSnapshot struct {
	Timestamp       time.Time
	ActiveTurns     float64
	Goroutines      float64
	TurnDurationP50 float64
	TurnDurationP95 float64
	TurnDurationP99 float64
	TTFTP50         float64
	TTFTP95         float64
	TTFTP99         float64
	TokensPerSecP50 float64
	TokensPerSecP95 float64
	TokensPerSecP99 float64
	TotalTurns      map[string]float64
	TotalTokens     float64
}

// LoadTest represents a load test run
type LoadTest struct {
	concurrency    int
	duration       time.Duration
	outputDir      string
	wg             sync.WaitGroup
	stopCh         chan struct{}
	results        chan Result
	stats          *Stats
	metricsHistory []MetricsSnapshot
	mu             sync.Mutex
}

// Result represents a single turn result
type Result struct {
	RoomID     string
	TurnNum    int
	StartTime  time.Time
	EndTime    time.Time
	Success    bool
	Error      string
	TokenCount int
}

// Stats aggregates test results
type Stats struct {
	mu           sync.Mutex
	totalTurns   int
	successTurns int
	failedTurns  int
	totalTokens  int64
	latencies    []time.Duration
}

func (s *Stats) Add(result Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalTurns++
	if result.Success {
		s.successTurns++
	} else {
		s.failedTurns++
	}
	s.totalTokens += int64(result.TokenCount)
	if !result.EndTime.IsZero() && !result.StartTime.IsZero() {
		s.latencies = append(s.latencies, result.EndTime.Sub(result.StartTime))
	}
}

func (s *Stats) Report(elapsed time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Printf("\n=== Baseline Load Test Results ===\n")
	fmt.Printf("Test Duration: %v\n", elapsed)
	fmt.Printf("Total Turns: %d\n", s.totalTurns)
	if s.totalTurns > 0 {
		fmt.Printf("Success: %d (%.1f%%)\n", s.successTurns, float64(s.successTurns)/float64(s.totalTurns)*100)
		fmt.Printf("Failed: %d (%.1f%%)\n", s.failedTurns, float64(s.failedTurns)/float64(s.totalTurns)*100)
	}
	fmt.Printf("Throughput: %.2f turns/sec\n", float64(s.totalTurns)/elapsed.Seconds())
	fmt.Printf("Total Tokens: %d\n", s.totalTokens)

	if len(s.latencies) > 0 {
		var totalLatency time.Duration
		for _, l := range s.latencies {
			totalLatency += l
		}
		avgLatency := totalLatency / time.Duration(len(s.latencies))
		fmt.Printf("Avg Latency: %v\n", avgLatency)

		sorted := make([]time.Duration, len(s.latencies))
		copy(sorted, s.latencies)
		for i := 0; i < len(sorted); i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[i] > sorted[j] {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}

		p50 := sorted[len(sorted)*50/100]
		p95 := sorted[len(sorted)*95/100]
		p99 := sorted[len(sorted)*99/100]

		fmt.Printf("P50 Latency: %v\n", p50)
		fmt.Printf("P95 Latency: %v\n", p95)
		fmt.Printf("P99 Latency: %v\n", p99)
	}
}

func main() {
	concurrency := flag.Int("c", 10, "Number of concurrent rooms")
	duration := flag.Duration("d", 60*time.Second, "Test duration")
	outputDir := flag.String("o", "", "Output directory for pprof profiles (default: benchmarks/baseline_<ts>_<c>r)")
	scenarioName := flag.String("scenario", "", "Fake LLM scenario: fast, slow, backpressure (optional)")
	fakeLLMBase := flag.String("fake-llm", "http://localhost:10181", "Fake LLM base URL")
	flag.Parse()

	// Setup output directory
	timestamp := time.Now()
	dir := *outputDir
	if dir == "" {
		suffix := ""
		if *scenarioName != "" {
			suffix = "_" + *scenarioName
		}
		dir = fmt.Sprintf("benchmarks/baseline_%s_%dr%s", timestamp.Format("20060102_150405"), *concurrency, suffix)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Warning: Failed to create output directory: %v\n", err)
		dir = "."
	}

	// Resolve scenario
	var scenario FakeLLMScenario
	if *scenarioName != "" {
		s, err := lookupScenario(*scenarioName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		scenario = s

		fmt.Printf("Configuring Fake LLM with scenario %q...\n", *scenarioName)
		if err := configureFakeLLM(*fakeLLMBase, scenario); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not configure Fake LLM: %v\n", err)
		} else {
			fmt.Println("Fake LLM configured.")
		}
		defer func() {
			fmt.Println("Resetting Fake LLM configuration...")
			if err := resetFakeLLM(*fakeLLMBase); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not reset Fake LLM: %v\n", err)
			}
		}()
	}

	test := &LoadTest{
		concurrency:    *concurrency,
		duration:       *duration,
		outputDir:      dir,
		stopCh:         make(chan struct{}),
		results:        make(chan Result, *concurrency*10),
		stats:          &Stats{},
		metricsHistory: make([]MetricsSnapshot, 0),
	}

	fmt.Printf("=== Prompt Endgame Baseline Load Test ===\n")
	fmt.Printf("Concurrency: %d rooms\n", test.concurrency)
	fmt.Printf("Duration: %v\n", test.duration)
	fmt.Printf("Scenario: %s\n", func() string {
		if *scenarioName == "" {
			return "(none)"
		}
		return *scenarioName
	}())
	fmt.Printf("Output Directory: %s\n", test.outputDir)
	fmt.Printf("Pattern: Loop turns with 1-10s random interval\n\n")

	// Create rooms
	rooms := test.createRooms()
	fmt.Printf("Created %d rooms\n\n", len(rooms))

	// Start result collector
	go test.collectResults()

	// Start metrics collector
	metricsStopCh := make(chan struct{})
	go test.collectMetrics(metricsStopCh)

	// Collect initial pprof
	fmt.Println("Collecting initial pprof...")
	test.collectPprof("initial")

	// Start load generators
	ctx, cancel := context.WithTimeout(context.Background(), test.duration)
	defer cancel()

	for i, roomID := range rooms {
		test.wg.Add(1)
		go test.roomWorker(ctx, i+1, roomID)
	}

	// Wait for test duration
	<-ctx.Done()
	fmt.Println("\nTest duration reached, stopping...")

	// Collect mid-test pprof
	fmt.Println("Collecting mid-test pprof...")
	test.collectPprof("mid")

	// Signal workers to stop
	close(test.stopCh)

	// Wait for graceful shutdown
	done := make(chan struct{})
	go func() {
		test.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Println("All workers stopped gracefully")
	case <-time.After(30 * time.Second):
		fmt.Println("Timeout waiting for workers, forcing exit")
	}

	// Stop metrics collection
	close(metricsStopCh)

	// Collect final pprof
	fmt.Println("Collecting final pprof...")
	test.collectPprof("final")

	// Final report
	test.stats.Report(test.duration)
	test.reportMetrics()

	// Write Markdown report
	s := test.stats
	s.mu.Lock()
	latencies := make([]time.Duration, len(s.latencies))
	copy(latencies, s.latencies)
	totalTurns := s.totalTurns
	successTurns := s.successTurns
	failedTurns := s.failedTurns
	totalTokens := s.totalTokens
	s.mu.Unlock()

	var p50, p95, p99, avg time.Duration
	if len(latencies) > 0 {
		// sort
		for i := 0; i < len(latencies); i++ {
			for j := i + 1; j < len(latencies); j++ {
				if latencies[i] > latencies[j] {
					latencies[i], latencies[j] = latencies[j], latencies[i]
				}
			}
		}
		var sum time.Duration
		for _, l := range latencies {
			sum += l
		}
		avg = sum / time.Duration(len(latencies))
		p50 = latencies[len(latencies)*50/100]
		p95 = latencies[len(latencies)*95/100]
		p99 = latencies[len(latencies)*99/100]
	}

	mdReport := MarkdownReport{
		Title:         "Stage B Baseline Load Test",
		Timestamp:     timestamp,
		Scenario:      *scenarioName,
		Concurrency:   *concurrency,
		Duration:      *duration,
		FakeLLMConfig: scenario,
		OutputDir:     dir,
		Results: TestResults{
			TotalTurns:   totalTurns,
			SuccessTurns: successTurns,
			FailedTurns:  failedTurns,
			TotalTokens:  totalTokens,
			Throughput:   float64(totalTurns) / test.duration.Seconds(),
			AvgLatency:   avg,
			P50Latency:   p50,
			P95Latency:   p95,
			P99Latency:   p99,
		},
	}

	if path, err := mdReport.WriteFile(dir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write Markdown report: %v\n", err)
	} else {
		fmt.Printf("Markdown report: %s\n", path)
	}

	// Cleanup
	fmt.Println("\nCleaning up...")
	test.cleanup(rooms)
	fmt.Printf("\nDone! Results saved to: %s\n", test.outputDir)
}

func (t *LoadTest) createRooms() []string {
	rooms := make([]string, 0, t.concurrency)
	for i := 0; i < t.concurrency; i++ {
		resp, err := http.Post(baseURL+"/rooms", "application/json", nil)
		if err != nil {
			fmt.Printf("Failed to create room %d: %v\n", i+1, err)
			continue
		}

		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			fmt.Printf("Failed to create room %d: HTTP %d, body: %s\n", i+1, resp.StatusCode, string(body))
			continue
		}

		var result struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			fmt.Printf("Failed to decode room %d: %v\n", i+1, err)
			continue
		}
		resp.Body.Close()

		rooms = append(rooms, result.ID)
		fmt.Printf("  Room %d: %s\n", i+1, result.ID)
	}
	return rooms
}

func (t *LoadTest) roomWorker(ctx context.Context, workerNum int, roomID string) {
	defer t.wg.Done()

	turnNum := 0
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("[Worker %d] Context cancelled, finishing...\n", workerNum)
			return
		case <-t.stopCh:
			fmt.Printf("[Worker %d] Stop signal received, exiting\n", workerNum)
			return
		default:
		}

		turnNum++
		result := t.executeTurn(roomID, turnNum)
		t.results <- result

		waitTime := time.Duration(rand.Intn(10)+1) * time.Second
		select {
		case <-ctx.Done():
			return
		case <-t.stopCh:
			return
		case <-time.After(waitTime):
		}
	}
}

func (t *LoadTest) executeTurn(roomID string, turnNum int) Result {
	result := Result{
		RoomID:  roomID,
		TurnNum: turnNum,
	}

	select {
	case <-t.stopCh:
		return result
	default:
	}

	result.StartTime = time.Now()
	fmt.Printf("[Room %s] Starting turn %d\n", roomID, turnNum)

	body, _ := json.Marshal(map[string]string{
		"user_input": fmt.Sprintf("Test message %d from room %s", turnNum, roomID),
	})

	// Submit answer with retry on room busy
	var resp *http.Response
	var err error
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		resp, err = http.Post(
			baseURL+"/rooms/"+roomID+"/answer",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			result.Error = err.Error()
			result.EndTime = time.Now()
			return result
		}

		// Check if room is busy (409 Conflict)
		if resp.StatusCode == http.StatusConflict {
			resp.Body.Close()
			fmt.Printf("[Room %s] Turn %d: Room busy, retrying in 1s... (%d/%d)\n", roomID, turnNum, i+1, maxRetries)
			time.Sleep(1 * time.Second)
			continue
		}

		// Other error
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
			result.EndTime = time.Now()
			return result
		}

		// Success
		resp.Body.Close()
		break
	}

	if resp.StatusCode == http.StatusConflict {
		result.Error = "room busy after max retries"
		result.EndTime = time.Now()
		return result
	}

	// Stream events and wait for turn completion
	streamResp, err := http.Get(baseURL + "/rooms/" + roomID + "/events")
	if err != nil {
		result.Error = err.Error()
		result.EndTime = time.Now()
		return result
	}
	defer streamResp.Body.Close()

	buf := make([]byte, 4096)
	var turnCompleted bool
streamLoop:
	for {
		select {
		case <-t.stopCh:
			break streamLoop
		default:
		}

		n, err := streamResp.Body.Read(buf)
		if err == io.EOF {
			break streamLoop
		}
		if err != nil {
			break streamLoop
		}

		result.TokenCount += n

		// Check for turn_completed event in the data
		data := string(buf[:n])
		if strings.Contains(data, `"type":"turn_completed"`) || strings.Contains(data, `event: turn_completed`) {
			turnCompleted = true
			fmt.Printf("[Room %s] Turn %d: Received turn_completed event\n", roomID, turnNum)
			// Continue reading to properly close the connection
		}
	}

	result.Success = true
	result.EndTime = time.Now()

	if !turnCompleted {
		fmt.Printf("[Room %s] Turn %d: Warning - did not receive turn_completed event\n", roomID, turnNum)
	}

	return result
}

func (t *LoadTest) collectResults() {
	for result := range t.results {
		t.stats.Add(result)
	}
}

func (t *LoadTest) collectMetrics(stopCh chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			snapshot := t.fetchMetrics()
			t.mu.Lock()
			t.metricsHistory = append(t.metricsHistory, snapshot)
			t.mu.Unlock()
		}
	}
}

func (t *LoadTest) fetchMetrics() MetricsSnapshot {
	snapshot := MetricsSnapshot{
		Timestamp:  time.Now(),
		TotalTurns: make(map[string]float64),
	}

	resp, err := http.Get(baseURL + "/metrics")
	if err != nil {
		return snapshot
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	metrics := string(body)

	// Parse metrics
	snapshot.ActiveTurns = parseGauge(metrics, "prompt_endgame_active_turns")
	snapshot.Goroutines = parseGauge(metrics, "prompt_endgame_goroutines")
	snapshot.TotalTokens = parseCounter(metrics, "prompt_endgame_tokens_total")

	// Parse histograms
	snapshot.TurnDurationP50 = parseHistogramPercentile(metrics, "prompt_endgame_turn_duration_seconds", 0.5)
	snapshot.TurnDurationP95 = parseHistogramPercentile(metrics, "prompt_endgame_turn_duration_seconds", 0.95)
	snapshot.TurnDurationP99 = parseHistogramPercentile(metrics, "prompt_endgame_turn_duration_seconds", 0.99)

	snapshot.TTFTP50 = parseHistogramPercentile(metrics, "prompt_endgame_ttft_seconds", 0.5)
	snapshot.TTFTP95 = parseHistogramPercentile(metrics, "prompt_endgame_ttft_seconds", 0.95)
	snapshot.TTFTP99 = parseHistogramPercentile(metrics, "prompt_endgame_ttft_seconds", 0.99)

	snapshot.TokensPerSecP50 = parseHistogramPercentile(metrics, "prompt_endgame_tokens_per_second", 0.5)
	snapshot.TokensPerSecP95 = parseHistogramPercentile(metrics, "prompt_endgame_tokens_per_second", 0.95)
	snapshot.TokensPerSecP99 = parseHistogramPercentile(metrics, "prompt_endgame_tokens_per_second", 0.99)

	// Parse turn totals by status
	re := regexp.MustCompile(`prompt_endgame_turn_total\{status="([^"]+)"\}\s+(\d+(?:\.\d+)?)`)
	matches := re.FindAllStringSubmatch(metrics, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			status := match[1]
			value, _ := strconv.ParseFloat(match[2], 64)
			snapshot.TotalTurns[status] = value
		}
	}

	return snapshot
}

func (t *LoadTest) collectPprof(phase string) {
	profiles := []string{"goroutine", "heap", "profile"}
	for _, profile := range profiles {
		url := fmt.Sprintf("%s/debug/pprof/%s", baseURL, profile)
		if profile == "profile" {
			url += "?seconds=5"
		}

		resp, err := http.Get(url)
		if err != nil {
			fmt.Printf("  Failed to collect %s pprof: %v\n", profile, err)
			continue
		}
		defer resp.Body.Close()

		filename := filepath.Join(t.outputDir, fmt.Sprintf("%s_%s.prof", phase, profile))
		f, err := os.Create(filename)
		if err != nil {
			fmt.Printf("  Failed to create %s: %v\n", filename, err)
			continue
		}

		_, err = io.Copy(f, resp.Body)
		f.Close()
		if err != nil {
			fmt.Printf("  Failed to write %s: %v\n", filename, err)
		} else {
			fmt.Printf("  Saved: %s\n", filename)
		}
	}
}

func (t *LoadTest) reportMetrics() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.metricsHistory) == 0 {
		return
	}

	fmt.Println("\n=== Prometheus Metrics Summary ===")

	// Latest metrics
	latest := t.metricsHistory[len(t.metricsHistory)-1]
	fmt.Printf("\nFinal State:\n")
	fmt.Printf("  Active Turns: %.0f\n", latest.ActiveTurns)
	fmt.Printf("  Goroutines: %.0f\n", latest.Goroutines)
	fmt.Printf("  Total Tokens: %.0f\n", latest.TotalTokens)

	fmt.Printf("\nTurn Duration (from Prometheus):\n")
	fmt.Printf("  P50: %.3fs\n", latest.TurnDurationP50)
	fmt.Printf("  P95: %.3fs\n", latest.TurnDurationP95)
	fmt.Printf("  P99: %.3fs\n", latest.TurnDurationP99)

	fmt.Printf("\nTime to First Token (TTFT):\n")
	fmt.Printf("  P50: %.3fs\n", latest.TTFTP50)
	fmt.Printf("  P95: %.3fs\n", latest.TTFTP95)
	fmt.Printf("  P99: %.3fs\n", latest.TTFTP99)

	fmt.Printf("\nTokens Per Second:\n")
	fmt.Printf("  P50: %.1f\n", latest.TokensPerSecP50)
	fmt.Printf("  P95: %.1f\n", latest.TokensPerSecP95)
	fmt.Printf("  P99: %.1f\n", latest.TokensPerSecP99)

	fmt.Printf("\nTurns by Status:\n")
	for status, count := range latest.TotalTurns {
		fmt.Printf("  %s: %.0f\n", status, count)
	}

	// Save metrics history to file
	metricsFile := filepath.Join(t.outputDir, "metrics_history.json")
	f, err := os.Create(metricsFile)
	if err != nil {
		fmt.Printf("Warning: Failed to save metrics history: %v\n", err)
		return
	}
	defer f.Close()

	type jsonSnapshot struct {
		Timestamp       string             `json:"timestamp"`
		ActiveTurns     float64            `json:"active_turns"`
		Goroutines      float64            `json:"goroutines"`
		TurnDurationP95 float64            `json:"turn_duration_p95"`
		TTFT95          float64            `json:"ttft_p95"`
		TokensPerSecP95 float64            `json:"tokens_per_sec_p95"`
		TotalTurns      map[string]float64 `json:"total_turns"`
	}

	jsonHistory := make([]jsonSnapshot, len(t.metricsHistory))
	for i, m := range t.metricsHistory {
		jsonHistory[i] = jsonSnapshot{
			Timestamp:       m.Timestamp.Format(time.RFC3339),
			ActiveTurns:     m.ActiveTurns,
			Goroutines:      m.Goroutines,
			TurnDurationP95: m.TurnDurationP95,
			TTFT95:          m.TTFTP95,
			TokensPerSecP95: m.TokensPerSecP95,
			TotalTurns:      m.TotalTurns,
		}
	}

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(jsonHistory); err != nil {
		fmt.Printf("Warning: Failed to encode metrics history: %v\n", err)
	} else {
		fmt.Printf("\nMetrics history saved to: %s\n", metricsFile)
	}
}

func (t *LoadTest) cleanup(roomIDs []string) {
	var wg sync.WaitGroup
	for _, roomID := range roomIDs {
		wg.Add(1)
		go func(rid string) {
			defer wg.Done()
			http.Post(baseURL+"/rooms/"+rid+"/cancel", "application/json", nil)
		}(roomID)
	}
	wg.Wait()
}

// Helper functions for parsing Prometheus metrics

func parseGauge(metrics, name string) float64 {
	re := regexp.MustCompile(name + `\s+(\d+(?:\.\d+)?)`)
	match := re.FindStringSubmatch(metrics)
	if len(match) >= 2 {
		v, _ := strconv.ParseFloat(match[1], 64)
		return v
	}
	return 0
}

func parseCounter(metrics, name string) float64 {
	return parseGauge(metrics, name)
}

func parseHistogramPercentile(metrics, name string, percentile float64) float64 {
	// Find the bucket lines for this histogram
	prefix := name + "_bucket"
	lines := strings.Split(metrics, "\n")

	totalCount := parseHistogramCount(metrics, name)

	if totalCount == 0 {
		return 0
	}

	targetCount := totalCount * percentile

	bucketRe := regexp.MustCompile(prefix + `{le="([^"]+)"}\s+(\d+(?:\.\d+)?)`)

	var buckets []struct {
		le    float64
		count float64
	}

	for _, line := range lines {
		if strings.Contains(line, prefix) {
			match := bucketRe.FindStringSubmatch(line)
			if len(match) >= 3 {
				le := match[1]
				count, _ := strconv.ParseFloat(match[2], 64)

				var leVal float64
				if le == "+Inf" {
					leVal = 1e308
				} else {
					leVal, _ = strconv.ParseFloat(le, 64)
				}

				buckets = append(buckets, struct {
					le    float64
					count float64
				}{leVal, count})
			}
		}
	}

	// Simple interpolation
	for _, bucket := range buckets {
		if bucket.count >= targetCount {
			return bucket.le
		}
	}

	return 0
}

func parseHistogramCount(metrics, name string) float64 {
	re := regexp.MustCompile(name + `_count\s+(\d+(?:\.\d+)?)`)
	match := re.FindStringSubmatch(metrics)
	if len(match) >= 2 {
		v, _ := strconv.ParseFloat(match[1], 64)
		return v
	}
	return 0
}

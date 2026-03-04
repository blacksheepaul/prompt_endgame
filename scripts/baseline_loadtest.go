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
	outputDir := flag.String("o", "", "Output directory for pprof profiles (default: baseline_<timestamp>)")
	flag.Parse()

	// Setup output directory
	timestamp := time.Now().Format("20060102_150405")
	dir := *outputDir
	if dir == "" {
		dir = fmt.Sprintf("benchmarks/baseline_%s_%dr", timestamp, *concurrency)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Warning: Failed to create output directory: %v\n", err)
		dir = "."
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

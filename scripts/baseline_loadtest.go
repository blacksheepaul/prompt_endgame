package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
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

const (
	defaultBaseURL = "http://localhost:10180"
	floatTolerance = 1e-6
)

// WorkloadProfile describes load test execution parameters.
type WorkloadProfile struct {
	Concurrency int `json:"concurrency"`
	DurationSec int `json:"duration_sec"`
	WarmupSec   int `json:"warmup_sec"`
}

// MockLLMAdminConfig mirrors mockllm GET /admin/config response.
type MockLLMAdminConfig struct {
	MaxConcurrent       int     `json:"max_concurrent"`
	MaxQueueDepth       int     `json:"max_queue_depth"`
	QueueTimeoutSec     float64 `json:"queue_timeout_sec"`
	TokensPerSecond     float64 `json:"tokens_per_second"`
	FirstTokenDelayMs   int     `json:"first_token_delay_ms"`
	FixedDelayMs        int     `json:"fixed_delay_ms"`
	JitterMs            int     `json:"jitter_ms"`
	TPSVariance         float64 `json:"tps_variance"`
	LoadCurveCenter     float64 `json:"load_curve_center"`
	LoadCurveSteepness  float64 `json:"load_curve_steepness"`
	MinEfficiency       float64 `json:"min_efficiency"`
	QueuePenaltyEnabled bool    `json:"queue_penalty_enabled"`
	QueuePenaltyFactor  float64 `json:"queue_penalty_factor"`
}

// MockLLMProfile declares expected deployment/config metadata for baseline runs.
type MockLLMProfile struct {
	Version        string             `json:"version"`
	Deployment     string             `json:"deployment"`
	Container      string             `json:"container"`
	AdminBaseURL   string             `json:"admin_base_url"`
	ExpectedConfig MockLLMAdminConfig `json:"expected_config"`
}

// BaselineProfile is the JSON profile contract for Stage B baseline.
type BaselineProfile struct {
	SchemaVersion string          `json:"schema_version"`
	ProfileID     string          `json:"profile_id"`
	Description   string          `json:"description"`
	Baseline      bool            `json:"baseline"`
	Workload      WorkloadProfile `json:"workload"`
	MockLLM       MockLLMProfile  `json:"mockllm"`
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
	ProfileID     string
	Concurrency   int
	Duration      time.Duration
	Warmup        time.Duration
	ProfilePath   string
	ProfileSHA256 string
	MockLLM       MockLLMProfile
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
| Profile ID | %s |
| Concurrency | %d rooms |
| Duration | %s |
| Warmup | %s |
| Profile Path | %s |
| Profile SHA256 | %s |

## MockLLM Metadata

| Parameter | Value |
|-----------|-------|
| Version | %s |
| Deployment | %s |
| Container | %s |
| Admin Base URL | %s |

## MockLLM Expected Config

| Parameter | Value |
|-----------|-------|
| Max Concurrent | %d |
| Max Queue Depth | %d |
| Queue Timeout | %.3fs |
| Tokens Per Second | %.3f |
| First Token Delay | %dms |
| Fixed Delay | %dms |
| Jitter | %dms |
| TPS Variance | %.3f |
| Load Curve Center | %.3f |
| Load Curve Steepness | %.3f |
| Min Efficiency | %.3f |
| Queue Penalty Enabled | %t |
| Queue Penalty Factor | %.3f |

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
		r.ProfileID,
		r.Concurrency,
		r.Duration.String(),
		r.Warmup.String(),
		r.ProfilePath,
		r.ProfileSHA256,
		r.MockLLM.Version,
		r.MockLLM.Deployment,
		r.MockLLM.Container,
		r.MockLLM.AdminBaseURL,
		r.MockLLM.ExpectedConfig.MaxConcurrent,
		r.MockLLM.ExpectedConfig.MaxQueueDepth,
		r.MockLLM.ExpectedConfig.QueueTimeoutSec,
		r.MockLLM.ExpectedConfig.TokensPerSecond,
		r.MockLLM.ExpectedConfig.FirstTokenDelayMs,
		r.MockLLM.ExpectedConfig.FixedDelayMs,
		r.MockLLM.ExpectedConfig.JitterMs,
		r.MockLLM.ExpectedConfig.TPSVariance,
		r.MockLLM.ExpectedConfig.LoadCurveCenter,
		r.MockLLM.ExpectedConfig.LoadCurveSteepness,
		r.MockLLM.ExpectedConfig.MinEfficiency,
		r.MockLLM.ExpectedConfig.QueuePenaltyEnabled,
		r.MockLLM.ExpectedConfig.QueuePenaltyFactor,
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
		r.ProfileID,
		r.Timestamp.Format("20060102_150405"),
	)
	path := filepath.Join(outputDir, filename)
	if err := os.WriteFile(path, []byte(r.Generate()), 0644); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}
	return path, nil
}

func loadProfile(path string) (*BaselineProfile, string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read profile: %w", err)
	}

	var profile BaselineProfile
	if err := json.Unmarshal(content, &profile); err != nil {
		return nil, "", fmt.Errorf("parse profile json: %w", err)
	}

	if err := validateProfile(profile); err != nil {
		return nil, "", err
	}

	hash := sha256.Sum256(content)
	return &profile, fmt.Sprintf("%x", hash), nil
}

func validateProfile(p BaselineProfile) error {
	if p.SchemaVersion == "" {
		return fmt.Errorf("invalid profile: schema_version is required")
	}
	if p.ProfileID == "" {
		return fmt.Errorf("invalid profile: profile_id is required")
	}
	if !p.Baseline {
		return fmt.Errorf("invalid profile: baseline must be true")
	}
	if p.Workload.Concurrency <= 0 {
		return fmt.Errorf("invalid profile: workload.concurrency must be > 0")
	}
	if p.Workload.DurationSec <= 0 {
		return fmt.Errorf("invalid profile: workload.duration_sec must be > 0")
	}
	if p.Workload.WarmupSec < 0 {
		return fmt.Errorf("invalid profile: workload.warmup_sec must be >= 0")
	}
	if p.MockLLM.AdminBaseURL == "" {
		return fmt.Errorf("invalid profile: mockllm.admin_base_url is required")
	}
	if p.MockLLM.Version == "" {
		return fmt.Errorf("invalid profile: mockllm.version is required")
	}
	return nil
}

func fetchMockLLMConfig(mockAdminBaseURL string) (MockLLMAdminConfig, error) {
	resp, err := http.Get(strings.TrimRight(mockAdminBaseURL, "/") + "/admin/config")
	if err != nil {
		return MockLLMAdminConfig{}, fmt.Errorf("GET /admin/config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return MockLLMAdminConfig{}, fmt.Errorf("GET /admin/config: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var cfg MockLLMAdminConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return MockLLMAdminConfig{}, fmt.Errorf("decode /admin/config response: %w", err)
	}
	return cfg, nil
}

func compareMockConfig(expected, actual MockLLMAdminConfig) []string {
	mismatches := make([]string, 0)
	checkInt := func(name string, exp, got int) {
		if exp != got {
			mismatches = append(mismatches, fmt.Sprintf("%s expected=%d actual=%d", name, exp, got))
		}
	}
	checkBool := func(name string, exp, got bool) {
		if exp != got {
			mismatches = append(mismatches, fmt.Sprintf("%s expected=%t actual=%t", name, exp, got))
		}
	}
	checkFloat := func(name string, exp, got float64) {
		if math.Abs(exp-got) > floatTolerance {
			mismatches = append(mismatches, fmt.Sprintf("%s expected=%.6f actual=%.6f", name, exp, got))
		}
	}

	checkInt("max_concurrent", expected.MaxConcurrent, actual.MaxConcurrent)
	checkInt("max_queue_depth", expected.MaxQueueDepth, actual.MaxQueueDepth)
	checkFloat("queue_timeout_sec", expected.QueueTimeoutSec, actual.QueueTimeoutSec)
	checkFloat("tokens_per_second", expected.TokensPerSecond, actual.TokensPerSecond)
	checkInt("first_token_delay_ms", expected.FirstTokenDelayMs, actual.FirstTokenDelayMs)
	checkInt("fixed_delay_ms", expected.FixedDelayMs, actual.FixedDelayMs)
	checkInt("jitter_ms", expected.JitterMs, actual.JitterMs)
	checkFloat("tps_variance", expected.TPSVariance, actual.TPSVariance)
	checkFloat("load_curve_center", expected.LoadCurveCenter, actual.LoadCurveCenter)
	checkFloat("load_curve_steepness", expected.LoadCurveSteepness, actual.LoadCurveSteepness)
	checkFloat("min_efficiency", expected.MinEfficiency, actual.MinEfficiency)
	checkBool("queue_penalty_enabled", expected.QueuePenaltyEnabled, actual.QueuePenaltyEnabled)
	checkFloat("queue_penalty_factor", expected.QueuePenaltyFactor, actual.QueuePenaltyFactor)

	return mismatches
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
	baseURL        string
	concurrency    int
	duration       time.Duration
	warmup         time.Duration
	profilePath    string
	profileSHA256  string
	profile        BaselineProfile
	outputDir      string
	wg             sync.WaitGroup
	stopCh         chan struct{}
	results        chan Result
	stats          *Stats
	metricsHistory []MetricsSnapshot
	mu             sync.Mutex
	roomOffsets    map[string]int64
}

// Result represents a single turn result
type Result struct {
	RoomID     string
	TurnNum    int
	TurnID     string
	StartTime  time.Time
	EndTime    time.Time
	Success    bool
	Error      string
	TokenCount int
}

type answerSubmitResponse struct {
	TurnID string `json:"turn_id"`
}

type streamEvent struct {
	Type    string `json:"type"`
	TurnID  string `json:"turn_id"`
	Offset  int64  `json:"offset"`
	Payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"payload"`
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

func (s *Stats) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalTurns = 0
	s.successTurns = 0
	s.failedTurns = 0
	s.totalTokens = 0
	s.latencies = nil
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
	profilePath := flag.String("profile", "", "Path to baseline profile JSON (required)")
	baseURL := flag.String("base-url", defaultBaseURL, "Prompt Endgame API base URL")
	outputDir := flag.String("o", "", "Output directory for pprof profiles (default: benchmarks/baseline_<ts>_<profile>)")
	flag.Parse()
	if *profilePath == "" {
		fmt.Fprintln(os.Stderr, "Error: --profile is required")
		os.Exit(1)
	}

	profile, profileSHA, err := loadProfile(*profilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	actualMockCfg, err := fetchMockLLMConfig(profile.MockLLM.AdminBaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to fetch mockllm config: %v\n", err)
		os.Exit(1)
	}
	mismatches := compareMockConfig(profile.MockLLM.ExpectedConfig, actualMockCfg)
	if len(mismatches) > 0 {
		fmt.Fprintln(os.Stderr, "Error: mockllm config does not match profile expected_config")
		for _, m := range mismatches {
			fmt.Fprintf(os.Stderr, "  - %s\n", m)
		}
		os.Exit(1)
	}

	// Setup output directory
	timestamp := time.Now()
	dir := *outputDir
	if dir == "" {
		dir = fmt.Sprintf("benchmarks/baseline_%s_%s", timestamp.Format("20060102_150405"), profile.ProfileID)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Warning: Failed to create output directory: %v\n", err)
		dir = "."
	}

	absProfilePath, err := filepath.Abs(*profilePath)
	if err != nil {
		absProfilePath = *profilePath
	}

	concurrency := profile.Workload.Concurrency
	duration := time.Duration(profile.Workload.DurationSec) * time.Second
	warmup := time.Duration(profile.Workload.WarmupSec) * time.Second

	test := &LoadTest{
		baseURL:        strings.TrimRight(*baseURL, "/"),
		concurrency:    concurrency,
		duration:       duration,
		warmup:         warmup,
		profilePath:    absProfilePath,
		profileSHA256:  profileSHA,
		profile:        *profile,
		outputDir:      dir,
		stopCh:         make(chan struct{}),
		results:        make(chan Result, concurrency*10),
		stats:          &Stats{},
		metricsHistory: make([]MetricsSnapshot, 0),
		roomOffsets:    make(map[string]int64),
	}

	fmt.Printf("=== Prompt Endgame Baseline Load Test ===\n")
	fmt.Printf("Profile ID: %s\n", profile.ProfileID)
	fmt.Printf("Concurrency: %d rooms\n", test.concurrency)
	fmt.Printf("Duration: %v\n", test.duration)
	fmt.Printf("Warmup: %v\n", test.warmup)
	fmt.Printf("Base URL: %s\n", test.baseURL)
	fmt.Printf("Profile Path: %s\n", absProfilePath)
	fmt.Printf("Profile SHA256: %s\n", profileSHA)
	fmt.Printf("MockLLM config check: OK (%s/admin/config)\n", profile.MockLLM.AdminBaseURL)
	fmt.Printf("Output Directory: %s\n", test.outputDir)
	fmt.Printf("Pattern: Loop turns with 1-10s random interval\n\n")

	// Create rooms
	rooms := test.createRooms()
	fmt.Printf("Created %d rooms\n\n", len(rooms))

	// Start result collector
	collectorDone := make(chan struct{})
	go func() {
		test.collectResults()
		close(collectorDone)
	}()

	// Start metrics collector
	metricsStopCh := make(chan struct{})
	go test.collectMetrics(metricsStopCh)

	// Collect initial pprof
	fmt.Println("Collecting initial pprof...")
	test.collectPprof("initial")

	// Start load generators
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i, roomID := range rooms {
		test.wg.Add(1)
		go test.roomWorker(ctx, i+1, roomID)
	}

	if test.warmup > 0 {
		fmt.Printf("Warmup phase: %v (results discarded)\n", test.warmup)
		time.Sleep(test.warmup)
		test.stats.Reset()
		test.mu.Lock()
		test.metricsHistory = test.metricsHistory[:0]
		test.mu.Unlock()
		fmt.Println("Warmup completed. Measurement phase started.")
	}

	// Wait for measurement duration
	time.Sleep(test.duration)
	fmt.Println("\nTest duration reached, stopping...")

	// Collect mid-test pprof
	fmt.Println("Collecting mid-test pprof...")
	test.collectPprof("mid")

	// Signal workers to stop
	close(test.stopCh)
	cancel()

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
	close(test.results)
	<-collectorDone

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
		ProfileID:     profile.ProfileID,
		Concurrency:   test.concurrency,
		Duration:      test.duration,
		Warmup:        test.warmup,
		ProfilePath:   absProfilePath,
		ProfileSHA256: profileSHA,
		MockLLM:       profile.MockLLM,
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
		resp, err := http.Post(t.baseURL+"/rooms", "application/json", nil)
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

func (t *LoadTest) nextOffset(roomID string) int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	offset, ok := t.roomOffsets[roomID]
	if !ok {
		return 0
	}
	return offset + 1
}

func (t *LoadTest) commitOffset(roomID string, offset int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if curr, ok := t.roomOffsets[roomID]; !ok || offset > curr {
		t.roomOffsets[roomID] = offset
	}
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
		result := t.executeTurn(ctx, roomID, turnNum)
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

func (t *LoadTest) executeTurn(ctx context.Context, roomID string, turnNum int) Result {
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
		req, reqErr := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			t.baseURL+"/rooms/"+roomID+"/answer",
			bytes.NewReader(body),
		)
		if reqErr != nil {
			result.Error = reqErr.Error()
			result.EndTime = time.Now()
			return result
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err = http.DefaultClient.Do(req)
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
		if resp.StatusCode != http.StatusAccepted {
			errBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(errBody))
			result.EndTime = time.Now()
			return result
		}

		var submitResp answerSubmitResponse
		if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
			resp.Body.Close()
			result.Error = fmt.Sprintf("decode answer response: %v", err)
			result.EndTime = time.Now()
			return result
		}
		resp.Body.Close()
		result.TurnID = submitResp.TurnID
		break
	}

	if resp.StatusCode == http.StatusConflict {
		result.Error = "room busy after max retries"
		result.EndTime = time.Now()
		return result
	}

	if result.TurnID == "" {
		result.Error = "empty turn_id in answer response"
		result.EndTime = time.Now()
		return result
	}

	// Stream events and wait for turn completion
	streamReq, reqErr := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/rooms/%s/events?fromOffset=%d", t.baseURL, roomID, t.nextOffset(roomID)),
		nil,
	)
	if reqErr != nil {
		result.Error = reqErr.Error()
		result.EndTime = time.Now()
		return result
	}
	streamResp, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		result.Error = err.Error()
		result.EndTime = time.Now()
		return result
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(streamResp.Body)
		result.Error = fmt.Sprintf("events HTTP %d: %s", streamResp.StatusCode, string(errBody))
		result.EndTime = time.Now()
		return result
	}

	reader := bufio.NewReader(streamResp.Body)
	var (
		lineEventType string
		lineData      strings.Builder
		lineEventID   int64 = -1
	)

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			result.Error = fmt.Sprintf("read events stream: %v", readErr)
			result.EndTime = time.Now()
			return result
		}

		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(trimmed, "id: "):
			if id, parseErr := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(trimmed, "id: ")), 10, 64); parseErr == nil {
				lineEventID = id
			}
		case strings.HasPrefix(trimmed, "event: "):
			lineEventType = strings.TrimSpace(strings.TrimPrefix(trimmed, "event: "))
		case strings.HasPrefix(trimmed, "data: "):
			if lineData.Len() > 0 {
				lineData.WriteByte('\n')
			}
			lineData.WriteString(strings.TrimPrefix(trimmed, "data: "))
		case trimmed == "":
			if lineData.Len() == 0 {
				lineEventType = ""
				lineEventID = -1
				continue
			}

			var evt streamEvent
			if err := json.Unmarshal([]byte(lineData.String()), &evt); err != nil {
				lineEventType = ""
				lineData.Reset()
				lineEventID = -1
				continue
			}
			if evt.Type == "" {
				evt.Type = lineEventType
			}
			if evt.Offset == 0 && lineEventID >= 0 {
				evt.Offset = lineEventID
			}
			if evt.Offset >= 0 {
				t.commitOffset(roomID, evt.Offset)
			}

			if evt.TurnID != result.TurnID {
				lineEventType = ""
				lineData.Reset()
				lineEventID = -1
				continue
			}

			switch evt.Type {
			case "token_received":
				result.TokenCount++
			case "error":
				if evt.Payload.Message != "" {
					result.Error = evt.Payload.Message
				} else {
					result.Error = "received error event"
				}
				result.EndTime = time.Now()
				return result
			case "turn_completed":
				result.Success = true
				result.EndTime = time.Now()
				fmt.Printf("[Room %s] Turn %d: Received turn_completed event\n", roomID, turnNum)
				return result
			}

			lineEventType = ""
			lineData.Reset()
			lineEventID = -1
		}
	}

	if result.Error == "" {
		result.Error = "stream ended before turn_completed"
	}
	result.EndTime = time.Now()
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

	resp, err := http.Get(t.baseURL + "/metrics")
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
		url := fmt.Sprintf("%s/debug/pprof/%s", t.baseURL, profile)
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
			http.Post(t.baseURL+"/rooms/"+rid+"/cancel", "application/json", nil)
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

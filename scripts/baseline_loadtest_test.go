package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFakeLLMScenario_PredefinedScenarios(t *testing.T) {
	// Verify all required scenarios are defined
	requiredScenarios := []string{"fast", "slow", "backpressure"}
	for _, name := range requiredScenarios {
		t.Run(name, func(t *testing.T) {
			s, ok := predefinedScenarios[name]
			if !ok {
				t.Errorf("predefinedScenarios missing required scenario %q", name)
				return
			}
			if s.Name != name {
				t.Errorf("scenario Name field: got %q, want %q", s.Name, name)
			}
		})
	}
}

func TestFakeLLMScenario_FastHasLowDelay(t *testing.T) {
	s := predefinedScenarios["fast"]
	if s.FixedDelayMs >= 100 {
		t.Errorf("fast scenario FixedDelayMs should be < 100ms, got %d", s.FixedDelayMs)
	}
}

func TestFakeLLMScenario_SlowHasHighDelay(t *testing.T) {
	s := predefinedScenarios["slow"]
	if s.FixedDelayMs < 100 {
		t.Errorf("slow scenario FixedDelayMs should be >= 100ms, got %d", s.FixedDelayMs)
	}
}

func TestFakeLLMScenario_BackpressureHasSlowdown(t *testing.T) {
	s := predefinedScenarios["backpressure"]
	if s.SlowdownQPSThreshold <= 0 {
		t.Errorf("backpressure scenario should have SlowdownQPSThreshold > 0, got %d", s.SlowdownQPSThreshold)
	}
	if s.SlowdownFactor <= 0 || s.SlowdownFactor >= 1 {
		t.Errorf("backpressure scenario SlowdownFactor should be in (0,1), got %f", s.SlowdownFactor)
	}
}

func TestMarkdownReport_ContainsDynamicValues(t *testing.T) {
	concurrency := 42
	duration := 45 * time.Second
	scenario := "fast"
	totalTurns := 137
	throughput := 7.89

	report := MarkdownReport{
		Title:       "Stage B Baseline Test",
		Timestamp:   time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC),
		Scenario:    scenario,
		Concurrency: concurrency,
		Duration:    duration,
		FakeLLMConfig: FakeLLMScenario{
			Name:          "fast",
			MaxConcurrent: 200,
			FixedDelayMs:  10,
			JitterMs:      5,
		},
		Results: TestResults{
			TotalTurns:   totalTurns,
			SuccessTurns: 130,
			FailedTurns:  7,
			TotalTokens:  6850,
			Throughput:   throughput,
			AvgLatency:   1234 * time.Millisecond,
			P50Latency:   1100 * time.Millisecond,
			P95Latency:   2300 * time.Millisecond,
			P99Latency:   3100 * time.Millisecond,
		},
	}

	content := report.Generate()

	// These values are dynamic — they come from struct fields, not hardcoded strings
	checks := []struct {
		label string
		value string
	}{
		{"concurrency", fmt.Sprintf("%d", concurrency)},
		{"duration", duration.String()},
		{"scenario", scenario},
		{"total turns", fmt.Sprintf("%d", totalTurns)},
		{"throughput", fmt.Sprintf("%.2f", throughput)},
		{"timestamp", "2026-03-06"},
		{"title", "Stage B Baseline Test"},
		{"total tokens", "6850"},
		{"P50 latency", "1.1s"},
		{"P95 latency", "2.3s"},
	}

	for _, c := range checks {
		if !strings.Contains(content, c.value) {
			t.Errorf("Generate() output should contain %s=%q\nGot:\n%s", c.label, c.value, content)
		}
	}
}

func TestMarkdownReport_RequiredSections(t *testing.T) {
	report := MarkdownReport{
		Title:    "Test Report",
		Scenario: "slow",
		Duration: 30 * time.Second,
	}

	content := report.Generate()

	sections := []string{
		"## Test Configuration",
		"## Fake LLM Configuration",
		"## Results Summary",
		"## Latency Distribution",
		"## Key Metrics",
	}

	for _, section := range sections {
		if !strings.Contains(content, section) {
			t.Errorf("Generate() missing section %q", section)
		}
	}
}

func TestMarkdownReport_SuccessRateCalculation(t *testing.T) {
	report := MarkdownReport{
		Title:    "Test",
		Scenario: "fast",
		Results: TestResults{
			TotalTurns:   200,
			SuccessTurns: 180,
			FailedTurns:  20,
		},
	}

	content := report.Generate()

	// 180/200 = 90.0%, 20/200 = 10.0%
	if !strings.Contains(content, "90.0%") {
		t.Errorf("Generate() should contain success rate 90.0%%\nGot:\n%s", content)
	}
	if !strings.Contains(content, "10.0%") {
		t.Errorf("Generate() should contain fail rate 10.0%%\nGot:\n%s", content)
	}
}

func TestLookupScenario_KnownNames(t *testing.T) {
	for _, name := range []string{"fast", "slow", "backpressure"} {
		s, err := lookupScenario(name)
		if err != nil {
			t.Errorf("lookupScenario(%q) returned unexpected error: %v", name, err)
		}
		if s.Name != name {
			t.Errorf("lookupScenario(%q) Name: got %q, want %q", name, s.Name, name)
		}
	}
}

func TestLookupScenario_UnknownName(t *testing.T) {
	_, err := lookupScenario("nonexistent")
	if err == nil {
		t.Error("lookupScenario(\"nonexistent\") should return an error, got nil")
	}
}

func TestFakeLLMConfigPayload_MatchesScenario(t *testing.T) {
	s := FakeLLMScenario{
		Name:                 "test",
		MaxConcurrent:        77,
		FixedDelayMs:         250,
		JitterMs:             30,
		SlowdownQPSThreshold: 40,
		SlowdownFactor:       0.3,
	}

	payload := fakeLLMConfigPayload(s)

	var m map[string]interface{}
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("fakeLLMConfigPayload() returned invalid JSON: %v", err)
	}

	checks := map[string]float64{
		"max_concurrent":         77,
		"fixed_delay_ms":         250,
		"jitter_ms":              30,
		"slowdown_qps_threshold": 40,
	}
	for key, want := range checks {
		got, ok := m[key]
		if !ok {
			t.Errorf("payload missing key %q", key)
			continue
		}
		if got.(float64) != want {
			t.Errorf("payload[%q]: got %v, want %v", key, got, want)
		}
	}

	if sf, ok := m["slowdown_factor"]; !ok || sf.(float64) != 0.3 {
		t.Errorf("payload[\"slowdown_factor\"]: got %v, want 0.3", m["slowdown_factor"])
	}
}

func TestMarkdownReport_WriteFile(t *testing.T) {
	dir := t.TempDir()
	report := MarkdownReport{
		Title:       "Write Test",
		Timestamp:   time.Date(2026, 3, 6, 10, 0, 0, 0, time.UTC),
		Scenario:    "fast",
		Concurrency: 10,
		Duration:    30 * time.Second,
		Results: TestResults{
			TotalTurns:   50,
			SuccessTurns: 50,
		},
	}

	outPath, err := report.WriteFile(dir)
	if err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	// File must exist
	if _, statErr := os.Stat(outPath); os.IsNotExist(statErr) {
		t.Fatalf("WriteFile() file not created at %s", outPath)
	}

	// File must be inside the output directory
	if !strings.HasPrefix(outPath, dir) {
		t.Errorf("WriteFile() path %q not inside dir %q", outPath, dir)
	}

	// File must have .md extension
	if filepath.Ext(outPath) != ".md" {
		t.Errorf("WriteFile() file extension: got %q, want \".md\"", filepath.Ext(outPath))
	}

	// File content must include the title
	data, readErr := os.ReadFile(outPath)
	if readErr != nil {
		t.Fatalf("could not read written file: %v", readErr)
	}
	if !strings.Contains(string(data), "Write Test") {
		t.Errorf("written file does not contain report title")
	}
}

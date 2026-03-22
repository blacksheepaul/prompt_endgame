// Package fake provides an enhanced mock LLM provider for load testing
// with configurable TTFT, throughput, latency jitter, and error simulation.
package fake

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/port"
)

// StreamBehavior defines how tokens are streamed
type StreamBehavior struct {
	// TTFTMin/TTFTMax define the range for Time To First Token (before jitter)
	TTFTMin time.Duration
	TTFTMax time.Duration

	// TokensPerSecMin/TokensPerSecMax define the token generation rate range
	TokensPerSecMin float64
	TokensPerSecMax float64

	// OutputTokensMin/OutputTokensMax define the response length range
	OutputTokensMin int
	OutputTokensMax int

	// JitterPercent adds random variation (0.0-1.0) to delays
	// e.g., 0.1 means ±10% variation
	JitterPercent float64

	// ErrorRate is the probability (0.0-1.0) of returning an error
	ErrorRate float64
}

// DefaultStreamBehavior returns a reasonable default behavior
func DefaultStreamBehavior() StreamBehavior {
	return StreamBehavior{
		TTFTMin:         100 * time.Millisecond,
		TTFTMax:         500 * time.Millisecond,
		TokensPerSecMin: 10,
		TokensPerSecMax: 50,
		OutputTokensMin: 20,
		OutputTokensMax: 100,
		JitterPercent:   0.1,
		ErrorRate:       0,
	}
}

// Scenario defines a specific load testing scenario
type Scenario struct {
	Name        string
	Description string
	Behavior    StreamBehavior
}

// Provider implements a configurable fake LLM provider for load testing
type Provider struct {
	scenarios   map[string]*Scenario
	defaultScen string
	responses   map[string]string // agentID -> base response template
	rng         *rand.Rand
	mu          sync.RWMutex
}

// Option configures the Provider
type Option func(*Provider)

// WithScenario adds a custom scenario
func WithScenario(s *Scenario) Option {
	return func(p *Provider) {
		p.scenarios[s.Name] = s
	}
}

// WithDefaultScenario sets the default scenario name
func WithDefaultScenario(name string) Option {
	return func(p *Provider) {
		p.defaultScen = name
	}
}

// WithResponseTemplate sets a response template for an agent
func WithResponseTemplate(agentID, template string) Option {
	return func(p *Provider) {
		p.responses[agentID] = template
	}
}

// NewProvider creates an enhanced fake LLM provider
func NewProvider(opts ...Option) *Provider {
	p := &Provider{
		scenarios: make(map[string]*Scenario),
		responses: make(map[string]string),
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// Add default scenarios
	p.addDefaultScenarios()

	// Apply options
	for _, opt := range opts {
		opt(p)
	}

	// Ensure we have a default scenario
	if p.defaultScen == "" {
		p.defaultScen = "default"
	}

	return p
}

// addDefaultScenarios adds built-in scenarios for different load testing needs
func (p *Provider) addDefaultScenarios() {
	// Fast: Low latency, high throughput
	p.scenarios["fast"] = &Scenario{
		Name:        "fast",
		Description: "Fast responses (low TTFT, high throughput)",
		Behavior: StreamBehavior{
			TTFTMin:         50 * time.Millisecond,
			TTFTMax:         150 * time.Millisecond,
			TokensPerSecMin: 50,
			TokensPerSecMax: 100,
			OutputTokensMin: 10,
			OutputTokensMax: 50,
			JitterPercent:   0.05,
			ErrorRate:       0,
		},
	}

	// Slow: High latency, low throughput
	p.scenarios["slow"] = &Scenario{
		Name:        "slow",
		Description: "Slow responses (high TTFT, low throughput)",
		Behavior: StreamBehavior{
			TTFTMin:         500 * time.Millisecond,
			TTFTMax:         2000 * time.Millisecond,
			TokensPerSecMin: 5,
			TokensPerSecMax: 15,
			OutputTokensMin: 50,
			OutputTokensMax: 200,
			JitterPercent:   0.2,
			ErrorRate:       0,
		},
	}

	// Erratic: High jitter for testing resilience
	p.scenarios["erratic"] = &Scenario{
		Name:        "erratic",
		Description: "Erratic responses (high jitter, variable length)",
		Behavior: StreamBehavior{
			TTFTMin:         100 * time.Millisecond,
			TTFTMax:         1000 * time.Millisecond,
			TokensPerSecMin: 5,
			TokensPerSecMax: 100,
			OutputTokensMin: 10,
			OutputTokensMax: 300,
			JitterPercent:   0.5,
			ErrorRate:       0.05, // 5% error rate
		},
	}

	// Default: Balanced behavior
	p.scenarios["default"] = &Scenario{
		Name:        "default",
		Description: "Default balanced behavior",
		Behavior:    DefaultStreamBehavior(),
	}
}

// SetScenario sets the active scenario
func (p *Provider) SetScenario(name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.scenarios[name]; !ok {
		return fmt.Errorf("scenario %q not found", name)
	}
	p.defaultScen = name
	return nil
}

// GetScenario returns the current scenario
func (p *Provider) GetScenario() *Scenario {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.scenarios[p.defaultScen]
}

// ListScenarios returns all available scenario names
func (p *Provider) ListScenarios() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	names := make([]string, 0, len(p.scenarios))
	for name := range p.scenarios {
		names = append(names, name)
	}
	return names
}

// StreamCompletion implements port.LLMProvider
func (p *Provider) StreamCompletion(ctx context.Context, agentID string, prompt string) <-chan port.StreamToken {
	ch := make(chan port.StreamToken)

	go func() {
		defer close(ch)

		// Get current behavior
		p.mu.RLock()
		scenario := p.scenarios[p.defaultScen]
		behavior := scenario.Behavior
		p.mu.RUnlock()

		// Simulate error
		if behavior.ErrorRate > 0 && p.rng.Float64() < behavior.ErrorRate {
			select {
			case <-ctx.Done():
				return
			case ch <- port.StreamToken{Error: errors.New("simulated LLM error")}:
				return
			}
		}

		// Calculate TTFT with jitter
		ttft := p.randomDuration(behavior.TTFTMin, behavior.TTFTMax)
		ttft = p.applyJitter(ttft, behavior.JitterPercent)

		// Wait for TTFT
		select {
		case <-ctx.Done():
			return
		case <-time.After(ttft):
		}

		// Generate response
		response := p.generateResponse(agentID, behavior)
		tokens := strings.Fields(response)

		// Calculate token delay based on throughput
		tokensPerSec := behavior.TokensPerSecMin + p.rng.Float64()*(behavior.TokensPerSecMax-behavior.TokensPerSecMin)
		tokenDelay := time.Duration(float64(time.Second) / tokensPerSec)
		tokenDelay = p.applyJitter(tokenDelay, behavior.JitterPercent)

		// Stream tokens
		for i, token := range tokens {
			select {
			case <-ctx.Done():
				return
			case <-time.After(tokenDelay):
				if i > 0 {
					token = " " + token
				}
				ch <- port.StreamToken{Token: token, Done: false}
			}
		}

		// Send done signal
		select {
		case <-ctx.Done():
		case ch <- port.StreamToken{Done: true}:
		}
	}()

	return ch
}

// generateResponse creates a response with appropriate token count
func (p *Provider) generateResponse(agentID string, behavior StreamBehavior) string {
	p.mu.RLock()
	template, ok := p.responses[agentID]
	p.mu.RUnlock()

	if !ok {
		template = p.responses["default"]
	}
	if template == "" {
		template = "This is a simulated response from the fake LLM provider designed for load testing purposes. It generates tokens at a configurable rate to help measure system performance metrics like TTFT throughput and latency percentiles under various conditions."
	}

	// Determine target token count
	targetTokens := behavior.OutputTokensMin + p.rng.Intn(behavior.OutputTokensMax-behavior.OutputTokensMin+1)

	// Expand or truncate template to match target
	tokens := strings.Fields(template)
	if len(tokens) >= targetTokens {
		return strings.Join(tokens[:targetTokens], " ")
	}

	// Need to extend - repeat the template
	var result strings.Builder
	result.WriteString(template)
	remaining := targetTokens - len(tokens)

	for remaining > 0 {
		result.WriteString(" ")
		toAdd := remaining
		if toAdd > len(tokens) {
			toAdd = len(tokens)
		}
		result.WriteString(strings.Join(tokens[:toAdd], " "))
		remaining -= toAdd
	}

	return result.String()
}

// randomDuration returns a random duration between min and max
func (p *Provider) randomDuration(min, max time.Duration) time.Duration {
	if min >= max {
		return min
	}
	delta := max - min
	return min + time.Duration(p.rng.Float64()*float64(delta))
}

// applyJitter adds random variation to a duration
func (p *Provider) applyJitter(d time.Duration, jitterPercent float64) time.Duration {
	if jitterPercent <= 0 {
		return d
	}

	// Clamp jitter to reasonable range
	if jitterPercent > 1.0 {
		jitterPercent = 1.0
	}

	jitter := d.Seconds() * jitterPercent * (2*p.rng.Float64() - 1) // ±jitterPercent
	return time.Duration(float64(d) * (1 + jitter))
}

// CalculateExpectedDuration estimates how long a turn will take
func (p *Provider) CalculateExpectedDuration(agentCount int) time.Duration {
	p.mu.RLock()
	scenario := p.scenarios[p.defaultScen]
	behavior := scenario.Behavior
	p.mu.RUnlock()

	// Estimate per-agent duration
	ttftAvg := (behavior.TTFTMin + behavior.TTFTMax) / 2
	tokensAvg := float64(behavior.OutputTokensMin+behavior.OutputTokensMax) / 2
	tokensPerSecAvg := (behavior.TokensPerSecMin + behavior.TokensPerSecMax) / 2
	streamTime := time.Duration(tokensAvg / tokensPerSecAvg * float64(time.Second))

	perAgent := ttftAvg + streamTime
	return time.Duration(agentCount) * perAgent
}

// CalculateThroughputStats returns statistical estimates for the current scenario
func (p *Provider) CalculateThroughputStats() ThroughputStats {
	p.mu.RLock()
	scenario := p.scenarios[p.defaultScen]
	behavior := scenario.Behavior
	p.mu.RUnlock()

	return ThroughputStats{
		ExpectedTTFT:         (behavior.TTFTMin + behavior.TTFTMax) / 2,
		ExpectedTokensPerSec: (behavior.TokensPerSecMin + behavior.TokensPerSecMax) / 2,
		ExpectedOutputTokens: (behavior.OutputTokensMin + behavior.OutputTokensMax) / 2,
		ExpectedDuration:     p.CalculateExpectedDuration(1),
		ErrorRate:            behavior.ErrorRate,
		Scenario:             scenario.Name,
	}
}

// ThroughputStats holds statistical estimates
type ThroughputStats struct {
	ExpectedTTFT         time.Duration
	ExpectedTokensPerSec float64
	ExpectedOutputTokens int
	ExpectedDuration     time.Duration
	ErrorRate            float64
	Scenario             string
}

// Format returns a formatted string representation
func (s ThroughputStats) Format() string {
	return fmt.Sprintf(
		"Scenario: %s\n"+
			"  Expected TTFT: %v\n"+
			"  Expected Tokens/sec: %.1f\n"+
			"  Expected Output Tokens: %d\n"+
			"  Expected Duration: %v\n"+
			"  Error Rate: %.1f%%",
		s.Scenario,
		s.ExpectedTTFT,
		s.ExpectedTokensPerSec,
		s.ExpectedOutputTokens,
		s.ExpectedDuration,
		s.ErrorRate*100,
	)
}

// Reset resets the random seed (useful for reproducible tests)
func (p *Provider) Reset(seed int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rng = rand.New(rand.NewSource(seed))
}

var _ port.LLMProvider = (*Provider)(nil)

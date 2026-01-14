package mock

import (
	"context"
	"strings"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/port"
)

// Provider implements port.LLMProvider with mock responses
type Provider struct {
	TokenDelay time.Duration
	Responses  map[string]string // agentID -> response
}

// NewProvider creates a mock provider with default settings
func NewProvider() *Provider {
	return &Provider{
		TokenDelay: 50 * time.Millisecond,
		Responses: map[string]string{
			"default": "Hello! I am a mock AI assistant. I'm here to help you test the streaming functionality. This response is being sent token by token.",
		},
	}
}

func (p *Provider) StreamCompletion(ctx context.Context, agentID string, prompt string) <-chan port.StreamToken {
	ch := make(chan port.StreamToken)

	go func() {
		defer close(ch)

		response, ok := p.Responses[agentID]
		if !ok {
			response = p.Responses["default"]
		}

		tokens := strings.Fields(response)
		for i, token := range tokens {
			select {
			case <-ctx.Done():
				return
			case <-time.After(p.TokenDelay):
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

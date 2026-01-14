package port

import "context"

// StreamToken represents a single token in the stream
type StreamToken struct {
	Token string
	Done  bool
	Error error
}

// LLMProvider defines operations for LLM interactions
type LLMProvider interface {
	// StreamCompletion streams tokens for the given prompt
	// The channel closes when streaming is complete or context is cancelled
	StreamCompletion(ctx context.Context, agentID string, prompt string) <-chan StreamToken
}

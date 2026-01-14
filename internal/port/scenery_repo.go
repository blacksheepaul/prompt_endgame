package port

import "context"

// Scenery represents a conversation scenario configuration
type Scenery struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Agents      []Agent `json:"agents"`
}

// Agent represents an AI agent in the scenery
type Agent struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	SystemPrompt string `json:"system_prompt"`
}

// SceneryRepository defines operations for scenery management
type SceneryRepository interface {
	// Get retrieves a scenery by ID
	Get(ctx context.Context, id string) (*Scenery, error)

	// List returns all available sceneries
	List(ctx context.Context) ([]Scenery, error)
}

package fs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/blacksheepaul/prompt_endgame/internal/port"
)

// Repo implements port.SceneryRepository with filesystem storage
type Repo struct {
	basePath  string
	sceneries map[string]*port.Scenery
}

// NewRepo creates a new filesystem scenery repository
func NewRepo(basePath string) *Repo {
	return &Repo{
		basePath:  basePath,
		sceneries: make(map[string]*port.Scenery),
	}
}

// LoadFromFile loads a scenery from a JSON file
func (r *Repo) LoadFromFile(filename string) error {
	path := filepath.Join(r.basePath, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var scenery port.Scenery
	if err := json.Unmarshal(data, &scenery); err != nil {
		return err
	}

	r.sceneries[scenery.ID] = &scenery
	return nil
}

// RegisterDefault adds a default scenery for testing
func (r *Repo) RegisterDefault() {
	r.sceneries["default"] = &port.Scenery{
		ID:          "default",
		Name:        "Default Scenery",
		Description: "A simple conversation scenario",
		Agents: []port.Agent{
			{
				ID:           "assistant",
				Name:         "Assistant",
				SystemPrompt: "You are a helpful assistant.",
			},
		},
	}
}

func (r *Repo) Get(ctx context.Context, id string) (*port.Scenery, error) {
	scenery, ok := r.sceneries[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return scenery, nil
}

func (r *Repo) List(ctx context.Context) ([]port.Scenery, error) {
	result := make([]port.Scenery, 0, len(r.sceneries))
	for _, s := range r.sceneries {
		result = append(result, *s)
	}
	return result, nil
}

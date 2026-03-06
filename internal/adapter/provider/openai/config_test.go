package openai

import (
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with all fields",
			cfg: Config{
				Endpoint:   "http://localhost:10181/v1",
				Model:      "gpt-4",
				APIKey:     "test-key",
				Timeout:    30 * time.Second,
				MaxRetries: 3,
			},
			wantErr: false,
		},
		{
			name: "valid config with minimal fields",
			cfg: Config{
				Endpoint: "http://localhost:10181/v1",
				Model:    "gpt-4",
			},
			wantErr: false,
		},
		{
			name: "empty endpoint should panic",
			cfg: Config{
				Endpoint: "",
				Model:    "gpt-4",
			},
			wantErr: true,
			errMsg:  "endpoint is required",
		},
		{
			name: "empty model should panic",
			cfg: Config{
				Endpoint: "http://localhost:10181/v1",
				Model:    "",
			},
			wantErr: true,
			errMsg:  "model is required",
		},
		{
			name: "both empty should panic",
			cfg: Config{
				Endpoint: "",
				Model:    "",
			},
			wantErr: true,
			errMsg:  "endpoint is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("NewProvider() did not panic for config: %v", tt.cfg)
					} else {
						errStr, ok := r.(string)
						if !ok {
							t.Errorf("Expected panic with string message, got: %v", r)
						} else if errStr != "openai provider: "+tt.errMsg {
							t.Errorf("Expected panic message 'openai provider: %s', got: %s", tt.errMsg, errStr)
						}
					}
				}()
				NewProvider(tt.cfg)
			} else {
				provider := NewProvider(tt.cfg)
				if provider == nil {
					t.Errorf("NewProvider() returned nil for valid config")
					return
				}
				if provider.config.Endpoint != tt.cfg.Endpoint {
					t.Errorf("Expected endpoint %s, got %s", tt.cfg.Endpoint, provider.config.Endpoint)
				}
				if provider.config.Model != tt.cfg.Model {
					t.Errorf("Expected model %s, got %s", tt.cfg.Model, provider.config.Model)
				}
			}
		})
	}
}

func TestNewProvider_DefaultValues(t *testing.T) {
	cfg := Config{
		Endpoint: "http://localhost:10181/v1",
		Model:    "gpt-4",
	}

	provider := NewProvider(cfg)

	// Check default timeout
	if provider.config.Timeout != 30*time.Second {
		t.Errorf("Expected default timeout 30s, got %v", provider.config.Timeout)
	}

	// Check default max retries
	if provider.config.MaxRetries != 3 {
		t.Errorf("Expected default max retries 3, got %d", provider.config.MaxRetries)
	}
}

func TestNewProvider_CustomValues(t *testing.T) {
	cfg := Config{
		Endpoint:   "http://localhost:10181/v1",
		Model:      "gpt-4",
		APIKey:     "custom-key",
		Timeout:    60 * time.Second,
		MaxRetries: 5,
	}

	provider := NewProvider(cfg)

	if provider.config.Timeout != 60*time.Second {
		t.Errorf("Expected timeout 60s, got %v", provider.config.Timeout)
	}

	if provider.config.MaxRetries != 5 {
		t.Errorf("Expected max retries 5, got %d", provider.config.MaxRetries)
	}

	if provider.config.APIKey != "custom-key" {
		t.Errorf("Expected API key 'custom-key', got %s", provider.config.APIKey)
	}
}

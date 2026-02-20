package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// PrimeConfig controls what the prime command outputs.
type PrimeConfig struct {
	Sections        []string `json:"sections"`                    // e.g. ["recall", "remember", "delegation"]
	DelegationModel string   `json:"delegation_model"`            // "sonnet" | "haiku"
	RememberText    string   `json:"remember_text,omitempty"`     // custom remember section; empty = use default
	Custom          string   `json:"custom"`                      // user-defined guidance text
}

// Config is the top-level mnemon configuration.
type Config struct {
	Prime PrimeConfig `json:"prime"`
}

// DefaultConfig returns a Config with all defaults applied.
func DefaultConfig() *Config {
	return &Config{
		Prime: PrimeConfig{
			Sections:        []string{"recall", "remember", "delegation"},
			DelegationModel: "sonnet",
		},
	}
}

// Load reads config from <dataDir>/config.json.
// Returns defaults if the file does not exist.
func Load(dataDir string) (*Config, error) {
	path := filepath.Join(dataDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return DefaultConfig(), nil
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes config to <dataDir>/config.json atomically via .tmp + rename.
func Save(dataDir string, cfg *Config) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	path := filepath.Join(dataDir, "config.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

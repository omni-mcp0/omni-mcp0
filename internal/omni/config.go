package omni

import (
	"encoding/json"
	"fmt"
	"os"
)

// ServerConfig describes one upstream MCP server that Omni should connect to.
// MVP: only stdio servers (a child process we talk to over stdin/stdout).
type ServerConfig struct {
	Name        string            `json:"name"`        // unique id the agent uses, e.g. "github"
	Command     string            `json:"command"`     // program to run, e.g. "npx"
	Args        []string          `json:"args"`        // program arguments
	Env         map[string]string `json:"env"`         // extra env vars, e.g. tokens
	Description string            `json:"description"` // used by discover_server search
	Tags        []string          `json:"tags"`        // extra search words
	TimeoutMS   int               `json:"timeout_ms"`  // per-call deadline (default 15000)
}

// Config is the whole omni.json file.
type Config struct {
	Servers []ServerConfig `json:"servers"`
}

// LoadConfig reads and parses the config file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &cfg, nil
}

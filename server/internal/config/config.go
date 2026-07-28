package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LLMModel LLMModelConfig `yaml:"llm_model"`
	Database DatabaseConfig `yaml:"database"`
	Server   ServerConfig   `yaml:"server"`
	OIDC     OIDCConfig     `yaml:"oidc"`
	MCP      MCPConfig      `yaml:"mcp"`
}

type MCPConfig struct {
	Servers []MCPServerConfig `yaml:"servers"`
}

type MCPServerConfig struct {
	Name      string            `yaml:"name"`
	Prefix    string            `yaml:"prefix"`
	Transport string            `yaml:"transport"`
	URL       string            `yaml:"url"`
	Headers   map[string]string `yaml:"headers"`
}

type LLMModelConfig struct {
	Anthropic AnthropicConfig `yaml:"anthropic"`
}

type AnthropicConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
}

type DatabaseConfig struct {
	DatabaseURL string `yaml:"database_url"`
}

type ServerConfig struct {
	Port string `yaml:"port"`
}

// OIDCConfig — deferred until IDP registration. See docs/deferred.md.
type OIDCConfig struct {
	ProviderURL  string `yaml:"oidc_provider_url"`
	ClientID     string `yaml:"oidc_client_id"`
	ClientSecret string `yaml:"oidc_client_secret"`
	SessionSecret string `yaml:"session_secret"`
	BaseURL      string `yaml:"base_url"`
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %s: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	if cfg.LLMModel.Anthropic.Model == "" {
		cfg.LLMModel.Anthropic.Model = "claude-sonnet-4-6"
	}
	if cfg.Server.Port == "" {
		cfg.Server.Port = "8080"
	}

	return &cfg, nil
}

// ProvideConfig loads config from the default path (config.yaml in cwd).
// Wire provider.
func ProvideConfig() (*Config, error) {
	return Load("config.yaml")
}

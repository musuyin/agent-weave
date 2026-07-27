package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github/musuyin/agent-weave/internal/config"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func TestLoad_FullConfig(t *testing.T) {
	path := writeConfigFile(t, `
llm_model:
  anthropic:
    api_key: "sk-test"
    base_url: "http://localhost:6655/"
    model: "claude-opus-4-7"
database:
  database_url: "mysql://user:pass@tcp(localhost:3306)/db"
server:
  port: "9090"
`)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "sk-test", cfg.LLMModel.Anthropic.APIKey)
	assert.Equal(t, "http://localhost:6655/", cfg.LLMModel.Anthropic.BaseURL)
	assert.Equal(t, "claude-opus-4-7", cfg.LLMModel.Anthropic.Model)
	assert.Equal(t, "mysql://user:pass@tcp(localhost:3306)/db", cfg.Database.DatabaseURL)
	assert.Equal(t, "9090", cfg.Server.Port)
}

func TestLoad_Defaults(t *testing.T) {
	path := writeConfigFile(t, `
llm_model:
  anthropic:
    api_key: "sk-x"
`)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-4-6", cfg.LLMModel.Anthropic.Model, "model default")
	assert.Equal(t, "8080", cfg.Server.Port, "port default")
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	assert.Error(t, err)
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeConfigFile(t, `this: {is: [invalid yaml`)
	_, err := config.Load(path)
	assert.Error(t, err)
}

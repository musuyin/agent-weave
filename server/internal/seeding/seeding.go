package seeding

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed orchestrator.md
var Orchestrator string

//go:embed skills/*.md
var skillFiles embed.FS

//go:embed skills.yaml
var skillDescYAML []byte

//go:embed agents/*.yaml
var agentFiles embed.FS

// SkillDef is a system skill loaded from skills/*.md + skills.yaml.
type SkillDef struct {
	Name        string
	Description string
	Body        string
}

// AgentDef is a system agent loaded from agents/*.yaml.
type AgentDef struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Prompt      string   `yaml:"prompt"`
	Skills      []string `yaml:"skills"` // skill names (must match SkillDef.Name)
}

// SystemSkills holds all system skills keyed by name.
var SystemSkills map[string]SkillDef

// SystemAgents is the ordered list of all system agents.
var SystemAgents []AgentDef

func init() {
	SystemSkills = loadSkills()
	SystemAgents = loadAgents()
}

func loadSkills() map[string]SkillDef {
	// Parse descriptions from skills.yaml (name → description).
	var descs map[string]string
	if err := yaml.Unmarshal(skillDescYAML, &descs); err != nil {
		panic(fmt.Sprintf("seeding: parse skills.yaml: %v", err))
	}

	skills := make(map[string]SkillDef)
	entries, err := fs.ReadDir(skillFiles, "skills")
	if err != nil {
		panic(fmt.Sprintf("seeding: read skills dir: %v", err))
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := skillFiles.ReadFile("skills/" + e.Name())
		if err != nil {
			panic(fmt.Sprintf("seeding: read skill %s: %v", e.Name(), err))
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		skills[name] = SkillDef{
			Name:        name,
			Description: descs[name],
			Body:        string(data),
		}
	}
	return skills
}

func loadAgents() []AgentDef {
	entries, err := fs.ReadDir(agentFiles, "agents")
	if err != nil {
		panic(fmt.Sprintf("seeding: read agents dir: %v", err))
	}
	var agents []AgentDef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := agentFiles.ReadFile("agents/" + e.Name())
		if err != nil {
			panic(fmt.Sprintf("seeding: read agent %s: %v", e.Name(), err))
		}
		var def AgentDef
		if err := yaml.Unmarshal(data, &def); err != nil {
			panic(fmt.Sprintf("seeding: parse agent %s: %v", e.Name(), err))
		}
		agents = append(agents, def)
	}
	return agents
}

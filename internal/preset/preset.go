// Package preset — load DAG definitions from YAML or built-in Go.
//
// A Preset is a named collection of tasks, edges, and side-DAG declarations.
// Presets make DAG definitions declarative and version-controllable.
//
// Built-in presets: "v3-180" (120 core + 60 side, 6 layers x 20 width).
package preset

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Task is a single DAG task as defined in a preset YAML.
type Task struct {
	ID          string `yaml:"id"`
	Stage       int    `yaml:"stage"`
	Slot        int    `yaml:"slot,omitempty"`
	Description string `yaml:"description"`
	Repo        string `yaml:"repo,omitempty"`
	Subproject  string `yaml:"subproject,omitempty"`
	Category    string `yaml:"category,omitempty"`
	Lane        string `yaml:"lane,omitempty"`
	Branch      string `yaml:"branch,omitempty"`
	Kind        string `yaml:"kind,omitempty"`
	Priority    int    `yaml:"priority,omitempty"`
	Status      string `yaml:"status,omitempty"`
	SideDAG     string `yaml:"side_dag,omitempty"`
}

// Edge connects two tasks in a DAG.
type Edge struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// Preset is a named DAG definition.
type Preset struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Tasks       []Task            `yaml:"tasks"`
	Edges       []Edge            `yaml:"edges"`
	SideDAGs    map[string]string `yaml:"side_dags"`
}

// Load reads a preset from a YAML file.
func Load(path string) (*Preset, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var p Preset
	if err := yaml.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &p, nil
}

// LoadFromEnv loads a preset by name from $PHENODAG_PRESETS or ./presets.
func LoadFromEnv(name string) (*Preset, error) {
	dir := os.Getenv("PHENODAG_PRESETS")
	if dir == "" {
		dir = "./presets"
	}
	candidates := []string{
		filepath.Join(dir, name+".yaml"),
		filepath.Join(dir, name+".yml"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return Load(p)
		}
	}
	return nil, fmt.Errorf("preset %q not found (looked in %s)", name, strings.Join(candidates, ", "))
}

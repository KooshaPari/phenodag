// Package preset loads phenodag fleet presets from `presets/<name>.yaml`.
//
// File format (v1, schema-frozen):
//
//	name: <string>
//	description: <string>
//	core:
//	  stages: <int>
//	  width: <int>
//	side_dags:
//	  - id: <string>
//	    name: <string>
//	    description: <string>
//	    size: <int>
//	    repo: <string>
//
// A side_dag with size N expands into N tasks; an empty list means no side-DAGs.
// The `empty` preset is reserved for clean init tests.
package preset

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SideDAG is a single side-DAG definition.
type SideDAG struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Size        int    `yaml:"size"`
	Repo        string `yaml:"repo"`
}

// Core is the core fleet shape.
type Core struct {
	Stages int `yaml:"stages"`
	Width  int `yaml:"width"`
}

// Preset is a complete fleet preset (v1 schema).
type Preset struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	Core        Core      `yaml:"core"`
	SideDAGs    []SideDAG `yaml:"side_dags"`
}

// CoreCount returns stages * width.
func (p *Preset) CoreCount() int {
	return p.Core.Stages * p.Core.Width
}

// SideCount returns the sum of side-DAG sizes.
func (p *Preset) SideCount() int {
	n := 0
	for _, sd := range p.SideDAGs {
		n += sd.Size
	}
	return n
}

// TotalCount returns CoreCount + SideCount.
func (p *Preset) TotalCount() int {
	return p.CoreCount() + p.SideCount()
}

// Validate enforces the v1 schema invariants.
func (p *Preset) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("preset.name is required")
	}
	if p.Core.Stages < 0 {
		return errors.New("preset.core.stages must be >= 0")
	}
	if p.Core.Width < 0 {
		return errors.New("preset.core.width must be >= 0")
	}
	for i, sd := range p.SideDAGs {
		if strings.TrimSpace(sd.ID) == "" {
			return fmt.Errorf("preset.side_dags[%d].id is required", i)
		}
		if sd.Size < 0 {
			return fmt.Errorf("preset.side_dags[%d].size must be >= 0", i)
		}
	}
	return nil
}

// LoadFromFile reads a single preset YAML file.
func LoadFromFile(path string) (*Preset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read preset %s: %w", path, err)
	}
	var p Preset
	if err := yaml.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parse preset %s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("invalid preset %s: %w", path, err)
	}
	return &p, nil
}

// LoadByName searches `presetsDir` for `<name>.yaml` and loads it.
// Falls back to looking in the current working directory's `presets/` subdir.
func LoadByName(name, presetsDir string) (*Preset, error) {
	candidates := []string{}
	if presetsDir != "" {
		candidates = append(candidates, filepath.Join(presetsDir, name+".yaml"))
	}
	cwd, _ := os.Getwd()
	candidates = append(candidates, filepath.Join(cwd, "presets", name+".yaml"))
	candidates = append(candidates, filepath.Join(cwd, "..", "presets", name+".yaml"))
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return LoadFromFile(c)
		}
	}
	return nil, fmt.Errorf("preset %q not found in: %s", name, strings.Join(candidates, ", "))
}

// ListAll enumerates every *.yaml in `presetsDir` (sorted, deterministic).
func ListAll(presetsDir string) ([]string, error) {
	entries, err := os.ReadDir(presetsDir)
	if err != nil {
		return nil, fmt.Errorf("read presets dir %s: %w", presetsDir, err)
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	return names, nil
}

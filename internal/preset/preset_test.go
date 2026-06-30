package preset

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestValidate_Empty(t *testing.T) {
	p := &Preset{Name: "empty", Core: Core{Stages: 0, Width: 0}}
	if err := p.Validate(); err != nil {
		t.Fatalf("empty preset should validate, got: %v", err)
	}
	if p.TotalCount() != 0 {
		t.Fatalf("empty preset total should be 0, got %d", p.TotalCount())
	}
}

func TestValidate_MissingName(t *testing.T) {
	p := &Preset{Core: Core{Stages: 1, Width: 1}}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidate_NegativeStages(t *testing.T) {
	p := &Preset{Name: "x", Core: Core{Stages: -1, Width: 0}}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for negative stages")
	}
}

func TestValidate_BadSideDAG(t *testing.T) {
	p := &Preset{
		Name: "x",
		Core: Core{Stages: 1, Width: 1},
		SideDAGs: []SideDAG{
			{ID: "", Size: 1},
		},
	}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for missing side-dag id")
	}
}

func TestCounts(t *testing.T) {
	p := &Preset{
		Name: "x",
		Core: Core{Stages: 6, Width: 20},
		SideDAGs: []SideDAG{
			{ID: "a", Size: 5},
			{ID: "b", Size: 7},
		},
	}
	if p.CoreCount() != 120 {
		t.Errorf("CoreCount = %d, want 120", p.CoreCount())
	}
	if p.SideCount() != 12 {
		t.Errorf("SideCount = %d, want 12", p.SideCount())
	}
	if p.TotalCount() != 132 {
		t.Errorf("TotalCount = %d, want 132", p.TotalCount())
	}
}

func TestLoadFromFile_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(path, []byte("name: empty\ndescription: test empty\ncore:\n  stages: 0\n  width: 0\nside_dags: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "empty" {
		t.Errorf("Name = %q, want %q", p.Name, "empty")
	}
	if p.TotalCount() != 0 {
		t.Errorf("TotalCount = %d, want 0", p.TotalCount())
	}
}

func TestLoadByName_FromPresetsDir(t *testing.T) {
	dir := t.TempDir()
	// create presets/foo.yaml
	if err := os.WriteFile(filepath.Join(dir, "foo.yaml"), []byte("name: foo\ncore:\n  stages: 1\n  width: 1\nside_dags: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadByName("foo", dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "foo" {
		t.Errorf("Name = %q, want %q", p.Name, "foo")
	}
}

func TestLoadByName_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadByName("missing", dir)
	if err == nil {
		t.Fatal("expected error for missing preset")
	}
}

func TestListAll(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha.yaml", "beta.yaml", "gamma.yaml", "readme.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("name: x\ncore:\n  stages: 0\n  width: 0\nside_dags: []\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	names, err := ListAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	want := []string{"alpha", "beta", "gamma"}
	if len(names) != len(want) {
		t.Fatalf("len(names) = %d, want %d (got %v)", len(names), len(want), names)
	}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("names[%d] = %q, want %q", i, names[i], n)
		}
	}
}

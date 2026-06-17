package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestProgressBar(t *testing.T) {
	got := ProgressBar(50, 20)
	if !strings.HasPrefix(got, "[") || !strings.HasSuffix(got, "]") {
		t.Errorf("ProgressBar missing brackets: %q", got)
	}
	if len(got) != 22 {
		t.Errorf("ProgressBar length = %d, want 22", len(got))
	}
}

func TestGanttASCIIStages(t *testing.T) {
	tasks := []StageTask{
		{ID: "task-01-01", Stage: 1, Status: "done"},
		{ID: "task-01-02", Stage: 1, Status: "ready"},
		{ID: "task-02-01", Stage: 2, Status: "done"},
	}
	var buf bytes.Buffer
	if err := GanttASCII(&buf, tasks); err != nil {
		t.Fatalf("GanttASCII: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "L1") || !strings.Contains(out, "L2") {
		t.Errorf("GanttASCII missing stage labels: %q", out)
	}
	if !strings.Contains(out, "50%") {
		t.Errorf("GanttASCII missing 50%%: %q", out)
	}
}

func TestGanttMermaidSections(t *testing.T) {
	tasks := []StageTask{
		{ID: "task-01-01", Stage: 1, Status: "ready"},
	}
	var buf bytes.Buffer
	if err := GanttMermaid(&buf, tasks); err != nil {
		t.Fatalf("GanttMermaid: %v", err)
	}
	if !strings.Contains(buf.String(), "section L1") {
		t.Errorf("GanttMermaid missing section: %q", buf.String())
	}
}

func TestMermaidFlowchart(t *testing.T) {
	nodes := []MermaidNode{{ID: "task-01-01", Subproject: "root"}}
	edges := [][2]string{{"task-01-01", "task-02-01"}}
	var buf bytes.Buffer
	if err := MermaidFlowchart(&buf, nodes, edges); err != nil {
		t.Fatalf("MermaidFlowchart: %v", err)
	}
	if !strings.Contains(buf.String(), "task_01_01") {
		t.Errorf("MermaidFlowchart missing sanitized id: %q", buf.String())
	}
}

func TestBurndown(t *testing.T) {
	var buf bytes.Buffer
	if err := Burndown(&buf, 100, 25); err != nil {
		t.Fatalf("Burndown: %v", err)
	}
	if !strings.Contains(buf.String(), "25%") {
		t.Errorf("Burndown missing 25%%: %q", buf.String())
	}
}

func TestSubstituteTemplate(t *testing.T) {
	tmpl := "var nodes = {{NODES}};\nvar edges = {{EDGES}};"
	nodes := []HTMLNode{{ID: "a", Status: "ready", Sub: "root", Stage: "1"}}
	edges := []HTMLEdge{{From: "a", To: "b"}}
	got, err := SubstituteTemplate(tmpl, nodes, edges)
	if err != nil {
		t.Fatalf("SubstituteTemplate: %v", err)
	}
	if strings.Contains(got, "{{NODES}}") || strings.Contains(got, "{{EDGES}}") {
		t.Errorf("SubstituteTemplate left placeholders: %q", got)
	}
}

func TestSanitizeID(t *testing.T) {
	if got := SanitizeID("task-01-01"); got != "task_01_01" {
		t.Errorf("SanitizeID = %q, want task_01_01", got)
	}
}

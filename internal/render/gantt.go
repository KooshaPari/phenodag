package render

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// GanttASCII writes an ASCII gantt summary to w, one row per stage,
// with a 20-char progress bar and percent-done annotation.
func GanttASCII(w io.Writer, tasks []StageTask) error {
	byStage := map[int][]string{}
	for _, t := range tasks {
		byStage[t.Stage] = append(byStage[t.Stage], t.ID)
	}
	stages := make([]int, 0, len(byStage))
	for s := range byStage {
		stages = append(stages, s)
	}
	sort.Ints(stages)
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	fmt.Fprintln(bw, "Gantt (one row per stage):")
	for _, s := range stages {
		done := 0
		for _, t := range tasks {
			if t.Stage == s && t.Status == "done" {
				done++
			}
		}
		bar := ProgressBar(0, 20)
		if n := len(byStage[s]); n > 0 {
			bar = ProgressBar((done*100)/n, 20)
			fmt.Fprintf(bw, "L%d |%s| %d%% (%d/%d)\n", s, strings.TrimSuffix(strings.TrimPrefix(bar, "["), "]"), (done*100)/n, done, n)
			continue
		}
		fmt.Fprintf(bw, "L%d |%s| 0%% (0/0)\n", s, strings.TrimSuffix(strings.TrimPrefix(bar, "["), "]"))
	}
	return nil
}

// GanttMermaid writes a Mermaid ```gantt``` block to w.
func GanttMermaid(w io.Writer, tasks []StageTask) error {
	byStage := map[int][]string{}
	for _, t := range tasks {
		byStage[t.Stage] = append(byStage[t.Stage], t.ID)
	}
	stages := make([]int, 0, len(byStage))
	for s := range byStage {
		stages = append(stages, s)
	}
	sort.Ints(stages)
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	fmt.Fprintln(bw, "```mermaid")
	fmt.Fprintln(bw, "gantt")
	fmt.Fprintln(bw, "    title phenodag Gantt")
	fmt.Fprintln(bw, "    dateFormat YYYY-MM-DD")
	_, _ = io.WriteString(bw, "    axisFormat %m-%d\n")
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for i, s := range stages {
		fmt.Fprintf(bw, "    section L%d\n", s)
		for j, id := range byStage[s] {
			start := base.AddDate(0, 0, i*5+j)
			fmt.Fprintf(bw, "        %-22s :a%d, %s, 1d\n", id, i*100+j, start.Format("2006-01-02"))
		}
	}
	fmt.Fprintln(bw, "```")
	return nil
}

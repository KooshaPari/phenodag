// Package render provides shared output formatters for phenodag and the
// merged dagctl command surface (Phase-4b superset-merge).
//
// Each formatter takes a *sql.DB and an io.Writer, queries the schema
// (tasks / edges / agents), and writes the result. This replaces the
// 30+ inline db-open/query/close blocks that used to live in
// phenodag_extras.go as verbatim dagctl ports, and which were the
// primary source of the 4.5% SonarCloud duplication that the
// ADR-dedup-baseline.md 5.0% gate was set to absorb.
//
// See ADR-dag-superset-merge.md for the merge plan and
// ADR-dedup-baseline.md for the dedup rationale.
package render

import (
	"bufio"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// StageTask is the minimal projection all formatters consume.
type StageTask struct {
	ID     string
	Stage  int
	Status string
}

// LoadStageTasks reads core-DAG tasks ordered by stage,id.
// Side-DAGs (side_dag != '') are excluded.
func LoadStageTasks(db *sql.DB) ([]StageTask, error) {
	rows, err := db.Query(`SELECT id, stage, status FROM tasks
		WHERE side_dag='' OR side_dag IS NULL
		ORDER BY stage, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StageTask
	for rows.Next() {
		var t StageTask
		if err := rows.Scan(&t.ID, &t.Stage, &t.Status); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ProgressBar returns a 20-char progress bar "====        " for pct 0-100.
func ProgressBar(pct, width int) string {
	if width <= 0 {
		width = 20
	}
	filled := (pct * width) / 100
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("=", filled) + strings.Repeat(" ", width-filled) + "]"
}

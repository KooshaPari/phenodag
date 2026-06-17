package render

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"strings"
)

// MermaidNode is the minimal projection for flowchart nodes.
type MermaidNode struct {
	ID         string
	Subproject string
}

// LoadMermaidNodes reads all core-DAG task ids+subprojects.
func LoadMermaidNodes(db *sql.DB) ([]MermaidNode, error) {
	rows, err := db.Query(`SELECT id, COALESCE(subproject,'') FROM tasks
		WHERE side_dag='' OR side_dag IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MermaidNode
	for rows.Next() {
		var n MermaidNode
		if err := rows.Scan(&n.ID, &n.Subproject); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// SanitizeID replaces non-alphanumeric chars in `s` with '_' for safe
// Mermaid node identifiers.
func SanitizeID(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

// MermaidFlowchart writes a Mermaid ```flowchart LR``` block to w.
// Edges are loaded from the `edges` table.
func MermaidFlowchart(w io.Writer, nodes []MermaidNode, edges [][2]string) error {
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	fmt.Fprintln(bw, "```mermaid")
	fmt.Fprintln(bw, "flowchart LR")
	for _, n := range nodes {
		fmt.Fprintf(bw, "    %s[\"%s<br/>%s\"]\n", SanitizeID(n.ID), n.ID, n.Subproject)
	}
	for _, e := range edges {
		fmt.Fprintf(bw, "    %s --> %s\n", SanitizeID(e[0]), SanitizeID(e[1]))
	}
	fmt.Fprintln(bw, "```")
	return nil
}

// LoadEdges reads the edges table.
func LoadEdges(db *sql.DB) ([][2]string, error) {
	rows, err := db.Query(`SELECT from_task, to_task FROM edges`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var f, t string
		if err := rows.Scan(&f, &t); err != nil {
			return nil, err
		}
		out = append(out, [2]string{f, t})
	}
	return out, rows.Err()
}

// Burndown writes a 50-char burndown bar to w.
func Burndown(w io.Writer, total, done int) error {
	if total < 0 {
		total = 0
	}
	if done < 0 {
		done = 0
	}
	if done > total {
		done = total
	}
	pending := total - done
	const width = 50
	bars := width
	if total > 0 {
		bars = (pending * width) / total
	}
	pct := 0
	if total > 0 {
		pct = (done * 100) / total
	}
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	fmt.Fprintf(bw, "Burndown (total=%d, done=%d, pending=%d):\n", total, done, pending)
	fmt.Fprintln(bw, "["+strings.Repeat("#", bars)+strings.Repeat(" ", width-bars)+"]")
	fmt.Fprintf(bw, "Progress: %d%%\n", pct)
	return nil
}

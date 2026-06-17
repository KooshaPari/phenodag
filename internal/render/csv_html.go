package render

import (
	"bufio"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

// CSVRow is the projection written to CSV.
type CSVRow struct {
	ID          string
	Stage       int
	Slot        int
	Status      string
	Subproject  string
	Category    string
	Kind        string
	Priority    int
	Description string
}

// LoadCSVRows reads tasks projected for CSV export.
func LoadCSVRows(db *sql.DB) ([]CSVRow, error) {
	rows, err := db.Query(`SELECT id, stage, slot, status,
		COALESCE(subproject,''), COALESCE(category,''), COALESCE(kind,''),
		COALESCE(priority,0), description
		FROM tasks ORDER BY stage, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CSVRow
	for rows.Next() {
		var r CSVRow
		if err := rows.Scan(&r.ID, &r.Stage, &r.Slot, &r.Status,
			&r.Subproject, &r.Category, &r.Kind, &r.Priority, &r.Description); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// WriteCSV writes rows to w with a stable header.
func WriteCSV(w io.Writer, rows []CSVRow) error {
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	cw := csv.NewWriter(bw)
	if err := cw.Write([]string{"id", "stage", "slot", "status", "subproject", "category", "kind", "priority", "description"}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := cw.Write([]string{
			r.ID, strconv.Itoa(r.Stage), strconv.Itoa(r.Slot),
			r.Status, r.Subproject, r.Category, r.Kind,
			strconv.Itoa(r.Priority), r.Description,
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// HTMLNode is the projection for the HTML template.
type HTMLNode struct {
	ID     string `json:"ID"`
	Status string `json:"Status"`
	Sub    string `json:"Sub"`
	Stage  string `json:"Stage"`
}

// HTMLEdge is the projection for the HTML template.
type HTMLEdge struct {
	From string `json:"From"`
	To   string `json:"To"`
}

// LoadHTMLNodes reads tasks projected for the HTML template.
func LoadHTMLNodes(db *sql.DB) ([]HTMLNode, error) {
	rows, err := db.Query(`SELECT id, COALESCE(status,''), COALESCE(subproject,''), stage FROM tasks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HTMLNode
	for rows.Next() {
		var n HTMLNode
		var stage int
		if err := rows.Scan(&n.ID, &n.Status, &n.Sub, &stage); err != nil {
			return nil, err
		}
		n.Stage = fmt.Sprintf("%d", stage)
		out = append(out, n)
	}
	return out, rows.Err()
}

// LoadHTMLEdges reads edges projected for the HTML template.
func LoadHTMLEdges(db *sql.DB) ([]HTMLEdge, error) {
	rows, err := db.Query(`SELECT from_task, to_task FROM edges`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HTMLEdge
	for rows.Next() {
		var e HTMLEdge
		if err := rows.Scan(&e.From, &e.To); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SubstituteTemplate replaces the {{NODES}} and {{EDGES}} placeholders
// in the HTML template with the JSON-marshalled projections.
func SubstituteTemplate(tmpl string, nodes []HTMLNode, edges []HTMLEdge) (string, error) {
	return substituteTemplate(tmpl, nodes, edges)
}

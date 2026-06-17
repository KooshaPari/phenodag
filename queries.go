// queries.go — Common database query helpers to reduce duplication.
// These helpers replace repeated query patterns throughout phenodag_extras.go
// and phenodag_v3.go, reducing code duplication and improving maintainability.
package main

import (
	"database/sql"
	"log"
)

// queryTasksIDStatus queries all tasks returning id,status pairs.
func queryTasksIDStatus(db *sql.DB) map[string]string {
	result := map[string]string{}
	rows, err := db.Query("SELECT id, status FROM tasks")
	if err != nil {
		log.Printf("queryTasksIDStatus: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			log.Printf("queryTasksIDStatus Scan: %v", err)
			continue
		}
		result[id] = status
	}
	return result
}

// queryTaskIDs queries all task IDs.
func queryTaskIDs(db *sql.DB) []string {
	var result []string
	rows, err := db.Query("SELECT id FROM tasks")
	if err != nil {
		log.Printf("queryTaskIDs: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			log.Printf("queryTaskIDs Scan: %v", err)
			continue
		}
		result = append(result, id)
	}
	return result
}

// queryFailedTasksBeforeThreshold queries failed tasks updated before threshold epoch.
func queryFailedTasksBeforeThreshold(db *sql.DB, threshold int64) []string {
	var result []string
	rows, err := db.Query("SELECT id FROM tasks WHERE status='failed' AND updated_at < ?", threshold)
	if err != nil {
		log.Printf("queryFailedTasksBeforeThreshold: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			log.Printf("queryFailedTasksBeforeThreshold Scan: %v", err)
			continue
		}
		result = append(result, id)
	}
	return result
}

// queryTasksMainDAG queries main DAG tasks (not side DAGs) with stage and status.
func queryTasksMainDAG(db *sql.DB) []struct {
	ID     string
	Stage  int
	Status string
} {
	var result []struct {
		ID     string
		Stage  int
		Status string
	}
	rows, err := db.Query(`SELECT id, stage, status FROM tasks WHERE (side_dag='' OR side_dag IS NULL) ORDER BY stage, id`)
	if err != nil {
		log.Printf("queryTasksMainDAG: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id, status string
		var stage int
		if err := rows.Scan(&id, &stage, &status); err != nil {
			log.Printf("queryTasksMainDAG Scan: %v", err)
			continue
		}
		result = append(result, struct {
			ID     string
			Stage  int
			Status string
		}{id, stage, status})
	}
	return result
}

// queryTasksWithSubproject queries tasks with subproject field.
func queryTasksWithSubproject(db *sql.DB) []struct {
	ID         string
	Subproject string
} {
	var result []struct {
		ID         string
		Subproject string
	}
	rows, err := db.Query("SELECT id, COALESCE(subproject,'') FROM tasks WHERE side_dag='' OR side_dag IS NULL ORDER BY id")
	if err != nil {
		log.Printf("queryTasksWithSubproject: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id, sp string
		if err := rows.Scan(&id, &sp); err != nil {
			log.Printf("queryTasksWithSubproject Scan: %v", err)
			continue
		}
		result = append(result, struct {
			ID         string
			Subproject string
		}{id, sp})
	}
	return result
}

// queryTasksMainDAGFull queries main DAG tasks with all fields.
func queryTasksMainDAGFull(db *sql.DB) []struct {
	ID          string
	Stage       int
	Status      string
	Description string
} {
	var result []struct {
		ID          string
		Stage       int
		Status      string
		Description string
	}
	rows, err := db.Query(`SELECT id, stage, status, description FROM tasks
		WHERE (side_dag='' OR side_dag IS NULL) ORDER BY stage, id`)
	if err != nil {
		log.Printf("queryTasksMainDAGFull: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id, status, desc string
		var stage int
		if err := rows.Scan(&id, &stage, &status, &desc); err != nil {
			log.Printf("queryTasksMainDAGFull Scan: %v", err)
			continue
		}
		result = append(result, struct {
			ID          string
			Stage       int
			Status      string
			Description string
		}{id, stage, status, desc})
	}
	return result
}

// queryAllTasksDetails queries all tasks with full details.
func queryAllTasksDetails(db *sql.DB) []struct {
	ID          string
	Stage       int
	Status      string
	Subproject  string
} {
	var result []struct {
		ID          string
		Stage       int
		Status      string
		Subproject  string
	}
	rows, err := db.Query("SELECT id, stage, COALESCE(status,''), COALESCE(subproject,'') FROM tasks")
	if err != nil {
		log.Printf("queryAllTasksDetails: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id, status, sp string
		var stage int
		if err := rows.Scan(&id, &stage, &status, &sp); err != nil {
			log.Printf("queryAllTasksDetails Scan: %v", err)
			continue
		}
		result = append(result, struct {
			ID          string
			Stage       int
			Status      string
			Subproject  string
		}{id, stage, status, sp})
	}
	return result
}

// queryTasksStageSummary queries task counts by stage.
func queryTasksStageSummary(db *sql.DB) []struct {
	Stage int
	Total int
	Done  int
} {
	var result []struct {
		Stage int
		Total int
		Done  int
	}
	rows, err := db.Query(`SELECT stage, COUNT(*), SUM(CASE WHEN status='done' THEN 1 ELSE 0 END) FROM tasks WHERE side_dag='' OR side_dag IS NULL GROUP BY stage ORDER BY stage`)
	if err != nil {
		log.Printf("queryTasksStageSummary: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var stage, total, done int
		if err := rows.Scan(&stage, &total, &done); err != nil {
			log.Printf("queryTasksStageSummary Scan: %v", err)
			continue
		}
		result = append(result, struct {
			Stage int
			Total int
			Done  int
		}{stage, total, done})
	}
	return result
}

// queryIncompleteTasksWithDescription queries incomplete tasks with descriptions.
func queryIncompleteTasksWithDescription(db *sql.DB) []struct {
	ID          string
	Description string
	Subproject  string
} {
	var result []struct {
		ID          string
		Description string
		Subproject  string
	}
	rows, err := db.Query("SELECT id, description, COALESCE(subproject,'') FROM tasks WHERE status NOT IN ('done','failed')")
	if err != nil {
		log.Printf("queryIncompleteTasksWithDescription: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id, desc, sp string
		if err := rows.Scan(&id, &desc, &sp); err != nil {
			log.Printf("queryIncompleteTasksWithDescription Scan: %v", err)
			continue
		}
		result = append(result, struct {
			ID          string
			Description string
			Subproject  string
		}{id, desc, sp})
	}
	return result
}

// queryEdges queries all edges (from_task → to_task).
func queryEdges(db *sql.DB) []struct {
	From, To string
} {
	var result []struct {
		From, To string
	}
	rows, err := db.Query("SELECT from_task, to_task FROM edges")
	if err != nil {
		log.Printf("queryEdges: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var f, t string
		if err := rows.Scan(&f, &t); err != nil {
			log.Printf("queryEdges Scan: %v", err)
			continue
		}
		result = append(result, struct {
			From, To string
		}{f, t})
	}
	return result
}

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
	if err := rows.Err(); err != nil {
		log.Printf("queryTasksIDStatus rows.Err: %v", err)
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
	if err := rows.Err(); err != nil {
		log.Printf("queryTaskIDs rows.Err: %v", err)
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
	if err := rows.Err(); err != nil {
		log.Printf("queryFailedTasksBeforeThreshold rows.Err: %v", err)
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
	if err := rows.Err(); err != nil {
		log.Printf("queryTasksMainDAG rows.Err: %v", err)
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
	if err := rows.Err(); err != nil {
		log.Printf("queryTasksWithSubproject rows.Err: %v", err)
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
	if err := rows.Err(); err != nil {
		log.Printf("queryTasksMainDAGFull rows.Err: %v", err)
	}
	return result
}

// queryAllTasksDetails queries all tasks with full details.
func queryAllTasksDetails(db *sql.DB) []struct {
	ID         string
	Stage      int
	Status     string
	Subproject string
} {
	var result []struct {
		ID         string
		Stage      int
		Status     string
		Subproject string
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
			ID         string
			Stage      int
			Status     string
			Subproject string
		}{id, stage, status, sp})
	}
	if err := rows.Err(); err != nil {
		log.Printf("queryAllTasksDetails rows.Err: %v", err)
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
	if err := rows.Err(); err != nil {
		log.Printf("queryTasksStageSummary rows.Err: %v", err)
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
	if err := rows.Err(); err != nil {
		log.Printf("queryIncompleteTasksWithDescription rows.Err: %v", err)
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
	if err := rows.Err(); err != nil {
		log.Printf("queryEdges rows.Err: %v", err)
	}
	return result
}

// queryAgents queries all agents with their status and last_seen timestamp.
func queryAgents(db *sql.DB) []struct {
	ID       string
	Status   string
	LastSeen string
} {
	var result []struct {
		ID       string
		Status   string
		LastSeen string
	}
	rows, err := db.Query(`SELECT id, COALESCE(status,''), COALESCE(last_seen,'') FROM agents ORDER BY id`)
	if err != nil {
		log.Printf("queryAgents: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id, status, lastSeen string
		if err := rows.Scan(&id, &status, &lastSeen); err != nil {
			log.Printf("queryAgents Scan: %v", err)
			continue
		}
		result = append(result, struct {
			ID       string
			Status   string
			LastSeen string
		}{id, status, lastSeen})
	}
	if err := rows.Err(); err != nil {
		log.Printf("queryAgents rows.Err: %v", err)
	}
	return result
}

// queryTaskCountByAgent counts tasks in a given status for an agent.
func queryTaskCountByAgent(db *sql.DB, agentID, status string) int {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE assigned_agent=? AND status=?`, agentID, status).Scan(&count)
	if err != nil {
		log.Printf("queryTaskCountByAgent: %v", err)
	}
	return count
}

// queryTasksWithAllFields queries all tasks with all available fields.
func queryTasksWithAllFields(db *sql.DB) []struct {
	ID          string
	Stage       int
	Slot        int
	Status      string
	Subproject  string
	Category    string
	Kind        string
	Priority    int
	Description string
} {
	var result []struct {
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
	rows, err := db.Query(`SELECT id, stage, slot, status, COALESCE(subproject,''), COALESCE(category,''), COALESCE(kind,''), COALESCE(priority,0), description FROM tasks ORDER BY stage, id`)
	if err != nil {
		log.Printf("queryTasksWithAllFields: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id, status, sp, cat, kind, desc string
		var stage, slot, priority int
		if err := rows.Scan(&id, &stage, &slot, &status, &sp, &cat, &kind, &priority, &desc); err != nil {
			log.Printf("queryTasksWithAllFields Scan: %v", err)
			continue
		}
		result = append(result, struct {
			ID          string
			Stage       int
			Slot        int
			Status      string
			Subproject  string
			Category    string
			Kind        string
			Priority    int
			Description string
		}{id, stage, slot, status, sp, cat, kind, priority, desc})
	}
	if err := rows.Err(); err != nil {
		log.Printf("queryTasksWithAllFields rows.Err: %v", err)
	}
	return result
}

// queryTasksByStageWithDescription queries tasks in a stage with descriptions.
func queryTasksByStageWithDescription(db *sql.DB) []struct {
	ID          string
	Stage       int
	Subproject  string
	Description string
} {
	var result []struct {
		ID          string
		Stage       int
		Subproject  string
		Description string
	}
	rows, err := db.Query(`SELECT id, stage, COALESCE(subproject,''), description FROM tasks WHERE (side_dag='' OR side_dag IS NULL) ORDER BY stage, id`)
	if err != nil {
		log.Printf("queryTasksByStageWithDescription: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id, sp, desc string
		var stage int
		if err := rows.Scan(&id, &stage, &sp, &desc); err != nil {
			log.Printf("queryTasksByStageWithDescription Scan: %v", err)
			continue
		}
		result = append(result, struct {
			ID          string
			Stage       int
			Subproject  string
			Description string
		}{id, stage, sp, desc})
	}
	if err := rows.Err(); err != nil {
		log.Printf("queryTasksByStageWithDescription rows.Err: %v", err)
	}
	return result
}

// queryTaskCountsByStatus returns task counts for each status.
func queryTaskCountsByStatus(db *sql.DB) map[string]int {
	result := map[string]int{}
	queries := map[string]string{
		"total":       "SELECT COUNT(*) FROM tasks",
		"done":        "SELECT COUNT(*) FROM tasks WHERE status='done'",
		"in_progress": "SELECT COUNT(*) FROM tasks WHERE status='in_progress'",
		"failed":      "SELECT COUNT(*) FROM tasks WHERE status='failed'",
		"ready":       "SELECT COUNT(*) FROM tasks WHERE status='ready'",
		"blocked":     "SELECT COUNT(*) FROM tasks WHERE status='blocked'",
	}
	for key, query := range queries {
		var count int
		err := db.QueryRow(query).Scan(&count)
		if err != nil {
			log.Printf("queryTaskCountsByStatus[%s]: %v", key, err)
		}
		result[key] = count
	}
	return result
}

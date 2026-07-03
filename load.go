package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type loadYAMLItem struct {
	Repo   string `yaml:"repo"`
	Branch string `yaml:"branch"`
	Task   string `yaml:"task"`
	State  string `yaml:"state"`
}

type loadYAMLFile struct {
	Items []loadYAMLItem `yaml:"items"`
	Tasks []loadYAMLItem `yaml:"tasks"`
}

func cmdLoad(args []string) error {
	fs := flag.NewFlagSet("load", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	path := fs.String("yaml", "", "YAML file of repo/branch/task/state tuples")
	agent := fs.String("agent", "load", "agent ID recorded on created claims")
	fs.Parse(args)
	if strings.TrimSpace(*path) == "" {
		return fmt.Errorf("--yaml required")
	}
	if strings.TrimSpace(*agent) == "" {
		return fmt.Errorf("--agent required")
	}
	items, err := readLoadYAML(*path)
	if err != nil {
		return err
	}
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		if err := migrate(db); err != nil {
			return err
		}
		return loadItems(db, items, *agent)
	})
}

func readLoadYAML(path string) ([]loadYAMLItem, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read load yaml %s: %w", path, err)
	}
	var items []loadYAMLItem
	if err := yaml.Unmarshal(raw, &items); err == nil && len(items) > 0 {
		return validateLoadItems(items)
	}
	var file loadYAMLFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse load yaml %s: %w", path, err)
	}
	switch {
	case len(file.Items) > 0:
		items = file.Items
	case len(file.Tasks) > 0:
		items = file.Tasks
	default:
		return nil, fmt.Errorf("load yaml %s: expected a non-empty list or items/tasks", path)
	}
	return validateLoadItems(items)
}

func validateLoadItems(items []loadYAMLItem) ([]loadYAMLItem, error) {
	for i := range items {
		items[i].Repo = strings.TrimSpace(items[i].Repo)
		items[i].Branch = strings.TrimSpace(items[i].Branch)
		items[i].Task = strings.TrimSpace(items[i].Task)
		items[i].State = strings.TrimSpace(items[i].State)
		if items[i].Repo == "" {
			return nil, fmt.Errorf("item %d: repo required", i)
		}
		if items[i].Task == "" {
			return nil, fmt.Errorf("item %d: task required", i)
		}
		if items[i].State == "" {
			items[i].State = "pending"
		}
	}
	return items, nil
}

func loadItems(db *sql.DB, items []loadYAMLItem, agent string) error {
	if len(items) == 0 {
		return errors.New("no load items")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	inserted := 0
	for _, item := range items {
		id, err := insertAdhocTask(tx, adhocTask{
			Description: item.Task,
			Repo:        item.Repo,
			Subproject:  item.Repo,
			Kind:        "task",
			Priority:    5,
			Branch:      item.Branch,
			Status:      item.State,
		})
		if err != nil {
			return fmt.Errorf("insert %s/%s %q: %w", item.Repo, item.Branch, item.Task, err)
		}
		if err := claimTask(tx, agent, id, item.Repo, item.Branch, ""); err != nil {
			return fmt.Errorf("claim %s/%s for %s: %w", item.Repo, item.Branch, id, err)
		}
		inserted++
		fmt.Printf("Loaded %s\n", id)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("loaded %d tasks\n", inserted)
	return nil
}

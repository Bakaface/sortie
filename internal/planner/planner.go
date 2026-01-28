package planner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/aface/ralph-tamer-kit/internal/config"
	"github.com/aface/ralph-tamer-kit/internal/db"
	"github.com/aface/ralph-tamer-kit/internal/task"
)

type Planner struct {
	cfg *config.Config
	db  *db.DB
}

type TaskDefinition struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	BlockedBy   []int  `json:"blocked_by"`
}

func New(cfg *config.Config, database *db.DB) *Planner {
	return &Planner{
		cfg: cfg,
		db:  database,
	}
}

func (p *Planner) PlanFromPRD(prdPath string, force bool) ([]*task.Task, error) {
	// Delete existing tasks if force flag is set
	if force {
		if err := p.db.DeleteAllTasks(); err != nil {
			return nil, fmt.Errorf("failed to delete existing tasks: %w", err)
		}
	}

	content, err := os.ReadFile(prdPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PRD file: %w", err)
	}

	prompt := BuildPlannerPrompt(string(content), p.cfg.Planner.Instructions)

	output, err := p.runClaude(prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to run Claude: %w", err)
	}

	definitions, err := parseTaskDefinitions(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse task definitions: %w", err)
	}

	if len(definitions) == 0 {
		return nil, fmt.Errorf("no tasks extracted from PRD")
	}

	var inputs []db.TaskInput
	for _, def := range definitions {
		// Use title from definition
		title := def.Title
		if title == "" {
			// Fallback: generate title from first 60 chars of description
			title = def.Description
			if len(title) > 60 {
				title = title[:60]
			}
		}

		slug := task.Slugify(title)

		// Convert []int to []int64 for BlockedBy
		var blockedBy []int64
		for _, idx := range def.BlockedBy {
			blockedBy = append(blockedBy, int64(idx))
		}

		inputs = append(inputs, db.TaskInput{
			Title:       title,
			Description: def.Description,
			Slug:        slug,
			Branch:      "", // Will be populated when daemon starts the task
			BlockedBy:   blockedBy,
		})
	}

	tasks, err := p.db.CreateTasksBatch(inputs)
	if err != nil {
		return nil, fmt.Errorf("failed to create tasks: %w", err)
	}

	return tasks, nil
}

func (p *Planner) runClaude(prompt string) (string, error) {
	args := []string{"-p", prompt, "--output-format", "text"}
	args = append(args, p.cfg.Claude.DefaultArgs...)

	cmd := exec.Command(p.cfg.Claude.Command, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude command failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}

func parseTaskDefinitions(output string) ([]TaskDefinition, error) {
	jsonPattern := regexp.MustCompile(`\[[\s\S]*\]`)
	matches := jsonPattern.FindString(output)

	if matches == "" {
		return nil, fmt.Errorf("no JSON array found in output")
	}

	var definitions []TaskDefinition
	if err := json.Unmarshal([]byte(matches), &definitions); err != nil {
		cleanedJSON := cleanJSON(matches)
		if err := json.Unmarshal([]byte(cleanedJSON), &definitions); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	}

	var validDefs []TaskDefinition
	for _, def := range definitions {
		// Validate that task has either title or description
		if strings.TrimSpace(def.Title) != "" || strings.TrimSpace(def.Description) != "" {
			// Initialize BlockedBy to empty slice if nil
			if def.BlockedBy == nil {
				def.BlockedBy = []int{}
			}
			validDefs = append(validDefs, def)
		}
	}

	return validDefs, nil
}

func cleanJSON(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", "")

	return s
}

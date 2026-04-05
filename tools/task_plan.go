package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kayushkin/agentkit"
	"github.com/kayushkin/agentkit/schema"
)

const taskPlanFile = ".task.md"

// TaskStatus represents a task's state.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskDone       TaskStatus = "done"
	TaskBlocked    TaskStatus = "blocked"
)

// TaskItem represents a single task in the plan.
type TaskItem struct {
	Task        string     `json:"task"`
	Description string     `json:"description,omitempty"`
	Status      TaskStatus `json:"status"`
}

// TaskPlan represents the full plan state.
type TaskPlan struct {
	Tasks []TaskItem        `json:"tasks"`
	Notes map[string]string `json:"notes,omitempty"`
}

// TaskPlanBuildCommand is the build command to run when all tasks are done.
// Configurable per-project. Defaults to "go build ./...".
var TaskPlanBuildCommand = "go build ./..."

// TaskPlanTool returns a tool that manages a task plan file.
// The file is always loaded into context by the engine.
// repoRoot is the project directory where .task.md lives.
func TaskPlanTool(repoRoot string) agentkit.Tool {
	return agentkit.Tool{
		Name: "task_plan",
		Description: `Manage your task plan. This is your working memory — it persists across turns and is always visible to you.

Actions:
- "update": Replace the full plan (tasks + notes). Use when restructuring.
- "complete": Mark a task done by index (0-based). Completed tasks are auto-removed next turn.
- "add": Add a new task.
- "note": Set a key-value note (e.g. file structure, API endpoints found).
- "remove_note": Remove a note by key.

The task plan drives your work. When all tasks are complete, a build/test runs automatically.
If it fails, a fix task is added for you. Keep the plan updated as you work.`,
		InputSchema: schema.Props([]string{"action"}, map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"update", "complete", "add", "note", "remove_note"},
				"description": "Action to perform",
			},
			"tasks": map[string]any{
				"type":        "array",
				"description": "Full task list for 'update' action",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"task":        map[string]any{"type": "string", "description": "Task name (imperative: 'Fix auth bug')"},
						"description": map[string]any{"type": "string", "description": "Optional details"},
						"status":      map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "done", "blocked"}},
					},
					"required": []string{"task", "status"},
				},
			},
			"notes": map[string]any{
				"type":        "object",
				"description": "Full notes map for 'update' action",
			},
			"index": map[string]any{
				"type":        "integer",
				"description": "Task index (0-based) for 'complete' action",
			},
			"task": map[string]any{
				"type":        "string",
				"description": "Task name for 'add' action",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Task description for 'add' action",
			},
			"key": map[string]any{
				"type":        "string",
				"description": "Note key for 'note'/'remove_note' action",
			},
			"value": map[string]any{
				"type":        "string",
				"description": "Note value for 'note' action",
			},
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			return runTaskPlan(repoRoot, raw)
		},
	}
}

type taskPlanInput struct {
	Action      string            `json:"action"`
	Tasks       []TaskItem        `json:"tasks,omitempty"`
	Notes       map[string]string `json:"notes,omitempty"`
	Index       *int              `json:"index,omitempty"`
	Task        string            `json:"task,omitempty"`
	Description string            `json:"description,omitempty"`
	Key         string            `json:"key,omitempty"`
	Value       string            `json:"value,omitempty"`
}

func runTaskPlan(repoRoot, raw string) (string, error) {
	var in taskPlanInput
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	plan := loadPlan(repoRoot)

	switch in.Action {
	case "update":
		plan.Tasks = in.Tasks
		if in.Notes != nil {
			plan.Notes = in.Notes
		}

	case "complete":
		if in.Index == nil {
			return "error: 'index' required for complete action", nil
		}
		idx := *in.Index
		if idx < 0 || idx >= len(plan.Tasks) {
			return fmt.Sprintf("error: index %d out of range (0-%d)", idx, len(plan.Tasks)-1), nil
		}
		plan.Tasks[idx].Status = TaskDone

	case "add":
		if in.Task == "" {
			return "error: 'task' required for add action", nil
		}
		plan.Tasks = append(plan.Tasks, TaskItem{
			Task:        in.Task,
			Description: in.Description,
			Status:      TaskPending,
		})

	case "note":
		if in.Key == "" {
			return "error: 'key' required for note action", nil
		}
		if plan.Notes == nil {
			plan.Notes = make(map[string]string)
		}
		plan.Notes[in.Key] = in.Value

	case "remove_note":
		if in.Key == "" {
			return "error: 'key' required for remove_note action", nil
		}
		delete(plan.Notes, in.Key)

	default:
		return fmt.Sprintf("error: unknown action %q", in.Action), nil
	}

	// Auto-remove completed tasks.
	filtered := plan.Tasks[:0]
	completed := 0
	for _, t := range plan.Tasks {
		if t.Status == TaskDone {
			completed++
			continue
		}
		filtered = append(filtered, t)
	}
	plan.Tasks = filtered

	if err := savePlan(repoRoot, plan); err != nil {
		return "", err
	}

	remaining := len(plan.Tasks)

	// Auto-build when all tasks complete.
	if remaining == 0 && completed > 0 {
		buildResult := runBuild(repoRoot)
		if buildResult.success {
			return fmt.Sprintf("✓ %d task(s) completed. All done!\n\n🔨 Auto-build passed:\n%s", completed, buildResult.output), nil
		}
		// Build failed — add fix task.
		if err := AddBuildErrorTask(repoRoot, buildResult.output); err != nil {
			return fmt.Sprintf("✓ %d task(s) completed but auto-build failed (and couldn't save fix task):\n%s", completed, buildResult.output), nil
		}
		// Re-read the plan to show the new task.
		return fmt.Sprintf("✓ %d task(s) completed but auto-build FAILED. Added 'Fix build error' task.\n\n🔨 Build output:\n%s", completed, buildResult.output), nil
	}

	if completed > 0 {
		return fmt.Sprintf("✓ %d task(s) completed and removed. %d remaining.", completed, remaining), nil
	}
	return fmt.Sprintf("Plan updated. %d task(s), %d note(s).", remaining, len(plan.Notes)), nil
}

func planPath(repoRoot string) string {
	return filepath.Join(repoRoot, taskPlanFile)
}

func loadPlan(repoRoot string) *TaskPlan {
	data, err := os.ReadFile(planPath(repoRoot))
	if err != nil {
		return &TaskPlan{Notes: make(map[string]string)}
	}
	plan := parsePlanMD(string(data))
	if plan.Notes == nil {
		plan.Notes = make(map[string]string)
	}
	return plan
}

func savePlan(repoRoot string, plan *TaskPlan) error {
	return os.WriteFile(planPath(repoRoot), []byte(renderPlanMD(plan)), 0644)
}

// parsePlanMD parses the markdown task plan format.
func parsePlanMD(content string) *TaskPlan {
	plan := &TaskPlan{Notes: make(map[string]string)}
	lines := strings.Split(content, "\n")

	section := ""
	var noteKey string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## Tasks") {
			section = "tasks"
			continue
		}
		if strings.HasPrefix(trimmed, "## Notes") {
			section = "notes"
			continue
		}

		switch section {
		case "tasks":
			if strings.HasPrefix(trimmed, "- [") {
				task := parseTaskLine(trimmed)
				if task != nil {
					plan.Tasks = append(plan.Tasks, *task)
				}
			}
		case "notes":
			if strings.HasPrefix(trimmed, "### ") {
				noteKey = strings.TrimPrefix(trimmed, "### ")
			} else if noteKey != "" && trimmed != "" {
				if existing, ok := plan.Notes[noteKey]; ok {
					plan.Notes[noteKey] = existing + "\n" + trimmed
				} else {
					plan.Notes[noteKey] = trimmed
				}
			}
		}
	}
	return plan
}

func parseTaskLine(line string) *TaskItem {
	var status TaskStatus
	var rest string

	if strings.HasPrefix(line, "- [x] ") {
		status = TaskDone
		rest = strings.TrimPrefix(line, "- [x] ")
	} else if strings.HasPrefix(line, "- [~] ") {
		status = TaskInProgress
		rest = strings.TrimPrefix(line, "- [~] ")
	} else if strings.HasPrefix(line, "- [!] ") {
		status = TaskBlocked
		rest = strings.TrimPrefix(line, "- [!] ")
	} else if strings.HasPrefix(line, "- [ ] ") {
		status = TaskPending
		rest = strings.TrimPrefix(line, "- [ ] ")
	} else {
		return nil
	}

	// Split task — description on same line separated by " — "
	parts := strings.SplitN(rest, " — ", 2)
	task := &TaskItem{Task: parts[0], Status: status}
	if len(parts) > 1 {
		task.Description = parts[1]
	}
	return task
}

// renderPlanMD renders the plan as markdown.
func renderPlanMD(plan *TaskPlan) string {
	var b strings.Builder
	b.WriteString("# Task Plan\n\n")

	b.WriteString("## Tasks\n")
	for _, t := range plan.Tasks {
		var check string
		switch t.Status {
		case TaskDone:
			check = "x"
		case TaskInProgress:
			check = "~"
		case TaskBlocked:
			check = "!"
		default:
			check = " "
		}
		b.WriteString(fmt.Sprintf("- [%s] %s", check, t.Task))
		if t.Description != "" {
			b.WriteString(fmt.Sprintf(" — %s", t.Description))
		}
		b.WriteString("\n")
	}

	if len(plan.Notes) > 0 {
		b.WriteString("\n## Notes\n")
		for k, v := range plan.Notes {
			b.WriteString(fmt.Sprintf("### %s\n%s\n\n", k, v))
		}
	}

	return b.String()
}

type buildResult struct {
	success bool
	output  string
}

func runBuild(repoRoot string) buildResult {
	cmd := TaskPlanBuildCommand
	if cmd == "" {
		return buildResult{success: true, output: "no build command configured"}
	}

	proc := exec.Command("bash", "-c", cmd)
	proc.Dir = repoRoot
	proc.Env = append(os.Environ(), "PATH="+os.Getenv("PATH"))
	out, err := proc.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if len(output) > 2000 {
		output = output[:2000] + "\n...(truncated)"
	}
	if err != nil {
		return buildResult{success: false, output: output}
	}
	if output == "" {
		output = "success (no output)"
	}
	return buildResult{success: true, output: output}
}

// LoadPlanContext reads the .task.md file and returns it for context injection.
// Returns empty string if no plan exists.
func LoadPlanContext(repoRoot string) string {
	data, err := os.ReadFile(filepath.Join(repoRoot, taskPlanFile))
	if err != nil {
		return ""
	}
	return string(data)
}

// AllTasksDone returns true if a plan exists and has no remaining tasks.
func AllTasksDone(repoRoot string) bool {
	plan := loadPlan(repoRoot)
	return len(plan.Tasks) == 0
}

// AddBuildErrorTask adds a task for a build failure.
func AddBuildErrorTask(repoRoot string, buildOutput string) error {
	plan := loadPlan(repoRoot)
	// Truncate build output to keep it manageable.
	if len(buildOutput) > 500 {
		buildOutput = buildOutput[:500] + "..."
	}
	plan.Tasks = append(plan.Tasks, TaskItem{
		Task:        "Fix build error",
		Description: buildOutput,
		Status:      TaskPending,
	})
	return savePlan(repoRoot, plan)
}

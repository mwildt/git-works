package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	statusTodo       = "todo"
	statusInProgress = "in_progress"
	statusDone       = "done"

	priorityLow    = "low"
	priorityNormal = "normal"
	priorityHigh   = "high"
)

func validStatus(s string) bool {
	return s == statusTodo || s == statusInProgress || s == statusDone
}

func validPriority(p string) bool {
	return p == priorityLow || p == priorityNormal || p == priorityHigh
}

func priorityRank(p string) int {
	switch p {
	case priorityHigh:
		return 3
	case priorityNormal:
		return 2
	default:
		return 1
	}
}

type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	Note        string     `json:"note,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type fileData struct {
	NextID int    `json:"next_id"`
	Tasks  []Task `json:"tasks"`
}

type Store struct {
	mu   sync.Mutex
	path string
	data fileData
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, data: fileData{NextID: 1, Tasks: []Task{}}}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := s.saveLocked(); err != nil {
				return nil, fmt.Errorf("create %s: %w", path, err)
			}
			return s, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if s.data.Tasks == nil {
		s.data.Tasks = []Task{}
	}
	if s.data.NextID < 1 {
		s.data.NextID = 1
	}
	return s, nil
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(s.path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) findLocked(id string) (*Task, error) {
	for i := range s.data.Tasks {
		if s.data.Tasks[i].ID == id {
			return &s.data.Tasks[i], nil
		}
	}
	return nil, fmt.Errorf("task %s not found", id)
}

func (s *Store) Add(title, description, priority string) (Task, error) {
	if title == "" {
		return Task{}, fmt.Errorf("title must not be empty")
	}
	if priority == "" {
		priority = priorityNormal
	}
	if !validPriority(priority) {
		return Task{}, fmt.Errorf("invalid priority %q: want low, normal or high", priority)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	t := Task{
		ID:          fmt.Sprintf("T-%d", s.data.NextID),
		Title:       title,
		Description: description,
		Status:      statusTodo,
		Priority:    priority,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.data.NextID++
	s.data.Tasks = append(s.data.Tasks, t)
	if err := s.saveLocked(); err != nil {
		return Task{}, err
	}
	return t, nil
}

func (s *Store) List(status string) ([]Task, error) {
	if status != "" && status != "all" && !validStatus(status) {
		return nil, fmt.Errorf("invalid status %q: want todo, in_progress, done or all", status)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Task{}
	for _, t := range s.data.Tasks {
		if status == "" || status == "all" || t.Status == status {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *Store) Next() (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var best *Task
	for i := range s.data.Tasks {
		t := &s.data.Tasks[i]
		if t.Status != statusTodo {
			continue
		}
		if best == nil ||
			priorityRank(t.Priority) > priorityRank(best.Priority) ||
			(priorityRank(t.Priority) == priorityRank(best.Priority) && t.ID < best.ID) {
			best = t
		}
	}
	if best == nil {
		return nil, nil
	}
	now := time.Now().UTC()
	best.Status = statusInProgress
	best.UpdatedAt = now
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	t := *best
	return &t, nil
}

func (s *Store) Update(id string, title, description, priority, status *string) (Task, error) {
	if priority != nil && !validPriority(*priority) {
		return Task{}, fmt.Errorf("invalid priority %q: want low, normal or high", *priority)
	}
	if status != nil && !validStatus(*status) {
		return Task{}, fmt.Errorf("invalid status %q: want todo, in_progress or done", *status)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.findLocked(id)
	if err != nil {
		return Task{}, err
	}
	if title != nil {
		if *title == "" {
			return Task{}, fmt.Errorf("title must not be empty")
		}
		t.Title = *title
	}
	if description != nil {
		t.Description = *description
	}
	if priority != nil {
		t.Priority = *priority
	}
	if status != nil {
		t.Status = *status
		if *status == statusDone && t.CompletedAt == nil {
			now := time.Now().UTC()
			t.CompletedAt = &now
		}
	}
	t.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(); err != nil {
		return Task{}, err
	}
	return *t, nil
}

func (s *Store) Complete(id, note string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.findLocked(id)
	if err != nil {
		return Task{}, err
	}
	now := time.Now().UTC()
	t.Status = statusDone
	t.Note = note
	t.CompletedAt = &now
	t.UpdatedAt = now
	if err := s.saveLocked(); err != nil {
		return Task{}, err
	}
	return *t, nil
}

func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Tasks {
		if s.data.Tasks[i].ID == id {
			s.data.Tasks = append(s.data.Tasks[:i], s.data.Tasks[i+1:]...)
			return s.saveLocked()
		}
	}
	return fmt.Errorf("task %s not found", id)
}

type addInput struct {
	Title       string `json:"title" jsonschema:"Short summary of the task"`
	Description string `json:"description,omitempty" jsonschema:"Optional longer description"`
	Priority    string `json:"priority,omitempty" jsonschema:"Priority of the task: low, normal or high (default: normal)"`
}

type taskOutput struct {
	Task Task `json:"task"`
}

type listInput struct {
	Status string `json:"status,omitempty" jsonschema:"Filter by status: todo, in_progress, done or all (default: all)"`
}

type listOutput struct {
	Count int    `json:"count"`
	Tasks []Task `json:"tasks"`
}

type nextInput struct{}

type nextOutput struct {
	Task    *Task  `json:"task"`
	Message string `json:"message"`
}

type updateInput struct {
	ID          string  `json:"id" jsonschema:"Task ID, e.g. T-3"`
	Title       *string `json:"title,omitempty" jsonschema:"New title"`
	Description *string `json:"description,omitempty" jsonschema:"New description"`
	Priority    *string `json:"priority,omitempty" jsonschema:"New priority: low, normal or high"`
	Status      *string `json:"status,omitempty" jsonschema:"New status: todo, in_progress or done"`
}

type completeInput struct {
	ID   string `json:"id" jsonschema:"Task ID, e.g. T-3"`
	Note string `json:"note,omitempty" jsonschema:"Optional completion note"`
}

type removeInput struct {
	ID string `json:"id" jsonschema:"Task ID, e.g. T-3"`
}

type removeOutput struct {
	ID      string `json:"id"`
	Removed bool   `json:"removed"`
}

const instructions = `Persistent task list for agentic work.
Workflow: call task_next to claim the next open task (it is marked in_progress),
do the work, then finish it with task_complete. Record new work with task_add,
inspect the list with task_list, change fields with task_update and delete
entries with task_remove. Tasks are stored in a JSON file and survive restarts.`

func main() {
	defaultFile := os.Getenv("TASKS_FILE")
	if defaultFile == "" {
		defaultFile = "tasks.json"
	}
	file := flag.String("file", defaultFile, "path to the JSON task file (env TASKS_FILE)")
	flag.Parse()

	store, err := OpenStore(*file)
	if err != nil {
		log.Fatalf("tasks: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "tasks", Version: "0.1.0"}, &mcp.ServerOptions{
		Instructions: instructions,
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_add",
		Description: "Add a new task to the list. Returns the created task with its ID.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in addInput) (*mcp.CallToolResult, taskOutput, error) {
		t, err := store.Add(in.Title, in.Description, in.Priority)
		if err != nil {
			return nil, taskOutput{}, err
		}
		return nil, taskOutput{Task: t}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_list",
		Description: "List tasks, optionally filtered by status (todo, in_progress, done, all).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in listInput) (*mcp.CallToolResult, listOutput, error) {
		tasks, err := store.List(in.Status)
		if err != nil {
			return nil, listOutput{}, err
		}
		return nil, listOutput{Count: len(tasks), Tasks: tasks}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_next",
		Description: "Claim the next open task: returns the highest-priority task with status todo (oldest first on ties) and marks it in_progress. Returns task=null when no open task exists.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ nextInput) (*mcp.CallToolResult, nextOutput, error) {
		t, err := store.Next()
		if err != nil {
			return nil, nextOutput{}, err
		}
		if t == nil {
			return nil, nextOutput{Task: nil, Message: "no open tasks"}, nil
		}
		return nil, nextOutput{Task: t, Message: fmt.Sprintf("claimed %s, status is now in_progress", t.ID)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_update",
		Description: "Update fields of an existing task. Only provided fields are changed.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in updateInput) (*mcp.CallToolResult, taskOutput, error) {
		t, err := store.Update(in.ID, in.Title, in.Description, in.Priority, in.Status)
		if err != nil {
			return nil, taskOutput{}, err
		}
		return nil, taskOutput{Task: t}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_complete",
		Description: "Mark a task as done, optionally with a completion note.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in completeInput) (*mcp.CallToolResult, taskOutput, error) {
		t, err := store.Complete(in.ID, in.Note)
		if err != nil {
			return nil, taskOutput{}, err
		}
		return nil, taskOutput{Task: t}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_remove",
		Description: "Delete a task from the list by ID.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in removeInput) (*mcp.CallToolResult, removeOutput, error) {
		if err := store.Remove(in.ID); err != nil {
			return nil, removeOutput{}, err
		}
		return nil, removeOutput{ID: in.ID, Removed: true}, nil
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("tasks: server exited: %v", err)
	}
}

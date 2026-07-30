// Package storage provides data persistence for the TaskFlow API.
//
// It follows a store-interface pattern so the in-memory implementation
// can be swapped for PostgreSQL (or any other backend) by implementing
// the TaskStore interface.
package storage

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ── Model ──────────────────────────────────────────────────────

// Task represents a unit of work within a project.
type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	ProjectID   string    `json:"project_id"`
	Assignees   []string  `json:"assignees"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TaskNotFound is returned when a task ID does not exist.
type TaskNotFound struct{ ID string }

func (e *TaskNotFound) Error() string {
	return fmt.Sprintf("task not found: %s", e.ID)
}

// ── Store Interface ────────────────────────────────────────────

// TaskStore is the persistence contract for tasks. Handlers and
// services depend on this interface, not a concrete implementation.
type TaskStore interface {
	Create(ctx context.Context, task Task) (Task, error)
	GetByID(ctx context.Context, id string) (Task, error)
	ListByProject(ctx context.Context, projectID string) ([]Task, error)
	Update(ctx context.Context, id string, task Task) (Task, error)
	Delete(ctx context.Context, id string) error
}

// ── In-Memory Implementation ───────────────────────────────────

// InMemoryTaskStore is a concurrency-safe in-memory task store.
// It generates auto-incrementing IDs (e.g. "task-1", "task-2").
type InMemoryTaskStore struct {
	mu     sync.RWMutex
	tasks  map[string]Task
	nextID int
}

// NewInMemoryTaskStore creates an empty in-memory task store.
func NewInMemoryTaskStore() *InMemoryTaskStore {
	return &InMemoryTaskStore{
		tasks:  make(map[string]Task),
		nextID: 1,
	}
}

// Create inserts a new task, assigns it a unique ID, and returns
// the stored copy with timestamps populated.
func (s *InMemoryTaskStore) Create(ctx context.Context, task Task) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	task.ID = fmt.Sprintf("task-%d", s.nextID)
	s.nextID++
	task.CreatedAt = now
	task.UpdatedAt = now

	// Default status
	if task.Status == "" {
		task.Status = "open"
	}

	s.tasks[task.ID] = task
	return task, nil
}

// GetByID retrieves a single task by its ID.
func (s *InMemoryTaskStore) GetByID(ctx context.Context, id string) (Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok {
		return Task{}, &TaskNotFound{ID: id}
	}
	return task, nil
}

// ListByProject returns all tasks belonging to the given project,
// ordered by creation time (oldest first).
func (s *InMemoryTaskStore) ListByProject(ctx context.Context, projectID string) ([]Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Task
	for _, t := range s.tasks {
		if t.ProjectID == projectID {
			result = append(result, t)
		}
	}

	// Sort by CreatedAt ascending (insertion order for in-memory,
	// but we still sort explicitly for deterministic output).
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].CreatedAt.After(result[j].CreatedAt) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result, nil
}

// Update applies partial or full updates to an existing task.
// Zero-value fields (empty string, nil slice) are left unchanged
// unless they were explicitly set. UpdatedAt is always refreshed.
func (s *InMemoryTaskStore) Update(ctx context.Context, id string, updates Task) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return Task{}, &TaskNotFound{ID: id}
	}

	if updates.Title != "" {
		task.Title = updates.Title
	}
	if updates.Description != "" {
		task.Description = updates.Description
	}
	if updates.Status != "" {
		task.Status = updates.Status
	}
	if updates.ProjectID != "" {
		task.ProjectID = updates.ProjectID
	}
	if updates.Assignees != nil {
		task.Assignees = updates.Assignees
	}

	task.UpdatedAt = time.Now().UTC()
	s.tasks[id] = task
	return task, nil
}

// Delete removes a task by ID. Returns an error if the task does
// not exist.
func (s *InMemoryTaskStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[id]; !ok {
		return &TaskNotFound{ID: id}
	}

	delete(s.tasks, id)
	return nil
}

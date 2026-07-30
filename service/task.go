// Package service implements the business-logic layer for the
// TaskFlow API. It depends on the storage.TaskStore interface,
// keeping handlers decoupled from persistence details.
package service

import (
	"context"
	"errors"

	"tessst/storage"
)

// ── Errors ─────────────────────────────────────────────────────

var (
	ErrTitleRequired   = errors.New("task title is required")
	ErrProjectRequired = errors.New("project ID is required")
)

// ── TaskService ────────────────────────────────────────────────

// TaskService bundles task business rules. Handlers call these
// methods instead of accessing storage directly.
type TaskService struct {
	store storage.TaskStore
}

// NewTaskService creates a service backed by the given store.
func NewTaskService(store storage.TaskStore) *TaskService {
	return &TaskService{store: store}
}

// CreateTask validates input and persists a new task.
func (s *TaskService) CreateTask(ctx context.Context, title, description, projectID string, assignees []string) (storage.Task, error) {
	if title == "" {
		return storage.Task{}, ErrTitleRequired
	}
	if projectID == "" {
		return storage.Task{}, ErrProjectRequired
	}

	task := storage.Task{
		Title:       title,
		Description: description,
		ProjectID:   projectID,
		Assignees:   assignees,
		Status:      "open",
	}

	return s.store.Create(ctx, task)
}

// GetProjectTasks retrieves all tasks for a project.
func (s *TaskService) GetProjectTasks(ctx context.Context, projectID string) ([]storage.Task, error) {
	return s.store.ListByProject(ctx, projectID)
}

// UpdateTask applies partial updates to an existing task.
// Fields that are empty are left unchanged (except Status which
// is always accepted if non-empty).
func (s *TaskService) UpdateTask(ctx context.Context, id, title, description, status string, assignees []string) (storage.Task, error) {
	// Verify the task exists first.
	existing, err := s.store.GetByID(ctx, id)
	if err != nil {
		return storage.Task{}, err
	}

	if title == "" {
		title = existing.Title
	}
	if description == "" {
		description = existing.Description
	}
	if status == "" {
		status = existing.Status
	}
	if assignees == nil {
		assignees = existing.Assignees
	}

	updates := storage.Task{
		Title:       title,
		Description: description,
		Status:      status,
		Assignees:   assignees,
	}

	return s.store.Update(ctx, id, updates)
}

// DeleteTask removes a task by ID. Returns an error if the task
// does not exist.
func (s *TaskService) DeleteTask(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

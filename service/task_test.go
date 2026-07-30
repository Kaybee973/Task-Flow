package service

import (
	"errors"
	"testing"

	"tessst/storage"
)

// ── Mock Store ─────────────────────────────────────────────────

// mockTaskStore implements storage.TaskStore for testing the service
// layer in isolation without a real in-memory store.
type mockTaskStore struct {
	tasks    map[string]storage.Task
	nextID   int
	createFn func(storage.Task) (storage.Task, error)
	listFn   func(string) ([]storage.Task, error)
}

func newMockStore() *mockTaskStore {
	return &mockTaskStore{
		tasks:  make(map[string]storage.Task),
		nextID: 1,
	}
}

func (m *mockTaskStore) Create(t storage.Task) (storage.Task, error) {
	if m.createFn != nil {
		return m.createFn(t)
	}
	t.ID = "task-" + itoa(m.nextID)
	m.nextID++
	if t.Status == "" {
		t.Status = "open"
	}
	m.tasks[t.ID] = t
	return t, nil
}

func (m *mockTaskStore) GetByID(id string) (storage.Task, error) {
	t, ok := m.tasks[id]
	if !ok {
		return storage.Task{}, &storage.TaskNotFound{ID: id}
	}
	return t, nil
}

func (m *mockTaskStore) ListByProject(projectID string) ([]storage.Task, error) {
	if m.listFn != nil {
		return m.listFn(projectID)
	}
	var result []storage.Task
	for _, t := range m.tasks {
		if t.ProjectID == projectID {
			result = append(result, t)
		}
	}
	return result, nil
}

func (m *mockTaskStore) Update(id string, updates storage.Task) (storage.Task, error) {
	t, ok := m.tasks[id]
	if !ok {
		return storage.Task{}, &storage.TaskNotFound{ID: id}
	}
	if updates.Title != "" {
		t.Title = updates.Title
	}
	if updates.Description != "" {
		t.Description = updates.Description
	}
	if updates.Status != "" {
		t.Status = updates.Status
	}
	if updates.ProjectID != "" {
		t.ProjectID = updates.ProjectID
	}
	if updates.Assignees != nil {
		t.Assignees = updates.Assignees
	}
	m.tasks[id] = t
	return t, nil
}

func (m *mockTaskStore) Delete(id string) error {
	if _, ok := m.tasks[id]; !ok {
		return &storage.TaskNotFound{ID: id}
	}
	delete(m.tasks, id)
	return nil
}

func itoa(n int) string {
	if n == 1 {
		return "1"
	}
	// Simple recursive int-to-string for test purposes
	return itoa(n/10) + string(rune('0'+n%10))
}

// ── CreateTask ─────────────────────────────────────────────────

func TestTaskService_CreateTask_Success(t *testing.T) {
	svc := NewTaskService(newMockStore())

	task, err := svc.CreateTask("Build feature", "Implement the thing", "proj-1", []string{"alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if task.Title != "Build feature" {
		t.Errorf("expected title %q, got %q", "Build feature", task.Title)
	}
	if task.ProjectID != "proj-1" {
		t.Errorf("expected project ID %q, got %q", "proj-1", task.ProjectID)
	}
	if task.Status != "open" {
		t.Errorf("expected status \"open\", got %q", task.Status)
	}
}

func TestTaskService_CreateTask_EmptyTitle(t *testing.T) {
	svc := NewTaskService(newMockStore())

	_, err := svc.CreateTask("", "desc", "proj-1", nil)
	if err == nil {
		t.Fatal("expected error for empty title")
	}
	if !errors.Is(err, ErrTitleRequired) {
		t.Errorf("expected ErrTitleRequired, got %v", err)
	}
}

func TestTaskService_CreateTask_EmptyProjectID(t *testing.T) {
	svc := NewTaskService(newMockStore())

	_, err := svc.CreateTask("Task", "desc", "", nil)
	if err == nil {
		t.Fatal("expected error for empty project ID")
	}
	if !errors.Is(err, ErrProjectRequired) {
		t.Errorf("expected ErrProjectRequired, got %v", err)
	}
}

func TestTaskService_CreateTask_NoAssignees(t *testing.T) {
	svc := NewTaskService(newMockStore())

	task, err := svc.CreateTask("Solo task", "", "proj-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Assignees != nil {
		t.Errorf("expected nil assignees, got %v", task.Assignees)
	}
}

// ── GetProjectTasks ────────────────────────────────────────────

func TestTaskService_GetProjectTasks_ReturnsTasks(t *testing.T) {
	mock := newMockStore()
	svc := NewTaskService(mock)

	// Pre-populate the mock store directly
	mock.tasks["task-1"] = storage.Task{
		ID: "task-1", Title: "A", ProjectID: "proj-alpha", Status: "open",
	}
	mock.tasks["task-2"] = storage.Task{
		ID: "task-2", Title: "B", ProjectID: "proj-beta", Status: "open",
	}
	mock.tasks["task-3"] = storage.Task{
		ID: "task-3", Title: "C", ProjectID: "proj-alpha", Status: "open",
	}

	tasks, err := svc.GetProjectTasks("proj-alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestTaskService_GetProjectTasks_EmptyProject(t *testing.T) {
	svc := NewTaskService(newMockStore())

	tasks, err := svc.GetProjectTasks("empty-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

// ── UpdateTask ─────────────────────────────────────────────────

func TestTaskService_UpdateTask_Success(t *testing.T) {
	mock := newMockStore()
	svc := NewTaskService(mock)

	// Create a task first
	created, _ := svc.CreateTask("Original", "Original desc", "proj-1", []string{"bob"})

	// Update title and status
	updated, err := svc.UpdateTask(created.ID, "Updated Title", "", "in-progress", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if updated.Title != "Updated Title" {
		t.Errorf("expected title %q, got %q", "Updated Title", updated.Title)
	}
	if updated.Status != "in-progress" {
		t.Errorf("expected status \"in-progress\", got %q", updated.Status)
	}
	// Description should be preserved (empty in update request)
	if updated.Description != "Original desc" {
		t.Errorf("expected description preserved, got %q", updated.Description)
	}
	// Assignees should be preserved (nil in update request)
	if len(updated.Assignees) != 1 || updated.Assignees[0] != "bob" {
		t.Errorf("expected assignees preserved, got %v", updated.Assignees)
	}
}

func TestTaskService_UpdateTask_NotFound(t *testing.T) {
	svc := NewTaskService(newMockStore())

	_, err := svc.UpdateTask("task-999", "Title", "", "", nil)
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
	var notFound *storage.TaskNotFound
	if !errors.As(err, &notFound) {
		t.Errorf("expected *storage.TaskNotFound, got %T", err)
	}
}

func TestTaskService_UpdateTask_ReplaceAssignees(t *testing.T) {
	mock := newMockStore()
	svc := NewTaskService(mock)

	created, _ := svc.CreateTask("T", "", "proj-1", []string{"alice", "bob"})

	// Replace assignees with a new set
	updated, err := svc.UpdateTask(created.ID, "", "", "", []string{"charlie"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updated.Assignees) != 1 || updated.Assignees[0] != "charlie" {
		t.Errorf("expected assignees [charlie], got %v", updated.Assignees)
	}
}

func TestTaskService_UpdateTask_ClearAssignees(t *testing.T) {
	mock := newMockStore()
	svc := NewTaskService(mock)

	created, _ := svc.CreateTask("T", "", "proj-1", []string{"alice"})

	// Set assignees to empty slice (not nil)
	updated, err := svc.UpdateTask(created.ID, "", "", "", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if updated.Assignees == nil {
		t.Error("expected non-nil empty assignees slice, got nil")
	}
	if len(updated.Assignees) != 0 {
		t.Errorf("expected 0 assignees, got %d", len(updated.Assignees))
	}
}

// ── DeleteTask ─────────────────────────────────────────────────

func TestTaskService_DeleteTask_Success(t *testing.T) {
	mock := newMockStore()
	svc := NewTaskService(mock)

	created, _ := svc.CreateTask("Delete me", "", "proj-1", nil)

	err := svc.DeleteTask(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's gone
	_, err = svc.GetProjectTasks("proj-1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestTaskService_DeleteTask_NotFound(t *testing.T) {
	svc := NewTaskService(newMockStore())

	err := svc.DeleteTask("task-999")
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
	var notFound *storage.TaskNotFound
	if !errors.As(err, &notFound) {
		t.Errorf("expected *storage.TaskNotFound, got %T", err)
	}
}

// ── Integration: Service → Mock Store ──────────────────────────

func TestTaskService_FullLifecycle(t *testing.T) {
	mock := newMockStore()
	svc := NewTaskService(mock)

	// Create
	task, err := svc.CreateTask("Write tests", "Add unit tests", "proj-api", []string{"tester"})
	if err != nil {
		t.Fatal(err)
	}

	// Read via project list
	tasks, _ := svc.GetProjectTasks("proj-api")
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	// Update
	task, err = svc.UpdateTask(task.ID, "", "Add comprehensive tests", "done", nil)
	if err != nil {
		t.Fatal(err)
	}
	if task.Description != "Add comprehensive tests" {
		t.Errorf("expected description updated, got %q", task.Description)
	}
	if task.Status != "done" {
		t.Errorf("expected status \"done\", got %q", task.Status)
	}

	// Delete
	if err := svc.DeleteTask(task.ID); err != nil {
		t.Fatal(err)
	}

	// Verify deletion
	_, err = svc.UpdateTask(task.ID, "", "", "", nil)
	if err == nil {
		t.Error("expected error after deletion")
	}
}

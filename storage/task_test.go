package storage

import (
	"testing"
	"time"
)

// ── Create ─────────────────────────────────────────────────────

func TestInMemoryStore_Create_AssignsID(t *testing.T) {
	s := NewInMemoryTaskStore()

	task := Task{Title: "My Task", ProjectID: "proj-1"}
	created, err := s.Create(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if created.ID != "task-1" {
		t.Errorf("expected ID \"task-1\", got %q", created.ID)
	}
}

func TestInMemoryStore_Create_IncrementsID(t *testing.T) {
	s := NewInMemoryTaskStore()

	t1, _ := s.Create(Task{Title: "A", ProjectID: "p1"})
	t2, _ := s.Create(Task{Title: "B", ProjectID: "p1"})
	t3, _ := s.Create(Task{Title: "C", ProjectID: "p1"})

	if t1.ID != "task-1" || t2.ID != "task-2" || t3.ID != "task-3" {
		t.Errorf("expected task-1, task-2, task-3; got %q, %q, %q", t1.ID, t2.ID, t3.ID)
	}
}

func TestInMemoryStore_Create_SetsTimestamps(t *testing.T) {
	s := NewInMemoryTaskStore()
	before := time.Now().UTC()

	task, err := s.Create(Task{Title: "T", ProjectID: "p1"})
	if err != nil {
		t.Fatal(err)
	}

	if task.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
	if task.UpdatedAt.IsZero() {
		t.Error("expected non-zero UpdatedAt")
	}
	if task.CreatedAt.Before(before.Add(-time.Second)) {
		t.Error("CreatedAt seems too far in the past")
	}
	if task.CreatedAt != task.UpdatedAt {
		t.Error("CreatedAt and UpdatedAt should match on creation")
	}
}

func TestInMemoryStore_Create_DefaultStatus(t *testing.T) {
	s := NewInMemoryTaskStore()

	task, _ := s.Create(Task{Title: "T", ProjectID: "p1"})
	if task.Status != "open" {
		t.Errorf("expected status \"open\", got %q", task.Status)
	}
}

func TestInMemoryStore_Create_PreservesExplicitStatus(t *testing.T) {
	s := NewInMemoryTaskStore()

	task, _ := s.Create(Task{Title: "T", ProjectID: "p1", Status: "in-progress"})
	if task.Status != "in-progress" {
		t.Errorf("expected status \"in-progress\", got %q", task.Status)
	}
}

// ── GetByID ────────────────────────────────────────────────────

func TestInMemoryStore_GetByID_Found(t *testing.T) {
	s := NewInMemoryTaskStore()
	created, _ := s.Create(Task{Title: "Find me", ProjectID: "p1"})

	got, err := s.GetByID(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "Find me" {
		t.Errorf("expected title %q, got %q", "Find me", got.Title)
	}
}

func TestInMemoryStore_GetByID_NotFound(t *testing.T) {
	s := NewInMemoryTaskStore()

	_, err := s.GetByID("task-999")
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}

	if _, ok := err.(*TaskNotFound); !ok {
		t.Errorf("expected *TaskNotFound error, got %T", err)
	}
}

// ── ListByProject ──────────────────────────────────────────────

func TestInMemoryStore_ListByProject_ReturnsMatching(t *testing.T) {
	s := NewInMemoryTaskStore()

	s.Create(Task{Title: "Task A", ProjectID: "proj-alpha"})
	s.Create(Task{Title: "Task B", ProjectID: "proj-beta"})
	s.Create(Task{Title: "Task C", ProjectID: "proj-alpha"})

	tasks, err := s.ListByProject("proj-alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestInMemoryStore_ListByProject_EmptyProject(t *testing.T) {
	s := NewInMemoryTaskStore()
	s.Create(Task{Title: "T", ProjectID: "p1"})

	tasks, err := s.ListByProject("non-existent")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestInMemoryStore_ListByProject_OrderedByCreatedAt(t *testing.T) {
	s := NewInMemoryTaskStore()

	t1, _ := s.Create(Task{Title: "First", ProjectID: "p1"})
	time.Sleep(time.Millisecond) // ensure distinct timestamps
	t2, _ := s.Create(Task{Title: "Second", ProjectID: "p1"})
	time.Sleep(time.Millisecond)
	t3, _ := s.Create(Task{Title: "Third", ProjectID: "p1"})

	tasks, _ := s.ListByProject("p1")

	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if tasks[0].ID != t1.ID || tasks[1].ID != t2.ID || tasks[2].ID != t3.ID {
		t.Errorf("expected order %s, %s, %s; got %s, %s, %s",
			t1.ID, t2.ID, t3.ID,
			tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

// ── Update ─────────────────────────────────────────────────────

func TestInMemoryStore_Update_PartialUpdate(t *testing.T) {
	s := NewInMemoryTaskStore()
	created, _ := s.Create(Task{
		Title:       "Original",
		Description: "Original desc",
		ProjectID:   "p1",
		Status:      "open",
		Assignees:   []string{"alice"},
	})

	// Only update title and status
	updated, err := s.Update(created.ID, Task{Title: "Updated", Status: "in-progress"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if updated.Title != "Updated" {
		t.Errorf("expected title \"Updated\", got %q", updated.Title)
	}
	if updated.Status != "in-progress" {
		t.Errorf("expected status \"in-progress\", got %q", updated.Status)
	}
	// These should be preserved
	if updated.Description != "Original desc" {
		t.Errorf("expected description preserved, got %q", updated.Description)
	}
	if len(updated.Assignees) != 1 || updated.Assignees[0] != "alice" {
		t.Errorf("expected assignees preserved, got %v", updated.Assignees)
	}
}

func TestInMemoryStore_Update_RefreshesUpdatedAt(t *testing.T) {
	s := NewInMemoryTaskStore()
	created, _ := s.Create(Task{Title: "T", ProjectID: "p1"})
	originalUpdated := created.UpdatedAt

	time.Sleep(time.Millisecond)
	updated, _ := s.Update(created.ID, Task{Title: "New"})

	if !updated.UpdatedAt.After(originalUpdated) {
		t.Error("expected UpdatedAt to be refreshed after update")
	}
}

func TestInMemoryStore_Update_NotFound(t *testing.T) {
	s := NewInMemoryTaskStore()

	_, err := s.Update("task-999", Task{Title: "Nope"})
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
	if _, ok := err.(*TaskNotFound); !ok {
		t.Errorf("expected *TaskNotFound, got %T", err)
	}
}

func TestInMemoryStore_Update_ReplaceAssignees(t *testing.T) {
	s := NewInMemoryTaskStore()
	created, _ := s.Create(Task{
		Title:     "T",
		ProjectID: "p1",
		Assignees: []string{"alice"},
	})

	// Set assignees explicitly (non-nil empty slice)
	updated, _ := s.Update(created.ID, Task{Assignees: []string{}})

	if updated.Assignees == nil {
		t.Error("expected non-nil empty assignees slice, got nil")
	}
	if len(updated.Assignees) != 0 {
		t.Errorf("expected 0 assignees, got %d", len(updated.Assignees))
	}
}

// ── Delete ─────────────────────────────────────────────────────

func TestInMemoryStore_Delete_RemovesTask(t *testing.T) {
	s := NewInMemoryTaskStore()
	created, _ := s.Create(Task{Title: "Delete me", ProjectID: "p1"})

	err := s.Delete(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = s.GetByID(created.ID)
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestInMemoryStore_Delete_NotFound(t *testing.T) {
	s := NewInMemoryTaskStore()

	err := s.Delete("task-999")
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
	if _, ok := err.(*TaskNotFound); !ok {
		t.Errorf("expected *TaskNotFound, got %T", err)
	}
}

// ── Integration: Full CRUD lifecycle ───────────────────────────

func TestInMemoryStore_FullCRUDLifecycle(t *testing.T) {
	s := NewInMemoryTaskStore()

	// Create
	task, _ := s.Create(Task{
		Title:       "Write docs",
		Description: "Document the API",
		ProjectID:   "proj-42",
		Assignees:   []string{"bob"},
	})
	if task.ID == "" {
		t.Fatal("expected non-empty ID")
	}

	// Read
	got, _ := s.GetByID(task.ID)
	if got.Title != "Write docs" {
		t.Errorf("expected title %q, got %q", "Write docs", got.Title)
	}

	// Update
	got, _ = s.Update(task.ID, Task{Status: "done"})
	if got.Status != "done" {
		t.Errorf("expected status \"done\", got %q", got.Status)
	}

	// List
	tasks, _ := s.ListByProject("proj-42")
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	// Delete
	if err := s.Delete(task.ID); err != nil {
		t.Fatal(err)
	}

	// Verify deleted
	remaining, _ := s.ListByProject("proj-42")
	if len(remaining) != 0 {
		t.Errorf("expected 0 tasks after delete, got %d", len(remaining))
	}
}

// ── Concurrency ────────────────────────────────────────────────

func TestInMemoryStore_ConcurrentAccess(t *testing.T) {
	s := NewInMemoryTaskStore()

	// Create a task first
	task, _ := s.Create(Task{Title: "Concurrent", ProjectID: "p1"})

	done := make(chan struct{})
	// Launch concurrent readers and writers
	go func() {
		for i := 0; i < 10; i++ {
			s.GetByID(task.ID)
			s.ListByProject("p1")
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 10; i++ {
			s.Update(task.ID, Task{Title: "Updated"})
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 5; i++ {
			s.Create(Task{Title: "New", ProjectID: "p1"})
		}
		done <- struct{}{}
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done

	// Verify store is still consistent
	tasks, _ := s.ListByProject("p1")
	// We started with 1, created 5 more = 6 (but updates are no-ops for count)
	if len(tasks) != 6 {
		t.Errorf("expected 6 tasks after concurrent ops, got %d", len(tasks))
	}
}

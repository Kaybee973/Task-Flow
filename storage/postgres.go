package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time check: *PostgresTaskStore implements TaskStore.
var _ TaskStore = (*PostgresTaskStore)(nil)

// PostgresTaskStore implements TaskStore backed by a PostgreSQL
// database via pgx/v5 connection pool.
type PostgresTaskStore struct {
	pool *pgxpool.Pool
}

// NewPostgresTaskStore connects to the database at the given URL
// and ensures the tasks table exists.
//
// The ctx is used for the initial connection and migration; the
// pool manages subsequent queries independently.
func NewPostgresTaskStore(ctx context.Context, databaseURL string) (*PostgresTaskStore, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	// Verify connectivity
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	s := &PostgresTaskStore{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: migrate: %w", err)
	}

	return s, nil
}

// Close shuts down the connection pool.
func (s *PostgresTaskStore) Close() {
	s.pool.Close()
}

// migrate ensures the tasks table and indexes exist.
func (s *PostgresTaskStore) migrate(ctx context.Context) error {
	sql := `
	CREATE TABLE IF NOT EXISTS tasks (
		id          TEXT        PRIMARY KEY,
		title       TEXT        NOT NULL,
		description TEXT        NOT NULL DEFAULT '',
		status      TEXT        NOT NULL DEFAULT 'open',
		project_id  TEXT        NOT NULL,
		assignees   TEXT[]      NOT NULL DEFAULT '{}',
		created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_tasks_project_id
		ON tasks (project_id, created_at ASC);
	`
	_, err := s.pool.Exec(ctx, sql)
	return err
}

// ── TaskStore implementation ───────────────────────────────────

// Create inserts a new task and returns it with generated ID and
// timestamps. IDs are ULID-style timestamps with a random suffix.
func (s *PostgresTaskStore) Create(ctx context.Context, task Task) (Task, error) {
	if task.Status == "" {
		task.Status = "open"
	}

	// Generate a unique ID: timestamp + random hex suffix (64 bits)
	now := time.Now().UTC()
	suffix := make([]byte, 8)
	rand.Read(suffix)
	task.ID = fmt.Sprintf("task-%s-%s", now.Format("20060102150405"), hex.EncodeToString(suffix))
	task.CreatedAt = now
	task.UpdatedAt = now

	// Handle empty assignees — pgx doesn't like nil slices for TEXT[]
	assignees := task.Assignees
	if assignees == nil {
		assignees = []string{}
	}

	sql := `
		INSERT INTO tasks (id, title, description, status, project_id, assignees, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := s.pool.Exec(ctx, sql,
		task.ID, task.Title, task.Description, task.Status,
		task.ProjectID, assignees, task.CreatedAt, task.UpdatedAt,
	)
	if err != nil {
		return Task{}, fmt.Errorf("postgres: create: %w", err)
	}

	return task, nil
}

// GetByID retrieves a single task by ID.
func (s *PostgresTaskStore) GetByID(ctx context.Context, id string) (Task, error) {
	sql := `
		SELECT id, title, description, status, project_id, assignees, created_at, updated_at
		FROM tasks WHERE id = $1
	`
	row := s.pool.QueryRow(ctx, sql, id)

	task, err := scanTask(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Task{}, &TaskNotFound{ID: id}
		}
		return Task{}, fmt.Errorf("postgres: get by id: %w", err)
	}
	return task, nil
}

// ListByProject returns all tasks for a project, ordered by
// creation time ascending.
func (s *PostgresTaskStore) ListByProject(ctx context.Context, projectID string) ([]Task, error) {
	sql := `
		SELECT id, title, description, status, project_id, assignees, created_at, updated_at
		FROM tasks
		WHERE project_id = $1
		ORDER BY created_at ASC
	`
	rows, err := s.pool.Query(ctx, sql, projectID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list by project: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan row: %w", err)
		}
		tasks = append(tasks, task)
	}

	if tasks == nil {
		tasks = []Task{}
	}
	return tasks, nil
}

// Update applies partial updates to an existing task. Only non-zero
// fields are applied. UpdatedAt is always refreshed.
func (s *PostgresTaskStore) Update(ctx context.Context, id string, updates Task) (Task, error) {
	now := time.Now().UTC()

	// Build a dynamic UPDATE statement for non-zero fields.
	// This avoids reading-then-writing while still supporting
	// partial updates.
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if updates.Title != "" {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, updates.Title)
		argIdx++
	}
	if updates.Description != "" {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, updates.Description)
		argIdx++
	}
	if updates.Status != "" {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, updates.Status)
		argIdx++
	}
	if updates.ProjectID != "" {
		setClauses = append(setClauses, fmt.Sprintf("project_id = $%d", argIdx))
		args = append(args, updates.ProjectID)
		argIdx++
	}
	if updates.Assignees != nil {
		setClauses = append(setClauses, fmt.Sprintf("assignees = $%d", argIdx))
		args = append(args, updates.Assignees)
		argIdx++
	}

	// Always update updated_at
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, now)
	argIdx++

	// ID is always the last parameter
	args = append(args, id)

	sql := fmt.Sprintf(
		"UPDATE tasks SET %s WHERE id = $%d",
		joinClauses(setClauses, ", "),
		argIdx,
	)

	tag, err := s.pool.Exec(ctx, sql, args...)
	if err != nil {
		return Task{}, fmt.Errorf("postgres: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Task{}, &TaskNotFound{ID: id}
	}

	// Return the updated row
	return s.GetByID(ctx, id)
}

// Delete removes a task by ID.
func (s *PostgresTaskStore) Delete(ctx context.Context, id string) error {
	sql := `DELETE FROM tasks WHERE id = $1`
	tag, err := s.pool.Exec(ctx, sql, id)
	if err != nil {
		return fmt.Errorf("postgres: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &TaskNotFound{ID: id}
	}
	return nil
}

// ── Helpers ────────────────────────────────────────────────────

// scanner is implemented by pgx.Row and pgx.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

// scanTask scans a single task row from a pgx query result.
func scanTask(row scanner) (Task, error) {
	var t Task
	err := row.Scan(
		&t.ID, &t.Title, &t.Description, &t.Status,
		&t.ProjectID, &t.Assignees, &t.CreatedAt, &t.UpdatedAt,
	)
	return t, err
}

// joinClauses joins strings with a separator. Avoids importing
// strings just for this one helper.
func joinClauses(clauses []string, sep string) string {
	if len(clauses) == 0 {
		return ""
	}
	result := clauses[0]
	for _, c := range clauses[1:] {
		result += sep + c
	}
	return result
}

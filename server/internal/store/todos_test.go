package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	taskflowdb "github.com/youruser/taskflow/internal/db"
)

func newTodoStoreForTest(t *testing.T) (*TodoStore, *UserStore, func()) {
	t.Helper()
	database, err := taskflowdb.Open(filepath.Join(t.TempDir(), "taskflow.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := taskflowdb.Migrate(ctx, database); err != nil {
		_ = database.Close()
		t.Fatalf("migrate db: %v", err)
	}
	events := NewSyncEventStore(database)
	return NewTodoStore(database, events), NewUserStore(database), func() {
		_ = database.Close()
	}
}

func TestTodoStorePersistsDurationMinutes(t *testing.T) {
	todos, users, cleanup := newTodoStoreForTest(t)
	defer cleanup()

	u, err := users.Create(context.Background(), "duration@example.com", "hash", "Duration", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	created, err := todos.Create(context.Background(), u.ID, TodoInput{
		Title:           "写 v1.4.0 发布说明",
		DurationMinutes: 45,
	})
	if err != nil {
		t.Fatalf("create todo: %v", err)
	}
	if created.DurationMinutes != 45 {
		t.Fatalf("created duration_minutes = %d, want 45", created.DurationMinutes)
	}

	updated, err := todos.Update(context.Background(), u.ID, created.ID, TodoInput{
		Title:           created.Title,
		DurationMinutes: 90,
	})
	if err != nil {
		t.Fatalf("update todo: %v", err)
	}
	if updated.DurationMinutes != 90 {
		t.Fatalf("updated duration_minutes = %d, want 90", updated.DurationMinutes)
	}
}

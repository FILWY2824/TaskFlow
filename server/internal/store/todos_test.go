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

func TestTodoStoreFiltersByStartAtAndKeepsCompletedPastTaskInToday(t *testing.T) {
	todos, users, cleanup := newTodoStoreForTest(t)
	defer cleanup()

	u, err := users.Create(context.Background(), "start-filter@example.com", "hash", "Start Filter", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	tomorrow := today.Add(24 * time.Hour)
	yesterday := today.Add(-24 * time.Hour)

	past, err := todos.Create(context.Background(), u.ID, TodoInput{
		Title:   "昨天开始但今天完成",
		StartAt: &yesterday,
	})
	if err != nil {
		t.Fatalf("create past todo: %v", err)
	}
	if _, err := todos.SetCompleted(context.Background(), u.ID, past.ID, true); err != nil {
		t.Fatalf("complete past todo: %v", err)
	}

	current, err := todos.Create(context.Background(), u.ID, TodoInput{
		Title:   "今天开始",
		StartAt: &now,
	})
	if err != nil {
		t.Fatalf("create current todo: %v", err)
	}

	future, err := todos.Create(context.Background(), u.ID, TodoInput{
		Title:   "明天开始",
		StartAt: &tomorrow,
	})
	if err != nil {
		t.Fatalf("create future todo: %v", err)
	}

	got, err := todos.List(context.Background(), u.ID, TodoFilter{
		DueAfter:              &today,
		DueBefore:             &tomorrow,
		IncludePastIncomplete: true,
		IncludeDone:           true,
	})
	if err != nil {
		t.Fatalf("list todos: %v", err)
	}

	ids := make([]int64, 0, len(got))
	for _, item := range got {
		ids = append(ids, item.ID)
	}
	want := []int64{past.ID, current.ID}
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v; future id %d must stay out", ids, want, future.ID)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	}
}

func TestTodoStoreScheduledFilterUsesStartAt(t *testing.T) {
	todos, users, cleanup := newTodoStoreForTest(t)
	defer cleanup()

	u, err := users.Create(context.Background(), "scheduled-start@example.com", "hash", "Scheduled Start", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	start := time.Now().UTC().Add(2 * time.Hour)
	withStart, err := todos.Create(context.Background(), u.ID, TodoInput{
		Title:   "有开始时间",
		StartAt: &start,
	})
	if err != nil {
		t.Fatalf("create scheduled todo: %v", err)
	}
	withoutStart, err := todos.Create(context.Background(), u.ID, TodoInput{
		Title: "无开始时间",
	})
	if err != nil {
		t.Fatalf("create no-start todo: %v", err)
	}

	zero := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	far := time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)
	got, err := todos.List(context.Background(), u.ID, TodoFilter{
		DueAfter:    &zero,
		DueBefore:   &far,
		IncludeDone: true,
	})
	if err != nil {
		t.Fatalf("list todos: %v", err)
	}

	ids := make([]int64, 0, len(got))
	for _, item := range got {
		ids = append(ids, item.ID)
	}
	if len(ids) != 1 || ids[0] != withStart.ID {
		t.Fatalf("ids = %v, want only %d; no-start id %d must stay out", ids, withStart.ID, withoutStart.ID)
	}
}

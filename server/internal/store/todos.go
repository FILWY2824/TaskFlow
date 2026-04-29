package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/youruser/todoalarm/internal/models"
)

type TodoStore struct {
	DB     *sql.DB
	Events *SyncEventStore
}

func NewTodoStore(db *sql.DB, events *SyncEventStore) *TodoStore {
	return &TodoStore{DB: db, Events: events}
}

type TodoInput struct {
	ListID      *int64
	Title       string
	Description string
	Priority    int
	Effort      int
	DueAt       *time.Time
	DueAllDay   bool
	StartAt     *time.Time
	SortOrder   int
	Timezone    string
}

// TodoFilter 列表筛选。所有 Has* 标志为 true 时才启用对应字段。
type TodoFilter struct {
	ListID      *int64
	IsCompleted *bool
	DueAfter    *time.Time // 含
	DueBefore   *time.Time // 不含
	NoDueDate   bool
	Search      string
	Limit       int
	Offset      int
	IncludeDone bool   // 默认不带已完成
	OrderBy     string // "due_at_asc" | "created_desc" | "priority_desc" | "sort_order"
}

func (s *TodoStore) Create(ctx context.Context, userID int64, in TodoInput) (*models.Todo, error) {
	tz := in.Timezone
	if tz == "" {
		tz = "UTC"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	// 校验 list_id 属于该用户
	if in.ListID != nil {
		if err := assertListOwned(ctx, tx, userID, *in.ListID); err != nil {
			return nil, err
		}
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO todos(user_id, list_id, title, description, priority, effort,
			due_at, due_all_day, start_at, sort_order, timezone)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, in.ListID, in.Title, in.Description, in.Priority, in.Effort,
		nullTime(in.DueAt), boolToInt(in.DueAllDay), nullTime(in.StartAt),
		in.SortOrder, tz)
	if err != nil {
		return nil, fmt.Errorf("insert todo: %w", err)
	}
	id, _ := res.LastInsertId()
	if err := s.Events.recordTx(ctx, tx, userID, "todo", id, "created"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, id)
}

func (s *TodoStore) Get(ctx context.Context, userID, id int64) (*models.Todo, error) {
	row := s.DB.QueryRowContext(ctx, todoSelect+`
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, id, userID)
	t, err := scanTodoRow(row)
	if err != nil {
		return nil, err
	}
	return t, nil
}

const todoSelect = `
	SELECT id, user_id, list_id, title, description, priority, effort,
		due_at, due_all_day, start_at, is_completed, completed_at,
		sort_order, timezone, created_at, updated_at
	FROM todos
`

// List 按筛选条件获取 todo。
func (s *TodoStore) List(ctx context.Context, userID int64, f TodoFilter) ([]*models.Todo, error) {
	var (
		conds []string
		args  []any
	)
	conds = append(conds, "user_id = ?", "deleted_at IS NULL")
	args = append(args, userID)

	if f.ListID != nil {
		conds = append(conds, "list_id = ?")
		args = append(args, *f.ListID)
	}
	if f.IsCompleted != nil {
		if *f.IsCompleted {
			conds = append(conds, "is_completed = 1")
		} else {
			conds = append(conds, "is_completed = 0")
		}
	} else if !f.IncludeDone {
		conds = append(conds, "is_completed = 0")
	}
	if f.NoDueDate {
		conds = append(conds, "due_at IS NULL")
	} else {
		if f.DueAfter != nil {
			conds = append(conds, "due_at >= ?")
			args = append(args, f.DueAfter.UTC())
		}
		if f.DueBefore != nil {
			conds = append(conds, "due_at < ?")
			args = append(args, f.DueBefore.UTC())
		}
	}
	if strings.TrimSpace(f.Search) != "" {
		conds = append(conds, "(title LIKE ? OR description LIKE ?)")
		like := "%" + strings.ReplaceAll(f.Search, "%", `\%`) + "%"
		args = append(args, like, like)
	}

	order := "due_at IS NULL, due_at ASC, sort_order ASC, id ASC"
	switch f.OrderBy {
	case "created_desc":
		order = "created_at DESC, id DESC"
	case "priority_desc":
		order = "priority DESC, due_at IS NULL, due_at ASC, id ASC"
	case "sort_order":
		order = "sort_order ASC, id ASC"
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	q := todoSelect + " WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY " + order + " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.Todo
	for rows.Next() {
		t, err := scanTodoRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *TodoStore) Update(ctx context.Context, userID, id int64, in TodoInput) (*models.Todo, error) {
	tz := in.Timezone
	if tz == "" {
		tz = "UTC"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	if in.ListID != nil {
		if err := assertListOwned(ctx, tx, userID, *in.ListID); err != nil {
			return nil, err
		}
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE todos
		SET list_id = ?, title = ?, description = ?, priority = ?, effort = ?,
		    due_at = ?, due_all_day = ?, start_at = ?, sort_order = ?, timezone = ?,
		    updated_at = ?
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, in.ListID, in.Title, in.Description, in.Priority, in.Effort,
		nullTime(in.DueAt), boolToInt(in.DueAllDay), nullTime(in.StartAt),
		in.SortOrder, tz, time.Now().UTC(), id, userID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	if err := s.Events.recordTx(ctx, tx, userID, "todo", id, "updated"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, id)
}

func (s *TodoStore) Delete(ctx context.Context, userID, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		UPDATE todos SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, now, now, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	// 软删除其下属的 reminder_rules
	if _, err := tx.ExecContext(ctx, `
		UPDATE reminder_rules SET deleted_at = ?, is_enabled = 0, updated_at = ?
		WHERE todo_id = ? AND user_id = ? AND deleted_at IS NULL
	`, now, now, id, userID); err != nil {
		return err
	}
	if err := s.Events.recordTx(ctx, tx, userID, "todo", id, "deleted"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *TodoStore) SetCompleted(ctx context.Context, userID, id int64, completed bool) (*models.Todo, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	var completedAt *time.Time
	if completed {
		completedAt = &now
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE todos
		SET is_completed = ?, completed_at = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, boolToInt(completed), nullTime(completedAt), now, id, userID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	if err := s.Events.recordTx(ctx, tx, userID, "todo", id, "updated"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, id)
}

// rowScanner 抽象 sql.Row 与 sql.Rows 的共同接口。
type rowScanner interface {
	Scan(dest ...any) error
}

func scanTodoRow(r *sql.Row) (*models.Todo, error) {
	t, err := scanTodo(r)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func scanTodoRows(r *sql.Rows) (*models.Todo, error) {
	return scanTodo(r)
}

func scanTodo(r rowScanner) (*models.Todo, error) {
	var t models.Todo
	var listID sql.NullInt64
	var dueAt, startAt, completedAt sql.NullTime
	var dueAllDay, isCompleted int
	if err := r.Scan(&t.ID, &t.UserID, &listID, &t.Title, &t.Description,
		&t.Priority, &t.Effort, &dueAt, &dueAllDay, &startAt, &isCompleted, &completedAt,
		&t.SortOrder, &t.Timezone, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	if listID.Valid {
		v := listID.Int64
		t.ListID = &v
	}
	if dueAt.Valid {
		v := dueAt.Time
		t.DueAt = &v
	}
	if startAt.Valid {
		v := startAt.Time
		t.StartAt = &v
	}
	if completedAt.Valid {
		v := completedAt.Time
		t.CompletedAt = &v
	}
	t.DueAllDay = dueAllDay != 0
	t.IsCompleted = isCompleted != 0
	return &t, nil
}

// assertListOwned 在事务里检查 list 属于当前用户且未删除。
func assertListOwned(ctx context.Context, tx *sql.Tx, userID, listID int64) error {
	var ok int
	err := tx.QueryRowContext(ctx, `
		SELECT 1 FROM lists WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, listID, userID).Scan(&ok)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("list %d: %w", listID, ErrNotFound)
		}
		return err
	}
	return nil
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

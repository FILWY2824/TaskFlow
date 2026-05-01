package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/youruser/taskflow/internal/models"
)

type SubtaskStore struct {
	DB     *sql.DB
	Events *SyncEventStore
}

func NewSubtaskStore(db *sql.DB, events *SyncEventStore) *SubtaskStore {
	return &SubtaskStore{DB: db, Events: events}
}

type SubtaskInput struct {
	Title     string
	SortOrder int
}

func (s *SubtaskStore) Create(ctx context.Context, userID, todoID int64, in SubtaskInput) (*models.Subtask, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	if err := assertTodoOwned(ctx, tx, userID, todoID); err != nil {
		return nil, err
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO subtasks(user_id, todo_id, title, sort_order)
		VALUES (?, ?, ?, ?)
	`, userID, todoID, in.Title, in.SortOrder)
	if err != nil {
		return nil, fmt.Errorf("insert subtask: %w", err)
	}
	id, _ := res.LastInsertId()
	if err := s.Events.recordTx(ctx, tx, userID, "subtask", id, "created"); err != nil {
		return nil, err
	}
	// 子任务变化也记一条 todo 更新事件,方便客户端整体重拉
	if err := s.Events.recordTx(ctx, tx, userID, "todo", todoID, "updated"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, id)
}

func (s *SubtaskStore) Get(ctx context.Context, userID, id int64) (*models.Subtask, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, user_id, todo_id, title, is_completed, completed_at, sort_order, created_at, updated_at
		FROM subtasks WHERE id = ? AND user_id = ?
	`, id, userID)
	return scanSubtask(row)
}

func (s *SubtaskStore) ListByTodo(ctx context.Context, userID, todoID int64) ([]*models.Subtask, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, user_id, todo_id, title, is_completed, completed_at, sort_order, created_at, updated_at
		FROM subtasks
		WHERE todo_id = ? AND user_id = ?
		ORDER BY sort_order ASC, id ASC
	`, todoID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.Subtask
	for rows.Next() {
		var st models.Subtask
		var completedAt sql.NullTime
		var isCompleted int
		if err := rows.Scan(&st.ID, &st.UserID, &st.TodoID, &st.Title,
			&isCompleted, &completedAt, &st.SortOrder, &st.CreatedAt, &st.UpdatedAt); err != nil {
			return nil, err
		}
		st.IsCompleted = isCompleted != 0
		if completedAt.Valid {
			v := completedAt.Time
			st.CompletedAt = &v
		}
		out = append(out, &st)
	}
	return out, rows.Err()
}

func (s *SubtaskStore) Update(ctx context.Context, userID, id int64, title string, sortOrder int) (*models.Subtask, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `
		UPDATE subtasks SET title = ?, sort_order = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, title, sortOrder, time.Now().UTC(), id, userID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	st, err := s.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if err := s.Events.recordTx(ctx, tx, userID, "subtask", id, "updated"); err != nil {
		return nil, err
	}
	if err := s.Events.recordTx(ctx, tx, userID, "todo", st.TodoID, "updated"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *SubtaskStore) SetCompleted(ctx context.Context, userID, id int64, completed bool) (*models.Subtask, error) {
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
		UPDATE subtasks SET is_completed = ?, completed_at = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, boolToInt(completed), nullTime(completedAt), now, id, userID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	st, err := s.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if err := s.Events.recordTx(ctx, tx, userID, "subtask", id, "updated"); err != nil {
		return nil, err
	}
	if err := s.Events.recordTx(ctx, tx, userID, "todo", st.TodoID, "updated"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *SubtaskStore) Delete(ctx context.Context, userID, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// 先取 todo_id 用于事件
	var todoID int64
	err = tx.QueryRowContext(ctx,
		`SELECT todo_id FROM subtasks WHERE id = ? AND user_id = ?`, id, userID).Scan(&todoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM subtasks WHERE id = ? AND user_id = ?`, id, userID); err != nil {
		return err
	}
	if err := s.Events.recordTx(ctx, tx, userID, "subtask", id, "deleted"); err != nil {
		return err
	}
	if err := s.Events.recordTx(ctx, tx, userID, "todo", todoID, "updated"); err != nil {
		return err
	}
	return tx.Commit()
}

func scanSubtask(row *sql.Row) (*models.Subtask, error) {
	var st models.Subtask
	var completedAt sql.NullTime
	var isCompleted int
	err := row.Scan(&st.ID, &st.UserID, &st.TodoID, &st.Title,
		&isCompleted, &completedAt, &st.SortOrder, &st.CreatedAt, &st.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	st.IsCompleted = isCompleted != 0
	if completedAt.Valid {
		v := completedAt.Time
		st.CompletedAt = &v
	}
	return &st, nil
}

func assertTodoOwned(ctx context.Context, tx *sql.Tx, userID, todoID int64) error {
	var ok int
	err := tx.QueryRowContext(ctx, `
		SELECT 1 FROM todos WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, todoID, userID).Scan(&ok)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("todo %d: %w", todoID, ErrNotFound)
		}
		return err
	}
	return nil
}

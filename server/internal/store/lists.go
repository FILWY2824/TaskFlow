package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/youruser/todoalarm/internal/models"
)

type ListStore struct {
	DB     *sql.DB
	Events *SyncEventStore
}

func NewListStore(db *sql.DB, events *SyncEventStore) *ListStore {
	return &ListStore{DB: db, Events: events}
}

type ListInput struct {
	Name       string
	Color      string
	Icon       string
	SortOrder  int
	IsDefault  bool
	IsArchived bool
}

func (s *ListStore) Create(ctx context.Context, userID int64, in ListInput) (*models.List, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `
		INSERT INTO lists(user_id, name, color, icon, sort_order, is_default, is_archived)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, userID, in.Name, in.Color, in.Icon, in.SortOrder, boolToInt(in.IsDefault), boolToInt(in.IsArchived))
	if err != nil {
		return nil, fmt.Errorf("insert list: %w", err)
	}
	id, _ := res.LastInsertId()
	if err := s.Events.recordTx(ctx, tx, userID, "list", id, "created"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, id)
}

func (s *ListStore) Get(ctx context.Context, userID, id int64) (*models.List, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, user_id, name, color, icon, sort_order, is_default, is_archived, created_at, updated_at
		FROM lists WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, id, userID)
	return scanList(row)
}

func (s *ListStore) List(ctx context.Context, userID int64) ([]*models.List, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, user_id, name, color, icon, sort_order, is_default, is_archived, created_at, updated_at
		FROM lists
		WHERE user_id = ? AND deleted_at IS NULL
		ORDER BY sort_order ASC, id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.List
	for rows.Next() {
		var l models.List
		var isDefault, isArchived int
		if err := rows.Scan(&l.ID, &l.UserID, &l.Name, &l.Color, &l.Icon, &l.SortOrder,
			&isDefault, &isArchived, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		l.IsDefault = isDefault != 0
		l.IsArchived = isArchived != 0
		out = append(out, &l)
	}
	return out, rows.Err()
}

func (s *ListStore) Update(ctx context.Context, userID, id int64, in ListInput) (*models.List, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `
		UPDATE lists
		SET name = ?, color = ?, icon = ?, sort_order = ?, is_default = ?, is_archived = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, in.Name, in.Color, in.Icon, in.SortOrder, boolToInt(in.IsDefault), boolToInt(in.IsArchived),
		time.Now().UTC(), id, userID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	if err := s.Events.recordTx(ctx, tx, userID, "list", id, "updated"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, id)
}

func (s *ListStore) Delete(ctx context.Context, userID, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		UPDATE lists SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, now, now, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	// 同时清空该 list 下 todo 的 list_id(外键 ON DELETE SET NULL 仅在硬删除时触发,这里软删除手动处理)
	if _, err := tx.ExecContext(ctx, `
		UPDATE todos SET list_id = NULL, updated_at = ?
		WHERE list_id = ? AND user_id = ? AND deleted_at IS NULL
	`, now, id, userID); err != nil {
		return err
	}
	if err := s.Events.recordTx(ctx, tx, userID, "list", id, "deleted"); err != nil {
		return err
	}
	return tx.Commit()
}

func scanList(row *sql.Row) (*models.List, error) {
	var l models.List
	var isDefault, isArchived int
	err := row.Scan(&l.ID, &l.UserID, &l.Name, &l.Color, &l.Icon, &l.SortOrder,
		&isDefault, &isArchived, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	l.IsDefault = isDefault != 0
	l.IsArchived = isArchived != 0
	return &l, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

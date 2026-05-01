package store

import (
	"context"
	"database/sql"

	"github.com/youruser/taskflow/internal/models"
)

type SyncEventStore struct{ DB *sql.DB }

func NewSyncEventStore(db *sql.DB) *SyncEventStore { return &SyncEventStore{DB: db} }

// recordTx 在传入的 tx 中追加一条同步事件。
func (s *SyncEventStore) recordTx(ctx context.Context, tx *sql.Tx, userID int64, entityType string, entityID int64, action string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO sync_events(user_id, entity_type, entity_id, action)
		VALUES (?, ?, ?, ?)
	`, userID, entityType, entityID, action)
	return err
}

// Pull 取自 since(不含)起的同步事件。limit <= 0 时使用默认 500。
func (s *SyncEventStore) Pull(ctx context.Context, userID, since int64, limit int) ([]*models.SyncEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, entity_type, entity_id, action, created_at
		FROM sync_events
		WHERE user_id = ? AND id > ?
		ORDER BY id ASC
		LIMIT ?
	`, userID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.SyncEvent
	for rows.Next() {
		var e models.SyncEvent
		if err := rows.Scan(&e.ID, &e.EntityType, &e.EntityID, &e.Action, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

// LatestCursor 返回该用户最新事件 id,客户端首次同步可作为基线。
func (s *SyncEventStore) LatestCursor(ctx context.Context, userID int64) (int64, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(id), 0) FROM sync_events WHERE user_id = ?
	`, userID)
	var id int64
	err := row.Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	return id, nil
}

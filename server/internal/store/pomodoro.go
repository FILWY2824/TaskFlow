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

// PomodoroStore 番茄专注会话的存储层。
//
// 状态机:
//
//	Start  -> active
//	active -> Complete  -> completed (ended_at = now, actual = clamp(now-started, 0, planned*4))
//	active -> Abandon   -> abandoned (同上,但记 abandoned)
//	已 终态 -> 不允许再切状态
type PomodoroStore struct {
	DB     *sql.DB
	Events *SyncEventStore
}

func NewPomodoroStore(db *sql.DB, events *SyncEventStore) *PomodoroStore {
	return &PomodoroStore{DB: db, Events: events}
}

type PomodoroInput struct {
	TodoID                 *int64
	PlannedDurationSeconds int
	Kind                   string // 默认 "focus"
	Note                   string
}

type PomodoroFilter struct {
	TodoID        *int64
	Status        string
	Kind          string
	StartedAfter  *time.Time
	StartedBefore *time.Time
	Limit         int
	Offset        int
}

const pomodoroSelect = `
	SELECT id, user_id, todo_id, started_at, ended_at,
		planned_duration_seconds, actual_duration_seconds,
		kind, status, note, created_at, updated_at
	FROM pomodoro_sessions
`

func (s *PomodoroStore) Create(ctx context.Context, userID int64, in PomodoroInput) (*models.PomodoroSession, error) {
	if in.PlannedDurationSeconds <= 0 || in.PlannedDurationSeconds > 6*3600 {
		return nil, fmt.Errorf("invalid planned_duration_seconds: must be in (0, 6h]")
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = "focus"
	}
	switch kind {
	case "focus", "short_break", "long_break":
	default:
		return nil, fmt.Errorf("invalid kind: %s", kind)
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	// 校验 todo_id 属于当前用户
	if in.TodoID != nil {
		var ok int
		if err := tx.QueryRowContext(ctx, `
			SELECT 1 FROM todos WHERE id = ? AND user_id = ? AND deleted_at IS NULL
		`, *in.TodoID, userID).Scan(&ok); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("todo %d: %w", *in.TodoID, ErrNotFound)
			}
			return nil, err
		}
	}

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO pomodoro_sessions(user_id, todo_id, started_at, planned_duration_seconds,
			kind, status, note)
		VALUES (?, ?, ?, ?, ?, 'active', ?)
	`, userID, in.TodoID, now, in.PlannedDurationSeconds, kind, in.Note)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	if err := s.Events.recordTx(ctx, tx, userID, "pomodoro", id, "created"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, id)
}

func (s *PomodoroStore) Get(ctx context.Context, userID, id int64) (*models.PomodoroSession, error) {
	row := s.DB.QueryRowContext(ctx, pomodoroSelect+`
		WHERE id = ? AND user_id = ?
	`, id, userID)
	p, err := scanPomodoro(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

func (s *PomodoroStore) List(ctx context.Context, userID int64, f PomodoroFilter) ([]*models.PomodoroSession, error) {
	var (
		conds = []string{"user_id = ?"}
		args  = []any{userID}
	)
	if f.TodoID != nil {
		conds = append(conds, "todo_id = ?")
		args = append(args, *f.TodoID)
	}
	if f.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, f.Status)
	}
	if f.Kind != "" {
		conds = append(conds, "kind = ?")
		args = append(args, f.Kind)
	}
	if f.StartedAfter != nil {
		conds = append(conds, "started_at >= ?")
		args = append(args, f.StartedAfter.UTC())
	}
	if f.StartedBefore != nil {
		conds = append(conds, "started_at < ?")
		args = append(args, f.StartedBefore.UTC())
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	q := pomodoroSelect + " WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.PomodoroSession
	for rows.Next() {
		p, err := scanPomodoro(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PomodoroStore) UpdateNote(ctx context.Context, userID, id int64, note string) (*models.PomodoroSession, error) {
	if len(note) > 5000 {
		return nil, fmt.Errorf("note too long")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `
		UPDATE pomodoro_sessions SET note = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, note, time.Now().UTC(), id, userID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	if err := s.Events.recordTx(ctx, tx, userID, "pomodoro", id, "updated"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, id)
}

// finalize 把 active 会话设为终态(completed/abandoned),计算 actual_duration_seconds。
func (s *PomodoroStore) finalize(ctx context.Context, userID, id int64, status string) (*models.PomodoroSession, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	// 取 started_at 与 planned 计算 actual
	var startedAt time.Time
	var planned int
	var curStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT started_at, planned_duration_seconds, status
		FROM pomodoro_sessions
		WHERE id = ? AND user_id = ?
	`, id, userID).Scan(&startedAt, &planned, &curStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if curStatus != "active" {
		return nil, fmt.Errorf("session %d already %s: %w", id, curStatus, ErrConflict)
	}

	now := time.Now().UTC()
	dur := int(now.Sub(startedAt).Seconds())
	if dur < 0 {
		dur = 0
	}
	// clamp:防止时钟回拨或挂机太久导致离谱大值
	maxDur := planned * 4
	if maxDur < 60 {
		maxDur = 60
	}
	if dur > maxDur {
		dur = maxDur
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE pomodoro_sessions
		SET ended_at = ?, actual_duration_seconds = ?, status = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND status = 'active'
	`, now, dur, status, now, id, userID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// 同时被另一并发写改了
		return nil, ErrConflict
	}
	if err := s.Events.recordTx(ctx, tx, userID, "pomodoro", id, "updated"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, id)
}

func (s *PomodoroStore) Complete(ctx context.Context, userID, id int64) (*models.PomodoroSession, error) {
	return s.finalize(ctx, userID, id, "completed")
}

func (s *PomodoroStore) Abandon(ctx context.Context, userID, id int64) (*models.PomodoroSession, error) {
	return s.finalize(ctx, userID, id, "abandoned")
}

func (s *PomodoroStore) Delete(ctx context.Context, userID, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `
		DELETE FROM pomodoro_sessions WHERE id = ? AND user_id = ?
	`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if err := s.Events.recordTx(ctx, tx, userID, "pomodoro", id, "deleted"); err != nil {
		return err
	}
	return tx.Commit()
}

func scanPomodoro(r rowScanner) (*models.PomodoroSession, error) {
	var p models.PomodoroSession
	var todoID sql.NullInt64
	var endedAt sql.NullTime
	if err := r.Scan(&p.ID, &p.UserID, &todoID, &p.StartedAt, &endedAt,
		&p.PlannedDurationSeconds, &p.ActualDurationSeconds,
		&p.Kind, &p.Status, &p.Note, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	if todoID.Valid {
		v := todoID.Int64
		p.TodoID = &v
	}
	if endedAt.Valid {
		v := endedAt.Time
		p.EndedAt = &v
	}
	return &p, nil
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/youruser/todoalarm/internal/models"
	"github.com/youruser/todoalarm/internal/rrule"
)

type ReminderStore struct {
	DB     *sql.DB
	Events *SyncEventStore
}

func NewReminderStore(db *sql.DB, events *SyncEventStore) *ReminderStore {
	return &ReminderStore{DB: db, Events: events}
}

type ReminderInput struct {
	TodoID          *int64
	Title           string
	TriggerAt       *time.Time // 单次提醒
	RRule           string     // 周期规则
	DTStart         *time.Time
	Timezone        string
	ChannelLocal    bool
	ChannelTelegram bool
	ChannelWebPush  bool
	IsEnabled       bool
	Ringtone        string
	Vibrate         bool
	Fullscreen      bool
}

// validateAndComputeNext 校验输入并计算 next_fire_at。
func (in *ReminderInput) validateAndComputeNext(now time.Time) (*time.Time, error) {
	hasTrigger := in.TriggerAt != nil
	hasRRule := strings.TrimSpace(in.RRule) != ""

	if !hasTrigger && !hasRRule {
		return nil, errors.New("reminder must have trigger_at or rrule")
	}
	if hasTrigger && hasRRule {
		return nil, errors.New("reminder cannot have both trigger_at and rrule")
	}
	if hasRRule && in.DTStart == nil {
		return nil, errors.New("rrule requires dtstart")
	}
	if !in.ChannelLocal && !in.ChannelTelegram && !in.ChannelWebPush {
		return nil, errors.New("at least one channel must be enabled")
	}
	if hasRRule {
		if err := rrule.ValidateRRule(in.RRule, in.Timezone, in.DTStart); err != nil {
			return nil, err
		}
	}
	return rrule.ComputeNextFire(in.TriggerAt, in.RRule, in.DTStart, in.Timezone, now)
}

func (s *ReminderStore) Create(ctx context.Context, userID int64, in ReminderInput) (*models.ReminderRule, error) {
	tz := in.Timezone
	if tz == "" {
		tz = "UTC"
	}
	in.Timezone = tz

	now := time.Now().UTC()
	next, err := in.validateAndComputeNext(now)
	if err != nil {
		return nil, fmt.Errorf("invalid reminder: %w", err)
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	if in.TodoID != nil {
		if err := assertTodoOwned(ctx, tx, userID, *in.TodoID); err != nil {
			return nil, err
		}
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO reminder_rules(user_id, todo_id, title,
			trigger_at, rrule, dtstart, timezone,
			channel_local, channel_telegram, channel_web_push,
			is_enabled, next_fire_at, ringtone, vibrate, fullscreen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, in.TodoID, in.Title,
		nullTime(in.TriggerAt), in.RRule, nullTime(in.DTStart), tz,
		boolToInt(in.ChannelLocal), boolToInt(in.ChannelTelegram), boolToInt(in.ChannelWebPush),
		boolToInt(in.IsEnabled), nullTime(next), in.Ringtone,
		boolToInt(in.Vibrate), boolToInt(in.Fullscreen))
	if err != nil {
		return nil, fmt.Errorf("insert reminder: %w", err)
	}
	id, _ := res.LastInsertId()
	if err := s.Events.recordTx(ctx, tx, userID, "reminder", id, "created"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, id)
}

func (s *ReminderStore) Update(ctx context.Context, userID, id int64, in ReminderInput) (*models.ReminderRule, error) {
	tz := in.Timezone
	if tz == "" {
		tz = "UTC"
	}
	in.Timezone = tz

	now := time.Now().UTC()
	next, err := in.validateAndComputeNext(now)
	if err != nil {
		return nil, fmt.Errorf("invalid reminder: %w", err)
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	if in.TodoID != nil {
		if err := assertTodoOwned(ctx, tx, userID, *in.TodoID); err != nil {
			return nil, err
		}
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE reminder_rules
		SET todo_id = ?, title = ?, trigger_at = ?, rrule = ?, dtstart = ?, timezone = ?,
			channel_local = ?, channel_telegram = ?, channel_web_push = ?,
			is_enabled = ?, next_fire_at = ?, ringtone = ?, vibrate = ?, fullscreen = ?,
			updated_at = ?
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, in.TodoID, in.Title, nullTime(in.TriggerAt), in.RRule, nullTime(in.DTStart), tz,
		boolToInt(in.ChannelLocal), boolToInt(in.ChannelTelegram), boolToInt(in.ChannelWebPush),
		boolToInt(in.IsEnabled), nullTime(next), in.Ringtone,
		boolToInt(in.Vibrate), boolToInt(in.Fullscreen),
		now, id, userID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	if err := s.Events.recordTx(ctx, tx, userID, "reminder", id, "updated"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, id)
}

func (s *ReminderStore) SetEnabled(ctx context.Context, userID, id int64, enabled bool) (*models.ReminderRule, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	// 重新加载并按需重新计算 next_fire_at
	r, err := s.getInTx(ctx, tx, userID, id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var next *time.Time
	if enabled {
		next, err = rrule.ComputeNextFire(r.TriggerAt, r.RRule, r.DTStart, r.Timezone, now)
		if err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE reminder_rules SET is_enabled = ?, next_fire_at = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, boolToInt(enabled), nullTime(next), now, id, userID); err != nil {
		return nil, err
	}
	if err := s.Events.recordTx(ctx, tx, userID, "reminder", id, "updated"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, id)
}

func (s *ReminderStore) Delete(ctx context.Context, userID, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		UPDATE reminder_rules SET deleted_at = ?, is_enabled = 0, updated_at = ?
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, now, now, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if err := s.Events.recordTx(ctx, tx, userID, "reminder", id, "deleted"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *ReminderStore) Get(ctx context.Context, userID, id int64) (*models.ReminderRule, error) {
	row := s.DB.QueryRowContext(ctx, reminderSelect+`
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, id, userID)
	return scanReminderRow(row)
}

func (s *ReminderStore) getInTx(ctx context.Context, tx *sql.Tx, userID, id int64) (*models.ReminderRule, error) {
	row := tx.QueryRowContext(ctx, reminderSelect+`
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, id, userID)
	return scanReminderRow(row)
}

type ReminderFilter struct {
	OnlyEnabled bool
	TodoID      *int64
	Limit       int
	Offset      int
}

func (s *ReminderStore) List(ctx context.Context, userID int64, f ReminderFilter) ([]*models.ReminderRule, error) {
	conds := []string{"user_id = ?", "deleted_at IS NULL"}
	args := []any{userID}
	if f.OnlyEnabled {
		conds = append(conds, "is_enabled = 1")
	}
	if f.TodoID != nil {
		conds = append(conds, "todo_id = ?")
		args = append(args, *f.TodoID)
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	q := reminderSelect +
		" WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY next_fire_at IS NULL, next_fire_at ASC, id ASC LIMIT ? OFFSET ?"
	args = append(args, limit, f.Offset)

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.ReminderRule
	for rows.Next() {
		r, err := scanReminderRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DueReminder 是调度器扫描时拿到的紧凑视图。
//
// 只读取调度器需要的字段(避免每行都重新解析 RRULE 等),并附带用户时区,
// 因为 RRULE 推进 next_fire 必须在用户时区里算。
type DueReminder struct {
	ID         int64
	UserID     int64
	TodoID     *int64
	Title      string
	TriggerAt  *time.Time
	RRule      string
	DTStart    *time.Time
	Timezone   string
	NextFireAt time.Time

	// 投递通道,scheduler 据此分发到 telegram / web push 等。
	ChannelLocal    bool
	ChannelTelegram bool
	ChannelWebPush  bool
}

// ListDue 返回 next_fire_at <= now 且 is_enabled=1、未删除的提醒,按时间升序。
//
// 注意:此处不限制 catch-up 窗口。如果服务器停机 3 天后启动,所有该期间内的
// 周期提醒都会被一次性视为 due,但 RecordFire 只会推进到下一个 > now 的时刻,
// 因此每条规则在追赶后只会触发一次,而不是把所有遗漏的次都补发。
func (s *ReminderStore) ListDue(ctx context.Context, now time.Time, limit int) ([]*DueReminder, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, user_id, todo_id, title, trigger_at, rrule, dtstart, timezone,
			channel_local, channel_telegram, channel_web_push, next_fire_at
		FROM reminder_rules
		WHERE deleted_at IS NULL
		  AND is_enabled = 1
		  AND next_fire_at IS NOT NULL
		  AND next_fire_at <= ?
		ORDER BY next_fire_at ASC, id ASC
		LIMIT ?
	`, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*DueReminder
	for rows.Next() {
		var d DueReminder
		var todoID sql.NullInt64
		var triggerAt, dtstart sql.NullTime
		var local, telegram, webPush int
		if err := rows.Scan(&d.ID, &d.UserID, &todoID, &d.Title,
			&triggerAt, &d.RRule, &dtstart, &d.Timezone,
			&local, &telegram, &webPush, &d.NextFireAt); err != nil {
			return nil, err
		}
		if todoID.Valid {
			v := todoID.Int64
			d.TodoID = &v
		}
		if triggerAt.Valid {
			v := triggerAt.Time
			d.TriggerAt = &v
		}
		if dtstart.Valid {
			v := dtstart.Time
			d.DTStart = &v
		}
		d.ChannelLocal = local != 0
		d.ChannelTelegram = telegram != 0
		d.ChannelWebPush = webPush != 0
		out = append(out, &d)
	}
	return out, rows.Err()
}

// RecordFire 把一条提醒标记为已触发,并写新的 next_fire_at。
//
// nextFire == nil 表示没有下一次(单次提醒已发完,或 RRULE 已枚举完)。
// 调度器会先调用 rrule.ComputeNextFire(... notBefore=now) 拿到结果再传进来,
// 这样 store 自己不依赖 rrule 包,层级更干净。
//
// 同时写一条 sync_event ("reminder", id, "updated"),让客户端通过 sync 拉到新 next_fire_at。
func (s *ReminderStore) RecordFire(ctx context.Context, userID, id int64, firedAt time.Time, nextFire *time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `
		UPDATE reminder_rules
		SET last_fired_at = ?, next_fire_at = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`, firedAt.UTC(), nullTime(nextFire), firedAt.UTC(), id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if err := s.Events.recordTx(ctx, tx, userID, "reminder", id, "updated"); err != nil {
		return err
	}
	return tx.Commit()
}

const reminderSelect = `
	SELECT id, user_id, todo_id, title,
		trigger_at, rrule, dtstart, timezone,
		channel_local, channel_telegram, channel_web_push,
		is_enabled, next_fire_at, last_fired_at,
		ringtone, vibrate, fullscreen, created_at, updated_at
	FROM reminder_rules
`

func scanReminderRow(r *sql.Row) (*models.ReminderRule, error) {
	rr, err := scanReminder(r)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return rr, err
}

func scanReminderRows(r *sql.Rows) (*models.ReminderRule, error) {
	return scanReminder(r)
}

func scanReminder(r rowScanner) (*models.ReminderRule, error) {
	var rule models.ReminderRule
	var todoID sql.NullInt64
	var triggerAt, dtstart, nextFireAt, lastFiredAt sql.NullTime
	var local, telegram, webPush, enabled, vibrate, fullscreen int
	if err := r.Scan(&rule.ID, &rule.UserID, &todoID, &rule.Title,
		&triggerAt, &rule.RRule, &dtstart, &rule.Timezone,
		&local, &telegram, &webPush,
		&enabled, &nextFireAt, &lastFiredAt,
		&rule.Ringtone, &vibrate, &fullscreen,
		&rule.CreatedAt, &rule.UpdatedAt); err != nil {
		return nil, err
	}
	if todoID.Valid {
		v := todoID.Int64
		rule.TodoID = &v
	}
	if triggerAt.Valid {
		v := triggerAt.Time
		rule.TriggerAt = &v
	}
	if dtstart.Valid {
		v := dtstart.Time
		rule.DTStart = &v
	}
	if nextFireAt.Valid {
		v := nextFireAt.Time
		rule.NextFireAt = &v
	}
	if lastFiredAt.Valid {
		v := lastFiredAt.Time
		rule.LastFiredAt = &v
	}
	rule.ChannelLocal = local != 0
	rule.ChannelTelegram = telegram != 0
	rule.ChannelWebPush = webPush != 0
	rule.IsEnabled = enabled != 0
	rule.Vibrate = vibrate != 0
	rule.Fullscreen = fullscreen != 0
	return &rule, nil
}

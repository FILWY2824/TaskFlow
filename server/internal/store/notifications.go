package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// NotificationStore 管理 notifications 与 notification_deliveries。
//
// 一条 notification 表示"某个 reminder 在某时刻被服务端认定为应当触发了一次",
// 是给通知中心 UI 用的;每条投递通道(telegram / local / web_push)再各写一条
// notification_deliveries,作为审计与重试的依据。
type NotificationStore struct {
	DB     *sql.DB
	Events *SyncEventStore
}

func NewNotificationStore(db *sql.DB, events *SyncEventStore) *NotificationStore {
	return &NotificationStore{DB: db, Events: events}
}

// Notification 给客户端的 JSON DTO。
type Notification struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	ReminderRuleID *int64     `json:"reminder_rule_id,omitempty"`
	TodoID         *int64     `json:"todo_id,omitempty"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	FireAt         time.Time  `json:"fire_at"`
	IsRead         bool       `json:"is_read"`
	CreatedAt      time.Time  `json:"created_at"`
	Deliveries     []Delivery `json:"deliveries,omitempty"`
}

// Delivery 一次投递尝试的记录。
type Delivery struct {
	ID             int64      `json:"id"`
	NotificationID int64      `json:"notification_id"`
	Channel        string     `json:"channel"`
	Status         string     `json:"status"`
	Error          string     `json:"error,omitempty"`
	Attempts       int        `json:"attempts"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// CreateInput 调度器投递时的输入。
type CreateNotificationInput struct {
	UserID         int64
	ReminderRuleID *int64
	TodoID         *int64
	Title          string
	Body           string
	FireAt         time.Time
}

// Create 写一条 notification + 一条 sync_event。
//
// 同一 reminder 在同一时刻多次调用是允许的(虽然调度器不应该这么干),不会做去重 ——
// notifications 表纯粹是事实日志,客户端需要自己按 ID 去重。
func (s *NotificationStore) Create(ctx context.Context, in CreateNotificationInput) (*Notification, error) {
	if in.UserID <= 0 {
		return nil, errors.New("user_id required")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `
		INSERT INTO notifications(user_id, reminder_rule_id, todo_id, title, body, fire_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, in.UserID, nullInt64(in.ReminderRuleID), nullInt64(in.TodoID),
		in.Title, in.Body, in.FireAt.UTC())
	if err != nil {
		return nil, fmt.Errorf("insert notification: %w", err)
	}
	id, _ := res.LastInsertId()
	if err := s.Events.recordTx(ctx, tx, in.UserID, "notification", id, "created"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, in.UserID, id)
}

// LogDelivery 写一次投递尝试。status 推荐 "queued"/"delivered"/"failed"/"skipped"。
//
// 失败时 errMsg 应当是 telegram 返回的 description 或 io 层的 error.Error()。
// deliveredAt == nil 的话(失败/排队中)就不写。
func (s *NotificationStore) LogDelivery(ctx context.Context, notificationID int64, channel, status, errMsg string, attempts int, deliveredAt *time.Time) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO notification_deliveries(notification_id, channel, status, error, attempts, delivered_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, notificationID, channel, status, errMsg, attempts, nullTime(deliveredAt))
	return err
}

// Get 取一条 notification(连带其所有投递日志,按时间升序)。
func (s *NotificationStore) Get(ctx context.Context, userID, id int64) (*Notification, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, user_id, reminder_rule_id, todo_id, title, body, fire_at, is_read, created_at
		FROM notifications WHERE id = ? AND user_id = ?
	`, id, userID)
	n, err := scanNotification(row)
	if err != nil {
		return nil, err
	}
	deliveries, err := s.listDeliveries(ctx, n.ID)
	if err != nil {
		return nil, err
	}
	n.Deliveries = deliveries
	return n, nil
}

// NotificationFilter 筛选条件。
type NotificationFilter struct {
	OnlyUnread bool
	Limit      int
	Offset     int
}

// List 列出一个用户的通知,按 fire_at 倒序(最近的在前)。
func (s *NotificationStore) List(ctx context.Context, userID int64, f NotificationFilter) ([]*Notification, error) {
	conds := []string{"user_id = ?"}
	args := []any{userID}
	if f.OnlyUnread {
		conds = append(conds, "is_read = 0")
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, user_id, reminder_rule_id, todo_id, title, body, fire_at, is_read, created_at
	      FROM notifications
	      WHERE ` + strings.Join(conds, " AND ") +
		` ORDER BY fire_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, f.Offset)

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Notification
	for rows.Next() {
		n, err := scanNotificationRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// MarkRead 把一条通知标为已读。
func (s *NotificationStore) MarkRead(ctx context.Context, userID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE notifications SET is_read = 1 WHERE id = ? AND user_id = ? AND is_read = 0
	`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	// 如果它本来就已读,SQL 不改任何行,但我们仍然要确认它真的属于该用户。
	if n == 0 {
		var exists int
		err := s.DB.QueryRowContext(ctx,
			`SELECT 1 FROM notifications WHERE id = ? AND user_id = ?`, id, userID).Scan(&exists)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
	}
	return nil
}

// MarkAllRead 把当前用户所有未读标为已读。返回已修改行数。
func (s *NotificationStore) MarkAllRead(ctx context.Context, userID int64) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE notifications SET is_read = 1 WHERE user_id = ? AND is_read = 0
	`, userID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// UnreadCount 返回未读数。
func (s *NotificationStore) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM notifications WHERE user_id = ? AND is_read = 0
	`, userID)
	var n int64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *NotificationStore) listDeliveries(ctx context.Context, notificationID int64) ([]Delivery, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, notification_id, channel, status, error, attempts, delivered_at, created_at
		FROM notification_deliveries WHERE notification_id = ?
		ORDER BY id ASC
	`, notificationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Delivery
	for rows.Next() {
		var d Delivery
		var deliveredAt sql.NullTime
		if err := rows.Scan(&d.ID, &d.NotificationID, &d.Channel, &d.Status,
			&d.Error, &d.Attempts, &deliveredAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		if deliveredAt.Valid {
			t := deliveredAt.Time
			d.DeliveredAt = &t
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func scanNotification(row *sql.Row) (*Notification, error) {
	n, err := doScanNotification(row)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return n, err
}

func scanNotificationRows(rows *sql.Rows) (*Notification, error) {
	return doScanNotification(rows)
}

func doScanNotification(r rowScanner) (*Notification, error) {
	var n Notification
	var ruleID, todoID sql.NullInt64
	var isRead int
	if err := r.Scan(&n.ID, &n.UserID, &ruleID, &todoID,
		&n.Title, &n.Body, &n.FireAt, &isRead, &n.CreatedAt); err != nil {
		return nil, err
	}
	if ruleID.Valid {
		v := ruleID.Int64
		n.ReminderRuleID = &v
	}
	if todoID.Valid {
		v := todoID.Int64
		n.TodoID = &v
	}
	n.IsRead = isRead != 0
	return &n, nil
}

// nullInt64 把可空指针转成 SQL NULL 友好的形式(database/sql 把 nil 接口当 NULL)。
func nullInt64(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

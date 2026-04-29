package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TelegramStore 管理 telegram_bind_tokens 与 telegram_bindings 两张表。
type TelegramStore struct {
	DB     *sql.DB
	Events *SyncEventStore
}

func NewTelegramStore(db *sql.DB, events *SyncEventStore) *TelegramStore {
	return &TelegramStore{DB: db, Events: events}
}

// BindToken 一次性的绑定凭证。token 是 16 字节(32 hex 字符)随机串。
type BindToken struct {
	Token     string
	UserID    int64
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// IsActive 还在有效期内且未被使用。
func (t *BindToken) IsActive(now time.Time) bool {
	return t.UsedAt == nil && t.ExpiresAt.After(now)
}

// Binding 一条用户 -> chat_id 的绑定。
type Binding struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	ChatID    string    `json:"chat_id"`
	Username  string    `json:"username"`
	IsEnabled bool      `json:"is_enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateBindToken 给 userID 生成一条新的、TTL 内的 bind_token。
// ttl <= 0 时取默认 10 分钟。
func (s *TelegramStore) CreateBindToken(ctx context.Context, userID int64, ttl time.Duration) (*BindToken, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	// 16 字节 = 128 bit,够防爆破。
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("rand: %w", err)
	}
	token := hex.EncodeToString(buf)
	expiresAt := time.Now().UTC().Add(ttl)
	if _, err := s.DB.ExecContext(ctx, `
		INSERT INTO telegram_bind_tokens(token, user_id, expires_at)
		VALUES (?, ?, ?)
	`, token, userID, expiresAt); err != nil {
		return nil, fmt.Errorf("insert bind token: %w", err)
	}
	return &BindToken{Token: token, UserID: userID, ExpiresAt: expiresAt, CreatedAt: time.Now().UTC()}, nil
}

// LookupBindToken 取出 token 详情(不论有效与否,调用方自己看 IsActive)。
func (s *TelegramStore) LookupBindToken(ctx context.Context, token string) (*BindToken, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	row := s.DB.QueryRowContext(ctx, `
		SELECT token, user_id, expires_at, used_at, created_at
		FROM telegram_bind_tokens WHERE token = ?
	`, token)
	var bt BindToken
	var used sql.NullTime
	if err := row.Scan(&bt.Token, &bt.UserID, &bt.ExpiresAt, &used, &bt.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if used.Valid {
		t := used.Time
		bt.UsedAt = &t
	}
	return &bt, nil
}

// ConsumeBindToken 把 token 标记为已使用,并把 chat_id 与该 user 绑定。
//
// 流程是事务性的:
//
//  1. 校验 token 仍然有效(未过期、未使用);
//  2. UPSERT 一条 telegram_bindings 行(同一用户重复点 START 不会报错,会刷新 username);
//  3. 标记 token used_at;
//  4. 写一条 sync_event,客户端通过它知道绑定状态变了。
//
// 如果同一 chat_id 已经被另一个 user_id 绑定,会返回 ErrConflict —— 我们不允许一个 chat 接收多个 user 的提醒,
// 否则 webhook 收到 /start 时无法区分。
func (s *TelegramStore) ConsumeBindToken(ctx context.Context, token, chatID, username string) (*Binding, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("token required")
	}
	if strings.TrimSpace(chatID) == "" {
		return nil, errors.New("chat_id required")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()

	// 1. 找 token
	var userID int64
	var expiresAt time.Time
	var usedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, expires_at, used_at
		FROM telegram_bind_tokens WHERE token = ?
	`, token).Scan(&userID, &expiresAt, &usedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if usedAt.Valid {
		return nil, errors.New("token already used")
	}
	if !expiresAt.After(now) {
		return nil, errors.New("token expired")
	}

	// 2. 检查 chat 是否被别的 user 占用
	var otherUserID int64
	err = tx.QueryRowContext(ctx, `
		SELECT user_id FROM telegram_bindings WHERE chat_id = ? LIMIT 1
	`, chatID).Scan(&otherUserID)
	if err == nil && otherUserID != userID {
		return nil, fmt.Errorf("%w: chat already bound to another user", ErrConflict)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// 3. UPSERT binding(同一 (user_id, chat_id) 已存在则刷新 username 与 is_enabled)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO telegram_bindings(user_id, chat_id, username, is_enabled)
		VALUES (?, ?, ?, 1)
		ON CONFLICT(user_id, chat_id) DO UPDATE SET
			username = excluded.username,
			is_enabled = 1
	`, userID, chatID, username); err != nil {
		return nil, fmt.Errorf("upsert binding: %w", err)
	}

	// 4. 标记 token 已使用
	if _, err := tx.ExecContext(ctx, `
		UPDATE telegram_bind_tokens SET used_at = ? WHERE token = ?
	`, now, token); err != nil {
		return nil, err
	}

	// 5. 取回完整 binding
	var b Binding
	var enabled int
	err = tx.QueryRowContext(ctx, `
		SELECT id, user_id, chat_id, username, is_enabled, created_at
		FROM telegram_bindings WHERE user_id = ? AND chat_id = ?
	`, userID, chatID).Scan(&b.ID, &b.UserID, &b.ChatID, &b.Username, &enabled, &b.CreatedAt)
	if err != nil {
		return nil, err
	}
	b.IsEnabled = enabled != 0

	// 6. sync event:让 web/客户端通过 sync 拉到绑定变化
	if err := s.Events.recordTx(ctx, tx, userID, "telegram_binding", b.ID, "created"); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &b, nil
}

// ListBindings 列出某用户所有绑定。
func (s *TelegramStore) ListBindings(ctx context.Context, userID int64) ([]*Binding, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, user_id, chat_id, username, is_enabled, created_at
		FROM telegram_bindings WHERE user_id = ?
		ORDER BY id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Binding
	for rows.Next() {
		var b Binding
		var enabled int
		if err := rows.Scan(&b.ID, &b.UserID, &b.ChatID, &b.Username, &enabled, &b.CreatedAt); err != nil {
			return nil, err
		}
		b.IsEnabled = enabled != 0
		out = append(out, &b)
	}
	return out, rows.Err()
}

// ListEnabledBindingsForUser 调度器在投递时用。只返回 is_enabled = 1 的绑定。
func (s *TelegramStore) ListEnabledBindingsForUser(ctx context.Context, userID int64) ([]*Binding, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, user_id, chat_id, username, is_enabled, created_at
		FROM telegram_bindings WHERE user_id = ? AND is_enabled = 1
		ORDER BY id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Binding
	for rows.Next() {
		var b Binding
		var enabled int
		if err := rows.Scan(&b.ID, &b.UserID, &b.ChatID, &b.Username, &enabled, &b.CreatedAt); err != nil {
			return nil, err
		}
		b.IsEnabled = enabled != 0
		out = append(out, &b)
	}
	return out, rows.Err()
}

// DeleteBinding 解绑。允许按 binding id(精确) 或 chat_id(为空时不用)。
func (s *TelegramStore) DeleteBinding(ctx context.Context, userID, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `
		DELETE FROM telegram_bindings WHERE id = ? AND user_id = ?
	`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if err := s.Events.recordTx(ctx, tx, userID, "telegram_binding", id, "deleted"); err != nil {
		return err
	}
	return tx.Commit()
}

// CleanupExpiredTokens 清理 24h 之前过期或已使用 24h 的 token。
func (s *TelegramStore) CleanupExpiredTokens(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	res, err := s.DB.ExecContext(ctx, `
		DELETE FROM telegram_bind_tokens
		WHERE expires_at < ? OR (used_at IS NOT NULL AND used_at < ?)
	`, time.Now().UTC(), cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Preference 是 user_preferences 表的一行。Value 是已序列化的字符串,
// 服务端不解释其语义 —— 客户端写什么,客户端读什么。
type Preference struct {
	Scope     string    `json:"scope"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PreferenceStore 用户偏好读写。
//
// 设计:
//   - scope 必须是 web/android/windows/common 之一(由 handler 层校验并强制),
//     存储层只负责忠实存取。
//   - key 限制 <= 64 字符,value <= 4 KB(实质上鼓励客户端只放标量与紧凑 JSON,
//     不要在偏好表里堆大对象 —— 那是 todos / lists 的地盘)。
type PreferenceStore struct{ DB *sql.DB }

func NewPreferenceStore(db *sql.DB) *PreferenceStore { return &PreferenceStore{DB: db} }

// ListByScope 返回指定 scope 下该用户的全部偏好。空 scope 表示返回所有 scope。
// 顺序按 scope, key 字典序,客户端可直接照原样渲染或转 map。
func (s *PreferenceStore) ListByScope(ctx context.Context, userID int64, scope string) ([]Preference, error) {
	q := `SELECT scope, key, value, updated_at
		  FROM user_preferences WHERE user_id = ?`
	args := []any{userID}
	if scope != "" {
		q += ` AND scope = ?`
		args = append(args, scope)
	}
	q += ` ORDER BY scope, key`

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query preferences: %w", err)
	}
	defer rows.Close()

	out := make([]Preference, 0, 16)
	for rows.Next() {
		var p Preference
		if err := rows.Scan(&p.Scope, &p.Key, &p.Value, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get 单条读取。未找到返回 ErrNotFound。
func (s *PreferenceStore) Get(ctx context.Context, userID int64, scope, key string) (*Preference, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT scope, key, value, updated_at
		FROM user_preferences
		WHERE user_id = ? AND scope = ? AND key = ?
	`, userID, scope, key)
	var p Preference
	err := row.Scan(&p.Scope, &p.Key, &p.Value, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

// Upsert 写入或更新单条。返回写入后的最新值。
func (s *PreferenceStore) Upsert(ctx context.Context, userID int64, scope, key, value string) (*Preference, error) {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO user_preferences(user_id, scope, key, value, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, scope, key)
		DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, userID, scope, key, value)
	if err != nil {
		return nil, fmt.Errorf("upsert preference: %w", err)
	}
	return s.Get(ctx, userID, scope, key)
}

// BulkUpsert 在单事务里写多个偏好,常用于客户端启动时一次性同步本地状态到服务端。
// items 中各项 scope/key 必须由调用方先校验。
func (s *PreferenceStore) BulkUpsert(ctx context.Context, userID int64, items []Preference) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO user_preferences(user_id, scope, key, value, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, scope, key)
		DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range items {
		if _, err := stmt.ExecContext(ctx, userID, p.Scope, p.Key, p.Value); err != nil {
			return fmt.Errorf("upsert %s/%s: %w", p.Scope, p.Key, err)
		}
	}
	return tx.Commit()
}

// Delete 删一条。未找到不报错(幂等)。
func (s *PreferenceStore) Delete(ctx context.Context, userID int64, scope, key string) error {
	_, err := s.DB.ExecContext(ctx, `
		DELETE FROM user_preferences
		WHERE user_id = ? AND scope = ? AND key = ?
	`, userID, scope, key)
	return err
}

// IsAllowedScope 校验 scope 字符串是否合法。供 handler 在写入前调用。
func IsAllowedScope(s string) bool {
	switch strings.ToLower(s) {
	case "web", "android", "windows", "common":
		return true
	}
	return false
}

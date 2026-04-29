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

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

// UserStore 用户表读写。
type UserStore struct{ DB *sql.DB }

func NewUserStore(db *sql.DB) *UserStore { return &UserStore{DB: db} }

func (s *UserStore) Create(ctx context.Context, email, passwordHash, displayName, timezone string) (*models.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if timezone == "" {
		timezone = "UTC"
	}
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO users(email, password_hash, display_name, timezone)
		VALUES (?, ?, ?, ?)
	`, email, passwordHash, displayName, timezone)
	if err != nil {
		if isUniqueErr(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetByID(ctx, id)
}

func (s *UserStore) GetByID(ctx context.Context, id int64) (*models.User, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, email, display_name, timezone, created_at, updated_at
		FROM users WHERE id = ?
	`, id)
	return scanUser(row)
}

// GetByEmailWithHash 同时返回 password_hash。仅供登录使用,不暴露给上层 JSON。
func (s *UserStore) GetByEmailWithHash(ctx context.Context, email string) (*models.User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, email, display_name, timezone, created_at, updated_at, password_hash
		FROM users WHERE email = ?
	`, email)
	var u models.User
	var hash string
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Timezone, &u.CreatedAt, &u.UpdatedAt, &hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	return &u, hash, nil
}

func scanUser(row *sql.Row) (*models.User, error) {
	var u models.User
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Timezone, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// RefreshTokenStore 管理 refresh tokens。token_hash 是 SHA-256(token)。
type RefreshTokenStore struct{ DB *sql.DB }

func NewRefreshTokenStore(db *sql.DB) *RefreshTokenStore { return &RefreshTokenStore{DB: db} }

func (s *RefreshTokenStore) Create(ctx context.Context, userID int64, tokenHash, deviceID string, expiresAt time.Time) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO refresh_tokens(user_id, token_hash, device_id, expires_at)
		VALUES (?, ?, ?, ?)
	`, userID, tokenHash, deviceID, expiresAt.UTC())
	return err
}

// LookupActive 返回有效(未过期、未撤销)的 token 对应的 user_id。
func (s *RefreshTokenStore) LookupActive(ctx context.Context, tokenHash string) (int64, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT user_id FROM refresh_tokens
		WHERE token_hash = ? AND revoked_at IS NULL AND expires_at > ?
	`, tokenHash, time.Now().UTC())
	var uid int64
	if err := row.Scan(&uid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return uid, nil
}

// Revoke 撤销单个 token(按 hash)。
func (s *RefreshTokenStore) Revoke(ctx context.Context, tokenHash string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE refresh_tokens SET revoked_at = ? WHERE token_hash = ? AND revoked_at IS NULL
	`, time.Now().UTC(), tokenHash)
	return err
}

// RevokeAllForUser 撤销某用户的全部 token(用于"退出所有设备")。
func (s *RefreshTokenStore) RevokeAllForUser(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE refresh_tokens SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL
	`, time.Now().UTC(), userID)
	return err
}

// CleanupExpired 周期清理过期/已撤销超过 7 天的 token,降低表体积。
func (s *RefreshTokenStore) CleanupExpired(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	res, err := s.DB.ExecContext(ctx, `
		DELETE FROM refresh_tokens
		WHERE expires_at < ? OR (revoked_at IS NOT NULL AND revoked_at < ?)
	`, time.Now().UTC(), cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// isUniqueErr 简单识别 SQLite 唯一约束冲突。
func isUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// modernc.org/sqlite 的错误形如:"constraint failed: UNIQUE constraint failed: users.email (2067)"
	return strings.Contains(msg, "UNIQUE constraint") || strings.Contains(msg, "constraint failed")
}

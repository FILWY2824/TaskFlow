package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/youruser/taskflow/internal/models"
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

// Update 修改 display_name / timezone。任意为 nil 表示该字段不变。
// 只触碰非 nil 的字段；timezone 传 "" 视为重置为 "UTC"。
func (s *UserStore) Update(ctx context.Context, id int64, displayName, timezone *string) (*models.User, error) {
	sets := []string{}
	args := []any{}
	if displayName != nil {
		sets = append(sets, "display_name = ?")
		args = append(args, *displayName)
	}
	if timezone != nil {
		v := *timezone
		if v == "" {
			v = "UTC"
		}
		sets = append(sets, "timezone = ?")
		args = append(args, v)
	}
	if len(sets) == 0 {
		return s.GetByID(ctx, id)
	}
	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id)
	q := "UPDATE users SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	if _, err := s.DB.ExecContext(ctx, q, args...); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	return s.GetByID(ctx, id)
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

// GetByOAuth 按 (provider, subject) 查找已绑定外部 IdP 的用户。
// 不存在返回 ErrNotFound。
func (s *UserStore) GetByOAuth(ctx context.Context, provider, subject string) (*models.User, error) {
	provider = strings.TrimSpace(provider)
	subject = strings.TrimSpace(subject)
	if provider == "" || subject == "" {
		return nil, ErrNotFound
	}
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, email, display_name, timezone, created_at, updated_at
		FROM users WHERE oauth_provider = ? AND oauth_subject = ?
	`, provider, subject)
	return scanUser(row)
}

// UpsertOAuth 按 (provider, subject) 找用户;找不到则创建。
//
// 创建逻辑:
//  1. 优先用认证中心给的 email 当本地 email;若该 email 已被本地账号占用(同一邮箱
//     之前注册过),为避免与既有的本地账号合并造成越权,改用占位邮箱 sub@provider。
//     管理员可以事后再决定是否手动合并。
//  2. password_hash 留空字符串,这种用户走不通本地登录(本地登录用 bcrypt.Compare,
//     空哈希必然失败),OAuth 流程才能进。
//  3. timezone 默认 UTC,展示名取 displayName,空则用 email 的 local-part。
//
// 已存在时只更新易变字段(email / display_name);timezone 由用户自己在设置里改,
// 不被覆盖。
func (s *UserStore) UpsertOAuth(ctx context.Context, provider, subject, email, displayName string) (*models.User, error) {
	provider = strings.TrimSpace(provider)
	subject = strings.TrimSpace(subject)
	if provider == "" || subject == "" {
		return nil, fmt.Errorf("provider and subject required")
	}
	emailLow := strings.ToLower(strings.TrimSpace(email))
	displayName = strings.TrimSpace(displayName)

	// 已绑定?直接返回(并尽量同步邮箱/展示名)。
	if u, err := s.GetByOAuth(ctx, provider, subject); err == nil {
		// 仅在新值非空且与旧值不同时更新,避免无意义写入(也避免触发 updated_at)。
		if (emailLow != "" && emailLow != u.Email) || (displayName != "" && displayName != u.DisplayName) {
			newEmail := u.Email
			if emailLow != "" {
				newEmail = emailLow
			}
			newName := u.DisplayName
			if displayName != "" {
				newName = displayName
			}
			_, err := s.DB.ExecContext(ctx, `
				UPDATE users SET email = ?, display_name = ?, updated_at = CURRENT_TIMESTAMP
				WHERE id = ?
			`, newEmail, newName, u.ID)
			// 邮箱冲突(本地用户占用了同一邮箱)就不更新邮箱,只更新展示名。
			if err != nil && isUniqueErr(err) {
				_, _ = s.DB.ExecContext(ctx, `
					UPDATE users SET display_name = ?, updated_at = CURRENT_TIMESTAMP
					WHERE id = ?
				`, newName, u.ID)
			} else if err != nil {
				return nil, fmt.Errorf("update oauth user: %w", err)
			}
		}
		return s.GetByID(ctx, u.ID)
	}

	// 未绑定 —— 新建。
	insertEmail := emailLow
	if insertEmail == "" {
		insertEmail = subject + "@" + provider
	}
	if displayName == "" {
		if at := strings.IndexByte(insertEmail, '@'); at > 0 {
			displayName = insertEmail[:at]
		} else {
			displayName = insertEmail
		}
	}
	tryInsert := func(emailToUse string) (int64, error) {
		res, err := s.DB.ExecContext(ctx, `
			INSERT INTO users(email, password_hash, display_name, timezone, oauth_provider, oauth_subject)
			VALUES (?, '', ?, 'UTC', ?, ?)
		`, emailToUse, displayName, provider, subject)
		if err != nil {
			return 0, err
		}
		id, _ := res.LastInsertId()
		return id, nil
	}
	id, err := tryInsert(insertEmail)
	if err != nil {
		// 邮箱已被占用 —— 退回到 sub@provider 占位邮箱,避免与本地账号无意中并列。
		if isUniqueErr(err) && insertEmail != subject+"@"+provider {
			id, err = tryInsert(subject + "@" + provider)
		}
		if err != nil {
			return nil, fmt.Errorf("create oauth user: %w", err)
		}
	}
	return s.GetByID(ctx, id)
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

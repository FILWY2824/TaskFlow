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
		SELECT id, email, display_name, timezone, is_admin, is_disabled, created_at, updated_at
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
		SELECT id, email, display_name, timezone, is_admin, is_disabled, created_at, updated_at, password_hash
		FROM users WHERE email = ?
	`, email)
	var u models.User
	var hash string
	var isAdmin, isDisabled int
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Timezone, &isAdmin, &isDisabled, &u.CreatedAt, &u.UpdatedAt, &hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	u.IsAdmin = isAdmin == 1
	u.IsDisabled = isDisabled == 1
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
		SELECT id, email, display_name, timezone, is_admin, is_disabled, created_at, updated_at
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
	var isAdmin, isDisabled int
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Timezone, &isAdmin, &isDisabled, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.IsAdmin = isAdmin == 1
	u.IsDisabled = isDisabled == 1
	return &u, nil
}

// ===== 管理员能力(管理面板新增) =====

// CountAdmins 统计当前管理员数量。
func (s *UserStore) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&n)
	return n, err
}

// SetAdmin 设置某用户的管理员标志。lastAdminGuard=true 时,
// 如果当前只剩一个管理员且就是 id 自己,且要求 isAdmin=false,会返回 ErrConflict
// 防止"摘掉自己"导致管理面板永远进不去。
func (s *UserStore) SetAdmin(ctx context.Context, id int64, isAdmin bool, lastAdminGuard bool) error {
	if !isAdmin && lastAdminGuard {
		var n int
		if err := s.DB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE is_admin = 1 AND id != ?`, id).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ErrConflict
		}
	}
	v := 0
	if isAdmin {
		v = 1
	}
	res, err := s.DB.ExecContext(ctx, `
		UPDATE users SET is_admin = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, v, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDisabled 设置某用户的禁用标志。禁用后,其本地登录会失败,
// refresh token 流程也会失败(见 LoginDisabled 在 GetByEmailWithHash 后由 handler 检查)。
// 注意:已签发的 access token 在过期前仍然有效,这是 JWT 的固有特性。
func (s *UserStore) SetDisabled(ctx context.Context, id int64, isDisabled bool) error {
	v := 0
	if isDisabled {
		v = 1
	}
	res, err := s.DB.ExecContext(ctx, `
		UPDATE users SET is_disabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, v, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete 永久删除用户。其名下所有数据(lists/todos/subtasks/...)由
// 各表的 ON DELETE CASCADE 一起带走;refresh_tokens 也会随之失效。
func (s *UserStore) Delete(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AdminUserRow 管理员后台展示用的扩展用户信息(带统计数)。
type AdminUserRow struct {
	models.User
	TodoCount   int        `json:"todo_count"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// ListForAdmin 列出全部用户(分页),带 todo 计数与最后一次活跃 refresh token 时间。
// search 非空时按 email/display_name 模糊匹配(不区分大小写)。
func (s *UserStore) ListForAdmin(ctx context.Context, search string, limit, offset int) ([]AdminUserRow, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	search = strings.TrimSpace(search)
	likeArg := "%" + strings.ToLower(search) + "%"

	// 总数
	var total int
	if search == "" {
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
			return nil, 0, err
		}
	} else {
		if err := s.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM users
			WHERE LOWER(email) LIKE ? OR LOWER(display_name) LIKE ?
		`, likeArg, likeArg).Scan(&total); err != nil {
			return nil, 0, err
		}
	}

	// 主查询。todo_count 用 LEFT JOIN 子查询(只算未软删的 todo)。
	// last_login_at 取该用户最近一次未撤销 refresh token 的 created_at,作为"最近活跃"近似指标。
	q := `
		SELECT u.id, u.email, u.display_name, u.timezone, u.is_admin, u.is_disabled,
		       u.created_at, u.updated_at,
		       COALESCE(tc.n, 0) AS todo_count,
		       rt.last_seen
		FROM users u
		LEFT JOIN (
			SELECT user_id, COUNT(*) AS n FROM todos WHERE deleted_at IS NULL GROUP BY user_id
		) tc ON tc.user_id = u.id
		LEFT JOIN (
			SELECT user_id, MAX(created_at) AS last_seen FROM refresh_tokens GROUP BY user_id
		) rt ON rt.user_id = u.id
	`
	args := []any{}
	if search != "" {
		q += ` WHERE LOWER(u.email) LIKE ? OR LOWER(u.display_name) LIKE ? `
		args = append(args, likeArg, likeArg)
	}
	q += ` ORDER BY u.id ASC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []AdminUserRow{}
	for rows.Next() {
		var r AdminUserRow
		var isAdmin, isDisabled int
		var lastSeen sql.NullTime
		if err := rows.Scan(&r.ID, &r.Email, &r.DisplayName, &r.Timezone,
			&isAdmin, &isDisabled, &r.CreatedAt, &r.UpdatedAt,
			&r.TodoCount, &lastSeen); err != nil {
			return nil, 0, err
		}
		r.IsAdmin = isAdmin == 1
		r.IsDisabled = isDisabled == 1
		if lastSeen.Valid {
			t := lastSeen.Time
			r.LastLoginAt = &t
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// IsAdmin 单点查询某 user 是否管理员且未禁用。中间件用它做权限闸。
// 已禁用账号即便 is_admin=1,也按非管理员处理(管理员被禁用 = 失能)。
func (s *UserStore) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	var isAdmin, isDisabled int
	err := s.DB.QueryRowContext(ctx,
		`SELECT is_admin, is_disabled FROM users WHERE id = ?`, userID,
	).Scan(&isAdmin, &isDisabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return isAdmin == 1 && isDisabled == 0, nil
}

// EnsureAdminByEmail 把指定邮箱的现有用户提升为管理员;不存在时返回 ErrNotFound。
// 用于"已经有用户后通过 .env 提升管理员"。
func (s *UserStore) EnsureAdminByEmail(ctx context.Context, email string) (*models.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, fmt.Errorf("email required")
	}
	row := s.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE email = ?`, email)
	var id int64
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if _, err := s.DB.ExecContext(ctx, `
		UPDATE users SET is_admin = 1, is_disabled = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, id); err != nil {
		return nil, err
	}
	return s.GetByID(ctx, id)
}

// CreateAdmin 用 (email, hash) 直接创建一个管理员账号。供启动引导使用。
func (s *UserStore) CreateAdmin(ctx context.Context, email, passwordHash, displayName, timezone string) (*models.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if timezone == "" {
		timezone = "UTC"
	}
	if displayName == "" {
		if at := strings.IndexByte(email, '@'); at > 0 {
			displayName = email[:at]
		} else {
			displayName = email
		}
	}
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO users(email, password_hash, display_name, timezone, is_admin, is_disabled)
		VALUES (?, ?, ?, ?, 1, 0)
	`, email, passwordHash, displayName, timezone)
	if err != nil {
		if isUniqueErr(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("insert admin: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetByID(ctx, id)
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

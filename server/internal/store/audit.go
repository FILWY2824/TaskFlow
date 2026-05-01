package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/youruser/taskflow/internal/models"
)

// AuditStore 写入与查询管理员审计日志。所有写操作都使用 actor_email 作为冗余,
// 即使 actor 用户后来被删,也仍能在审计里看到当时是谁做的。
type AuditStore struct{ DB *sql.DB }

func NewAuditStore(db *sql.DB) *AuditStore { return &AuditStore{DB: db} }

// Write 写一条审计日志。actorID = nil 表示系统事件。
func (s *AuditStore) Write(ctx context.Context,
	actorID *int64, actorEmail, action, targetType, targetID, detail, ip string,
) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO audit_logs(actor_id, actor_email, action, target_type, target_id, detail, ip)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, actorID, actorEmail, action, targetType, targetID, detail, ip)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

// AuditFilter 列表查询条件。
type AuditFilter struct {
	Search  string // 在 action / target_type / target_id / actor_email / detail 做模糊匹配
	Action  string // 精确匹配
	ActorID *int64
	From    *time.Time
	To      *time.Time
	Limit   int
	Offset  int
}

// List 列出审计日志(按 created_at DESC 分页)。
func (s *AuditStore) List(ctx context.Context, f AuditFilter) ([]models.AuditLog, int, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	conds := []string{}
	args := []any{}
	if v := strings.TrimSpace(f.Search); v != "" {
		like := "%" + strings.ToLower(v) + "%"
		conds = append(conds,
			`(LOWER(action) LIKE ? OR LOWER(target_type) LIKE ? OR LOWER(target_id) LIKE ? OR LOWER(actor_email) LIKE ? OR LOWER(detail) LIKE ?)`,
		)
		args = append(args, like, like, like, like, like)
	}
	if v := strings.TrimSpace(f.Action); v != "" {
		conds = append(conds, `action = ?`)
		args = append(args, v)
	}
	if f.ActorID != nil {
		conds = append(conds, `actor_id = ?`)
		args = append(args, *f.ActorID)
	}
	if f.From != nil {
		conds = append(conds, `created_at >= ?`)
		args = append(args, f.From.UTC())
	}
	if f.To != nil {
		conds = append(conds, `created_at <= ?`)
		args = append(args, f.To.UTC())
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	// 总数
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// 主查询
	q := `SELECT id, actor_id, actor_email, action, target_type, target_id, detail, ip, created_at
	      FROM audit_logs` + where + ` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	args2 := append([]any{}, args...)
	args2 = append(args2, f.Limit, f.Offset)

	rows, err := s.DB.QueryContext(ctx, q, args2...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.AuditLog{}
	for rows.Next() {
		var a models.AuditLog
		var actorID sql.NullInt64
		if err := rows.Scan(&a.ID, &actorID, &a.ActorEmail, &a.Action,
			&a.TargetType, &a.TargetID, &a.Detail, &a.IP, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		if actorID.Valid {
			v := actorID.Int64
			a.ActorID = &v
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Cleanup 删除指定天数之前的旧日志,返回删除数量。days <= 0 表示不清理。
func (s *AuditStore) Cleanup(ctx context.Context, days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	res, err := s.DB.ExecContext(ctx, `DELETE FROM audit_logs WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

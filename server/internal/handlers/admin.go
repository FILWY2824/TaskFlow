package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/youruser/taskflow/internal/auth"
	"github.com/youruser/taskflow/internal/middleware"
	"github.com/youruser/taskflow/internal/models"
	"github.com/youruser/taskflow/internal/store"
)

// AdminHandler 管理面板的所有路由。
//
// 设计要点:
//   - 路由本身在 server.go 用 RequireAdmin 中间件裹住,因此进入这里的请求都已确认是管理员。
//   - 所有"会改变状态"的动作都会写一条 audit_log;读操作不写。
//   - DBPath 用于"磁盘占用"统计:取数据库文件所在分区的可用 / 总空间。
type AdminHandler struct {
	DB           *sql.DB
	Issuer       *auth.Issuer
	Users        *store.UserStore
	RefreshTok   *store.RefreshTokenStore
	Audit        *store.AuditStore
	Logger       *slog.Logger
	DBPath       string    // 数据库文件路径(用于磁盘占用)
	StartedAt    time.Time // 进程启动时间
	Version      string    // 服务端版本字符串
	OAuthEnabled bool      // /api/auth/config 也对外暴露这个,管理面板用来在前端做提示

	settingsFn func() SettingsView // 由 SetSettingsProvider 注入,见下
}

func NewAdminHandler(
	db *sql.DB,
	issuer *auth.Issuer,
	users *store.UserStore,
	refresh *store.RefreshTokenStore,
	audit *store.AuditStore,
	logger *slog.Logger,
	dbPath string,
	startedAt time.Time,
	version string,
	oauthEnabled bool,
) *AdminHandler {
	return &AdminHandler{
		DB:           db,
		Issuer:       issuer,
		Users:        users,
		RefreshTok:   refresh,
		Audit:        audit,
		Logger:       logger,
		DBPath:       dbPath,
		StartedAt:    startedAt,
		Version:      version,
		OAuthEnabled: oauthEnabled,
	}
}

// =============================================================
// 系统状态:GET /api/admin/system
// =============================================================

type sysMemoryInfo struct {
	AllocBytes      uint64 `json:"alloc_bytes"`       // 当前 Go 堆已分配
	TotalAllocBytes uint64 `json:"total_alloc_bytes"` // 进程累计分配总量
	SysBytes        uint64 `json:"sys_bytes"`         // Go 运行时向 OS 申请的总量
	HeapInUseBytes  uint64 `json:"heap_inuse_bytes"`
	HeapIdleBytes   uint64 `json:"heap_idle_bytes"`
	NumGC           uint32 `json:"num_gc"`
}

type sysDiskInfo struct {
	Path        string  `json:"path"`         // 哪个分区
	TotalBytes  uint64  `json:"total_bytes"`  // 总空间
	FreeBytes   uint64  `json:"free_bytes"`   // 可用
	UsedBytes   uint64  `json:"used_bytes"`   // 已用
	UsedPercent float64 `json:"used_percent"` // 0~100
}

type sysDBInfo struct {
	Path             string `json:"path"`
	FileSizeBytes    int64  `json:"file_size_bytes"`     // SQLite 主文件大小
	WALFileSizeBytes int64  `json:"wal_file_size_bytes"` // -wal 文件
	PageCount        int64  `json:"page_count"`
	PageSize         int64  `json:"page_size"`
	UserCount        int64  `json:"user_count"`
	TodoCount        int64  `json:"todo_count"`
	ListCount        int64  `json:"list_count"`
	ReminderCount    int64  `json:"reminder_count"`
	NotificationCnt  int64  `json:"notification_count"`
	PomodoroCnt      int64  `json:"pomodoro_count"`
	AuditCount       int64  `json:"audit_count"`
}

type sysInfoResponse struct {
	Version       string        `json:"version"`
	GoVersion     string        `json:"go_version"`
	OS            string        `json:"os"`
	Arch          string        `json:"arch"`
	NumCPU        int           `json:"num_cpu"`
	NumGoroutine  int           `json:"num_goroutine"`
	StartedAt     string        `json:"started_at"`
	UptimeSeconds int64         `json:"uptime_seconds"`
	NowTime       string        `json:"now"`
	OAuthEnabled  bool          `json:"oauth_enabled"`
	Memory        sysMemoryInfo `json:"memory"`
	Disk          sysDiskInfo   `json:"disk"`
	Database      sysDBInfo     `json:"database"`
}

// System GET /api/admin/system —— 拉一份当前进程 / 数据库 / 磁盘 / 内存的快照。
func (h *AdminHandler) System(w http.ResponseWriter, r *http.Request) {
	resp := sysInfoResponse{
		Version:       h.Version,
		GoVersion:     runtime.Version(),
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		NumCPU:        runtime.NumCPU(),
		NumGoroutine:  runtime.NumGoroutine(),
		StartedAt:     h.StartedAt.UTC().Format(time.RFC3339),
		UptimeSeconds: int64(time.Since(h.StartedAt).Seconds()),
		NowTime:       time.Now().UTC().Format(time.RFC3339),
		OAuthEnabled:  h.OAuthEnabled,
	}

	// === 内存 ===
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	resp.Memory = sysMemoryInfo{
		AllocBytes:      m.Alloc,
		TotalAllocBytes: m.TotalAlloc,
		SysBytes:        m.Sys,
		HeapInUseBytes:  m.HeapInuse,
		HeapIdleBytes:   m.HeapIdle,
		NumGC:           m.NumGC,
	}

	// === 磁盘 ===
	resp.Disk = collectDisk(h.DBPath)

	// === 数据库 ===
	resp.Database = collectDB(r.Context(), h.DB, h.DBPath)

	writeJSON(w, http.StatusOK, resp)
}

// collectDisk 用 statfs 拿数据库所在分区的容量。失败时返回 path 但其它字段为 0。
// 实现拆到 admin_disk_unix.go / admin_disk_windows.go,按平台编译。

func collectDB(ctx context.Context, db *sql.DB, dbPath string) sysDBInfo {
	out := sysDBInfo{Path: dbPath}

	// SQLite 主文件大小 + WAL 文件大小。失败时悄悄略过(不让整个端点 500)。
	if dbPath != "" {
		if fi, err := os.Stat(dbPath); err == nil {
			out.FileSizeBytes = fi.Size()
		}
		if fi, err := os.Stat(dbPath + "-wal"); err == nil {
			out.WALFileSizeBytes = fi.Size()
		}
	}

	// page_count / page_size pragma
	_ = db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&out.PageCount)
	_ = db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&out.PageSize)

	// 各表行数
	queries := map[string]*int64{
		`SELECT COUNT(*) FROM users`:                                   &out.UserCount,
		`SELECT COUNT(*) FROM todos WHERE deleted_at IS NULL`:          &out.TodoCount,
		`SELECT COUNT(*) FROM lists WHERE deleted_at IS NULL`:          &out.ListCount,
		`SELECT COUNT(*) FROM reminder_rules WHERE deleted_at IS NULL`: &out.ReminderCount,
		`SELECT COUNT(*) FROM notifications`:                           &out.NotificationCnt,
		`SELECT COUNT(*) FROM pomodoro_sessions`:                       &out.PomodoroCnt,
		`SELECT COUNT(*) FROM audit_logs`:                              &out.AuditCount,
	}
	for q, dst := range queries {
		_ = db.QueryRowContext(ctx, q).Scan(dst)
	}
	return out
}

// =============================================================
// 用户管理:GET/PATCH/DELETE /api/admin/users
// =============================================================

type adminUserListResp struct {
	Items  []store.AdminUserRow `json:"items"`
	Total  int                  `json:"total"`
	Limit  int                  `json:"limit"`
	Offset int                  `json:"offset"`
}

func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("search")
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	rows, total, err := h.Users.ListForAdmin(r.Context(), search, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, adminUserListResp{
		Items: rows, Total: total, Limit: limit, Offset: offset,
	})
}

type adminPatchUserReq struct {
	IsAdmin    *bool `json:"is_admin,omitempty"`
	IsDisabled *bool `json:"is_disabled,omitempty"`
}

// PatchUser PATCH /api/admin/users/{id} —— 改动 is_admin / is_disabled 字段。
//
// 防呆:
//  1. 不能把"全系统最后一位管理员"撤掉(否则面板永久进不去)。
//  2. 不允许管理员把自己 is_disabled=true(必须由其他管理员操作)。
func (h *AdminHandler) PatchUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req adminPatchUserReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.IsAdmin == nil && req.IsDisabled == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "nothing to update")
		return
	}

	currentUID := middleware.UserIDFrom(r.Context())
	current, err := h.Users.GetByID(r.Context(), currentUID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "current user not found")
		return
	}

	target, err := h.Users.GetByID(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	if req.IsDisabled != nil {
		// 禁用自己被禁止 —— 必须由别的管理员操作。
		if *req.IsDisabled && id == currentUID {
			writeError(w, http.StatusBadRequest, "self_disable", "cannot disable your own account")
			return
		}
		if err := h.Users.SetDisabled(r.Context(), id, *req.IsDisabled); err != nil {
			writeStoreError(w, err)
			return
		}
		// 禁用后撤销该用户全部 refresh token —— 立即让他下线。
		if *req.IsDisabled {
			_ = h.RefreshTok.RevokeAllForUser(r.Context(), id)
		}
		action := "user.enable"
		if *req.IsDisabled {
			action = "user.disable"
		}
		_ = h.Audit.Write(r.Context(), &currentUID, current.Email, action,
			"user", strconv.FormatInt(id, 10),
			fmt.Sprintf("target_email=%s", target.Email),
			middleware.ClientIP(r))
	}

	if req.IsAdmin != nil {
		if err := h.Users.SetAdmin(r.Context(), id, *req.IsAdmin, true); err != nil {
			if errors.Is(err, store.ErrConflict) {
				writeError(w, http.StatusBadRequest, "last_admin", "cannot demote the last admin")
				return
			}
			writeStoreError(w, err)
			return
		}
		action := "user.demote"
		if *req.IsAdmin {
			action = "user.promote"
		}
		_ = h.Audit.Write(r.Context(), &currentUID, current.Email, action,
			"user", strconv.FormatInt(id, 10),
			fmt.Sprintf("target_email=%s", target.Email),
			middleware.ClientIP(r))
	}

	out, err := h.Users.GetByID(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// DeleteUser DELETE /api/admin/users/{id} —— 永久删除用户(级联删除所有数据)。
func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	currentUID := middleware.UserIDFrom(r.Context())
	if id == currentUID {
		writeError(w, http.StatusBadRequest, "self_delete", "cannot delete your own account")
		return
	}
	current, err := h.Users.GetByID(r.Context(), currentUID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "current user not found")
		return
	}

	target, err := h.Users.GetByID(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	// 不允许删除最后一位管理员
	if target.IsAdmin {
		nAdmins, err := h.Users.CountAdmins(r.Context())
		if err == nil && nAdmins <= 1 {
			writeError(w, http.StatusBadRequest, "last_admin", "cannot delete the last admin")
			return
		}
	}
	if err := h.Users.Delete(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = h.Audit.Write(r.Context(), &currentUID, current.Email, "user.delete",
		"user", strconv.FormatInt(id, 10),
		fmt.Sprintf("target_email=%s", target.Email),
		middleware.ClientIP(r))
	w.WriteHeader(http.StatusNoContent)
}

// CreateUser POST /api/admin/users —— 管理员后台新建用户(含可选 is_admin 标志)。
type adminCreateUserReq struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
	IsAdmin     bool   `json:"is_admin"`
}

func (h *AdminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req adminCreateUserReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "email required")
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid email")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "bad_request", "password must be at least 8 characters")
		return
	}
	if len(req.Password) > 128 {
		writeError(w, http.StatusBadRequest, "bad_request", "password too long")
		return
	}
	if req.Timezone != "" {
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid timezone")
			return
		}
	}
	hash, err := h.Issuer.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	var u *models.User
	if req.IsAdmin {
		u, err = h.Users.CreateAdmin(r.Context(), req.Email, hash, req.DisplayName, req.Timezone)
	} else {
		u, err = h.Users.Create(r.Context(), req.Email, hash, req.DisplayName, req.Timezone)
	}
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "email_taken", "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	currentUID := middleware.UserIDFrom(r.Context())
	current, _ := h.Users.GetByID(r.Context(), currentUID)
	currentEmail := ""
	if current != nil {
		currentEmail = current.Email
	}
	action := "user.create"
	if req.IsAdmin {
		action = "user.create_admin"
	}
	_ = h.Audit.Write(r.Context(), &currentUID, currentEmail, action,
		"user", strconv.FormatInt(u.ID, 10),
		fmt.Sprintf("target_email=%s", u.Email),
		middleware.ClientIP(r))
	writeJSON(w, http.StatusCreated, u)
}

// =============================================================
// 审计日志:GET /api/admin/audit
// =============================================================

type auditListResp struct {
	Items  []models.AuditLog `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

func (h *AdminHandler) ListAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	f := store.AuditFilter{
		Search: q.Get("search"),
		Action: q.Get("action"),
		Limit:  limit,
		Offset: offset,
	}
	if v := q.Get("actor_id"); v != "" {
		if x, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.ActorID = &x
		}
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.From = &t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.To = &t
		}
	}
	items, total, err := h.Audit.List(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, auditListResp{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

// =============================================================
// 数据清理:POST /api/admin/cleanup
//
// scope 取值:
//   "completed_todos"     : 删除已完成且早于 days 天的 todo(物理删除,慎用)
//   "soft_deleted_todos"  : 物理清掉 deleted_at IS NOT NULL 且早于 days 天的 todo
//   "soft_deleted_lists"  : 物理清掉 deleted_at IS NOT NULL 且早于 days 天的 list
//   "old_notifications"   : 删除早于 days 天的通知
//   "old_pomodoros"       : 删除早于 days 天的番茄会话
//   "expired_refresh"     : 立即跑一次 refresh_tokens 过期清理(忽略 days)
//   "audit_logs"          : 物理清掉早于 days 天的审计日志
//   "vacuum"              : 跑 VACUUM,回收空间(忽略 days)
//
// 所有 scope 都会写一条 audit_log。
// =============================================================

type cleanupReq struct {
	Scope   string `json:"scope"`
	Days    int    `json:"days"`    // 仅对带时间过滤的 scope 有意义
	Confirm bool   `json:"confirm"` // 二次确认。VACUUM 与会真的删数据的 scope 都要求 true。
	DryRun  bool   `json:"dry_run"` // 只统计,不真的删
}

type cleanupResp struct {
	Scope    string `json:"scope"`
	Affected int64  `json:"affected"`
	DryRun   bool   `json:"dry_run"`
	Message  string `json:"message,omitempty"`
}

func (h *AdminHandler) Cleanup(w http.ResponseWriter, r *http.Request) {
	var req cleanupReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Scope == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "scope required")
		return
	}
	if !req.DryRun && !req.Confirm {
		writeError(w, http.StatusBadRequest, "confirm_required",
			"this operation is destructive: pass confirm=true or dry_run=true")
		return
	}
	days := req.Days
	if days < 0 {
		days = 0
	}

	ctx := r.Context()
	var affected int64
	var msg string

	switch req.Scope {
	case "completed_todos":
		if days <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "days must be > 0")
			return
		}
		cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
		var err error
		if req.DryRun {
			affected, err = countQuery(ctx, h.DB,
				`SELECT COUNT(*) FROM todos
				 WHERE is_completed = 1 AND completed_at IS NOT NULL AND completed_at < ?`, cutoff)
		} else {
			res, e := h.DB.ExecContext(ctx,
				`DELETE FROM todos
				 WHERE is_completed = 1 AND completed_at IS NOT NULL AND completed_at < ?`, cutoff)
			err = e
			if e == nil {
				affected, _ = res.RowsAffected()
			}
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}

	case "soft_deleted_todos":
		if days < 0 {
			days = 0
		}
		cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
		var err error
		if req.DryRun {
			affected, err = countQuery(ctx, h.DB,
				`SELECT COUNT(*) FROM todos WHERE deleted_at IS NOT NULL AND deleted_at < ?`, cutoff)
		} else {
			res, e := h.DB.ExecContext(ctx,
				`DELETE FROM todos WHERE deleted_at IS NOT NULL AND deleted_at < ?`, cutoff)
			err = e
			if e == nil {
				affected, _ = res.RowsAffected()
			}
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}

	case "soft_deleted_lists":
		cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
		var err error
		if req.DryRun {
			affected, err = countQuery(ctx, h.DB,
				`SELECT COUNT(*) FROM lists WHERE deleted_at IS NOT NULL AND deleted_at < ?`, cutoff)
		} else {
			res, e := h.DB.ExecContext(ctx,
				`DELETE FROM lists WHERE deleted_at IS NOT NULL AND deleted_at < ?`, cutoff)
			err = e
			if e == nil {
				affected, _ = res.RowsAffected()
			}
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}

	case "old_notifications":
		if days <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "days must be > 0")
			return
		}
		cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
		var err error
		if req.DryRun {
			affected, err = countQuery(ctx, h.DB,
				`SELECT COUNT(*) FROM notifications WHERE created_at < ?`, cutoff)
		} else {
			res, e := h.DB.ExecContext(ctx,
				`DELETE FROM notifications WHERE created_at < ?`, cutoff)
			err = e
			if e == nil {
				affected, _ = res.RowsAffected()
			}
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}

	case "old_pomodoros":
		if days <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "days must be > 0")
			return
		}
		cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
		var err error
		if req.DryRun {
			affected, err = countQuery(ctx, h.DB,
				`SELECT COUNT(*) FROM pomodoro_sessions WHERE created_at < ?`, cutoff)
		} else {
			res, e := h.DB.ExecContext(ctx,
				`DELETE FROM pomodoro_sessions WHERE created_at < ?`, cutoff)
			err = e
			if e == nil {
				affected, _ = res.RowsAffected()
			}
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}

	case "expired_refresh":
		if req.DryRun {
			// 估算:已过期或撤销超过 7 天的 token 数。
			n, err := countQuery(ctx, h.DB,
				`SELECT COUNT(*) FROM refresh_tokens
				 WHERE expires_at < ? OR (revoked_at IS NOT NULL AND revoked_at < ?)`,
				time.Now().UTC(), time.Now().UTC().Add(-7*24*time.Hour))
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal", err.Error())
				return
			}
			affected = n
		} else {
			n, err := h.RefreshTok.CleanupExpired(ctx)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal", err.Error())
				return
			}
			affected = n
		}

	case "audit_logs":
		if days <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "days must be > 0")
			return
		}
		if req.DryRun {
			cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
			n, err := countQuery(ctx, h.DB, `SELECT COUNT(*) FROM audit_logs WHERE created_at < ?`, cutoff)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal", err.Error())
				return
			}
			affected = n
		} else {
			n, err := h.Audit.Cleanup(ctx, days)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal", err.Error())
				return
			}
			affected = n
		}

	case "vacuum":
		if req.DryRun {
			msg = "VACUUM has no dry-run mode; skipped"
		} else {
			if _, err := h.DB.ExecContext(ctx, `VACUUM`); err != nil {
				writeError(w, http.StatusInternalServerError, "internal", err.Error())
				return
			}
			msg = "vacuum done"
		}

	default:
		writeError(w, http.StatusBadRequest, "bad_request", "unknown scope: "+req.Scope)
		return
	}

	currentUID := middleware.UserIDFrom(r.Context())
	current, _ := h.Users.GetByID(r.Context(), currentUID)
	email := ""
	if current != nil {
		email = current.Email
	}
	action := "cleanup." + req.Scope
	if req.DryRun {
		action += ".dry_run"
	}
	_ = h.Audit.Write(r.Context(), &currentUID, email, action,
		"cleanup", req.Scope,
		fmt.Sprintf("days=%d affected=%d", days, affected),
		middleware.ClientIP(r))

	writeJSON(w, http.StatusOK, cleanupResp{
		Scope: req.Scope, Affected: affected, DryRun: req.DryRun, Message: msg,
	})
}

// =============================================================
// 系统设置:GET /api/admin/settings
//
// 不直接暴露原始环境变量(避免泄密),只回当前配置的有效摘要,例如:
//   - oauth.enabled / provider / redirect_url
//   - auth.access_ttl_seconds / refresh_ttl_seconds
//   - bot.enabled
//   - scheduler.tick / batch
// 这些值由启动时注入,运行时不变。
// =============================================================

// SettingsView 由 server.go 注入(避免 handler 直接依赖 config 包)。
type SettingsView struct {
	OAuthEnabled        bool   `json:"oauth_enabled"`
	OAuthProvider       string `json:"oauth_provider,omitempty"`
	OAuthRedirectURL    string `json:"oauth_redirect_url,omitempty"`
	BotEnabled          bool   `json:"telegram_bot_enabled"`
	BotUsername         string `json:"telegram_bot_username,omitempty"`
	AccessTTLSeconds    int    `json:"access_ttl_seconds"`
	RefreshTTLSeconds   int    `json:"refresh_ttl_seconds"`
	BcryptCost          int    `json:"bcrypt_cost"`
	SchedulerTick       int    `json:"scheduler_tick_seconds"`
	SchedulerBatch      int    `json:"scheduler_batch_size"`
	SchedulerDisabled   bool   `json:"scheduler_disabled"`
	ServerListen        string `json:"server_listen"`
	DatabasePath        string `json:"database_path"`
	AdminBootstrapEmail string `json:"admin_bootstrap_email,omitempty"`
}

// 用闭包注入,避免 AdminHandler 依赖 config 包。
type settingsProvider func() SettingsView

func (h *AdminHandler) settingsProvider() settingsProvider {
	if h.settingsFn == nil {
		return func() SettingsView { return SettingsView{OAuthEnabled: h.OAuthEnabled} }
	}
	return h.settingsFn
}

// SetSettingsProvider 让 server.go 在装配时注入设置快照函数。
func (h *AdminHandler) SetSettingsProvider(fn func() SettingsView) {
	h.settingsFn = fn
}

// Settings GET /api/admin/settings —— 当前生效配置摘要。
func (h *AdminHandler) Settings(w http.ResponseWriter, r *http.Request) {
	view := h.settingsProvider()()
	writeJSON(w, http.StatusOK, view)
}

// =============================================================
// 内部工具
// =============================================================

func countQuery(ctx context.Context, db *sql.DB, q string, args ...any) (int64, error) {
	var n int64
	err := db.QueryRowContext(ctx, q, args...).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

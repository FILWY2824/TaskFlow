package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite"
)

// Open 打开 SQLite 连接并应用 WAL 等 PRAGMA。
// modernc.org/sqlite 的驱动名为 "sqlite"。
func Open(path string) (*sql.DB, error) {
	// 通过 URI 参数设置常用 PRAGMA。剩余的 PRAGMA 在连接建立后再执行。
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_time_format=sqlite",
		url.PathEscape(path))

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite 单写多读,过多连接会阻塞。
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	// 显式应用一遍 PRAGMA(防止某些驱动版本不识别 URI 形式)。
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA temp_store=MEMORY",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("exec %s: %w", p, err)
		}
	}
	return db, nil
}

// Migrate 应用所有迁移(简单的版本号方案)。
func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, m := range migrations {
		var exists int
		err := db.QueryRowContext(ctx,
			`SELECT 1 FROM schema_migrations WHERE version = ?`, m.Version).Scan(&exists)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check migration %d: %w", m.Version, err)
		}
		if exists == 1 {
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.Version, err)
		}
		if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", m.Version, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations(version) VALUES (?)`, m.Version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}
	}
	return nil
}

type migration struct {
	Version int
	SQL     string
}

// 注意:每个 migration.SQL 内的多条语句会用 ExecContext 一次性执行。
// modernc.org/sqlite 支持单次 Exec 中包含多个 ; 分隔的语句。
var migrations = []migration{
	{Version: 1, SQL: schemaV1},
	{Version: 2, SQL: schemaV2},
	{Version: 3, SQL: schemaV3},
	{Version: 4, SQL: schemaV4},
}

const schemaV1 = `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	display_name TEXT NOT NULL DEFAULT '',
	timezone TEXT NOT NULL DEFAULT 'UTC',
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	token_hash TEXT NOT NULL UNIQUE,
	device_id TEXT NOT NULL DEFAULT '',
	expires_at TIMESTAMP NOT NULL,
	revoked_at TIMESTAMP,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires ON refresh_tokens(expires_at);

CREATE TABLE IF NOT EXISTS devices (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	platform TEXT NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	last_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_devices_user ON devices(user_id);

CREATE TABLE IF NOT EXISTS lists (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	color TEXT NOT NULL DEFAULT '',
	icon TEXT NOT NULL DEFAULT '',
	sort_order INTEGER NOT NULL DEFAULT 0,
	is_default INTEGER NOT NULL DEFAULT 0,
	is_archived INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_lists_user ON lists(user_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS todos (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	list_id INTEGER REFERENCES lists(id) ON DELETE SET NULL,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	priority INTEGER NOT NULL DEFAULT 0,
	effort INTEGER NOT NULL DEFAULT 0,
	due_at TIMESTAMP,
	due_all_day INTEGER NOT NULL DEFAULT 0,
	start_at TIMESTAMP,
	is_completed INTEGER NOT NULL DEFAULT 0,
	completed_at TIMESTAMP,
	sort_order INTEGER NOT NULL DEFAULT 0,
	timezone TEXT NOT NULL DEFAULT 'UTC',
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_todos_user ON todos(user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_todos_list ON todos(list_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_todos_due ON todos(user_id, due_at) WHERE deleted_at IS NULL AND due_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_todos_completed ON todos(user_id, is_completed) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS subtasks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	todo_id INTEGER NOT NULL REFERENCES todos(id) ON DELETE CASCADE,
	title TEXT NOT NULL,
	is_completed INTEGER NOT NULL DEFAULT 0,
	completed_at TIMESTAMP,
	sort_order INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_subtasks_todo ON subtasks(todo_id);
CREATE INDEX IF NOT EXISTS idx_subtasks_user ON subtasks(user_id);

CREATE TABLE IF NOT EXISTS reminder_rules (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	todo_id INTEGER REFERENCES todos(id) ON DELETE CASCADE,
	title TEXT NOT NULL DEFAULT '',
	trigger_at TIMESTAMP,
	rrule TEXT NOT NULL DEFAULT '',
	dtstart TIMESTAMP,
	timezone TEXT NOT NULL DEFAULT 'UTC',
	channel_local INTEGER NOT NULL DEFAULT 1,
	channel_telegram INTEGER NOT NULL DEFAULT 0,
	channel_web_push INTEGER NOT NULL DEFAULT 0,
	is_enabled INTEGER NOT NULL DEFAULT 1,
	next_fire_at TIMESTAMP,
	last_fired_at TIMESTAMP,
	ringtone TEXT NOT NULL DEFAULT 'default',
	vibrate INTEGER NOT NULL DEFAULT 1,
	fullscreen INTEGER NOT NULL DEFAULT 1,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_reminders_user ON reminder_rules(user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_reminders_todo ON reminder_rules(todo_id);
CREATE INDEX IF NOT EXISTS idx_reminders_next ON reminder_rules(next_fire_at) WHERE is_enabled = 1 AND deleted_at IS NULL AND next_fire_at IS NOT NULL;

-- Telegram(Phase 4 才会用)
CREATE TABLE IF NOT EXISTS telegram_bind_tokens (
	token TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at TIMESTAMP NOT NULL,
	used_at TIMESTAMP,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_telegram_bind_tokens_user ON telegram_bind_tokens(user_id);

CREATE TABLE IF NOT EXISTS telegram_bindings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	chat_id TEXT NOT NULL,
	username TEXT NOT NULL DEFAULT '',
	is_enabled INTEGER NOT NULL DEFAULT 1,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(user_id, chat_id)
);
CREATE INDEX IF NOT EXISTS idx_telegram_bindings_user ON telegram_bindings(user_id);

CREATE TABLE IF NOT EXISTS notifications (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	reminder_rule_id INTEGER REFERENCES reminder_rules(id) ON DELETE SET NULL,
	todo_id INTEGER REFERENCES todos(id) ON DELETE SET NULL,
	title TEXT NOT NULL,
	body TEXT NOT NULL DEFAULT '',
	fire_at TIMESTAMP NOT NULL,
	is_read INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id);

CREATE TABLE IF NOT EXISTS notification_deliveries (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	notification_id INTEGER NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
	channel TEXT NOT NULL,
	status TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	attempts INTEGER NOT NULL DEFAULT 0,
	delivered_at TIMESTAMP,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_deliveries_notification ON notification_deliveries(notification_id);

-- 增量同步事件流。客户端按 cursor 拉取本用户的变更。
CREATE TABLE IF NOT EXISTS sync_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	entity_type TEXT NOT NULL,
	entity_id INTEGER NOT NULL,
	action TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_sync_events_user ON sync_events(user_id, id);
`

// schemaV2:番茄专注会话(规格 §10、§14 阶段 11)。
//
// status 状态机:
//
//	active     -> 进行中(刚开始,actual_duration_seconds 不计)
//	completed  -> 用户完成 / 自动完成(actual_duration_seconds 等于实际持续秒)
//	abandoned  -> 用户主动放弃(actual_duration_seconds 是实际坚持秒,可 < planned)
//
// kind:
//
//	focus       -> 专注番茄
//	short_break -> 短休
//	long_break  -> 长休
//	learning    -> 学习专注(深度学习/读书等场景与普通 focus 区分,便于统计)
//	review      -> 复盘整理(回顾、写日志、整理资料等较低强度的"输出"时段)
//
// 设计要点:
//   - todo_id 为 NULL 表示"自由专注",不绑定具体 todo;todo 软删时置为 NULL,保留历史统计。
//   - actual_duration_seconds 由服务端在 complete/abandon 时计算,前端不能伪造。
//   - 不软删,删除直接 DELETE(历史会话不会与其他实体级联删除)。
const schemaV2 = `
CREATE TABLE IF NOT EXISTS pomodoro_sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	todo_id INTEGER REFERENCES todos(id) ON DELETE SET NULL,
	started_at TIMESTAMP NOT NULL,
	ended_at TIMESTAMP,
	planned_duration_seconds INTEGER NOT NULL,
	actual_duration_seconds INTEGER NOT NULL DEFAULT 0,
	kind TEXT NOT NULL DEFAULT 'focus',
	status TEXT NOT NULL DEFAULT 'active',
	note TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_pomodoro_user ON pomodoro_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_pomodoro_started ON pomodoro_sessions(user_id, started_at);
CREATE INDEX IF NOT EXISTS idx_pomodoro_todo ON pomodoro_sessions(todo_id);
`

// schemaV3 —— 跨端用户偏好(规格 §17:Web / Android / Windows 各自的通知与提醒开关
// 都需要服务端持久化,但每个客户端只展示自己 scope 的开关)。
//
// 表设计:
//   - 主键 (user_id, scope, key) —— 每个 scope 下 key 唯一。
//   - scope 取值约定:'web' / 'android' / 'windows' / 'common'。
//     'common' 用于跨端共用的非通知类偏好(主题、是否折叠已完成等);
//     另外三者用于平台专属的通知 / 强提醒开关。
//   - value 用 TEXT 存,客户端按需把 bool/int/json 序列化成字符串再写。
//     这样新增偏好不需要改 schema,服务端不解释语义、只做透明键值存储。
//   - 不做软删,DELETE 直接物理删除(没必要保留历史)。
const schemaV3 = `
CREATE TABLE IF NOT EXISTS user_preferences (
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	scope TEXT NOT NULL,
	key TEXT NOT NULL,
	value TEXT NOT NULL DEFAULT '',
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (user_id, scope, key)
);
CREATE INDEX IF NOT EXISTS idx_user_preferences_user_scope ON user_preferences(user_id, scope);
`

// schemaV4 —— 外部 OAuth2 / OIDC 登录(规格新增)。
//
// 设计要点:
//   - 不新建 oauth_identities 表,直接给 users 加两列;同一个用户只跟一个外部 provider
//     绑定(单认证中心场景下绑多个意义不大)。如以后要支持多 IdP 再升级到独立表。
//   - oauth_provider 用稳定的提供方标识符(配置中的 provider),例如 "teamcy.eu.cc"。
//   - oauth_subject 是认证中心返回的 sub(或等价字段),配合 provider 唯一定位用户。
//   - 部分唯一索引仅约束「绑定了 oauth 的用户」,本地用户(provider=”)不参与冲突判定。
//   - email 不再作为 OAuth 用户的唯一定位手段,只作为可读副本(认证中心改邮箱不会让用户失联)。
//     由于 users.email 仍是 UNIQUE,创建时若邮箱冲突,会退回到「用 sub@provider 占位」策略
//     (见 store.UserStore.UpsertOAuth)。
const schemaV4 = `
ALTER TABLE users ADD COLUMN oauth_provider TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN oauth_subject TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_oauth_identity
    ON users(oauth_provider, oauth_subject)
    WHERE oauth_provider != '' AND oauth_subject != '';
`

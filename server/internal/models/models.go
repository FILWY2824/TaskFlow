package models

import "time"

// 时间使用 RFC3339 (UTC) 序列化。

type User struct {
	ID          int64     `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Timezone    string    `json:"timezone"`
	IsAdmin     bool      `json:"is_admin"`
	IsDisabled  bool      `json:"is_disabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type List struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	Name       string    `json:"name"`
	Color      string    `json:"color"`
	Icon       string    `json:"icon"`
	SortOrder  int       `json:"sort_order"`
	IsDefault  bool      `json:"is_default"`
	IsArchived bool      `json:"is_archived"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Todo struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	ListID      *int64     `json:"list_id,omitempty"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Priority    int        `json:"priority"` // 0..4
	Effort      int        `json:"effort"`   // 0..5
	DueAt       *time.Time `json:"due_at,omitempty"`
	DueAllDay   bool       `json:"due_all_day"`
	StartAt     *time.Time `json:"start_at,omitempty"`
	IsCompleted bool       `json:"is_completed"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	SortOrder   int        `json:"sort_order"`
	Timezone    string     `json:"timezone"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Subtask struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	TodoID      int64      `json:"todo_id"`
	Title       string     `json:"title"`
	IsCompleted bool       `json:"is_completed"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	SortOrder   int        `json:"sort_order"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type ReminderRule struct {
	ID              int64      `json:"id"`
	UserID          int64      `json:"user_id"`
	TodoID          *int64     `json:"todo_id,omitempty"`
	Title           string     `json:"title"`
	TriggerAt       *time.Time `json:"trigger_at,omitempty"` // 单次提醒
	RRule           string     `json:"rrule"`                // 周期规则,如 FREQ=MONTHLY;INTERVAL=6
	DTStart         *time.Time `json:"dtstart,omitempty"`
	Timezone        string     `json:"timezone"`
	ChannelLocal    bool       `json:"channel_local"`
	ChannelTelegram bool       `json:"channel_telegram"`
	ChannelWebPush  bool       `json:"channel_web_push"`
	IsEnabled       bool       `json:"is_enabled"`
	NextFireAt      *time.Time `json:"next_fire_at,omitempty"`
	LastFiredAt     *time.Time `json:"last_fired_at,omitempty"`
	Ringtone        string     `json:"ringtone"`
	Vibrate         bool       `json:"vibrate"`
	Fullscreen      bool       `json:"fullscreen"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type SyncEvent struct {
	ID         int64     `json:"id"` // 也作为 cursor
	EntityType string    `json:"entity_type"`
	EntityID   int64     `json:"entity_id"`
	Action     string    `json:"action"` // created | updated | deleted
	CreatedAt  time.Time `json:"created_at"`
}

// PomodoroSession 番茄专注会话(阶段 11)。
//
// 字段约定:
//   - PlannedDurationSeconds 在 Create 时由客户端给定(典型 1500=25min)。
//   - ActualDurationSeconds 服务端在 complete/abandon 时计算 = ended_at - started_at(秒,clamp 到 [0, planned*4])。
//   - Status: "active" / "completed" / "abandoned"。
//   - Kind:   "focus" / "short_break" / "long_break"。
type PomodoroSession struct {
	ID                     int64      `json:"id"`
	UserID                 int64      `json:"user_id"`
	TodoID                 *int64     `json:"todo_id,omitempty"`
	StartedAt              time.Time  `json:"started_at"`
	EndedAt                *time.Time `json:"ended_at,omitempty"`
	PlannedDurationSeconds int        `json:"planned_duration_seconds"`
	ActualDurationSeconds  int        `json:"actual_duration_seconds"`
	Kind                   string     `json:"kind"`
	Status                 string     `json:"status"`
	Note                   string     `json:"note"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// AuditLog 管理员操作审计记录(管理面板新增)。
//
// actor_id = NULL 表示系统动作(如启动时根据 .env 引导建管理员)。
// detail 是结构化或简短描述,客户端展示原文,不强制 JSON。
type AuditLog struct {
	ID         int64     `json:"id"`
	ActorID    *int64    `json:"actor_id,omitempty"`
	ActorEmail string    `json:"actor_email"`
	Action     string    `json:"action"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Detail     string    `json:"detail"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
}

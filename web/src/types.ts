// 与服务端 internal/models/models.go 保持一致。
// 时间字段统一是 RFC3339 字符串(服务端用 UTC),前端用 Date 处理时按字符串解析。

export interface ApiError {
  error: { code: string; message: string }
}

// === User ===
export interface User {
  id: number
  email: string
  display_name: string
  timezone: string
  created_at: string
  updated_at: string
}

// === Auth ===
export interface AuthResponse {
  access_token: string
  access_token_expires_at: string
  refresh_token: string
  refresh_token_expires_at: string
  user: User
}

// === List ===
export interface List {
  id: number
  user_id: number
  name: string
  color: string
  icon: string
  sort_order: number
  is_default: boolean
  is_archived: boolean
  created_at: string
  updated_at: string
}

// === Todo ===
export interface Todo {
  id: number
  user_id: number
  list_id?: number | null
  title: string
  description: string
  priority: number // 0..4
  effort: number // 0..5
  due_at?: string | null
  due_all_day: boolean
  start_at?: string | null
  is_completed: boolean
  completed_at?: string | null
  sort_order: number
  timezone: string
  created_at: string
  updated_at: string
}

export interface TodoInput {
  list_id?: number | null
  title: string
  description?: string
  priority?: number
  effort?: number
  due_at?: string | null
  due_all_day?: boolean
  start_at?: string | null
  sort_order?: number
  timezone?: string
}

// === Subtask ===
export interface Subtask {
  id: number
  user_id: number
  todo_id: number
  title: string
  is_completed: boolean
  completed_at?: string | null
  sort_order: number
  created_at: string
  updated_at: string
}

// === Reminder ===
export interface ReminderRule {
  id: number
  user_id: number
  todo_id?: number | null
  title: string
  trigger_at?: string | null
  rrule: string
  dtstart?: string | null
  timezone: string
  channel_local: boolean
  channel_telegram: boolean
  channel_web_push: boolean
  is_enabled: boolean
  next_fire_at?: string | null
  last_fired_at?: string | null
  ringtone: string
  vibrate: boolean
  fullscreen: boolean
  created_at: string
  updated_at: string
}

export interface ReminderInput {
  todo_id?: number | null
  title?: string
  trigger_at?: string | null
  rrule?: string
  dtstart?: string | null
  timezone?: string
  channel_local?: boolean
  channel_telegram?: boolean
  channel_web_push?: boolean
  is_enabled?: boolean
  ringtone?: string
  vibrate?: boolean
  fullscreen?: boolean
}

// === Notification ===
export interface Notification {
  id: number
  user_id: number
  reminder_rule_id?: number | null
  todo_id?: number | null
  title: string
  body: string
  fire_at: string
  is_read: boolean
  created_at: string
}

export interface NotificationDetail extends Notification {
  deliveries: NotificationDelivery[]
}

export interface NotificationDelivery {
  id: number
  notification_id: number
  channel: string
  status: string
  error: string
  attempts: number
  delivered_at?: string | null
  created_at: string
}

// === Telegram ===
export interface TelegramBindToken {
  token: string
  expires_at: string
  bot_username: string
  deep_link_web: string
  deep_link_app: string
}

export interface TelegramBinding {
  id: number
  user_id: number
  chat_id: string
  username: string
  is_enabled: boolean
  created_at: string
}

export interface TelegramBindStatus {
  status: 'pending' | 'bound' | 'expired'
  binding?: TelegramBinding
}

// === Pomodoro ===
export type PomodoroKind = 'focus' | 'short_break' | 'long_break'
export type PomodoroStatus = 'active' | 'completed' | 'abandoned'

export interface PomodoroSession {
  id: number
  user_id: number
  todo_id?: number | null
  started_at: string
  ended_at?: string | null
  planned_duration_seconds: number
  actual_duration_seconds: number
  kind: PomodoroKind
  status: PomodoroStatus
  note: string
  created_at: string
  updated_at: string
}

// === Stats ===
export interface StatsSummary {
  todos_total: number
  todos_open: number
  todos_completed: number
  todos_overdue: number
  todos_due_today: number
  completed_today: number
  completed_this_week: number
  pomodoro_today_seconds: number
  pomodoro_this_week_seconds: number
}

export interface DailyBucket {
  date: string
  created: number
  completed: number
  pomodoro_seconds: number
  pomodoro_count: number
}

export interface WeeklyBucket {
  week_start: string
  week_end: string
  created: number
  completed: number
  pomodoro_seconds: number
  pomodoro_count: number
}

export interface PomodoroAggregate {
  from: string
  to: string
  total_sessions: number
  total_seconds: number
  by_status: Record<string, number>
  by_kind_seconds: Record<string, number>
  daily: DailyBucket[]
}

// === Sync ===
export interface SyncEvent {
  id: number
  entity_type:
    | 'todo'
    | 'list'
    | 'subtask'
    | 'reminder'
    | 'notification'
    | 'telegram_binding'
    | 'pomodoro'
  entity_id: number
  action: 'created' | 'updated' | 'deleted'
  created_at: string
}

export interface SyncPullResponse {
  events: SyncEvent[]
  next_cursor: number
  has_more: boolean
}

// === SSE event payload ===
export interface SSENotification {
  type: 'notification'
  notification_id: number
  reminder_rule_id?: number
  todo_id?: number | null
  title: string
  body?: string
  fire_at: string
}

// === TODO 视图过滤名 ===
export type TodoFilterName =
  | 'today'
  | 'tomorrow'
  | 'this_week'
  | 'overdue'
  | 'no_date'
  | 'completed'
  | 'all'

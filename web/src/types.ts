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
  is_admin: boolean
  is_disabled: boolean
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

// AuthConfig 是 GET /api/auth/config 返回体。
// 登录页据此决定显示「邮箱密码表单」还是「通过认证中心登录」按钮。
export interface AuthConfig {
  oauth_enabled: boolean
  oauth_provider?: string
  oauth_start_url?: string
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
export interface TelegramConfig {
  enabled: boolean
  bot_username: string
}

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
// 番茄类型:focus / short_break / long_break 是经典三件套;learning / review 是
// 后加的"输入(深度学习)"和"输出(复盘整理)"两段,目的是让用户做更细粒度的
// 时段标签与统计区分。服务端 store/handlers 与 db.go 均已同步放开校验。
export type PomodoroKind = 'focus' | 'short_break' | 'long_break' | 'learning' | 'review'
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
  | 'recent_week'   // 今日起未来 7 天（含今日）
  | 'recent_month'  // 今日起未来 30 天（含今日）
  | 'overdue'
  | 'no_date'
  | 'no_list'       // 未分类（list_id IS NULL）
  | 'completed'
  | 'scheduled'     // 日程·全部：所有"有日期"的任务（包含已完成；不包含无日期）
  | 'all'

// === 管理面板（仅管理员可见）===

// /api/admin/system —— 进程 / 数据库 / 磁盘 / 内存快照。
export interface AdminMemoryInfo {
  alloc_bytes: number
  total_alloc_bytes: number
  sys_bytes: number
  heap_inuse_bytes: number
  heap_idle_bytes: number
  num_gc: number
}
export interface AdminDiskInfo {
  path: string
  total_bytes: number
  free_bytes: number
  used_bytes: number
  used_percent: number
}
export interface AdminDBInfo {
  path: string
  file_size_bytes: number
  wal_file_size_bytes: number
  page_count: number
  page_size: number
  user_count: number
  todo_count: number
  list_count: number
  reminder_count: number
  notification_count: number
  pomodoro_count: number
  audit_count: number
}
export interface AdminSystemInfo {
  version: string
  go_version: string
  os: string
  arch: string
  num_cpu: number
  num_goroutine: number
  started_at: string
  uptime_seconds: number
  now: string
  oauth_enabled: boolean
  memory: AdminMemoryInfo
  disk: AdminDiskInfo
  database: AdminDBInfo
}

// /api/admin/users —— 列表条目带 todo 计数与最近活跃时间。
export interface AdminUserRow extends User {
  todo_count: number
  last_login_at?: string | null
}
export interface AdminUserListResponse {
  items: AdminUserRow[]
  total: number
  limit: number
  offset: number
}

// /api/admin/audit
export interface AuditLogEntry {
  id: number
  actor_id?: number | null
  actor_email: string
  action: string
  target_type: string
  target_id: string
  detail: string
  ip: string
  created_at: string
}
export interface AuditListResponse {
  items: AuditLogEntry[]
  total: number
  limit: number
  offset: number
}

// /api/admin/settings —— 当前生效配置摘要(只读)。
export interface AdminSettingsView {
  oauth_enabled: boolean
  oauth_provider?: string
  oauth_redirect_url?: string
  telegram_bot_enabled: boolean
  telegram_bot_username?: string
  access_ttl_seconds: number
  refresh_ttl_seconds: number
  bcrypt_cost: number
  scheduler_tick_seconds: number
  scheduler_batch_size: number
  scheduler_disabled: boolean
  server_listen: string
  database_path: string
  admin_bootstrap_email?: string
}

// /api/admin/cleanup
export type CleanupScope =
  | 'completed_todos'
  | 'soft_deleted_todos'
  | 'soft_deleted_lists'
  | 'old_notifications'
  | 'old_pomodoros'
  | 'expired_refresh'
  | 'audit_logs'
  | 'vacuum'

export interface CleanupRequest {
  scope: CleanupScope
  days?: number
  confirm?: boolean
  dry_run?: boolean
}
export interface CleanupResponse {
  scope: CleanupScope
  affected: number
  dry_run: boolean
  message?: string
}

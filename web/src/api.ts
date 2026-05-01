import type {
  AdminSettingsView,
  AdminSystemInfo,
  AdminUserListResponse,
  AuditListResponse,
  AuthConfig,
  AuthResponse,
  CleanupRequest,
  CleanupResponse,
  DailyBucket,
  List,
  Notification,
  NotificationDetail,
  PomodoroAggregate,
  PomodoroSession,
  PomodoroKind,
  ReminderInput,
  ReminderRule,
  StatsSummary,
  Subtask,
  SyncPullResponse,
  TelegramBindStatus,
  TelegramBindToken,
  TelegramBinding,
  TelegramConfig,
  Todo,
  TodoInput,
  User,
  WeeklyBucket,
} from './types'

// =============================================================
// Token 管理 + fetch 包装
//
// 设计要点:
//   - access_token / refresh_token 都放 localStorage,key 名 v0.3.0 锁定。
//   - 401 自动用 refresh 刷一次,旋转后重试一次原请求。重试仍 401 -> 通知 onUnauthorized,跳登录。
//   - 单飞:同一时刻只发一次 refresh。其他请求等同一个 promise。
//   - API base URL:
//       * 浏览器(同源部署):用相对路径 ""(发到当前 origin)。
//       * Tauri / 跨域开发:用 setApiBase(url) 设成 https://taskflow.example.com,
//         所有 fetch 改为绝对路径,绕过 webview 的 tauri://localhost 协议。
// =============================================================

// 注:这些 localStorage 键名带 taskflow.* 前缀(品牌即名)。如果旧版本曾用过
// taskflow.* 的键,升级后会被视为未登录,用户需要重新登录一次。
const KEY_ACCESS = 'taskflow.access'
const KEY_ACCESS_EXP = 'taskflow.access_exp'
const KEY_REFRESH = 'taskflow.refresh'
const KEY_REFRESH_EXP = 'taskflow.refresh_exp'
const KEY_USER = 'taskflow.user'
const KEY_API_BASE = 'taskflow.api_base'

// API base URL —— 默认空串 = 同源相对路径(普通浏览器场景)。
// Tauri 客户端启动时会调 setApiBase() 把它改成 "https://your-taskflow.example.com"。
let apiBase = (() => {
  try {
    return localStorage.getItem(KEY_API_BASE) || ''
  } catch {
    return ''
  }
})()

/** 设置 API 基址。空串 = 同源。会被持久化以便下次启动直接生效。 */
export function setApiBase(base: string) {
  const trimmed = (base || '').trim().replace(/\/+$/, '')
  apiBase = trimmed
  try {
    if (trimmed) localStorage.setItem(KEY_API_BASE, trimmed)
    else localStorage.removeItem(KEY_API_BASE)
  } catch {
    /* ignore */
  }
}

export function getApiBase(): string {
  return apiBase
}

/** 把一个相对 path 拼成最终 URL。已经是绝对 URL 的原样返回。 */
export function absUrl(path: string): string {
  if (/^https?:\/\//i.test(path)) return path
  if (!apiBase) return path
  return apiBase + (path.startsWith('/') ? path : '/' + path)
}

let onUnauthorized: (() => void) | null = null
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

export interface Tokens {
  accessToken: string
  accessExp: string
  refreshToken: string
  refreshExp: string
}

export function loadTokens(): Tokens | null {
  const a = localStorage.getItem(KEY_ACCESS)
  const r = localStorage.getItem(KEY_REFRESH)
  if (!a || !r) return null
  return {
    accessToken: a,
    accessExp: localStorage.getItem(KEY_ACCESS_EXP) || '',
    refreshToken: r,
    refreshExp: localStorage.getItem(KEY_REFRESH_EXP) || '',
  }
}

export function saveTokens(t: Tokens, user?: User) {
  localStorage.setItem(KEY_ACCESS, t.accessToken)
  localStorage.setItem(KEY_ACCESS_EXP, t.accessExp)
  localStorage.setItem(KEY_REFRESH, t.refreshToken)
  localStorage.setItem(KEY_REFRESH_EXP, t.refreshExp)
  if (user) localStorage.setItem(KEY_USER, JSON.stringify(user))
}

export function loadUser(): User | null {
  const s = localStorage.getItem(KEY_USER)
  return s ? (JSON.parse(s) as User) : null
}

export function clearTokens() {
  localStorage.removeItem(KEY_ACCESS)
  localStorage.removeItem(KEY_ACCESS_EXP)
  localStorage.removeItem(KEY_REFRESH)
  localStorage.removeItem(KEY_REFRESH_EXP)
  localStorage.removeItem(KEY_USER)
}

// =============================================================
// 内部 fetch 实现:统一错误形状、自动 refresh
// =============================================================

export class ApiError extends Error {
  code: string
  status: number
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

let refreshing: Promise<Tokens> | null = null

async function doRefresh(): Promise<Tokens> {
  const t = loadTokens()
  if (!t) throw new ApiError(401, 'no_session', '会话已过期,请重新登录')
  const res = await fetch(absUrl('/api/auth/refresh'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: t.refreshToken }),
  })
  if (!res.ok) {
    clearTokens()
    onUnauthorized?.()
    throw new ApiError(401, 'refresh_failed', '会话已过期,请重新登录')
  }
  const data = (await res.json()) as AuthResponse
  const tokens: Tokens = {
    accessToken: data.access_token,
    accessExp: data.access_token_expires_at,
    refreshToken: data.refresh_token,
    refreshExp: data.refresh_token_expires_at,
  }
  saveTokens(tokens, data.user)
  return tokens
}

async function ensureRefresh(): Promise<Tokens> {
  if (!refreshing) {
    refreshing = doRefresh().finally(() => {
      refreshing = null
    })
  }
  return refreshing
}

interface RequestOptions {
  method?: string
  body?: unknown
  query?: Record<string, string | number | boolean | undefined>
  noAuth?: boolean
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const url = absUrl(buildURL(path, opts.query))
  const headers: Record<string, string> = {}
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json'
  if (!opts.noAuth) {
    const t = loadTokens()
    if (t) headers['Authorization'] = `Bearer ${t.accessToken}`
  }

  let res: Response
  try {
    res = await fetch(url, {
      method: opts.method || 'GET',
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    })
  } catch (e) {
    throw new ApiError(0, 'network', (e as Error).message || '网络错误')
  }

  // access 过期 → 刷一次再重试一次
  if (res.status === 401 && !opts.noAuth) {
    try {
      const t = await ensureRefresh()
      headers['Authorization'] = `Bearer ${t.accessToken}`
      res = await fetch(url, {
        method: opts.method || 'GET',
        headers,
        body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      })
    } catch {
      throw new ApiError(401, 'unauthorized', '会话已过期,请重新登录')
    }
  }

  if (res.status === 204) {
    return undefined as T
  }

  let payload: unknown = null
  try {
    payload = await res.json()
  } catch {
    payload = null
  }

  if (!res.ok) {
    if (res.status === 401) {
      clearTokens()
      onUnauthorized?.()
    }
    const err =
      payload && typeof payload === 'object' && 'error' in payload
        ? (payload as { error: { code: string; message: string } }).error
        : { code: 'http_' + res.status, message: res.statusText || '请求失败' }
    throw new ApiError(res.status, err.code, err.message)
  }
  return payload as T
}

function buildURL(path: string, q?: Record<string, string | number | boolean | undefined>): string {
  if (!q) return path
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries(q)) {
    if (v === undefined || v === null || v === '') continue
    params.set(k, String(v))
  }
  const s = params.toString()
  return s ? `${path}?${s}` : path
}

// =============================================================
// API: auth
// =============================================================
export const auth = {
  async register(input: {
    email: string
    password: string
    display_name?: string
    timezone?: string
  }): Promise<AuthResponse> {
    return request('/api/auth/register', { method: 'POST', body: input, noAuth: true })
  },
  async login(input: { email: string; password: string }): Promise<AuthResponse> {
    return request('/api/auth/login', { method: 'POST', body: input, noAuth: true })
  },
  async logout(refresh_token?: string): Promise<void> {
    return request('/api/auth/logout', { method: 'POST', body: { refresh_token } })
  },
  async me(): Promise<User> {
    return request('/api/auth/me')
  },
  async updateMe(input: { display_name?: string; timezone?: string }): Promise<User> {
    return request('/api/auth/me', { method: 'PATCH', body: input })
  },
  // 公开端点:登录页用它判断后端是不是启了 OAuth(决定显示哪种登录形式)。
  async config(): Promise<AuthConfig> {
    return request('/api/auth/config', { noAuth: true })
  },
  // OAuth 流程的最后一步:前端在 /oauth/callback 拿到 fragment 里的 handoff code 后,
  // 用它换本服务的 access/refresh token。
  async oauthFinalize(code: string): Promise<AuthResponse> {
    return request('/api/auth/oauth/finalize', { method: 'POST', body: { code }, noAuth: true })
  },
}

// =============================================================
// API: lists
// =============================================================
export const lists = {
  async list(): Promise<List[]> {
    const res = await request<{ items: List[] }>('/api/lists')
    return res.items || []
  },
  async create(body: Partial<List>): Promise<List> {
    return request('/api/lists', { method: 'POST', body })
  },
  async update(id: number, body: Partial<List>): Promise<List> {
    return request(`/api/lists/${id}`, { method: 'PUT', body })
  },
  async remove(id: number): Promise<void> {
    return request(`/api/lists/${id}`, { method: 'DELETE' })
  },
}

// =============================================================
// API: todos
// =============================================================
export const todos = {
  async list(query: {
    filter?: string
    list_id?: number
    due_on?: string // YYYY-MM-DD：按用户时区取该日的全部 todo
    search?: string
    limit?: number
    offset?: number
    order_by?: string
    include_done?: boolean
  } = {}): Promise<Todo[]> {
    const res = await request<{ items: Todo[] }>('/api/todos', { query })
    return res.items || []
  },
  async get(id: number): Promise<Todo> {
    return request(`/api/todos/${id}`)
  },
  async create(body: TodoInput): Promise<Todo> {
    return request('/api/todos', { method: 'POST', body })
  },
  async update(id: number, body: TodoInput): Promise<Todo> {
    return request(`/api/todos/${id}`, { method: 'PUT', body })
  },
  async remove(id: number): Promise<void> {
    return request(`/api/todos/${id}`, { method: 'DELETE' })
  },
  async complete(id: number): Promise<Todo> {
    return request(`/api/todos/${id}/complete`, { method: 'POST' })
  },
  async uncomplete(id: number): Promise<Todo> {
    return request(`/api/todos/${id}/uncomplete`, { method: 'POST' })
  },
}

// =============================================================
// API: subtasks
// =============================================================
export const subtasks = {
  async list(todoId: number): Promise<Subtask[]> {
    const res = await request<{ items: Subtask[] }>(`/api/todos/${todoId}/subtasks`)
    return res.items || []
  },
  async create(todoId: number, body: { title: string; sort_order?: number }): Promise<Subtask> {
    return request(`/api/todos/${todoId}/subtasks`, { method: 'POST', body })
  },
  async update(id: number, body: { title?: string; sort_order?: number }): Promise<Subtask> {
    return request(`/api/subtasks/${id}`, { method: 'PUT', body })
  },
  async remove(id: number): Promise<void> {
    return request(`/api/subtasks/${id}`, { method: 'DELETE' })
  },
  async complete(id: number): Promise<Subtask> {
    return request(`/api/subtasks/${id}/complete`, { method: 'POST' })
  },
  async uncomplete(id: number): Promise<Subtask> {
    return request(`/api/subtasks/${id}/uncomplete`, { method: 'POST' })
  },
}

// =============================================================
// API: reminders
// =============================================================
export const reminders = {
  async list(query: { todo_id?: number; only_enabled?: boolean } = {}): Promise<ReminderRule[]> {
    const res = await request<{ items: ReminderRule[] }>('/api/reminders', { query })
    return res.items || []
  },
  async get(id: number): Promise<ReminderRule> {
    return request(`/api/reminders/${id}`)
  },
  async create(body: ReminderInput): Promise<ReminderRule> {
    return request('/api/reminders', { method: 'POST', body })
  },
  async update(id: number, body: ReminderInput): Promise<ReminderRule> {
    return request(`/api/reminders/${id}`, { method: 'PUT', body })
  },
  async remove(id: number): Promise<void> {
    return request(`/api/reminders/${id}`, { method: 'DELETE' })
  },
  async enable(id: number): Promise<ReminderRule> {
    return request(`/api/reminders/${id}/enable`, { method: 'POST' })
  },
  async disable(id: number): Promise<ReminderRule> {
    return request(`/api/reminders/${id}/disable`, { method: 'POST' })
  },
}

// =============================================================
// API: notifications
// =============================================================
export const notifications = {
  async list(query: { only_unread?: boolean; limit?: number; offset?: number } = {}): Promise<{
    items: Notification[]
    unread_count: number
  }> {
    return request('/api/notifications', { query })
  },
  async unreadCount(): Promise<{ count: number }> {
    return request('/api/notifications/unread-count')
  },
  async get(id: number): Promise<NotificationDetail> {
    return request(`/api/notifications/${id}`)
  },
  async markRead(id: number): Promise<void> {
    return request(`/api/notifications/${id}/read`, { method: 'POST' })
  },
  async markAllRead(): Promise<void> {
    return request('/api/notifications/read-all', { method: 'POST' })
  },
}

// =============================================================
// API: telegram
// =============================================================
export const telegram = {
  async getConfig(): Promise<TelegramConfig> {
    // 此端点不需要登录态——前端在绑定页打开时即可探测
    return request('/api/telegram/config', { noAuth: true })
  },
  async createBindToken(): Promise<TelegramBindToken> {
    return request('/api/telegram/bind-token', { method: 'POST' })
  },
  async bindStatus(token: string): Promise<TelegramBindStatus> {
    return request('/api/telegram/bind-status', { query: { token } })
  },
  async listBindings(): Promise<TelegramBinding[]> {
    const res = await request<{ items: TelegramBinding[] }>('/api/telegram/bindings')
    return res.items || []
  },
  async unbind(id: number): Promise<void> {
    return request('/api/telegram/unbind', { method: 'POST', body: { id } })
  },
  async sendTest(binding_id: number): Promise<void> {
    return request('/api/telegram/test', { method: 'POST', body: { binding_id } })
  },
}

// =============================================================
// API: pomodoro
// =============================================================
export const pomodoro = {
  async list(query: {
    todo_id?: number
    status?: string
    kind?: string
    from?: string
    to?: string
    limit?: number
    offset?: number
  } = {}): Promise<PomodoroSession[]> {
    const res = await request<{ items: PomodoroSession[] }>('/api/pomodoro/sessions', { query })
    return res.items || []
  },
  async create(body: {
    todo_id?: number | null
    planned_duration_seconds: number
    kind?: PomodoroKind
    note?: string
  }): Promise<PomodoroSession> {
    return request('/api/pomodoro/sessions', { method: 'POST', body })
  },
  async updateNote(id: number, note: string): Promise<PomodoroSession> {
    return request(`/api/pomodoro/sessions/${id}`, { method: 'PUT', body: { note } })
  },
  async complete(id: number): Promise<PomodoroSession> {
    return request(`/api/pomodoro/sessions/${id}/complete`, { method: 'POST' })
  },
  async abandon(id: number): Promise<PomodoroSession> {
    return request(`/api/pomodoro/sessions/${id}/abandon`, { method: 'POST' })
  },
  async remove(id: number): Promise<void> {
    return request(`/api/pomodoro/sessions/${id}`, { method: 'DELETE' })
  },
}

// =============================================================
// API: stats
// =============================================================
export const stats = {
  async summary(): Promise<StatsSummary> {
    return request('/api/stats/summary')
  },
  async daily(query: { from?: string; to?: string } = {}): Promise<{
    from: string
    to: string
    items: DailyBucket[]
  }> {
    return request('/api/stats/daily', { query })
  },
  async weekly(query: { from?: string; to?: string } = {}): Promise<{
    from: string
    to: string
    items: WeeklyBucket[]
  }> {
    return request('/api/stats/weekly', { query })
  },
  async pomodoro(query: { from?: string; to?: string } = {}): Promise<PomodoroAggregate> {
    return request('/api/stats/pomodoro', { query })
  },
}

// =============================================================
// API: sync
// =============================================================
export const sync = {
  async pull(since: number, limit = 500): Promise<SyncPullResponse> {
    return request('/api/sync/pull', { query: { since, limit } })
  },
  async cursor(): Promise<{ cursor: number }> {
    return request('/api/sync/cursor')
  },
}

// =============================================================
// API: preferences (跨端用户偏好,规格 §17)
//
// 每端只展示自己 scope 的开关(见 stores/prefs.ts),所有持久化都经过这一组接口。
// scope 取值:'web' | 'android' | 'windows' | 'common'。
// =============================================================
export interface ServerPreference {
  scope: string
  key: string
  value: string
  updated_at: string
}

export const prefsApi = {
  async list(scope?: string): Promise<ServerPreference[]> {
    const res = await request<{ items: ServerPreference[] }>('/api/me/preferences', {
      query: scope ? { scope } : undefined,
    })
    return res.items || []
  },
  async putOne(scope: string, key: string, value: string): Promise<ServerPreference> {
    return request(`/api/me/preferences/${encodeURIComponent(scope)}/${encodeURIComponent(key)}`, {
      method: 'PUT',
      body: { value },
    })
  },
  async putBulk(items: Array<{ scope: string; key: string; value: string }>): Promise<ServerPreference[]> {
    const res = await request<{ items: ServerPreference[] }>('/api/me/preferences', {
      method: 'PUT',
      body: { items },
    })
    return res.items || []
  },
  async remove(scope: string, key: string): Promise<void> {
    return request(`/api/me/preferences/${encodeURIComponent(scope)}/${encodeURIComponent(key)}`, {
      method: 'DELETE',
    })
  },
}

// =============================================================
// API: admin (仅管理员账号有权访问;非管理员调用会 403)
//
// 后端路由位于 /api/admin/*。本模块只是个简单的 typed wrapper,前端在
// 进入管理面板前会先用 useAuthStore().user.is_admin 做 UI 守卫。
// =============================================================

export interface AdminUserPatchInput {
  is_admin?: boolean
  is_disabled?: boolean
}

export interface AdminCreateUserInput {
  email: string
  password: string
  display_name?: string
  timezone?: string
  is_admin?: boolean
}

export const adminApi = {
  // 系统状态:进程、内存、磁盘、数据库统计
  async system(): Promise<AdminSystemInfo> {
    return request('/api/admin/system')
  },
  // 当前生效配置摘要(只读)
  async settings(): Promise<AdminSettingsView> {
    return request('/api/admin/settings')
  },
  // 用户管理
  async listUsers(query: { search?: string; limit?: number; offset?: number } = {}): Promise<AdminUserListResponse> {
    return request('/api/admin/users', { query })
  },
  async createUser(body: AdminCreateUserInput): Promise<User> {
    return request('/api/admin/users', { method: 'POST', body })
  },
  async patchUser(id: number, body: AdminUserPatchInput): Promise<User> {
    return request(`/api/admin/users/${id}`, { method: 'PATCH', body })
  },
  async deleteUser(id: number): Promise<void> {
    return request(`/api/admin/users/${id}`, { method: 'DELETE' })
  },
  // 审计
  async listAudit(query: {
    search?: string
    action?: string
    actor_id?: number
    from?: string
    to?: string
    limit?: number
    offset?: number
  } = {}): Promise<AuditListResponse> {
    return request('/api/admin/audit', { query })
  },
  // 数据清理。destructive 操作必须传 confirm=true,否则后端会拒绝。
  async cleanup(body: CleanupRequest): Promise<CleanupResponse> {
    return request('/api/admin/cleanup', { method: 'POST', body })
  },
}

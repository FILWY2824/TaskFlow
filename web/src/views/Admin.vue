<script setup lang="ts">
// 管理面板 —— 仅 is_admin = true 的账号能进入。
// 路由层 / 侧栏层都做了守卫,但本视图本身也再校验一次,直接渲染"无权限"状态。
//
// UI 风格沿用项目既有的 settings-card / card-head / card-body 套件,
// 顶部一个标签条,正文按当前 tab 渲染对应的子区块,与"设置"页一致,
// 不弹层、不抽屉,就是普通的右侧主页面内容。
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { adminApi, ApiError } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { alertDialog, confirmDialog } from '@/dialogs'
import type {
  AdminSettingsView,
  AdminSystemInfo,
  AdminUserRow,
  AuditLogEntry,
  CleanupRequest,
  CleanupResponse,
  CleanupScope,
} from '@/types'
import { fmtDateTime } from '@/utils'

const auth = useAuthStore()
const router = useRouter()

// 顶层 tab 切换。
type Tab = 'system' | 'users' | 'audit' | 'cleanup' | 'settings'
const tab = ref<Tab>('system')
const TABS: { id: Tab; label: string; icon: string }[] = [
  { id: 'system',   label: '系统状态', icon: 'gauge' },
  { id: 'users',    label: '用户管理', icon: 'users' },
  { id: 'audit',    label: '审计日志', icon: 'clipboard' },
  { id: 'cleanup',  label: '数据清理', icon: 'broom' },
  { id: 'settings', label: '系统设置', icon: 'cog' },
]

const isAdmin = computed(() => !!auth.user?.is_admin)

// === 全局错误 / 提示 ===
const okMsg = ref('')
const errMsg = ref('')
function flashOk(m: string)  { okMsg.value  = m; setTimeout(() => (okMsg.value  = ''), 3000) }
function flashErr(m: string) { errMsg.value = m; setTimeout(() => (errMsg.value = ''), 5000) }
function captureErr(e: unknown): string {
  if (e instanceof ApiError) return `${e.message}（${e.code}）`
  return (e as Error)?.message || String(e)
}

// =============================================================
// 系统状态
// =============================================================
const sys = ref<AdminSystemInfo | null>(null)
const sysLoading = ref(false)
let sysTimer: ReturnType<typeof setInterval> | null = null

async function loadSystem() {
  sysLoading.value = true
  try {
    sys.value = await adminApi.system()
  } catch (e) {
    flashErr(captureErr(e))
  } finally {
    sysLoading.value = false
  }
}

function startSystemPolling() {
  stopSystemPolling()
  sysTimer = setInterval(() => {
    if (tab.value === 'system' && document.visibilityState === 'visible') {
      void loadSystem()
    }
  }, 10_000)
}
function stopSystemPolling() {
  if (sysTimer) { clearInterval(sysTimer); sysTimer = null }
}

function fmtBytes(n: number | null | undefined): string {
  if (n === null || n === undefined || !isFinite(n)) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n; let i = 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  const p = v >= 100 ? 0 : v >= 10 ? 1 : 2
  return `${v.toFixed(p)} ${units[i]}`
}
function fmtUptime(seconds: number | null | undefined): string {
  if (!seconds || seconds < 0) return '—'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (d > 0) return `${d}天 ${h}小时 ${m}分`
  if (h > 0) return `${h}小时 ${m}分钟`
  if (m > 0) return `${m}分 ${s}秒`
  return `${s}秒`
}

// =============================================================
// 用户管理
// =============================================================
const users = ref<AdminUserRow[]>([])
const usersTotal = ref(0)
const usersLoading = ref(false)
const usersSearch = ref('')
const usersOffset = ref(0)
const USERS_LIMIT = 50

async function loadUsers() {
  usersLoading.value = true
  try {
    const r = await adminApi.listUsers({
      search: usersSearch.value.trim() || undefined,
      limit: USERS_LIMIT,
      offset: usersOffset.value,
    })
    users.value = r.items
    usersTotal.value = r.total
  } catch (e) {
    flashErr(captureErr(e))
  } finally {
    usersLoading.value = false
  }
}
function nextUsersPage() {
  if (usersOffset.value + USERS_LIMIT < usersTotal.value) {
    usersOffset.value += USERS_LIMIT
    void loadUsers()
  }
}
function prevUsersPage() {
  if (usersOffset.value > 0) {
    usersOffset.value = Math.max(0, usersOffset.value - USERS_LIMIT)
    void loadUsers()
  }
}
function searchUsers() {
  usersOffset.value = 0
  void loadUsers()
}

async function toggleAdmin(u: AdminUserRow) {
  const next = !u.is_admin
  const ok = await confirmDialog({
    title: next ? `提升 ${u.email} 为管理员?` : `撤销 ${u.email} 的管理员?`,
    message: next
      ? '管理员可以查看本面板、管理用户、清理数据。请仅授予可信账号。'
      : '撤销后该账号将无法再访问管理面板。',
    confirmText: next ? '提升' : '撤销',
    cancelText: '取消',
    danger: !next,
  })
  if (!ok) return
  try {
    await adminApi.patchUser(u.id, { is_admin: next })
    flashOk('已更新管理员状态')
    await loadUsers()
  } catch (e) { flashErr(captureErr(e)) }
}

async function toggleDisabled(u: AdminUserRow) {
  if (u.id === auth.user?.id) {
    await alertDialog({ title: '不能禁用自己', message: '请由其他管理员操作。', confirmText: '知道了' })
    return
  }
  const next = !u.is_disabled
  const ok = await confirmDialog({
    title: next ? `禁用 ${u.email}?` : `启用 ${u.email}?`,
    message: next
      ? '禁用后,该用户将无法登录,且现有登录会话会被立即注销。账号数据保留。'
      : '启用后,该用户可恢复登录。',
    confirmText: next ? '禁用' : '启用',
    cancelText: '取消',
    danger: next,
  })
  if (!ok) return
  try {
    await adminApi.patchUser(u.id, { is_disabled: next })
    flashOk(next ? '已禁用' : '已启用')
    await loadUsers()
  } catch (e) { flashErr(captureErr(e)) }
}

async function deleteUser(u: AdminUserRow) {
  if (u.id === auth.user?.id) {
    await alertDialog({ title: '不能删除自己', message: '请由其他管理员操作。', confirmText: '知道了' })
    return
  }
  const ok = await confirmDialog({
    title: `永久删除用户 ${u.email}?`,
    message: '该账号下的全部任务、清单、提醒、番茄记录都会被一并删除,无法恢复。',
    confirmText: '永久删除',
    cancelText: '取消',
    danger: true,
  })
  if (!ok) return
  try {
    await adminApi.deleteUser(u.id)
    flashOk('用户已删除')
    await loadUsers()
  } catch (e) { flashErr(captureErr(e)) }
}

// 新建用户 modal
const showNewUser = ref(false)
const newUserForm = reactive({
  email: '',
  password: '',
  display_name: '',
  is_admin: false,
})
function openNewUser() {
  newUserForm.email = ''
  newUserForm.password = ''
  newUserForm.display_name = ''
  newUserForm.is_admin = false
  showNewUser.value = true
}
async function submitNewUser() {
  if (!newUserForm.email.trim() || newUserForm.password.length < 8) {
    flashErr('邮箱必填,密码至少 8 位')
    return
  }
  try {
    await adminApi.createUser({
      email: newUserForm.email.trim(),
      password: newUserForm.password,
      display_name: newUserForm.display_name.trim() || undefined,
      is_admin: newUserForm.is_admin,
    })
    showNewUser.value = false
    flashOk('用户已创建')
    usersOffset.value = 0
    await loadUsers()
  } catch (e) { flashErr(captureErr(e)) }
}

// =============================================================
// 审计日志
// =============================================================
const audit = ref<AuditLogEntry[]>([])
const auditTotal = ref(0)
const auditLoading = ref(false)
const auditSearch = ref('')
const auditAction = ref('')
const auditOffset = ref(0)
const AUDIT_LIMIT = 100

async function loadAudit() {
  auditLoading.value = true
  try {
    const r = await adminApi.listAudit({
      search: auditSearch.value.trim() || undefined,
      action: auditAction.value.trim() || undefined,
      limit: AUDIT_LIMIT,
      offset: auditOffset.value,
    })
    audit.value = r.items
    auditTotal.value = r.total
  } catch (e) {
    flashErr(captureErr(e))
  } finally {
    auditLoading.value = false
  }
}
function searchAudit() {
  auditOffset.value = 0
  void loadAudit()
}
function nextAuditPage() {
  if (auditOffset.value + AUDIT_LIMIT < auditTotal.value) {
    auditOffset.value += AUDIT_LIMIT
    void loadAudit()
  }
}
function prevAuditPage() {
  if (auditOffset.value > 0) {
    auditOffset.value = Math.max(0, auditOffset.value - AUDIT_LIMIT)
    void loadAudit()
  }
}

// =============================================================
// 数据清理
// =============================================================
type CleanupTask = {
  scope: CleanupScope
  title: string
  desc: string
  hasDays: boolean
  days: number
  busy: boolean
  lastResult?: CleanupResponse
}
const cleanupTasks = reactive<CleanupTask[]>([
  { scope: 'completed_todos',    title: '清理已完成任务', desc: '物理删除已完成超过 N 天的任务(慎用,删除后无法恢复)。', hasDays: true,  days: 90, busy: false },
  { scope: 'soft_deleted_todos', title: '清理已删除任务', desc: '物理删除"软删除"超过 N 天的任务,释放空间。',         hasDays: true,  days: 30, busy: false },
  { scope: 'soft_deleted_lists', title: '清理已删除分类', desc: '物理删除"软删除"超过 N 天的分类。',                  hasDays: true,  days: 30, busy: false },
  { scope: 'old_notifications',  title: '清理旧通知',    desc: '删除创建时间早于 N 天前的通知记录。',                  hasDays: true,  days: 60, busy: false },
  { scope: 'old_pomodoros',      title: '清理旧番茄记录', desc: '删除创建时间早于 N 天前的番茄会话记录。',              hasDays: true,  days: 180, busy: false },
  { scope: 'audit_logs',         title: '清理审计日志',  desc: '删除早于 N 天前的审计日志(不影响管理员当前操作)。', hasDays: true,  days: 180, busy: false },
  { scope: 'expired_refresh',    title: '清理过期 Token', desc: '清理已过期 / 已撤销超过 7 天的 refresh token。',     hasDays: false, days: 0,  busy: false },
  { scope: 'vacuum',             title: '数据库 VACUUM', desc: '回收碎片空间,SQLite 文件可能临时翻倍,执行期间会短暂阻塞写入。', hasDays: false, days: 0, busy: false },
])

async function runCleanup(t: CleanupTask, dryRun: boolean) {
  const req: CleanupRequest = { scope: t.scope, dry_run: dryRun, confirm: !dryRun }
  if (t.hasDays) req.days = Math.max(1, Math.floor(t.days))
  if (!dryRun) {
    const ok = await confirmDialog({
      title: `执行：${t.title}?`,
      message: t.hasDays
        ? `将物理删除满足条件且早于 ${req.days} 天的数据,无法恢复。建议先用"试运行"确认数量。`
        : `这会立即修改数据库,确认要执行?`,
      confirmText: '执行',
      cancelText: '取消',
      danger: true,
    })
    if (!ok) return
  }
  t.busy = true
  try {
    t.lastResult = await adminApi.cleanup(req)
    flashOk(dryRun
      ? `试运行完成,预计影响 ${t.lastResult.affected} 条`
      : t.lastResult.message || `已完成,影响 ${t.lastResult.affected} 条`)
  } catch (e) { flashErr(captureErr(e)) }
  finally { t.busy = false }
}

// =============================================================
// 系统设置(只读)
// =============================================================
const settings = ref<AdminSettingsView | null>(null)
const settingsLoading = ref(false)

async function loadSettings() {
  settingsLoading.value = true
  try {
    settings.value = await adminApi.settings()
  } catch (e) { flashErr(captureErr(e)) }
  finally { settingsLoading.value = false }
}

// =============================================================
// 生命周期
// =============================================================
function switchTab(t: Tab) {
  tab.value = t
  // 切到新 tab 时加载该 tab 的数据(若为空)
  if (t === 'system'   && !sys.value)      void loadSystem()
  if (t === 'users'    && users.value.length === 0) void loadUsers()
  if (t === 'audit'    && audit.value.length === 0) void loadAudit()
  if (t === 'settings' && !settings.value) void loadSettings()
}

onMounted(() => {
  if (!isAdmin.value) {
    // 双保险:非管理员直接踢回日程
    void router.replace({ name: 'schedule' })
    return
  }
  void loadSystem()
  startSystemPolling()
})
onBeforeUnmount(() => {
  stopSystemPolling()
})
</script>

<template>
  <div class="admin-page" v-if="isAdmin">
    <Transition name="fade">
      <div v-if="okMsg" class="banner banner-ok">{{ okMsg }}</div>
    </Transition>
    <Transition name="fade">
      <div v-if="errMsg" class="banner banner-err">{{ errMsg }}</div>
    </Transition>

    <!-- ========== Tab 条 ========== -->
    <div class="admin-tabs" role="tablist">
      <button
        v-for="t in TABS"
        :key="t.id"
        role="tab"
        :aria-selected="tab === t.id"
        class="admin-tab"
        :class="{ 'is-active': tab === t.id }"
        @click="switchTab(t.id)"
      >
        <span class="admin-tab-icon" v-html="tabIcon(t.icon)"></span>
        <span>{{ t.label }}</span>
      </button>
    </div>

    <!-- ========== 系统状态 ========== -->
    <section v-if="tab === 'system'" class="admin-section">
      <div class="settings-card">
        <div class="card-head">
          <h3>进程信息</h3>
          <button class="btn-secondary btn-sm" :disabled="sysLoading" @click="loadSystem">
            {{ sysLoading ? '刷新中…' : '立即刷新' }}
          </button>
        </div>
        <div class="card-body">
          <div v-if="!sys" class="muted">加载中…</div>
          <template v-else>
            <div class="kv-grid">
              <div><label>版本</label><span>{{ sys.version }}</span></div>
              <div><label>Go</label><span>{{ sys.go_version }}</span></div>
              <div><label>OS / Arch</label><span>{{ sys.os }} / {{ sys.arch }}</span></div>
              <div><label>CPU</label><span>{{ sys.num_cpu }} 核</span></div>
              <div><label>Goroutine</label><span>{{ sys.num_goroutine }}</span></div>
              <div><label>启动于</label><span>{{ fmtDateTime(sys.started_at) }}</span></div>
              <div><label>已运行</label><span>{{ fmtUptime(sys.uptime_seconds) }}</span></div>
              <div><label>OAuth</label><span>{{ sys.oauth_enabled ? '已启用' : '未启用' }}</span></div>
            </div>
          </template>
        </div>
      </div>

      <div class="settings-card">
        <div class="card-head"><h3>内存(Go 进程内)</h3></div>
        <div class="card-body">
          <div v-if="!sys" class="muted">—</div>
          <div v-else class="kv-grid">
            <div><label>当前堆分配</label><span>{{ fmtBytes(sys.memory.alloc_bytes) }}</span></div>
            <div><label>累计分配</label><span>{{ fmtBytes(sys.memory.total_alloc_bytes) }}</span></div>
            <div><label>系统申请</label><span>{{ fmtBytes(sys.memory.sys_bytes) }}</span></div>
            <div><label>HeapInUse</label><span>{{ fmtBytes(sys.memory.heap_inuse_bytes) }}</span></div>
            <div><label>HeapIdle</label><span>{{ fmtBytes(sys.memory.heap_idle_bytes) }}</span></div>
            <div><label>GC 次数</label><span>{{ sys.memory.num_gc }}</span></div>
          </div>
        </div>
      </div>

      <div class="settings-card">
        <div class="card-head"><h3>磁盘(数据库所在分区)</h3></div>
        <div class="card-body">
          <div v-if="!sys" class="muted">—</div>
          <template v-else-if="sys.disk.total_bytes > 0">
            <div class="kv-grid">
              <div><label>路径</label><span>{{ sys.disk.path }}</span></div>
              <div><label>容量</label><span>{{ fmtBytes(sys.disk.total_bytes) }}</span></div>
              <div><label>已用</label><span>{{ fmtBytes(sys.disk.used_bytes) }} ({{ sys.disk.used_percent.toFixed(1) }}%)</span></div>
              <div><label>可用</label><span>{{ fmtBytes(sys.disk.free_bytes) }}</span></div>
            </div>
            <div class="bar-wrap">
              <div class="bar-fill"
                   :class="{ 'is-warn': sys.disk.used_percent >= 80, 'is-danger': sys.disk.used_percent >= 92 }"
                   :style="{ width: Math.min(100, sys.disk.used_percent) + '%' }" />
            </div>
          </template>
          <div v-else class="muted">该平台暂未提供磁盘统计</div>
        </div>
      </div>

      <div class="settings-card">
        <div class="card-head"><h3>数据库</h3></div>
        <div class="card-body">
          <div v-if="!sys" class="muted">—</div>
          <div v-else class="kv-grid">
            <div><label>主文件</label><span>{{ fmtBytes(sys.database.file_size_bytes) }}</span></div>
            <div><label>WAL 文件</label><span>{{ fmtBytes(sys.database.wal_file_size_bytes) }}</span></div>
            <div><label>页大小 × 页数</label><span>{{ fmtBytes(sys.database.page_size) }} × {{ sys.database.page_count }}</span></div>
            <div><label>用户数</label><span>{{ sys.database.user_count }}</span></div>
            <div><label>任务数</label><span>{{ sys.database.todo_count }}</span></div>
            <div><label>分类数</label><span>{{ sys.database.list_count }}</span></div>
            <div><label>提醒规则</label><span>{{ sys.database.reminder_count }}</span></div>
            <div><label>通知</label><span>{{ sys.database.notification_count }}</span></div>
            <div><label>番茄记录</label><span>{{ sys.database.pomodoro_count }}</span></div>
            <div><label>审计日志</label><span>{{ sys.database.audit_count }}</span></div>
          </div>
        </div>
      </div>
    </section>

    <!-- ========== 用户管理 ========== -->
    <section v-if="tab === 'users'" class="admin-section">
      <div class="settings-card">
        <div class="card-head">
          <h3>用户管理</h3>
          <div class="row-actions">
            <button class="btn-secondary btn-sm" @click="openNewUser">新建用户</button>
          </div>
        </div>
        <div class="card-body">
          <div class="users-toolbar">
            <div class="pretty-input-wrap" style="flex:1; max-width:360px">
              <input v-model="usersSearch" class="pretty-input"
                     placeholder="按邮箱 / 显示名搜索…" @keydown.enter="searchUsers" />
              <span class="pretty-input-glow" aria-hidden="true" />
            </div>
            <button class="btn-secondary btn-sm" @click="searchUsers">搜索</button>
            <span class="muted small">共 {{ usersTotal }} 个账号</span>
          </div>

          <div class="admin-table-wrap">
            <table class="admin-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>邮箱</th>
                  <th>显示名</th>
                  <th class="cell-num">任务</th>
                  <th>注册</th>
                  <th>最近活跃</th>
                  <th>状态</th>
                  <th class="cell-actions">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="usersLoading"><td colspan="8" class="muted">加载中…</td></tr>
                <tr v-else-if="users.length === 0"><td colspan="8" class="muted">没有数据</td></tr>
                <tr v-for="u in users" :key="u.id" :class="{ 'row-disabled': u.is_disabled }">
                  <td class="mono">{{ u.id }}</td>
                  <td>{{ u.email }}<span v-if="u.id === auth.user?.id" class="self-tag">本人</span></td>
                  <td>{{ u.display_name || '—' }}</td>
                  <td class="cell-num">{{ u.todo_count }}</td>
                  <td class="muted small">{{ fmtDateTime(u.created_at) }}</td>
                  <td class="muted small">{{ u.last_login_at ? fmtDateTime(u.last_login_at) : '—' }}</td>
                  <td>
                    <span v-if="u.is_admin" class="pill pill-admin">管理员</span>
                    <span v-if="u.is_disabled" class="pill pill-disabled">已禁用</span>
                    <span v-if="!u.is_admin && !u.is_disabled" class="pill pill-normal">普通</span>
                  </td>
                  <td class="cell-actions">
                    <button class="btn-ghost btn-sm" @click="toggleAdmin(u)">
                      {{ u.is_admin ? '撤销管理' : '设为管理' }}
                    </button>
                    <button class="btn-ghost btn-sm" :disabled="u.id === auth.user?.id" @click="toggleDisabled(u)">
                      {{ u.is_disabled ? '启用' : '禁用' }}
                    </button>
                    <button class="btn-ghost btn-danger btn-sm" :disabled="u.id === auth.user?.id" @click="deleteUser(u)">
                      删除
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="pager">
            <button class="btn-secondary btn-sm" :disabled="usersOffset === 0 || usersLoading" @click="prevUsersPage">上一页</button>
            <span class="muted small">{{ usersOffset + 1 }} ~ {{ Math.min(usersOffset + USERS_LIMIT, usersTotal) }} / {{ usersTotal }}</span>
            <button class="btn-secondary btn-sm" :disabled="usersOffset + USERS_LIMIT >= usersTotal || usersLoading" @click="nextUsersPage">下一页</button>
          </div>
        </div>
      </div>
    </section>

    <!-- ========== 审计日志 ========== -->
    <section v-if="tab === 'audit'" class="admin-section">
      <div class="settings-card">
        <div class="card-head">
          <h3>审计日志</h3>
          <button class="btn-secondary btn-sm" :disabled="auditLoading" @click="loadAudit">
            {{ auditLoading ? '刷新中…' : '刷新' }}
          </button>
        </div>
        <div class="card-body">
          <div class="users-toolbar">
            <div class="pretty-input-wrap" style="flex:1; max-width:320px">
              <input v-model="auditSearch" class="pretty-input"
                     placeholder="搜索:邮箱 / 动作 / 详情…" @keydown.enter="searchAudit" />
              <span class="pretty-input-glow" aria-hidden="true" />
            </div>
            <div class="pretty-input-wrap" style="width:200px">
              <input v-model="auditAction" class="pretty-input"
                     placeholder="精确 action(如 user.delete)" @keydown.enter="searchAudit" />
              <span class="pretty-input-glow" aria-hidden="true" />
            </div>
            <button class="btn-secondary btn-sm" @click="searchAudit">搜索</button>
          </div>

          <div class="admin-table-wrap">
            <table class="admin-table">
              <thead>
                <tr>
                  <th>时间</th>
                  <th>操作者</th>
                  <th>动作</th>
                  <th>对象</th>
                  <th>详情</th>
                  <th>IP</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="auditLoading"><td colspan="6" class="muted">加载中…</td></tr>
                <tr v-else-if="audit.length === 0"><td colspan="6" class="muted">没有日志</td></tr>
                <tr v-for="a in audit" :key="a.id">
                  <td class="muted small mono">{{ fmtDateTime(a.created_at) }}</td>
                  <td>{{ a.actor_email || '系统' }}</td>
                  <td><span class="pill pill-action">{{ a.action }}</span></td>
                  <td class="mono small">{{ a.target_type ? `${a.target_type}#${a.target_id}` : '—' }}</td>
                  <td class="small">{{ a.detail || '—' }}</td>
                  <td class="muted small mono">{{ a.ip || '—' }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="pager">
            <button class="btn-secondary btn-sm" :disabled="auditOffset === 0 || auditLoading" @click="prevAuditPage">上一页</button>
            <span class="muted small">{{ auditOffset + 1 }} ~ {{ Math.min(auditOffset + AUDIT_LIMIT, auditTotal) }} / {{ auditTotal }}</span>
            <button class="btn-secondary btn-sm" :disabled="auditOffset + AUDIT_LIMIT >= auditTotal || auditLoading" @click="nextAuditPage">下一页</button>
          </div>
        </div>
      </div>
    </section>

    <!-- ========== 数据清理 ========== -->
    <section v-if="tab === 'cleanup'" class="admin-section">
      <div class="settings-card">
        <div class="card-head">
          <h3>数据清理</h3>
          <p class="card-hint">所有"试运行"只统计不修改;实际执行前会再弹一次确认。</p>
        </div>
        <div class="card-body">
          <div class="cleanup-grid">
            <div v-for="t in cleanupTasks" :key="t.scope" class="cleanup-card">
              <div class="cleanup-title">{{ t.title }}</div>
              <div class="cleanup-desc">{{ t.desc }}</div>
              <div v-if="t.hasDays" class="cleanup-row">
                <label>保留天数</label>
                <input v-model.number="t.days" type="number" min="1" max="3650" class="pretty-input" style="width:88px" />
              </div>
              <div class="cleanup-actions">
                <button class="btn-secondary btn-sm" :disabled="t.busy" @click="runCleanup(t, true)">试运行</button>
                <button class="btn-primary btn-sm" :disabled="t.busy" @click="runCleanup(t, false)">
                  {{ t.busy ? '执行中…' : '执行' }}
                </button>
              </div>
              <div v-if="t.lastResult" class="cleanup-result muted small">
                上次:{{ t.lastResult.dry_run ? '试运行' : '实际执行' }},影响 <b>{{ t.lastResult.affected }}</b> 条
                <span v-if="t.lastResult.message">— {{ t.lastResult.message }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ========== 系统设置(只读) ========== -->
    <section v-if="tab === 'settings'" class="admin-section">
      <div class="settings-card">
        <div class="card-head">
          <h3>系统设置(只读)</h3>
          <p class="card-hint">这些值由 config.toml / .env 注入,运行时不可修改;改完需要重启服务进程。</p>
        </div>
        <div class="card-body">
          <div v-if="!settings" class="muted">{{ settingsLoading ? '加载中…' : '点击右上角刷新' }}</div>
          <div v-else class="kv-grid">
            <div><label>监听</label><span class="mono">{{ settings.server_listen }}</span></div>
            <div><label>数据库</label><span class="mono">{{ settings.database_path }}</span></div>
            <div><label>Access TTL</label><span>{{ settings.access_ttl_seconds }} 秒</span></div>
            <div><label>Refresh TTL</label><span>{{ settings.refresh_ttl_seconds }} 秒</span></div>
            <div><label>Bcrypt cost</label><span>{{ settings.bcrypt_cost }}</span></div>
            <div><label>OAuth</label><span>{{ settings.oauth_enabled ? '启用' : '未启用' }}</span></div>
            <div v-if="settings.oauth_enabled"><label>OAuth provider</label><span>{{ settings.oauth_provider || '—' }}</span></div>
            <div v-if="settings.oauth_enabled"><label>OAuth redirect</label><span class="mono small">{{ settings.oauth_redirect_url || '—' }}</span></div>
            <div><label>Telegram bot</label><span>{{ settings.telegram_bot_enabled ? `启用 (@${settings.telegram_bot_username})` : '未启用' }}</span></div>
            <div><label>调度器</label><span>{{ settings.scheduler_disabled ? '已停用' : `${settings.scheduler_tick_seconds}s tick / ${settings.scheduler_batch_size}/批` }}</span></div>
            <div v-if="settings.admin_bootstrap_email"><label>引导管理员邮箱</label><span class="mono">{{ settings.admin_bootstrap_email }}</span></div>
          </div>
          <div class="card-actions">
            <button class="btn-secondary btn-sm" :disabled="settingsLoading" @click="loadSettings">
              {{ settingsLoading ? '刷新中…' : '刷新' }}
            </button>
          </div>
        </div>
      </div>
    </section>

    <!-- ========== 新建用户 modal ========== -->
    <Transition name="fade">
      <div v-if="showNewUser" class="modal-backdrop" @click.self="showNewUser = false">
        <div class="modal-card">
          <header class="modal-head">
            <span class="modal-title">新建用户</span>
            <button class="btn-icon" @click="showNewUser = false" aria-label="关闭">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </header>
          <div class="modal-body">
            <div class="field">
              <label>邮箱</label>
              <div class="pretty-input-wrap">
                <input v-model="newUserForm.email" class="pretty-input" type="email" autofocus placeholder="user@example.com" />
                <span class="pretty-input-glow" aria-hidden="true" />
              </div>
            </div>
            <div class="field">
              <label>密码(≥ 8 位)</label>
              <div class="pretty-input-wrap">
                <input v-model="newUserForm.password" class="pretty-input" type="password" placeholder="至少 8 位" />
                <span class="pretty-input-glow" aria-hidden="true" />
              </div>
            </div>
            <div class="field">
              <label>显示名(可选)</label>
              <div class="pretty-input-wrap">
                <input v-model="newUserForm.display_name" class="pretty-input" maxlength="64" />
                <span class="pretty-input-glow" aria-hidden="true" />
              </div>
            </div>
            <label class="check-row">
              <input type="checkbox" v-model="newUserForm.is_admin" />
              <span>同时设为管理员</span>
            </label>
          </div>
          <footer class="modal-foot">
            <button class="btn-secondary" @click="showNewUser = false">取消</button>
            <button class="btn-primary" @click="submitNewUser">创建</button>
          </footer>
        </div>
      </div>
    </Transition>
  </div>

  <div v-else class="admin-page">
    <div class="settings-card">
      <div class="card-body">
        <div class="muted">无权限。仅管理员可访问本页面。</div>
      </div>
    </div>
  </div>
</template>

<script lang="ts">
// 简单 inline 图标(避免引入图标库)
function tabIcon(name: string): string {
  const stroke = `stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"`
  const wrap = (inner: string) => `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" ${stroke}>${inner}</svg>`
  switch (name) {
    case 'gauge':     return wrap(`<path d="M12 14l3-3"/><circle cx="12" cy="14" r="9"/><path d="M3 14a9 9 0 0 1 18 0"/>`)
    case 'users':     return wrap(`<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>`)
    case 'clipboard': return wrap(`<path d="M9 5H7a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2"/><rect x="9" y="3" width="6" height="4" rx="1"/>`)
    case 'broom':     return wrap(`<path d="M19 5l-7 7"/><path d="M14 4l6 6"/><path d="M3 21l6-6"/><path d="M5 21h7l3-3-7-7-3 3v7z"/>`)
    case 'cog':       return wrap(`<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.6 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.6 1.65 1.65 0 0 0 10 3.09V3a2 2 0 0 1 4 0v.09A1.65 1.65 0 0 0 15 4.6a1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9c.39.16.78.39 1.09.7"/>`)
    default:          return wrap('')
  }
}
export { tabIcon }
</script>

<style scoped>
.admin-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 4px 0 80px;
  max-width: 1100px;
  margin: 0 auto;
  width: 100%;
}

.banner {
  padding: 10px 14px;
  border-radius: var(--tg-radius-sm);
  font-size: 13.5px;
  font-weight: 500;
}
.banner-ok  { background: var(--tg-success-soft, #dcfce7); color: var(--tg-success, #15803d); }
.banner-err { background: var(--tg-danger-soft, #fee2e2); color: var(--tg-danger, #b91c1c); }

/* ===== Tab 条 ===== */
.admin-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 6px;
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-md);
  box-shadow: var(--tg-shadow-xs);
}
.admin-tab {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 14px;
  background: transparent;
  border: none;
  border-radius: var(--tg-radius-sm);
  color: var(--tg-text-secondary);
  font-size: 13.5px;
  font-weight: 600;
  cursor: pointer;
  transition: background 120ms, color 120ms;
}
.admin-tab:hover { background: var(--tg-primary-soft); color: var(--tg-primary); }
.admin-tab.is-active {
  background: var(--tg-primary);
  color: var(--tg-on-primary, #fff);
  box-shadow: var(--tg-shadow-sm);
}
.admin-tab-icon { display: inline-flex; }

.admin-section { display: flex; flex-direction: column; gap: 16px; }

/* ===== KV 网格 ===== */
.kv-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px 18px;
}
.kv-grid > div {
  display: flex; align-items: baseline;
  gap: 8px;
  padding: 6px 0;
  border-bottom: 1px dashed var(--tg-divider);
  min-width: 0;
}
.kv-grid label {
  flex-shrink: 0;
  width: 110px;
  font-size: 12px; font-weight: 700;
  color: var(--tg-text-secondary);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}
.kv-grid span {
  flex: 1; min-width: 0;
  word-break: break-all;
  font-size: 13.5px;
  color: var(--tg-text);
}
.mono { font-family: var(--tg-font-mono, ui-monospace, SFMono-Regular, Menlo, monospace); }
.small { font-size: 12px; }

/* ===== 进度条 ===== */
.bar-wrap {
  margin-top: 14px;
  width: 100%;
  height: 10px;
  background: var(--tg-divider);
  border-radius: var(--tg-radius-pill);
  overflow: hidden;
}
.bar-fill {
  height: 100%;
  background: var(--tg-primary);
  transition: width 240ms;
}
.bar-fill.is-warn   { background: #f59e0b; }
.bar-fill.is-danger { background: var(--tg-danger, #ef4444); }

/* ===== 表格 ===== */
.users-toolbar {
  display: flex; gap: 10px; align-items: center; flex-wrap: wrap;
  margin-bottom: 14px;
}
.admin-table-wrap { overflow-x: auto; }
.admin-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13.5px;
}
.admin-table th, .admin-table td {
  padding: 10px 12px;
  text-align: left;
  border-bottom: 1px solid var(--tg-divider);
  vertical-align: middle;
}
.admin-table th {
  font-size: 11.5px; font-weight: 700;
  letter-spacing: 0.04em; text-transform: uppercase;
  color: var(--tg-text-secondary);
  background: color-mix(in srgb, var(--tg-bg) 50%, transparent);
}
.admin-table tr.row-disabled td { opacity: 0.55; }
.admin-table .cell-num { text-align: right; font-variant-numeric: tabular-nums; }
.admin-table .cell-actions {
  text-align: right; white-space: nowrap;
  display: flex; justify-content: flex-end; gap: 4px;
}

.pill {
  display: inline-block;
  padding: 2px 9px;
  margin-right: 4px;
  font-size: 11.5px; font-weight: 700;
  border-radius: var(--tg-radius-pill);
  background: var(--tg-divider);
  color: var(--tg-text-secondary);
}
.pill-admin    { background: var(--tg-primary-soft); color: var(--tg-primary); }
.pill-disabled { background: color-mix(in srgb, var(--tg-danger, #ef4444) 18%, transparent); color: var(--tg-danger, #b91c1c); }
.pill-normal   { background: color-mix(in srgb, var(--tg-success, #16a34a) 16%, transparent); color: var(--tg-success, #15803d); }
.pill-action   { background: var(--tg-primary-soft); color: var(--tg-primary); font-family: var(--tg-font-mono, monospace); font-size: 11px; }
.self-tag {
  margin-left: 6px;
  font-size: 11px; font-weight: 600;
  color: var(--tg-primary);
}

.pager {
  display: flex; gap: 12px; align-items: center; justify-content: flex-end;
  margin-top: 12px;
}

.row-actions { display: flex; gap: 8px; align-items: center; }

/* ===== 清理卡片 ===== */
.cleanup-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 14px;
}
.cleanup-card {
  display: flex; flex-direction: column; gap: 10px;
  padding: 14px;
  background: var(--tg-bg);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-md);
}
.cleanup-title { font-weight: 700; font-size: 14px; }
.cleanup-desc  { font-size: 12.5px; color: var(--tg-text-secondary); line-height: 1.5; }
.cleanup-row {
  display: flex; align-items: center; gap: 10px;
  font-size: 12.5px; color: var(--tg-text-secondary);
}
.cleanup-row label { font-weight: 600; }
.cleanup-actions { display: flex; gap: 8px; margin-top: auto; }
.cleanup-result { padding-top: 6px; border-top: 1px dashed var(--tg-divider); }

/* ===== modal 复用既有规范 ===== */
.modal-backdrop {
  position: fixed; inset: 0;
  background: color-mix(in srgb, #0f172a 40%, transparent);
  z-index: 60;
  display: flex; align-items: center; justify-content: center;
  padding: 20px;
}
.modal-card {
  width: min(440px, 95vw);
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-lg);
  box-shadow: var(--tg-shadow-lg);
  overflow: hidden;
}
.modal-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 18px 22px;
  border-bottom: 1px solid var(--tg-divider);
}
.modal-title { font-weight: 700; font-size: 16px; }
.modal-body  { padding: 18px 22px; display: flex; flex-direction: column; gap: 14px; }
.modal-foot {
  display: flex; gap: 10px; justify-content: flex-end;
  padding: 14px 22px;
  border-top: 1px solid var(--tg-divider);
}

.field { display: flex; flex-direction: column; gap: 6px; }
.field label {
  font-size: 12px; font-weight: 700;
  color: var(--tg-text-secondary);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}
.check-row {
  display: inline-flex; align-items: center; gap: 8px;
  font-size: 13.5px;
  cursor: pointer;
  user-select: none;
}

.card-actions {
  margin-top: 14px;
  display: flex; gap: 8px; justify-content: flex-end;
}

.btn-sm { padding: 6px 12px; font-size: 12.5px; }

@media (max-width: 720px) {
  .admin-tab { padding: 7px 10px; font-size: 12.5px; }
  .admin-tab span:not(.admin-tab-icon) { display: none; }
  .kv-grid { grid-template-columns: 1fr; }
}
</style>

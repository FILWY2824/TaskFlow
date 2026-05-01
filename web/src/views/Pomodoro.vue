<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { pomodoro as pomoApi, todos as todosApi, ApiError } from '@/api'
import type { PomodoroKind, PomodoroSession, Todo } from '@/types'
import { fmtDuration } from '@/utils'
import { useDataStore } from '@/stores/data'
import { usePrefsStore } from '@/stores/prefs'
import { useNotificationsStore } from '@/stores/notifications'
import { confirmDialog } from '@/dialogs'
import { playPomodoroEnd } from '@/sound'

const data = useDataStore()
const prefs = usePrefsStore()
const notif = useNotificationsStore()

const plannedMinutes = ref(25)
const planned = ref(25 * 60)
watch(plannedMinutes, (val) => {
  planned.value = (val || 0) * 60
})

const kind = ref<PomodoroKind>('focus')
const todoId = ref<number | null>(null)
const note = ref('')
const errMsg = ref('')
const okMsg = ref('')

const session = ref<PomodoroSession | null>(null)
const tickHandle = ref<number | null>(null)
const remaining = ref(0)
const elapsed = ref(0)
// 标记本次会话的"到点"事件已经处理过，避免重复触发自动完成。
const expiredHandled = ref(false)

const recent = ref<PomodoroSession[]>([])
const todoOptions = ref<Todo[]>([])

const presets: { label: string; seconds: number; kind: PomodoroKind }[] = [
  { label: '专注 25 分', seconds: 25 * 60, kind: 'focus' },
  { label: '专注 50 分', seconds: 50 * 60, kind: 'focus' },
  { label: '短休 5 分', seconds: 5 * 60, kind: 'short_break' },
  { label: '长休 15 分', seconds: 15 * 60, kind: 'long_break' },
  { label: '学习 45 分', seconds: 45 * 60, kind: 'learning' },
  { label: '复盘 20 分', seconds: 20 * 60, kind: 'review' },
]

const display = computed(() => {
  // 当没有进行中的 session 时，时间表盘应当反映"用户当前选择的时长"，
  // 而不是上次倒计时结束/被放弃时残留的 remaining 值。
  // 这同时修复了两个问题：
  //   1) "放弃当前番茄"后页面仍显示放弃时刻的剩余时间（必须刷新才能恢复）。
  //   2) 用户改动"时长（分钟）"输入框，但表盘上的时间不跟着变。
  const sec = session.value
    ? Math.max(0, remaining.value)
    : Math.max(0, planned.value)
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
})

// 进度环：基于"已设定时长 - 已过秒数"，但封顶到 planned。
// 重要：自动完成关闭时，倒计时结束后我们仍按 planned 计算，不再让 elapsed 继续往上跑。
const progress = computed(() => {
  if (!session.value || session.value.planned_duration_seconds <= 0) return 0
  const eff = Math.min(elapsed.value, session.value.planned_duration_seconds)
  return Math.max(0, Math.min(1, eff / session.value.planned_duration_seconds))
})

async function loadRecent() {
  try { recent.value = await pomoApi.list({ limit: 20 }) }
  catch (e) { errMsg.value = e instanceof ApiError ? e.message : (e as Error).message }
}

async function loadTodos() {
  try {
    await data.loadLists()
    todoOptions.value = await todosApi.list({ limit: 200 })
  } catch { /* ignore */ }
}

onMounted(async () => { await Promise.all([loadRecent(), loadTodos()]) })
onBeforeUnmount(() => {
  if (tickHandle.value) window.clearInterval(tickHandle.value)
})

function ringNotify(title: string, body: string) {
  // 优先级：浏览器桌面通知（如果用户允许）→ 应用内 toast
  try {
    if (prefs.desktopNotification && 'Notification' in window && Notification.permission === 'granted') {
      new Notification(title, { body })
    }
  } catch { /* ignore */ }
  if (prefs.inAppToast) {
    notif.pushToast({ id: -Date.now(), title, body })
  }
}

function startTick() {
  if (tickHandle.value) window.clearInterval(tickHandle.value)
  tickHandle.value = window.setInterval(() => {
    if (!session.value) return
    const startMs = new Date(session.value.started_at).getTime()
    const planned = session.value.planned_duration_seconds
    elapsed.value = Math.floor((Date.now() - startMs) / 1000)
    remaining.value = planned - elapsed.value

    if (remaining.value <= 0 && !expiredHandled.value) {
      expiredHandled.value = true
      remaining.value = 0
      // 提醒
      ringNotify('🍅 番茄到点！', '专注时间已结束。')
      // 清脆的"叮咚——叮咚"完成提示音（受用户偏好控制）
      playPomodoroEnd({ muted: !prefs.pomodoroSound })

      if (prefs.pomodoroAutoComplete) {
        // 自动结束：以"设定时长"为准入库，actual=planned，状态=completed。
        // 这避免了"用户没点击 -> 用两次点击间隔时间"的问题。
        autoCompleteOnExpire()
      } else {
        // 不自动结束：停在 0:00，等用户点"完成"或"放弃"
        if (tickHandle.value) {
          window.clearInterval(tickHandle.value)
          tickHandle.value = null
        }
      }
    }
  }, 1000)
}

async function autoCompleteOnExpire() {
  if (!session.value) return
  const sid = session.value.id
  // 立刻停 tick，避免边界 race
  if (tickHandle.value) {
    window.clearInterval(tickHandle.value)
    tickHandle.value = null
  }
  try {
    const s = await pomoApi.complete(sid)
    // 用服务端返回的为准；如果服务端将 actual 设为两次点击之差，
    // 我们也尊重它——但 UI 上"剩余时间已为 0、planned 即结束"逻辑保持。
    session.value = null
    elapsed.value = 0
    remaining.value = 0
    expiredHandled.value = false
    recent.value.unshift(s)
    okMsg.value = '✓ 已自动结束并入库'
    setTimeout(() => { okMsg.value = '' }, 3000)
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

async function start() {
  errMsg.value = ''
  okMsg.value = ''
  if (!plannedMinutes.value || plannedMinutes.value <= 0) {
    errMsg.value = '时长必须大于 0'
    return
  }
  try {
    const s = await pomoApi.create({
      todo_id: todoId.value || null,
      planned_duration_seconds: planned.value,
      kind: kind.value,
      note: note.value || undefined,
    })
    session.value = s
    elapsed.value = 0
    remaining.value = s.planned_duration_seconds
    expiredHandled.value = false
    startTick()
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

async function complete() {
  if (!session.value) return
  try {
    const s = await pomoApi.complete(session.value.id)
    session.value = null
    expiredHandled.value = false
    // 复位倒计时残值,让表盘回到"用户当前选择的时长"。display() 在
    // session=null 时会改用 planned, 这里同步把 remaining/elapsed 也清掉,
    // 避免下次开始时旧值在 startTick 设置前一闪而过。
    remaining.value = 0
    elapsed.value = 0
    if (tickHandle.value) { window.clearInterval(tickHandle.value); tickHandle.value = null }
    recent.value.unshift(s)
    // 手动结束番茄也响一声"叮咚——叮咚"
    playPomodoroEnd({ muted: !prefs.pomodoroSound })
  } catch (e) { errMsg.value = e instanceof ApiError ? e.message : (e as Error).message }
}

async function abandon() {
  if (!session.value) return
  if (!(await confirmDialog({
    title: '放弃当前番茄？',
    message: '这条番茄会被记录为 "abandoned"（放弃），后续统计中也会保留。',
    confirmText: '放弃',
    cancelText: '继续专注',
    danger: true,
  }))) return
  try {
    const s = await pomoApi.abandon(session.value.id)
    session.value = null
    expiredHandled.value = false
    // 同 complete(): 把残值清零,防止"放弃后还显示原剩余时间"的视觉残留。
    remaining.value = 0
    elapsed.value = 0
    if (tickHandle.value) { window.clearInterval(tickHandle.value); tickHandle.value = null }
    recent.value.unshift(s)
  } catch (e) { errMsg.value = e instanceof ApiError ? e.message : (e as Error).message }
}

function applyPreset(p: { seconds: number; kind: PomodoroKind }) {
  plannedMinutes.value = p.seconds / 60
  kind.value = p.kind
}

const isActive = computed(() => !!session.value && session.value.status === 'active')
// 当倒计时结束、且未自动完成（用户偏好关闭），UI 上要让用户能手动结束。
const isExpiredWaiting = computed(() => isActive.value && remaining.value <= 0)

// 圆环 stroke 计算
const RADIUS = 110
const CIRC = 2 * Math.PI * RADIUS
const dashOffset = computed(() => CIRC * (1 - progress.value))

// ============================================================
// 番茄类型 → 中文标签 / emoji。新增 learning / review 两类。
// ============================================================
const KIND_OPTIONS: { value: PomodoroKind; label: string; emoji: string }[] = [
  { value: 'focus',       label: '专注', emoji: '🎯' },
  { value: 'short_break', label: '短休', emoji: '☕' },
  { value: 'long_break',  label: '长休', emoji: '🛌' },
  { value: 'learning',    label: '学习', emoji: '📚' },
  { value: 'review',      label: '复盘', emoji: '🔍' },
]
function kindLabel(k: PomodoroKind): string {
  return KIND_OPTIONS.find((x) => x.value === k)?.label || k
}
// 每种番茄类型对应一个色系,与全局分类色 token 拉齐,保证按钮组的彩色感与"分类"一致。
function kindColor(k: PomodoroKind): string {
  switch (k) {
    case 'focus':       return 'var(--cat-rose)'
    case 'short_break': return 'var(--cat-teal)'
    case 'long_break':  return 'var(--cat-sky)'
    case 'learning':    return 'var(--cat-violet)'
    case 'review':      return 'var(--cat-amber)'
    default:            return 'var(--tg-primary)'
  }
}

// ============================================================
// "关联任务" 选择弹窗
// ----------------------------------------------------------------
// 旧版本是个朴素的 <select>,任务一多就拉到很长且无法筛选。这里换成一个弹窗,
// 内部支持:
//   - 搜索框(按标题模糊匹配)
//   - 日期段筛选(今日 / 明天 / 本周 / 全部),命中 due_at 落在该窗口的任务
//   - 分类筛选(复用 .cat-picker 样式,与 DayDetail / 编辑抽屉视觉一致)
// 选择后回填到 todoId,关闭弹窗。
// ============================================================
const showTodoPicker = ref(false)
const todoPickerSearch = ref('')
const todoPickerDateFilter = ref<'all' | 'today' | 'tomorrow' | 'week' | 'no_date'>('all')
const todoPickerListFilter = ref<'all' | 'none' | number>('all')

function openTodoPicker() {
  // 进入时把搜索 / 筛选重置到默认("全部"),避免上次的状态残留。
  todoPickerSearch.value = ''
  todoPickerDateFilter.value = 'all'
  todoPickerListFilter.value = 'all'
  showTodoPicker.value = true
}

// 当前已选择的任务对象(用于触发器按钮上展示标题/分类色)
const selectedTodo = computed<Todo | null>(() => {
  if (todoId.value == null) return null
  return todoOptions.value.find((t) => t.id === todoId.value) || null
})
function todoListColor(t: Todo): string {
  if (!t.list_id) return 'var(--tg-text-tertiary)'
  const l = data.lists.find((x) => x.id === t.list_id)
  return l?.color || 'var(--tg-primary)'
}
function todoListName(t: Todo): string {
  if (!t.list_id) return '未分类'
  const l = data.lists.find((x) => x.id === t.list_id)
  return l?.name || '未分类'
}

// 0:00 起算的"今日 / 明日 / 本周末"三个时间锚点
function dateBounds(kind: 'today' | 'tomorrow' | 'week'): { from: number; to: number } {
  const now = new Date()
  const start = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const day = 24 * 3600 * 1000
  if (kind === 'today') return { from: start, to: start + day }
  if (kind === 'tomorrow') return { from: start + day, to: start + 2 * day }
  // week: 含今日,共 7 天(直到第 7 日 24:00)
  return { from: start, to: start + 7 * day }
}

const filteredTodoOptions = computed<Todo[]>(() => {
  const search = todoPickerSearch.value.trim().toLowerCase()
  const dateF = todoPickerDateFilter.value
  const listF = todoPickerListFilter.value
  return todoOptions.value.filter((t) => {
    // 1) 搜索:命中标题或描述任意一段
    if (search) {
      const hay = (t.title + ' ' + (t.description || '')).toLowerCase()
      if (!hay.includes(search)) return false
    }
    // 2) 日期窗
    if (dateF === 'no_date') {
      if (t.due_at) return false
    } else if (dateF !== 'all') {
      if (!t.due_at) return false
      const ts = new Date(t.due_at).getTime()
      const { from, to } = dateBounds(dateF)
      if (ts < from || ts >= to) return false
    }
    // 3) 分类
    if (listF === 'none') {
      if (t.list_id) return false
    } else if (listF !== 'all') {
      if (t.list_id !== listF) return false
    }
    return true
  })
})

function pickTodo(t: Todo | null) {
  todoId.value = t ? t.id : null
  showTodoPicker.value = false
}
</script>

<template>
  <div class="pomo-wrap">
    <Transition name="fade">
      <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>
    </Transition>
    <Transition name="fade">
      <div v-if="okMsg" class="success-banner">{{ okMsg }}</div>
    </Transition>

    <div class="pomo-card">
      <div class="pomo-disc">
        <svg :width="260" :height="260" viewBox="0 0 260 260">
          <circle cx="130" cy="130" :r="RADIUS" fill="none" stroke="var(--tg-divider)" stroke-width="6" />
          <circle
            cx="130" cy="130" :r="RADIUS" fill="none"
            :stroke="isExpiredWaiting ? 'var(--tg-success)' : 'var(--tg-primary)'"
            stroke-width="6"
            stroke-linecap="round"
            transform="rotate(-90 130 130)"
            :stroke-dasharray="CIRC"
            :stroke-dashoffset="dashOffset"
            style="transition: stroke-dashoffset 1s linear, stroke 0.3s"
          />
        </svg>
        <div class="pomo-disc-content">
          <div class="pomo-time" :class="{ expired: isExpiredWaiting }">{{ display }}</div>
          <div v-if="isActive" class="pomo-progress-text">
            <span v-if="isExpiredWaiting">已到点 · 待结束</span>
            <span v-else>{{ fmtDuration(elapsed) }} / {{ fmtDuration(planned) }}</span>
          </div>
          <div v-else class="pomo-progress-text">
            {{ kindLabel(kind) }}
          </div>
        </div>
      </div>

      <div v-if="!isActive" class="pomo-presets">
        <button v-for="p in presets" :key="p.label" class="btn-secondary preset-btn" @click="applyPreset(p)">
          {{ p.label }}
        </button>
      </div>

      <div v-if="!isActive" class="pomo-form">
        <div class="field full-width">
          <label>类型</label>
          <!--
            类型从 <select> 改为 cat-picker 风格的按钮组,与"分类"/"优先级"
            视觉一致;同时新增 learning(📚 学习) / review(🔍 复盘) 两类。
          -->
          <div class="cat-picker pomo-kind-picker">
            <button
              v-for="k in KIND_OPTIONS"
              :key="k.value"
              type="button"
              class="cat-option"
              :class="{ 'is-selected': kind === k.value }"
              :style="{ '--cat-color': kindColor(k.value) }"
              @click="kind = k.value"
            >
              <span class="kind-emoji" aria-hidden="true">{{ k.emoji }}</span>
              {{ k.label }}
            </button>
          </div>
        </div>
        <div class="field">
          <label>时长（分钟）</label>
          <div class="pretty-input-wrap">
            <input class="pretty-input" type="number" min="1" max="360" v-model.number="plannedMinutes" />
            <span class="pretty-input-glow" aria-hidden="true" />
          </div>
        </div>
        <div class="field">
          <label>关联任务（可选）</label>
          <!--
            关联任务原本是个朴素 <select>,任务多了又长又难找。换成"按钮触发器 + 弹窗",
            弹窗内有搜索框、日期段筛选(今/明/周/无日期)、分类筛选(cat-picker 风格)。
          -->
          <button
            type="button"
            class="todo-trigger"
            :class="{ 'is-empty': !selectedTodo }"
            :style="selectedTodo ? { '--cat-color': todoListColor(selectedTodo) } : {}"
            @click="openTodoPicker"
          >
            <template v-if="selectedTodo">
              <span class="todo-trigger-dot" />
              <span class="todo-trigger-text">
                <span class="todo-trigger-title">{{ selectedTodo.title }}</span>
                <span class="todo-trigger-cat muted">{{ todoListName(selectedTodo) }}</span>
              </span>
              <span
                class="todo-trigger-clear"
                title="清除选择"
                @click.stop="todoId = null"
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                     stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                  <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
                </svg>
              </span>
            </template>
            <template v-else>
              <span class="todo-trigger-placeholder muted">不关联,点击选择…</span>
              <svg class="todo-trigger-arrow" width="14" height="14" viewBox="0 0 24 24" fill="none"
                   stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="9 18 15 12 9 6"/>
              </svg>
            </template>
          </button>
        </div>
        <div class="field full-width">
          <label>备注（可选）</label>
          <div class="pretty-input-wrap">
            <input v-model="note" class="pretty-input" placeholder="比如：专注于重构 UI" @keydown.enter="start" />
            <span class="pretty-input-glow" aria-hidden="true" />
          </div>
        </div>
        <div class="field full-width pomo-pref-hint">
          <span class="muted">
            到点行为：<strong>{{ prefs.pomodoroAutoComplete ? '自动结束并入库' : '停留等手动结束' }}</strong>
            <span style="margin-left:6px">（在「设置 → 提醒与通知」里调整）</span>
          </span>
        </div>
      </div>

      <div class="pomo-actions">
        <button v-if="!isActive" class="btn-primary start-btn" @click="start">开始</button>
        <button v-if="isActive" class="btn-primary start-btn" @click="complete">完成</button>
        <button v-if="isActive" class="btn-ghost btn-danger" @click="abandon">放弃</button>
      </div>
    </div>

    <div class="pomo-history-link-wrap">
      <RouterLink :to="{ name: 'pomodoro-history' }" class="pomo-history-link">
        <span class="phl-icon">📜</span>
        <span class="phl-text">
          <span class="phl-title">查看历史记录</span>
          <span class="phl-sub">最近 {{ recent.length || 0 }} 条 · 完成 / 放弃 / 进行中</span>
        </span>
        <svg class="phl-arrow" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="9 18 15 12 9 6"/>
        </svg>
      </RouterLink>
    </div>

    <!-- ============ 关联任务 选择弹窗 ============ -->
    <Transition name="modal">
      <div v-if="showTodoPicker" class="modal-backdrop" @click.self="showTodoPicker = false">
        <div class="modal-card todo-picker-modal">
          <header class="modal-head">
            <div class="modal-title-wrap">
              <span class="modal-title">关联任务</span>
              <span class="modal-subtitle">挑一个 todo 让本次番茄归到它名下;搜索 / 日期 / 分类皆可筛</span>
            </div>
            <button class="btn-icon" @click="showTodoPicker = false" aria-label="关闭">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
              </svg>
            </button>
          </header>

          <div class="modal-body todo-picker-body">
            <!-- 搜索框 -->
            <div class="todo-picker-search-wrap">
              <svg class="todo-picker-search-icon" width="14" height="14" viewBox="0 0 24 24"
                   fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="11" cy="11" r="7"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>
              </svg>
              <input
                v-model="todoPickerSearch"
                class="todo-picker-search"
                placeholder="按标题 / 描述搜索…"
                autofocus
              />
              <button
                v-if="todoPickerSearch"
                class="todo-picker-search-clear"
                title="清空"
                @click="todoPickerSearch = ''"
              >
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                     stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                  <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
                </svg>
              </button>
            </div>

            <!-- 日期段筛选 -->
            <div class="picker-section">
              <div class="picker-section-label">日期</div>
              <div class="cat-picker">
                <button
                  type="button"
                  class="cat-option"
                  :class="{ 'is-selected': todoPickerDateFilter === 'all' }"
                  :style="{ '--cat-color': 'var(--cat-sky)' }"
                  @click="todoPickerDateFilter = 'all'"
                ><span class="dot" />全部</button>
                <button
                  type="button"
                  class="cat-option"
                  :class="{ 'is-selected': todoPickerDateFilter === 'today' }"
                  :style="{ '--cat-color': 'var(--cat-rose)' }"
                  @click="todoPickerDateFilter = 'today'"
                ><span class="dot" />今天</button>
                <button
                  type="button"
                  class="cat-option"
                  :class="{ 'is-selected': todoPickerDateFilter === 'tomorrow' }"
                  :style="{ '--cat-color': 'var(--cat-amber)' }"
                  @click="todoPickerDateFilter = 'tomorrow'"
                ><span class="dot" />明天</button>
                <button
                  type="button"
                  class="cat-option"
                  :class="{ 'is-selected': todoPickerDateFilter === 'week' }"
                  :style="{ '--cat-color': 'var(--cat-violet)' }"
                  @click="todoPickerDateFilter = 'week'"
                ><span class="dot" />本周内</button>
                <button
                  type="button"
                  class="cat-option"
                  :class="{ 'is-selected': todoPickerDateFilter === 'no_date' }"
                  :style="{ '--cat-color': 'var(--tg-text-tertiary)' }"
                  @click="todoPickerDateFilter = 'no_date'"
                ><span class="dot" />无日期</button>
              </div>
            </div>

            <!-- 分类筛选(完全复用 cat-picker 样式) -->
            <div class="picker-section">
              <div class="picker-section-label">分类</div>
              <div class="cat-picker">
                <button
                  type="button"
                  class="cat-option"
                  :class="{ 'is-selected': todoPickerListFilter === 'all' }"
                  :style="{ '--cat-color': 'var(--cat-sky)' }"
                  @click="todoPickerListFilter = 'all'"
                ><span class="dot" />全部</button>
                <button
                  type="button"
                  class="cat-option"
                  :class="{ 'is-selected': todoPickerListFilter === 'none' }"
                  :style="{ '--cat-color': 'var(--tg-text-tertiary)' }"
                  @click="todoPickerListFilter = 'none'"
                ><span class="dot" />未分类</button>
                <button
                  v-for="l in data.lists"
                  :key="l.id"
                  type="button"
                  class="cat-option"
                  :class="{ 'is-selected': todoPickerListFilter === l.id }"
                  :style="{ '--cat-color': l.color || 'var(--tg-primary)' }"
                  @click="todoPickerListFilter = l.id"
                ><span class="dot" />{{ l.name }}</button>
              </div>
            </div>

            <!-- 任务列表(命中筛选的) -->
            <div class="picker-section">
              <div class="picker-section-label">
                匹配 <span class="picker-count">{{ filteredTodoOptions.length }}</span>
              </div>
              <div class="todo-picker-list">
                <button
                  type="button"
                  class="todo-picker-row is-none-row"
                  :class="{ 'is-active': todoId === null }"
                  @click="pickTodo(null)"
                >
                  <span class="row-dot none-dot" />
                  <span class="row-body">
                    <span class="row-title">不关联任何任务</span>
                    <span class="row-sub muted">本次番茄不归属任何 todo</span>
                  </span>
                </button>
                <button
                  v-for="t in filteredTodoOptions"
                  :key="t.id"
                  type="button"
                  class="todo-picker-row"
                  :class="{ 'is-active': todoId === t.id }"
                  :style="{ '--cat-color': todoListColor(t) }"
                  @click="pickTodo(t)"
                >
                  <span class="row-dot" />
                  <span class="row-body">
                    <span class="row-title">{{ t.title }}</span>
                    <span class="row-meta">
                      <span class="row-cat">{{ todoListName(t) }}</span>
                      <span v-if="t.due_at" class="row-due muted">
                        · {{ new Date(t.due_at).toLocaleString() }}
                      </span>
                    </span>
                  </span>
                </button>
                <div v-if="filteredTodoOptions.length === 0" class="todo-picker-empty muted">
                  没有命中任何任务,试试调整搜索词或筛选项。
                </div>
              </div>
            </div>
          </div>

          <footer class="modal-foot">
            <button class="btn-secondary" @click="showTodoPicker = false">关闭</button>
          </footer>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.pomo-wrap { max-width: 640px; margin: 0 auto; }
.success-banner {
  background: var(--tg-success-soft);
  color: var(--tg-success);
  padding: 10px 14px;
  border-radius: var(--tg-radius-md);
  font-size: 13.5px;
  font-weight: 500;
  margin-bottom: 12px;
}
.pomo-card {
  background: var(--tg-side);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-lg);
  padding: 32px 24px;
  margin-bottom: 24px;
  text-align: center;
  box-shadow: var(--tg-shadow-sm);
}
.pomo-disc { position: relative; width: 260px; height: 260px; margin: 0 auto 24px; }
.pomo-disc-content {
  position: absolute; inset: 0;
  display: flex; flex-direction: column;
  align-items: center; justify-content: center;
}
.pomo-time {
  font-size: 56px;
  font-weight: 200;
  font-variant-numeric: tabular-nums;
  color: var(--tg-text);
  letter-spacing: -2px;
  line-height: 1;
  transition: color 0.3s;
}
.pomo-time.expired { color: var(--tg-success); }
.pomo-progress-text {
  margin-top: 8px;
  font-size: 13px;
  color: var(--tg-text-secondary);
  font-weight: 500;
}
.pomo-presets {
  display: flex; gap: 8px; justify-content: center; flex-wrap: wrap;
  margin-bottom: 20px;
}
.preset-btn { border-radius: 999px; padding: 6px 14px; font-size: 13px; }
.pomo-form {
  display: grid; grid-template-columns: 1fr 1fr; gap: 14px;
  text-align: left; margin-bottom: 20px;
}
.pomo-form .field { display: flex; flex-direction: column; gap: 6px; }
.pomo-form .field.full-width { grid-column: span 2; }
.pomo-form label { font-size: 12px; font-weight: 600; color: var(--tg-primary); }
.pomo-pref-hint { padding-top: 4px; font-size: 12px; }
.pomo-actions { display: flex; gap: 10px; justify-content: center; }
.start-btn { padding: 11px 36px; font-size: 14.5px; border-radius: 999px; min-width: 140px; }
/* === 跳转到历史记录页的卡片 === */
.pomo-history-link-wrap { margin-top: 8px; }
.pomo-history-link {
  display: flex; align-items: center; gap: 14px;
  padding: 14px 18px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-md);
  color: var(--tg-text);
  text-decoration: none;
  transition: border-color var(--tg-trans-fast),
              background var(--tg-trans-fast),
              transform var(--tg-trans-fast),
              box-shadow var(--tg-trans-fast);
}
.pomo-history-link:hover {
  border-color: color-mix(in srgb, var(--tg-primary) 45%, transparent);
  background: color-mix(in srgb, var(--tg-primary) 5%, var(--tg-bg-elev));
  transform: translateY(-1px);
  box-shadow: 0 4px 14px -6px rgba(99, 102, 241, 0.28);
}
.phl-icon {
  font-size: 22px;
  width: 40px; height: 40px;
  display: inline-flex; align-items: center; justify-content: center;
  background: var(--tg-hover);
  border-radius: 50%;
  flex-shrink: 0;
}
.phl-text { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.phl-title {
  font-size: 14.5px; font-weight: 700;
  color: var(--tg-primary);
  letter-spacing: -0.005em;
}
.phl-sub {
  font-size: 12px; color: var(--tg-text-secondary);
}
.phl-arrow {
  color: var(--tg-text-tertiary);
  transition: transform var(--tg-trans-fast), color var(--tg-trans-fast);
}
.pomo-history-link:hover .phl-arrow {
  color: var(--tg-primary);
  transform: translateX(2px);
}

@media (max-width: 600px) {
  .pomo-form { grid-template-columns: 1fr; }
  .pomo-form .field.full-width { grid-column: span 1; }
  .pomo-disc { width: 220px; height: 220px; }
  .pomo-disc svg { width: 220px; height: 220px; }
  .pomo-time { font-size: 44px; }
}

/* ============================================================
 * 番茄类型 picker(cat-picker 风格变体)
 * ============================================================ */
.pomo-kind-picker { gap: 6px; }
.pomo-kind-picker .cat-option {
  padding: 6px 12px;
  font-size: 12.5px;
}
.pomo-kind-picker .kind-emoji { font-size: 14px; line-height: 1; }

/* ============================================================
 * 关联任务 触发器(单行按钮,被点击后展开弹窗)
 * ============================================================ */
.todo-trigger {
  --cat-color: var(--tg-text-tertiary);
  width: 100%;
  display: flex; align-items: center; gap: 10px;
  padding: 9px 10px 9px 14px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-pill);
  cursor: pointer;
  transition: all var(--tg-trans-fast);
  text-align: left;
  font-family: 'Manrope', sans-serif;
  font-size: 13px;
  box-shadow: var(--tg-shadow-xs);
}
.todo-trigger:hover {
  border-color: color-mix(in srgb, var(--cat-color) 50%, var(--tg-divider-strong));
  box-shadow: var(--tg-shadow-sm);
  transform: translateY(-1px);
}
.todo-trigger.is-empty {
  color: var(--tg-text-tertiary);
}
.todo-trigger-dot {
  width: 9px; height: 9px;
  border-radius: 50%;
  background: var(--cat-color);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cat-color) 22%, transparent);
  flex-shrink: 0;
}
.todo-trigger-text {
  flex: 1; min-width: 0;
  display: flex; flex-direction: column; gap: 1px;
  overflow: hidden;
}
.todo-trigger-title {
  font-weight: 700;
  color: var(--tg-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.todo-trigger-cat {
  font-size: 11px;
}
.todo-trigger-placeholder {
  flex: 1;
  font-size: 13px;
}
.todo-trigger-clear {
  width: 22px; height: 22px;
  display: inline-flex; align-items: center; justify-content: center;
  border-radius: 50%;
  color: var(--tg-text-tertiary);
  transition: background var(--tg-trans-fast), color var(--tg-trans-fast);
  flex-shrink: 0;
}
.todo-trigger-clear:hover {
  background: var(--tg-press);
  color: var(--tg-danger);
}
.todo-trigger-arrow {
  color: var(--tg-text-tertiary);
  flex-shrink: 0;
}

/* ============================================================
 * 关联任务 选择弹窗
 * ============================================================ */
.todo-picker-modal {
  width: min(560px, 95vw);
}
/* —— 弹窗 head / foot(本组件不依赖外部 modal head 样式,这里独立定义) —— */
.modal-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--tg-divider);
}
.modal-title-wrap { display: flex; flex-direction: column; gap: 2px; }
.modal-title {
  font-family: 'Sora', sans-serif;
  font-size: 16px; font-weight: 700;
  letter-spacing: -0.018em;
}
.modal-subtitle {
  font-size: 12px;
  color: var(--tg-text-secondary);
  font-weight: 500;
}
.modal-foot {
  display: flex; gap: 10px; justify-content: flex-end;
  padding: 12px 20px;
  border-top: 1px solid var(--tg-divider);
}

.todo-picker-body {
  padding: 18px 20px;
  display: flex; flex-direction: column;
  gap: 14px;
  max-height: 70vh; overflow-y: auto;
}

/* —— 搜索框 —— */
.todo-picker-search-wrap {
  position: relative;
  display: flex; align-items: center;
  background: var(--tg-bg);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-pill);
  padding: 0 10px;
  transition: border-color var(--tg-trans-fast), box-shadow var(--tg-trans-fast);
}
.todo-picker-search-wrap:focus-within {
  border-color: var(--tg-primary);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--tg-primary) 14%, transparent);
}
.todo-picker-search-icon {
  color: var(--tg-text-tertiary);
  flex-shrink: 0;
  margin-right: 6px;
}
.todo-picker-search {
  flex: 1;
  border: none; outline: none; background: transparent;
  padding: 9px 4px;
  font-size: 13.5px;
  font-family: 'Manrope', sans-serif;
  color: var(--tg-text);
}
.todo-picker-search-clear {
  width: 22px; height: 22px;
  display: inline-flex; align-items: center; justify-content: center;
  background: var(--tg-hover);
  color: var(--tg-text-secondary);
  border: none;
  border-radius: 50%;
  cursor: pointer;
  transition: background var(--tg-trans-fast);
  flex-shrink: 0;
}
.todo-picker-search-clear:hover { background: var(--tg-press); }

/* —— 各小节(日期 / 分类 / 列表) —— */
.picker-section { display: flex; flex-direction: column; gap: 8px; }
.picker-section-label {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--tg-text-secondary);
  display: flex; align-items: center; gap: 8px;
}
.picker-count {
  display: inline-flex; align-items: center; justify-content: center;
  min-width: 20px; height: 18px; padding: 0 7px;
  background: var(--tg-primary-soft);
  color: var(--tg-primary);
  font-size: 11px; font-weight: 700;
  border-radius: 999px;
  font-variant-numeric: tabular-nums;
}

/* —— 任务行列表 —— */
.todo-picker-list {
  display: flex; flex-direction: column;
  gap: 4px;
  max-height: 280px;
  overflow-y: auto;
  padding-right: 2px;
}
.todo-picker-row {
  --cat-color: var(--tg-text-tertiary);
  display: flex; align-items: center; gap: 10px;
  padding: 9px 12px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-md);
  cursor: pointer;
  transition: all var(--tg-trans-fast);
  text-align: left;
  font-family: 'Manrope', sans-serif;
  position: relative;
}
.todo-picker-row::before {
  content: '';
  position: absolute; left: 0; top: 0; bottom: 0;
  width: 3px;
  background: var(--cat-color);
  border-radius: var(--tg-radius-md) 0 0 var(--tg-radius-md);
  opacity: 0;
  transition: opacity var(--tg-trans-fast);
}
.todo-picker-row:hover {
  border-color: color-mix(in srgb, var(--cat-color) 45%, transparent);
  background: color-mix(in srgb, var(--cat-color) 4%, var(--tg-bg-elev));
}
.todo-picker-row:hover::before { opacity: 0.7; }
.todo-picker-row.is-active {
  border-color: var(--cat-color);
  background: color-mix(in srgb, var(--cat-color) 10%, var(--tg-bg-elev));
}
.todo-picker-row.is-active::before { opacity: 1; }

.todo-picker-row .row-dot {
  width: 9px; height: 9px;
  border-radius: 50%;
  background: var(--cat-color);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cat-color) 22%, transparent);
  flex-shrink: 0;
}
.todo-picker-row .row-dot.none-dot {
  background: repeating-linear-gradient(
    45deg, var(--tg-text-tertiary),
    var(--tg-text-tertiary) 2px,
    transparent 2px, transparent 4px
  );
}
.todo-picker-row .row-body {
  flex: 1; min-width: 0;
  display: flex; flex-direction: column;
  gap: 2px;
}
.todo-picker-row .row-title {
  font-size: 13.5px; font-weight: 600;
  color: var(--tg-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.todo-picker-row .row-meta,
.todo-picker-row .row-sub {
  font-size: 11.5px;
  color: var(--tg-text-secondary);
}
.todo-picker-row .row-cat {
  display: inline-flex; align-items: center;
  padding: 1px 7px;
  background: color-mix(in srgb, var(--cat-color) 12%, transparent);
  color: color-mix(in srgb, var(--cat-color) 75%, var(--tg-text));
  border-radius: 999px;
  font-weight: 700;
  font-size: 10.5px;
}
.todo-picker-row .row-due {
  font-variant-numeric: tabular-nums;
}
.todo-picker-row.is-none-row { border-style: dashed; }

.todo-picker-empty {
  padding: 24px 16px;
  text-align: center;
  font-size: 13px;
  background: var(--tg-bg);
  border: 1px dashed var(--tg-divider-strong);
  border-radius: var(--tg-radius-md);
}
</style>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { pomodoro as pomoApi, todos as todosApi, ApiError } from '@/api'
import type { PomodoroKind, PomodoroSession, Todo } from '@/types'
import { fmtDateTime, fmtDuration } from '@/utils'
import { useDataStore } from '@/stores/data'
import { usePrefsStore } from '@/stores/prefs'
import { useNotificationsStore } from '@/stores/notifications'

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
]

const display = computed(() => {
  const sec = Math.max(0, remaining.value)
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
    if (tickHandle.value) { window.clearInterval(tickHandle.value); tickHandle.value = null }
    recent.value.unshift(s)
  } catch (e) { errMsg.value = e instanceof ApiError ? e.message : (e as Error).message }
}

async function abandon() {
  if (!session.value) return
  if (!confirm('放弃当前番茄？这条会记为 abandoned。')) return
  try {
    const s = await pomoApi.abandon(session.value.id)
    session.value = null
    expiredHandled.value = false
    if (tickHandle.value) { window.clearInterval(tickHandle.value); tickHandle.value = null }
    recent.value.unshift(s)
  } catch (e) { errMsg.value = e instanceof ApiError ? e.message : (e as Error).message }
}

function applyPreset(p: { seconds: number; kind: PomodoroKind }) {
  plannedMinutes.value = p.seconds / 60
  kind.value = p.kind
}

function statusText(s: PomodoroSession): string {
  switch (s.status) {
    case 'completed': return '完成'
    case 'abandoned': return '放弃'
    case 'active': return '进行中'
    default: return s.status
  }
}

const isActive = computed(() => !!session.value && session.value.status === 'active')
// 当倒计时结束、且未自动完成（用户偏好关闭），UI 上要让用户能手动结束。
const isExpiredWaiting = computed(() => isActive.value && remaining.value <= 0)

// 圆环 stroke 计算
const RADIUS = 110
const CIRC = 2 * Math.PI * RADIUS
const dashOffset = computed(() => CIRC * (1 - progress.value))
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
            {{ kind === 'focus' ? '专注' : (kind === 'short_break' ? '短休' : '长休') }}
          </div>
        </div>
      </div>

      <div v-if="!isActive" class="pomo-presets">
        <button v-for="p in presets" :key="p.label" class="btn-secondary preset-btn" @click="applyPreset(p)">
          {{ p.label }}
        </button>
      </div>

      <div v-if="!isActive" class="pomo-form">
        <div class="field">
          <label>类型</label>
          <div class="pretty-input-wrap">
            <select v-model="kind" class="pretty-input">
              <option value="focus">🎯 专注</option>
              <option value="short_break">☕ 短休</option>
              <option value="long_break">🛌 长休</option>
            </select>
            <span class="pretty-input-glow" aria-hidden="true" />
          </div>
        </div>
        <div class="field">
          <label>时长（分钟）</label>
          <div class="pretty-input-wrap">
            <input class="pretty-input" type="number" min="1" max="360" v-model.number="plannedMinutes" />
            <span class="pretty-input-glow" aria-hidden="true" />
          </div>
        </div>
        <div class="field full-width">
          <label>关联任务（可选）</label>
          <div class="pretty-input-wrap">
            <select v-model.number="todoId" class="pretty-input">
              <option :value="null">不关联</option>
              <option v-for="t in todoOptions" :key="t.id" :value="t.id">{{ t.title }}</option>
            </select>
            <span class="pretty-input-glow" aria-hidden="true" />
          </div>
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

    <h3 class="section-title">最近记录</h3>
    <div class="history-list" v-if="recent.length > 0">
      <div v-for="s in recent" :key="s.id" class="history-item">
        <div class="hi-icon">
          <span v-if="s.kind === 'focus'">🎯</span>
          <span v-else-if="s.kind === 'short_break'">☕</span>
          <span v-else>🛌</span>
        </div>
        <div class="hi-info">
          <div class="hi-title">
            {{ s.kind === 'focus' ? '专注' : (s.kind === 'short_break' ? '短休' : '长休') }}
            <span v-if="s.note" class="muted"> — {{ s.note }}</span>
          </div>
          <div class="hi-time">{{ fmtDateTime(s.started_at) }}</div>
        </div>
        <div class="hi-status-wrap">
          <span class="hi-status-badge" :class="s.status">{{ statusText(s) }}</span>
          <span class="hi-duration">{{ fmtDuration(s.actual_duration_seconds || s.planned_duration_seconds) }}</span>
        </div>
      </div>
    </div>
    <div v-else class="empty">
      <div class="empty-icon">🍅</div>
      <div class="empty-title">还没有番茄记录</div>
      <div class="empty-hint">开始你的第一次专注吧</div>
    </div>
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
.history-list { display: flex; flex-direction: column; gap: 6px; }
.history-item {
  display: flex; align-items: center; gap: 14px;
  padding: 12px 14px; border-radius: var(--tg-radius-md);
  transition: background 0.15s;
}
.history-item:hover { background: var(--tg-hover); }
.hi-icon {
  font-size: 20px; background: var(--tg-hover);
  width: 40px; height: 40px; border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  flex-shrink: 0;
}
.hi-info { flex: 1; overflow: hidden; min-width: 0; }
.hi-title {
  font-size: 14px; font-weight: 600; margin-bottom: 2px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.hi-time { font-size: 12px; color: var(--tg-text-secondary); }
.hi-status-wrap {
  display: flex; flex-direction: column; align-items: flex-end;
  gap: 4px; flex-shrink: 0;
}
.hi-status-badge {
  font-size: 11px; font-weight: 600;
  padding: 2px 8px; border-radius: 999px;
}
.hi-status-badge.completed { background: var(--tg-success-soft); color: var(--tg-success); }
.hi-status-badge.abandoned { background: var(--tg-danger-soft); color: var(--tg-danger); }
.hi-status-badge.active { background: var(--tg-primary-soft); color: var(--tg-primary); }
.hi-duration { font-size: 13px; font-weight: 600; font-variant-numeric: tabular-nums; }

@media (max-width: 600px) {
  .pomo-form { grid-template-columns: 1fr; }
  .pomo-form .field.full-width { grid-column: span 1; }
  .pomo-disc { width: 220px; height: 220px; }
  .pomo-disc svg { width: 220px; height: 220px; }
  .pomo-time { font-size: 44px; }
}
</style>

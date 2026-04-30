<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { pomodoro as pomoApi, ApiError } from '@/api'
import type { PomodoroKind, PomodoroSession, Todo } from '@/types'
import { fmtDateTime, fmtDuration } from '@/utils'
import { useDataStore } from '@/stores/data'

const data = useDataStore()

const plannedMinutes = ref(25)
const planned = ref(25 * 60)
watch(plannedMinutes, (val) => {
  planned.value = (val || 0) * 60
})

const kind = ref<PomodoroKind>('focus')
const todoId = ref<number | null>(null)
const note = ref('')
const errMsg = ref('')

const session = ref<PomodoroSession | null>(null)
const tickHandle = ref<number | null>(null)
const remaining = ref(0)
const elapsed = ref(0)

const recent = ref<PomodoroSession[]>([])
const todoOptions = ref<Todo[]>([])

const presets: { label: string; seconds: number; kind: PomodoroKind }[] = [
  { label: '专注 25min', seconds: 25 * 60, kind: 'focus' },
  { label: '专注 50min', seconds: 50 * 60, kind: 'focus' },
  { label: '短休 5min', seconds: 5 * 60, kind: 'short_break' },
  { label: '长休 15min', seconds: 15 * 60, kind: 'long_break' },
]

const display = computed(() => {
  const sec = remaining.value
  const sign = sec < 0 ? '+' : ''
  const abs = Math.abs(sec)
  const m = Math.floor(abs / 60)
  const s = abs % 60
  return `${sign}${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
})

async function loadRecent() {
  try {
    recent.value = await pomoApi.list({ limit: 20 })
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

async function loadTodos() {
  try {
    await data.loadLists()
    const { todos: todosApi } = await import('@/api')
    todoOptions.value = await todosApi.list({ limit: 200 })
  } catch {
    // ignore
  }
}

onMounted(async () => {
  await Promise.all([loadRecent(), loadTodos()])
})
onBeforeUnmount(() => {
  if (tickHandle.value) window.clearInterval(tickHandle.value)
})

function startTick() {
  if (tickHandle.value) window.clearInterval(tickHandle.value)
  tickHandle.value = window.setInterval(() => {
    if (!session.value) return
    const startMs = new Date(session.value.started_at).getTime()
    elapsed.value = Math.floor((Date.now() - startMs) / 1000)
    remaining.value = session.value.planned_duration_seconds - elapsed.value
    if (remaining.value <= 0) {
      remaining.value = 0 // Prevent negative time
      if (tickHandle.value) {
        window.clearInterval(tickHandle.value)
        tickHandle.value = null
      }
      try {
        if ('Notification' in window && Notification.permission === 'granted') {
          new Notification('🍅 番茄到点!', { body: '可以休息或继续。' })
        }
      } catch { /* ignore */ }
    }
  }, 1000)
}

async function start() {
  errMsg.value = ''
  if (!plannedMinutes.value || plannedMinutes.value <= 0) {
    errMsg.value = '时长必须大于0'
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
    if (tickHandle.value) {
      window.clearInterval(tickHandle.value)
      tickHandle.value = null
    }
    recent.value.unshift(s)
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

async function abandon() {
  if (!session.value) return
  if (!confirm('放弃当前番茄? 这条会记为 abandoned。')) return
  try {
    const s = await pomoApi.abandon(session.value.id)
    session.value = null
    if (tickHandle.value) {
      window.clearInterval(tickHandle.value)
      tickHandle.value = null
    }
    recent.value.unshift(s)
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
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
</script>

<template>
  <div class="pomo-wrap">
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

    <div class="pomo-card">
      <div class="pomo-disc-wrap" :class="{ active: isActive }">
        <div class="pomo-disc">{{ display }}</div>
        <div v-if="isActive" class="pomo-progress">
          已进行 {{ fmtDuration(elapsed) }} / 计划 {{ fmtDuration(planned) }}
        </div>
      </div>

      <div v-if="!isActive" class="pomo-controls">
        <button v-for="p in presets" :key="p.label" class="btn-secondary preset-btn" @click="applyPreset(p)">
          {{ p.label }}
        </button>
      </div>

      <div v-if="!isActive" class="pomo-form">
        <div class="field">
          <label>类型</label>
          <select v-model="kind">
            <option value="focus">🎯 专注 (Focus)</option>
            <option value="short_break">☕ 短休 (Short Break)</option>
            <option value="long_break">🛌 长休 (Long Break)</option>
          </select>
        </div>
        <div class="field">
          <label>时长 (分钟)</label>
          <input type="number" min="1" max="360" v-model.number="plannedMinutes" />
        </div>
        <div class="field full-width">
          <label>关联任务 (可选)</label>
          <select v-model.number="todoId">
            <option :value="null">不关联</option>
            <option v-for="t in todoOptions" :key="t.id" :value="t.id">{{ t.title }}</option>
          </select>
        </div>
        <div class="field full-width">
          <label>备注 (可选)</label>
          <input v-model="note" placeholder="比如: 专注于重构 UI" @keydown.enter="start" />
        </div>
      </div>

      <div class="pomo-actions">
        <button v-if="!isActive" class="btn-primary start-btn" @click="start">开始</button>
        <button v-if="isActive" class="btn-primary complete-btn" @click="complete">完成</button>
        <button v-if="isActive" class="btn-ghost btn-danger" @click="abandon">放弃</button>
      </div>
    </div>

    <div class="pomo-history">
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
              <span class="hi-kind">{{ s.kind === 'focus' ? '专注' : (s.kind === 'short_break' ? '短休' : '长休') }}</span>
              <span v-if="s.note" class="hi-note"> — {{ s.note }}</span>
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
        <div class="empty-title">暂无番茄记录</div>
        <div style="font-size: 14px; color: var(--c-text-soft);">开始您的第一次专注吧</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pomo-wrap {
  max-width: 600px;
  margin: 0 auto;
}
.pomo-card {
  background: var(--c-surface);
  border: 1px solid var(--c-border);
  border-radius: var(--radius-xl);
  padding: 32px 24px;
  box-shadow: var(--shadow-sm);
  margin-bottom: 32px;
  text-align: center;
}
.pomo-disc-wrap {
  margin: 20px 0 32px;
  transition: transform 0.3s ease;
}
.pomo-disc-wrap.active {
  transform: scale(1.1);
}
.pomo-disc {
  font-size: 80px;
  font-weight: 200;
  line-height: 1;
  font-variant-numeric: tabular-nums;
  color: var(--c-primary);
  letter-spacing: -2px;
}
.pomo-progress {
  margin-top: 12px;
  font-size: 14px;
  color: var(--c-text-soft);
  font-weight: 500;
}
.pomo-controls {
  display: flex;
  gap: 8px;
  justify-content: center;
  flex-wrap: wrap;
  margin-bottom: 24px;
}
.preset-btn {
  border-radius: 20px;
  padding: 6px 14px;
  font-size: 13px;
  background: var(--c-surface-2);
}
.preset-btn:hover {
  background: var(--c-border);
}
.pomo-form {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  text-align: left;
  margin-bottom: 24px;
}
.pomo-form .field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.pomo-form .field.full-width {
  grid-column: span 2;
}
.pomo-form label {
  font-size: 13px;
  font-weight: 600;
  color: var(--c-text-soft);
}
.pomo-actions {
  display: flex;
  gap: 12px;
  justify-content: center;
}
.start-btn, .complete-btn {
  padding: 12px 32px;
  font-size: 16px;
  border-radius: 24px;
}
.history-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.history-item {
  display: flex;
  align-items: center;
  gap: 16px;
  background: var(--c-surface);
  border: 1px solid var(--c-border);
  border-radius: var(--radius-md);
  padding: 16px;
  transition: all 0.2s ease;
}
.history-item:hover {
  border-color: var(--c-primary-soft);
  box-shadow: var(--shadow-sm);
}
.hi-icon {
  font-size: 24px;
  background: var(--c-surface-2);
  width: 48px;
  height: 48px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
}
.hi-info {
  flex: 1;
  overflow: hidden;
}
.hi-title {
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.hi-kind { color: var(--c-text); }
.hi-note { color: var(--c-text-muted); font-weight: 400; }
.hi-time {
  font-size: 13px;
  color: var(--c-text-soft);
}
.hi-status-wrap {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
}
.hi-status-badge {
  font-size: 12px;
  font-weight: 600;
  padding: 4px 8px;
  border-radius: 6px;
}
.hi-status-badge.completed { background: var(--c-success-soft); color: var(--c-success); }
.hi-status-badge.abandoned { background: var(--c-danger-soft); color: var(--c-danger); }
.hi-status-badge.active { background: var(--c-primary-soft); color: var(--c-primary); }
.hi-duration {
  font-size: 14px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

@media (max-width: 600px) {
  .pomo-form { grid-template-columns: 1fr; }
  .pomo-form .field.full-width { grid-column: span 1; }
}
</style>

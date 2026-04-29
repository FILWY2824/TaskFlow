<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { pomodoro as pomoApi, ApiError } from '@/api'
import type { PomodoroKind, PomodoroSession, Todo } from '@/types'
import { fmtDateTime, fmtDuration } from '@/utils'
import { useDataStore } from '@/stores/data'

const data = useDataStore()

const planned = ref(25 * 60) // 默认 25 分
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
    // 拉一个轻量任务列表用于选择
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
    if (remaining.value <= 0 && remaining.value === 0) {
      // 超时 0 秒时短促提示一次,不自动 complete:让用户决定 complete 还是延长
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
  if (!confirm('放弃当前番茄?这条会记为 abandoned。')) return
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
  planned.value = p.seconds
  kind.value = p.kind
}

function statusBadge(s: PomodoroSession): string {
  switch (s.status) {
    case 'completed':
      return '✓ 完成'
    case 'abandoned':
      return '× 放弃'
    case 'active':
      return '▶ 进行中'
  }
}

const isActive = computed(() => !!session.value && session.value.status === 'active')
</script>

<template>
  <div class="pomo-wrap">
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

    <div class="pomo-disc">{{ display }}</div>

    <div v-if="!isActive" class="pomo-controls">
      <button v-for="p in presets" :key="p.label" class="btn-secondary" @click="applyPreset(p)">
        {{ p.label }}
      </button>
    </div>

    <div v-if="!isActive" style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px; max-width: 480px; margin: 0 auto 12px;">
      <div class="field" style="text-align: left">
        <label>类型</label>
        <select v-model="kind">
          <option value="focus">专注 focus</option>
          <option value="short_break">短休 short_break</option>
          <option value="long_break">长休 long_break</option>
        </select>
      </div>
      <div class="field" style="text-align: left">
        <label>时长(分钟)</label>
        <input type="number" min="1" max="360" :value="planned / 60" @input="(e) => planned = Number((e.target as HTMLInputElement).value) * 60" />
      </div>
      <div class="field" style="grid-column: span 2; text-align: left">
        <label>关联任务(可选)</label>
        <select v-model.number="todoId">
          <option :value="null">不关联</option>
          <option v-for="t in todoOptions" :key="t.id" :value="t.id">{{ t.title }}</option>
        </select>
      </div>
      <div class="field" style="grid-column: span 2; text-align: left">
        <label>备注(可选)</label>
        <input v-model="note" placeholder="比如:复盘 v0.3.0 设计" />
      </div>
    </div>

    <div class="pomo-actions">
      <button v-if="!isActive" class="btn-primary" @click="start">开始</button>
      <button v-if="isActive" class="btn-primary" @click="complete">完成</button>
      <button v-if="isActive" class="btn-secondary btn-danger" @click="abandon">放弃</button>
    </div>

    <div v-if="isActive" class="muted">
      已进行 {{ fmtDuration(elapsed) }} / 计划 {{ fmtDuration(planned) }}
    </div>

    <div class="pomo-history">
      <h3 style="margin-bottom: 8px">最近记录</h3>
      <ul v-if="recent.length > 0" style="list-style: none; padding: 0; margin: 0">
        <li v-for="s in recent" :key="s.id">
          <span>
            {{ fmtDateTime(s.started_at) }} · {{ s.kind }}
            <span v-if="s.note" class="muted"> — {{ s.note }}</span>
          </span>
          <span>
            {{ statusBadge(s) }}
            · {{ fmtDuration(s.actual_duration_seconds || s.planned_duration_seconds) }}
          </span>
        </li>
      </ul>
      <div v-else class="muted">暂无记录</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { stats as statsApi, ApiError } from '@/api'
import type { DailyBucket, PomodoroAggregate, StatsSummary } from '@/types'
import { fmtDuration } from '@/utils'

const summary = ref<StatsSummary | null>(null)
const daily = ref<DailyBucket[]>([])
const pomoAgg = ref<PomodoroAggregate | null>(null)
const errMsg = ref('')
const loading = ref(false)
const range = ref<'7' | '14' | '30'>('14')

async function load() {
  errMsg.value = ''
  loading.value = true
  try {
    const days = parseInt(range.value, 10)
    const today = new Date()
    const from = new Date(today)
    from.setDate(today.getDate() - days + 1)
    const to = new Date(today)
    to.setDate(today.getDate() + 1)
    const fromStr = ymd(from)
    const toStr = ymd(to)
    const [s, d, p] = await Promise.all([
      statsApi.summary(),
      statsApi.daily({ from: fromStr, to: toStr }),
      statsApi.pomodoro({ from: fromStr, to: toStr }),
    ])
    summary.value = s
    daily.value = d.items
    pomoAgg.value = p
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    loading.value = false
  }
}

function ymd(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

onMounted(load)

// 用 SVG 画一个简易柱状图(创建/完成 双柱),不引入图表库。
const maxCompletedOrCreated = computed(() => {
  let max = 1
  for (const b of daily.value) {
    if (b.created > max) max = b.created
    if (b.completed > max) max = b.completed
  }
  return max
})
const maxPomo = computed(() => {
  let max = 60 // 至少 1 小时
  for (const b of daily.value) {
    if (b.pomodoro_seconds / 60 > max) max = b.pomodoro_seconds / 60
  }
  return max
})

const W = 760
const H = 200
const PAD = 28
const innerW = computed(() => W - PAD * 2)
const innerH = H - PAD * 2

const barWidth = computed(() => Math.max(8, innerW.value / Math.max(daily.value.length, 1) - 6))
function bar(idx: number): number {
  return PAD + idx * (innerW.value / Math.max(daily.value.length, 1)) + 3
}
</script>

<template>
  <div>
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

    <div class="row-flex" style="margin-bottom: 16px">
      <strong>时间范围</strong>
      <select v-model="range" @change="load">
        <option value="7">最近 7 天</option>
        <option value="14">最近 14 天</option>
        <option value="30">最近 30 天</option>
      </select>
    </div>

    <div v-if="loading" class="muted">加载中…</div>

    <template v-else>
      <div v-if="summary" class="stats-grid">
        <div class="stat-card">
          <div class="label">未完成任务</div>
          <div class="value">{{ summary.todos_open }}</div>
        </div>
        <div class="stat-card">
          <div class="label">过期任务</div>
          <div class="value danger-text">{{ summary.todos_overdue }}</div>
        </div>
        <div class="stat-card">
          <div class="label">今日截止</div>
          <div class="value">{{ summary.todos_due_today }}</div>
        </div>
        <div class="stat-card">
          <div class="label">今日已完成</div>
          <div class="value success-text">{{ summary.completed_today }}</div>
        </div>
        <div class="stat-card">
          <div class="label">本周已完成</div>
          <div class="value success-text">{{ summary.completed_this_week }}</div>
        </div>
        <div class="stat-card">
          <div class="label">今日专注</div>
          <div class="value">{{ fmtDuration(summary.pomodoro_today_seconds) }}</div>
        </div>
        <div class="stat-card">
          <div class="label">本周专注</div>
          <div class="value">{{ fmtDuration(summary.pomodoro_this_week_seconds) }}</div>
        </div>
        <div class="stat-card">
          <div class="label">总任务</div>
          <div class="value">{{ summary.todos_total }}</div>
        </div>
      </div>

      <div class="section-card">
        <h3>每日:创建 vs 完成</h3>
        <svg :viewBox="`0 0 ${W} ${H}`" :width="W" :height="H" style="width: 100%; height: auto">
          <line :x1="PAD" :y1="H - PAD" :x2="W - PAD" :y2="H - PAD" stroke="var(--c-border)" />
          <g v-for="(b, i) in daily" :key="b.date">
            <!-- created (深色) -->
            <rect
              :x="bar(i)"
              :y="(H - PAD) - (b.created / maxCompletedOrCreated) * innerH"
              :width="barWidth / 2 - 1"
              :height="(b.created / maxCompletedOrCreated) * innerH"
              fill="var(--c-text-muted)"
            >
              <title>{{ b.date }}: 创建 {{ b.created }}</title>
            </rect>
            <!-- completed (主色) -->
            <rect
              :x="bar(i) + barWidth / 2"
              :y="(H - PAD) - (b.completed / maxCompletedOrCreated) * innerH"
              :width="barWidth / 2 - 1"
              :height="(b.completed / maxCompletedOrCreated) * innerH"
              fill="var(--c-primary)"
            >
              <title>{{ b.date }}: 完成 {{ b.completed }}</title>
            </rect>
            <text
              :x="bar(i) + barWidth / 2"
              :y="H - 6"
              text-anchor="middle"
              font-size="9"
              fill="var(--c-text-muted)"
            >{{ b.date.slice(5) }}</text>
          </g>
        </svg>
        <div class="row-flex" style="font-size: 12px; gap: 16px; margin-top: 6px">
          <span><span class="priority-dot" style="background: var(--c-text-muted)"></span>创建</span>
          <span><span class="priority-dot" style="background: var(--c-primary)"></span>完成</span>
        </div>
      </div>

      <div class="section-card">
        <h3>每日:番茄专注分钟</h3>
        <svg :viewBox="`0 0 ${W} ${H}`" :width="W" :height="H" style="width: 100%; height: auto">
          <line :x1="PAD" :y1="H - PAD" :x2="W - PAD" :y2="H - PAD" stroke="var(--c-border)" />
          <g v-for="(b, i) in daily" :key="b.date">
            <rect
              :x="bar(i)"
              :y="(H - PAD) - (b.pomodoro_seconds / 60 / maxPomo) * innerH"
              :width="barWidth - 2"
              :height="(b.pomodoro_seconds / 60 / maxPomo) * innerH"
              fill="var(--c-warn)"
            >
              <title>{{ b.date }}: {{ fmtDuration(b.pomodoro_seconds) }} ({{ b.pomodoro_count }} 次)</title>
            </rect>
            <text
              :x="bar(i) + barWidth / 2"
              :y="H - 6"
              text-anchor="middle"
              font-size="9"
              fill="var(--c-text-muted)"
            >{{ b.date.slice(5) }}</text>
          </g>
        </svg>
        <div v-if="pomoAgg" class="muted" style="font-size: 13px; margin-top: 8px">
          区间共 {{ pomoAgg.total_sessions }} 次,合计 {{ fmtDuration(pomoAgg.total_seconds) }}
        </div>
      </div>
    </template>
  </div>
</template>

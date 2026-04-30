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

const maxTask = computed(() => {
  let max = 1
  for (const b of daily.value) {
    if (b.created > max) max = b.created
    if (b.completed > max) max = b.completed
  }
  return max
})
const maxPomo = computed(() => {
  let max = 1
  for (const b of daily.value) {
    if (b.pomodoro_seconds / 60 > max) max = b.pomodoro_seconds / 60
  }
  return max
})

const innerH = 150

function getBarHeight(val: number, max: number) {
  return Math.max((val / max) * innerH, 0)
}

// BUGFIX: 此前 width 用 (length * 40) + '%'，单位错（应为 px）。
// 在小尺寸屏幕上会撑出无穷宽。改为正确的 px 单位 + max-width 兜底。
const chartWidth = computed(() => Math.max(daily.value.length * 44, 320) + 'px')
</script>

<template>
  <div>
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

    <div class="segment-control">
      <button :class="{ active: range === '7' }" @click="range = '7'; load()">最近 7 天</button>
      <button :class="{ active: range === '14' }" @click="range = '14'; load()">最近 14 天</button>
      <button :class="{ active: range === '30' }" @click="range = '30'; load()">最近 30 天</button>
    </div>

    <div v-if="loading" class="muted" style="text-align: center; padding: 40px 0;">
      数据加载中…
    </div>

    <template v-else>
      <div v-if="summary" class="stats-grid">
        <div class="stat-card">
          <div class="label">今日完成</div>
          <div class="value success-text">{{ summary.completed_today }}</div>
        </div>
        <div class="stat-card">
          <div class="label">本周完成</div>
          <div class="value success-text">{{ summary.completed_this_week }}</div>
        </div>
        <div class="stat-card">
          <div class="label">过期任务</div>
          <div class="value danger-text">{{ summary.todos_overdue }}</div>
        </div>
        <div class="stat-card">
          <div class="label">未完成</div>
          <div class="value">{{ summary.todos_open }}</div>
        </div>
        <div class="stat-card">
          <div class="label">今日专注</div>
          <div class="value">{{ fmtDuration(summary.pomodoro_today_seconds) }}</div>
        </div>
        <div class="stat-card">
          <div class="label">本周专注</div>
          <div class="value">{{ fmtDuration(summary.pomodoro_this_week_seconds) }}</div>
        </div>
      </div>

      <div class="chart-section">
        <div class="chart-header">
          <h3>任务趋势 · 创建 vs 完成</h3>
          <div class="chart-legend">
            <span><span class="legend-dot" style="background: var(--tg-divider-strong)"></span>创建</span>
            <span><span class="legend-dot" style="background: var(--tg-primary)"></span>完成</span>
          </div>
        </div>
        <div class="chart-scroll">
          <div class="chart-container" :style="{ width: chartWidth }">
            <div class="chart-bars">
              <div v-for="b in daily" :key="b.date" class="bar-group" :title="`${b.date}\n创建: ${b.created}\n完成: ${b.completed}`">
                <div class="bar-pair">
                  <div class="bar bar-bg" :style="{ height: getBarHeight(b.created, maxTask) + 'px' }"></div>
                  <div class="bar bar-fg" :style="{ height: getBarHeight(b.completed, maxTask) + 'px' }"></div>
                </div>
                <div class="bar-label">{{ b.date.slice(5) }}</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="chart-section">
        <div class="chart-header">
          <h3>番茄专注 · 分钟</h3>
          <div v-if="pomoAgg" class="chart-legend muted">
            共 {{ pomoAgg.total_sessions }} 次，合计 {{ fmtDuration(pomoAgg.total_seconds) }}
          </div>
        </div>
        <div class="chart-scroll">
          <div class="chart-container" :style="{ width: chartWidth }">
            <div class="chart-bars">
              <div v-for="b in daily" :key="b.date" class="bar-group" :title="`${b.date}\n专注时长: ${fmtDuration(b.pomodoro_seconds)}\n专注次数: ${b.pomodoro_count}`">
                <div class="bar-pair single">
                  <div class="bar bar-warn" :style="{ height: getBarHeight(b.pomodoro_seconds / 60, maxPomo) + 'px' }"></div>
                </div>
                <div class="bar-label">{{ b.date.slice(5) }}</div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.chart-section {
  background: var(--tg-side);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-lg);
  padding: 18px 20px;
  margin-bottom: 16px;
}
.chart-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 18px;
  flex-wrap: wrap;
  gap: 10px;
}
.chart-header h3 {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
}
.chart-legend {
  display: flex;
  gap: 14px;
  font-size: 12.5px;
  color: var(--tg-text-secondary);
}
.legend-dot {
  display: inline-block;
  width: 9px;
  height: 9px;
  border-radius: 50%;
  margin-right: 5px;
  vertical-align: -1px;
}
.chart-scroll {
  overflow-x: auto;
  overflow-y: hidden;
  padding-bottom: 8px;
}
.chart-container {
  height: 180px;
  position: relative;
  border-bottom: 1px solid var(--tg-divider);
}
.chart-bars {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  height: 100%;
  padding: 0 6px;
  gap: 4px;
}
.bar-group {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-end;
  flex: 1;
  min-width: 32px;
  height: 100%;
  cursor: pointer;
  padding: 0 2px;
  transition: opacity 0.2s;
}
.bar-group:hover { opacity: 0.85; }
.bar-pair {
  display: flex;
  align-items: flex-end;
  gap: 2px;
  width: 100%;
  justify-content: center;
  height: 150px;
}
.bar-pair.single .bar {
  width: 14px;
  border-radius: 4px 4px 0 0;
}
.bar {
  width: 10px;
  border-radius: 4px 4px 0 0;
  transition: height 0.4s cubic-bezier(0.4, 0, 0.2, 1);
}
.bar-bg { background: var(--tg-divider-strong); }
.bar-fg { background: var(--tg-primary); }
.bar-warn { background: var(--tg-warn); }
.bar-label {
  margin-top: 8px;
  font-size: 11px;
  color: var(--tg-text-tertiary);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
</style>

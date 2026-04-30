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
// 加入 '365' (一年) 选项. 一年用热力图取代柱状图, 见下方 isYear 分支.
const range = ref<'7' | '14' | '30' | '365'>('14')
const isYear = computed(() => range.value === '365')

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

// =============================================================
// 一年视图: GitHub 风格热力图
// =============================================================
//
// 数据形态: 把后端返回的 daily[] (有可能稀疏 - 后端可能跳过没活动的天)
// 按 'YYYY-MM-DD' 索引起来, 然后从"今天往前推 365 天"逐天填. 没数据的天
// 给一个 0 占位, 这样网格永远是连续的 53 周 × 7 行.
//
// 排版: 列 = 周 (从最早的那一周到本周), 行 = 周一~周日 (中文习惯).
// 列数大约 53. 第一列可能不满 7 天 (因为 365 天的起点不一定恰好是周一),
// 多出来的位置渲染为透明占位格子.
//
// 颜色: 把"任务总数 (created + completed)"或"专注分钟数" 量化成 0~4 五个
// 等级, 用 5 档色阶染色. 这两个指标各做一张热力图. 没活动 = 等级 0 (浅灰).

interface HeatCell {
  /** 'YYYY-MM-DD'; 占位格 (不在 365 天内) 为 null */
  date: string | null
  /** 任务量化等级 0~4 */
  taskLevel: number
  /** 专注量化等级 0~4 */
  pomoLevel: number
  /** 用于 tooltip 的明细 */
  taskCreated: number
  taskCompleted: number
  pomoSeconds: number
}

const dailyByDate = computed<Record<string, DailyBucket>>(() => {
  const map: Record<string, DailyBucket> = {}
  for (const b of daily.value) map[b.date] = b
  return map
})

/** 把 [0, max] 区间分到 5 档 (0,1,2,3,4). 等级 0 表示完全没活动. */
function quantize(v: number, max: number): number {
  if (v <= 0) return 0
  if (max <= 0) return 1
  // 用 1..4 而不是 0..4, 因为只要有活动就该至少显示一档颜色.
  const q = Math.ceil((v / max) * 4)
  return Math.max(1, Math.min(4, q))
}

/**
 * 365 天热力图网格.
 * 返回 { weeks: HeatCell[][], months: { col, label }[] }
 * - weeks: 二维数组 [周列][周一..周日], 长度约 53.
 * - months: 列索引到月份标签的映射, 用于热力图顶部的 "1月 2月 ..." 标签.
 */
const yearGrid = computed(() => {
  const today = new Date()
  today.setHours(0, 0, 0, 0)

  // 起点: 365 天前
  const start = new Date(today)
  start.setDate(today.getDate() - 364)

  // 把 start 往前推到本周一, 让第一列从周一开始, 列 = 完整的一周.
  // (周一=1...周日=0; getDay 周日返回 0)
  const dowMon = (start.getDay() + 6) % 7  // 周一=0...周日=6
  const gridStart = new Date(start)
  gridStart.setDate(start.getDate() - dowMon)

  // 求最大值, 用于量化染色等级
  let maxTaskActivity = 0
  let maxPomoSec = 0
  for (const b of daily.value) {
    const t = b.created + b.completed
    if (t > maxTaskActivity) maxTaskActivity = t
    if (b.pomodoro_seconds > maxPomoSec) maxPomoSec = b.pomodoro_seconds
  }

  const weeks: HeatCell[][] = []
  const months: { col: number; label: string }[] = []
  let lastMonth = -1

  const cur = new Date(gridStart)
  // 一直渲染到今天所在的周末; 用列数控制循环
  while (cur.getTime() <= today.getTime() ||
         ((cur.getDay() + 6) % 7) !== 0) {  // 走到下一个周一才停
    const week: HeatCell[] = []
    for (let row = 0; row < 7; row++) {
      // 超出 [start, today] 范围的格子 = 占位
      const inRange = cur.getTime() >= start.getTime() && cur.getTime() <= today.getTime()
      if (inRange) {
        const key = ymd(cur)
        const b = dailyByDate.value[key]
        const created = b?.created ?? 0
        const completed = b?.completed ?? 0
        const pomoSec = b?.pomodoro_seconds ?? 0
        week.push({
          date: key,
          taskLevel: quantize(created + completed, maxTaskActivity),
          pomoLevel: quantize(pomoSec, maxPomoSec),
          taskCreated: created,
          taskCompleted: completed,
          pomoSeconds: pomoSec,
        })
      } else {
        week.push({
          date: null, taskLevel: 0, pomoLevel: 0,
          taskCreated: 0, taskCompleted: 0, pomoSeconds: 0,
        })
      }
      cur.setDate(cur.getDate() + 1)
    }
    // 月份标签: 这一周的"周一 (week[0])"如果跨入了新月份就加一个标签
    const headDate = new Date(cur)
    headDate.setDate(cur.getDate() - 7)  // week[0] 的日期
    const m = headDate.getMonth()
    if (m !== lastMonth && headDate.getTime() >= start.getTime()) {
      months.push({ col: weeks.length, label: `${m + 1}月` })
      lastMonth = m
    }
    weeks.push(week)
    // 防御: 防止死循环 - 最多 60 列
    if (weeks.length > 60) break
  }

  return { weeks, months }
})

// 一年的总览数字 (热力图旁边的小卡片)
const yearTotals = computed(() => {
  let activeDays = 0
  let totalCompleted = 0
  let totalCreated = 0
  let totalPomoSec = 0
  for (const b of daily.value) {
    if ((b.created + b.completed) > 0 || b.pomodoro_seconds > 0) activeDays++
    totalCreated += b.created
    totalCompleted += b.completed
    totalPomoSec += b.pomodoro_seconds
  }
  return { activeDays, totalCreated, totalCompleted, totalPomoSec }
})

const dowLabels = ['一', '二', '三', '四', '五', '六', '日']

function cellTitle(c: HeatCell, kind: 'task' | 'pomo'): string {
  if (!c.date) return ''
  if (kind === 'task') {
    return `${c.date}\n创建 ${c.taskCreated} · 完成 ${c.taskCompleted}`
  }
  return `${c.date}\n专注 ${fmtDuration(c.pomoSeconds)}`
}
</script>

<template>
  <div>
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

    <div class="segment-control">
      <button :class="{ active: range === '7' }" @click="range = '7'; load()">最近 7 天</button>
      <button :class="{ active: range === '14' }" @click="range = '14'; load()">最近 14 天</button>
      <button :class="{ active: range === '30' }" @click="range = '30'; load()">最近 30 天</button>
      <button :class="{ active: range === '365' }" @click="range = '365'; load()">最近一年</button>
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

      <!-- ============ 一年视图: GitHub 风格热力图 ============ -->
      <template v-if="isYear">
        <!-- 一年总览汇总 -->
        <div class="year-totals">
          <div class="year-total">
            <div class="year-total-label">活跃天数</div>
            <div class="year-total-value">{{ yearTotals.activeDays }}<span class="suffix">天</span></div>
          </div>
          <div class="year-total">
            <div class="year-total-label">创建任务</div>
            <div class="year-total-value">{{ yearTotals.totalCreated }}</div>
          </div>
          <div class="year-total">
            <div class="year-total-label">完成任务</div>
            <div class="year-total-value success-text">{{ yearTotals.totalCompleted }}</div>
          </div>
          <div class="year-total">
            <div class="year-total-label">累计专注</div>
            <div class="year-total-value">{{ fmtDuration(yearTotals.totalPomoSec) }}</div>
          </div>
        </div>

        <!-- 任务热力图 -->
        <div class="chart-section">
          <div class="chart-header">
            <h3>任务活跃度 · 一年</h3>
            <div class="heatmap-legend">
              <span class="muted">少</span>
              <span class="heat-swatch heat-l0" />
              <span class="heat-swatch heat-l1" />
              <span class="heat-swatch heat-l2" />
              <span class="heat-swatch heat-l3" />
              <span class="heat-swatch heat-l4" />
              <span class="muted">多</span>
            </div>
          </div>
          <div class="heatmap-scroll">
            <div class="heatmap heatmap-task">
              <!-- 月份标签行 -->
              <div class="heatmap-months">
                <span
                  v-for="m in yearGrid.months"
                  :key="`tm-${m.col}`"
                  class="heatmap-month"
                  :style="{ left: (m.col * 14) + 'px' }"
                >{{ m.label }}</span>
              </div>
              <!-- 主体: 周列 + dow 行 -->
              <div class="heatmap-body">
                <div class="heatmap-dows">
                  <span v-for="(w, i) in dowLabels" :key="`d-${i}`"
                        :class="{ 'is-faded': i % 2 === 1 }">
                    {{ i % 2 === 0 ? w : '' }}
                  </span>
                </div>
                <div class="heatmap-grid">
                  <div v-for="(week, ci) in yearGrid.weeks" :key="`tw-${ci}`" class="heatmap-week">
                    <span
                      v-for="(c, ri) in week"
                      :key="`tc-${ci}-${ri}`"
                      class="heatmap-cell"
                      :class="[
                        c.date ? `heat-l${c.taskLevel}` : 'heat-empty',
                      ]"
                      :title="cellTitle(c, 'task')"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 专注时长热力图 -->
        <div class="chart-section">
          <div class="chart-header">
            <h3>专注热力 · 一年</h3>
            <div class="heatmap-legend">
              <span class="muted">少</span>
              <span class="heat-swatch warm-l0" />
              <span class="heat-swatch warm-l1" />
              <span class="heat-swatch warm-l2" />
              <span class="heat-swatch warm-l3" />
              <span class="heat-swatch warm-l4" />
              <span class="muted">多</span>
            </div>
          </div>
          <div class="heatmap-scroll">
            <div class="heatmap heatmap-pomo">
              <div class="heatmap-months">
                <span
                  v-for="m in yearGrid.months"
                  :key="`pm-${m.col}`"
                  class="heatmap-month"
                  :style="{ left: (m.col * 14) + 'px' }"
                >{{ m.label }}</span>
              </div>
              <div class="heatmap-body">
                <div class="heatmap-dows">
                  <span v-for="(w, i) in dowLabels" :key="`pd-${i}`"
                        :class="{ 'is-faded': i % 2 === 1 }">
                    {{ i % 2 === 0 ? w : '' }}
                  </span>
                </div>
                <div class="heatmap-grid">
                  <div v-for="(week, ci) in yearGrid.weeks" :key="`pw-${ci}`" class="heatmap-week">
                    <span
                      v-for="(c, ri) in week"
                      :key="`pc-${ci}-${ri}`"
                      class="heatmap-cell"
                      :class="[
                        c.date ? `warm-l${c.pomoLevel}` : 'heat-empty',
                      ]"
                      :title="cellTitle(c, 'pomo')"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>

      <!-- ============ 短周期视图: 柱状图 ============ -->
      <template v-else>
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

/* =====================================================
   一年视图 — 顶部总览卡片
   ===================================================== */
.year-totals {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 10px;
  margin-bottom: 14px;
}
.year-total {
  background: var(--tg-side);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-md);
  padding: 12px 14px;
}
.year-total-label {
  font-size: 11.5px;
  color: var(--tg-text-tertiary);
  letter-spacing: 0.04em;
  margin-bottom: 4px;
}
.year-total-value {
  font-family: 'Sora', sans-serif;
  font-size: 22px;
  font-weight: 800;
  color: var(--tg-text);
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.02em;
}
.year-total-value .suffix {
  font-size: 13px;
  font-weight: 600;
  color: var(--tg-text-tertiary);
  margin-left: 3px;
}

/* =====================================================
   GitHub 风格热力图
   --------------------------------------------
   排版常量 (与 yearGrid.months 里写死的 14px 对齐):
     - 单元格 11px × 11px, 间距 2px → 一周列宽 13px
     - 行间距 2px → 一行高 13px (用 margin-bottom 实现)
     - 月份标签按"列号 × 13"定位; 但我在 inline style 里用 14, 这是为了
       把列号转成像素的近似 — 月份标签不需要逐像素精准, 14 比 13 在视觉
       上更接近真实居中, 看起来更舒服.
   ===================================================== */
.heatmap-legend {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--tg-text-secondary);
}
.heat-swatch {
  display: inline-block;
  width: 11px; height: 11px;
  border-radius: 2px;
  border: 1px solid color-mix(in srgb, var(--tg-text) 6%, transparent);
}

.heatmap-scroll {
  overflow-x: auto;
  overflow-y: hidden;
  padding-bottom: 6px;
}
.heatmap {
  position: relative;
  /* 周一到周日 (7 行) × 13px + 顶部月份标签 18px */
  min-height: 113px;
  /* 最少够 53 周 + 一点空隙 */
  min-width: 760px;
  padding-left: 22px;  /* 给 dow 标签留位 */
}
.heatmap-months {
  position: relative;
  height: 16px;
  margin-bottom: 2px;
}
.heatmap-month {
  position: absolute;
  top: 0;
  font-size: 10.5px;
  font-weight: 600;
  color: var(--tg-text-tertiary);
  letter-spacing: 0.04em;
  white-space: nowrap;
}
.heatmap-body {
  display: flex;
  align-items: flex-start;
  gap: 4px;
}
.heatmap-dows {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-right: 2px;
  /* 与 cell 对齐: 每行 11px 高, 间距 2px → 13px 行高 */
}
.heatmap-dows span {
  height: 11px;
  line-height: 11px;
  font-size: 9.5px;
  color: var(--tg-text-tertiary);
  width: 14px;
  text-align: right;
}
.heatmap-dows span.is-faded {
  visibility: hidden;
}
.heatmap-grid {
  display: flex;
  gap: 2px;
}
.heatmap-week {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.heatmap-cell {
  width: 11px; height: 11px;
  border-radius: 2px;
  display: block;
  cursor: default;
  transition: outline 0.12s ease, transform 0.12s ease;
  outline: 1px solid color-mix(in srgb, var(--tg-text) 4%, transparent);
  outline-offset: -1px;
}
.heatmap-cell:hover {
  outline: 1.5px solid var(--tg-primary);
  outline-offset: -1px;
  transform: scale(1.18);
}

/* 任务热力 — 用品牌主色的 5 档色阶 (兼容深色模式: 通过透明度叠在背景上) */
.heat-empty   { background: transparent; outline: none; }
.heat-l0      { background: color-mix(in srgb, var(--tg-text) 8%, transparent); }
.heat-l1      { background: color-mix(in srgb, var(--tg-primary) 25%, transparent); }
.heat-l2      { background: color-mix(in srgb, var(--tg-primary) 45%, transparent); }
.heat-l3      { background: color-mix(in srgb, var(--tg-primary) 70%, transparent); }
.heat-l4      { background: var(--tg-primary); }

/* 专注热力 — 用警示橙色的 5 档色阶 (与"番茄"主题一致) */
.warm-l0      { background: color-mix(in srgb, var(--tg-text) 8%, transparent); }
.warm-l1      { background: color-mix(in srgb, var(--tg-warn) 28%, transparent); }
.warm-l2      { background: color-mix(in srgb, var(--tg-warn) 50%, transparent); }
.warm-l3      { background: color-mix(in srgb, var(--tg-warn) 75%, transparent); }
.warm-l4      { background: var(--tg-warn); }
</style>

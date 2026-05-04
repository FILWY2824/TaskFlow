<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { stats as statsApi, todos as todosApi, ApiError } from '@/api'
import type { DailyBucket, PomodoroAggregate, StatsSummary, Todo } from '@/types'
import { fmtDuration, taskStartAt } from '@/utils'

const summary = ref<StatsSummary | null>(null)
const daily = ref<DailyBucket[]>([])
const pomoAgg = ref<PomodoroAggregate | null>(null)
// 用于「最近 N 天」视图里"过期任务/未完成"的范围统计:
// 后端 summary 给的是"全部"过期/未完成,这里把所有未完成 todo 拉到前端,
// 按各视图自己的时间窗口(最近 7 / 30 天)再过滤计数。
const openTodos = ref<Todo[]>([])
const errMsg = ref('')
const loading = ref(false)

// 视图分段控制: 今日 / 最近 7 天 / 最近 30 天 / 最近一年 / 全部
// (按需求去掉了原来的 "最近 14 天",新增了 "今日" 和 "全部")
type RangeKey = 'today' | '7' | '30' | '365' | 'all'
const range = ref<RangeKey>('today')
const isYear = computed(() => range.value === '365')
const isToday = computed(() => range.value === 'today')
const isAll = computed(() => range.value === 'all')
const is7 = computed(() => range.value === '7')
const is30 = computed(() => range.value === '30')

// 一次性需要拉多少天的 daily/pomodoro 明细。
// 'today'   → 1 天;  '7' → 7 天;  '30' → 30 天;
// '365'/'all' → 366 天 (后端硬上限)。
function rangeDays(r: RangeKey): number {
  switch (r) {
    case 'today': return 1
    case '7': return 7
    case '30': return 30
    case '365': return 366
    case 'all': return 366
  }
}

async function load() {
  errMsg.value = ''
  loading.value = true
  try {
    const days = rangeDays(range.value)
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

    // 仅 7/30 天视图需要"过期/未完成 (该范围内)"指标 → 拉一次未完成清单。
    // 'all' 视图直接用 summary.todos_overdue / todos_open(全部计数)。
    if (range.value === '7' || range.value === '30') {
      try {
        openTodos.value = await todosApi.list({ include_done: false, limit: 500 })
      } catch {
        openTodos.value = []
      }
    } else {
      openTodos.value = []
    }

    // 切换视图时收起 7 天柱状图的"已选中"指示。
    selectedBarIdx.value = null
    hoveredDotIdx.value = null
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

// =============================================================
// 客户端聚合: 用 daily 求和给"本月/本周完成 & 专注"用
// =============================================================
const sumCompleted = computed(() => daily.value.reduce((a, b) => a + b.completed, 0))
const sumPomoSeconds = computed(() => daily.value.reduce((a, b) => a + b.pomodoro_seconds, 0))
const sumPomoCount = computed(() => daily.value.reduce((a, b) => a + b.pomodoro_count, 0))
const sumCreated = computed(() => daily.value.reduce((a, b) => a + b.created, 0))

// 今日的 daily bucket(today 视图渲染用,可能为空)
const todayBucket = computed<DailyBucket | null>(() => {
  return daily.value.length > 0 ? daily.value[daily.value.length - 1] : null
})

// =============================================================
// 客户端聚合: 用 openTodos 算"最近 N 天的过期/未完成"
//   - 过期: 未完成 ∩ 开始时间 < now ∩ 开始时间在最近 N 天内
//   - 未完成: 未完成 ∩ created_at 在最近 N 天内
// =============================================================
function inLastDaysCount(days: number, mode: 'overdue' | 'open'): number {
  const cutoff = Date.now() - days * 86400 * 1000
  const now = Date.now()
  let n = 0
  for (const t of openTodos.value) {
    if (t.is_completed) continue
    if (mode === 'overdue') {
      const start = taskStartAt(t)
      if (!start) continue
      const ts = new Date(start).getTime()
      if (ts < now && ts >= cutoff) n++
    } else {
      const created = new Date(t.created_at).getTime()
      if (created >= cutoff) n++
    }
  }
  return n
}
const overdueIn7 = computed(() => inLastDaysCount(7, 'overdue'))
const openIn7 = computed(() => inLastDaysCount(7, 'open'))
const overdueIn30 = computed(() => inLastDaysCount(30, 'overdue'))
const openIn30 = computed(() => inLastDaysCount(30, 'open'))

// =============================================================
// 7 天柱状图 (保留, 但加上"点击柱子查看明细"的交互)
// =============================================================
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
const chartWidth = computed(() => Math.max(daily.value.length * 44, 320) + 'px')

// 7 天柱状图: 点击某天高亮 + 在图表上方显示该天的具体数值。
const selectedBarIdx = ref<number | null>(null)
function selectBar(idx: number) {
  selectedBarIdx.value = selectedBarIdx.value === idx ? null : idx
}
function isBarSelected(idx: number) {
  return selectedBarIdx.value === idx
}
const selectedBar = computed(() => {
  if (selectedBarIdx.value === null) return null
  return daily.value[selectedBarIdx.value] || null
})

// =============================================================
// 30 天面积图 (取代柱状图, 更美观)
//   - 用 SVG path 绘制平滑曲线 (Catmull-Rom -> Cubic Bezier)
//   - 曲线下方用渐变填充
//   - 鼠标 hover 在最近的数据点上,显示十字辅助线 + tooltip
// =============================================================
const areaW = 720
const areaH = 220
const areaPad = { top: 18, right: 18, bottom: 32, left: 40 }

interface SeriesPoint { x: number; y: number; v: number; date: string }
interface SeriesPath { line: string; area: string; dots: SeriesPoint[] }

function buildSmoothPath(points: SeriesPoint[]): string {
  if (points.length === 0) return ''
  if (points.length === 1) return `M ${points[0].x} ${points[0].y}`
  // Catmull-Rom -> Cubic Bezier 转换, 让线条平滑而不像锯齿。
  const segs: string[] = [`M ${points[0].x} ${points[0].y}`]
  for (let i = 0; i < points.length - 1; i++) {
    const p0 = points[Math.max(i - 1, 0)]
    const p1 = points[i]
    const p2 = points[i + 1]
    const p3 = points[Math.min(i + 2, points.length - 1)]
    const cp1x = p1.x + (p2.x - p0.x) / 6
    const cp1y = p1.y + (p2.y - p0.y) / 6
    const cp2x = p2.x - (p3.x - p1.x) / 6
    const cp2y = p2.y - (p3.y - p1.y) / 6
    segs.push(`C ${cp1x} ${cp1y}, ${cp2x} ${cp2y}, ${p2.x} ${p2.y}`)
  }
  return segs.join(' ')
}

function areaSeries(getVal: (b: DailyBucket) => number, max: number): SeriesPath {
  const innerW = areaW - areaPad.left - areaPad.right
  const innerHh = areaH - areaPad.top - areaPad.bottom
  const n = daily.value.length
  if (n === 0) return { line: '', area: '', dots: [] }
  const stepX = n === 1 ? 0 : innerW / (n - 1)
  const points: SeriesPoint[] = daily.value.map((b, i) => {
    const v = getVal(b)
    const x = areaPad.left + i * stepX
    const y = areaPad.top + innerHh * (1 - v / Math.max(max, 1))
    return { x, y, v, date: b.date }
  })
  const line = buildSmoothPath(points)
  const last = points[points.length - 1]
  const first = points[0]
  const baseY = areaPad.top + innerHh
  const area = line + ` L ${last.x} ${baseY} L ${first.x} ${baseY} Z`
  return { line, area, dots: points }
}

const taskCreatedSeries = computed(() => areaSeries(b => b.created, maxTask.value))
const taskCompletedSeries = computed(() => areaSeries(b => b.completed, maxTask.value))
const pomoSeries = computed(() => areaSeries(b => b.pomodoro_seconds / 60, maxPomo.value))

// 面积图 hover: 鼠标在 svg 范围内移动,找到 X 距离最近的索引。
const hoveredDotIdx = ref<number | null>(null)
function onAreaMove(e: MouseEvent, kind: 'task' | 'pomo') {
  const series = kind === 'task' ? taskCreatedSeries.value : pomoSeries.value
  if (series.dots.length === 0) { hoveredDotIdx.value = null; return }
  const target = e.currentTarget as SVGElement
  const rect = target.getBoundingClientRect()
  // svg viewBox 是 areaW × areaH, 屏幕宽是 rect.width, 简单等比换算。
  const xInVB = ((e.clientX - rect.left) / rect.width) * areaW
  let bestIdx = 0
  let bestDist = Infinity
  for (let i = 0; i < series.dots.length; i++) {
    const d = Math.abs(series.dots[i].x - xInVB)
    if (d < bestDist) { bestDist = d; bestIdx = i }
  }
  hoveredDotIdx.value = bestIdx
}
function onAreaLeave() { hoveredDotIdx.value = null }

// hover 时的辅助线 X / tooltip 信息(任务图表)
const hoverInfoTask = computed(() => {
  if (hoveredDotIdx.value === null) return null
  const c = taskCreatedSeries.value.dots[hoveredDotIdx.value]
  const cm = taskCompletedSeries.value.dots[hoveredDotIdx.value]
  if (!c) return null
  return { x: c.x, date: c.date, created: c.v, completed: cm?.v ?? 0 }
})
// hover 时的辅助线 X / tooltip 信息(番茄图表)
const hoverInfoPomo = computed(() => {
  if (hoveredDotIdx.value === null) return null
  const p = pomoSeries.value.dots[hoveredDotIdx.value]
  if (!p) return null
  // p.v 是分钟数, 同步取原 daily 拿次数
  const b = daily.value[hoveredDotIdx.value]
  return { x: p.x, date: p.date, minutes: p.v, count: b?.pomodoro_count ?? 0 }
})

// 面积图的 Y 轴刻度(0, max/2, max),只用作参考线
const yTicks = computed(() => {
  const inner = areaH - areaPad.top - areaPad.bottom
  return [0, 0.5, 1].map(t => ({
    y: areaPad.top + inner * (1 - t),
    pct: t,
  }))
})

// 面积图的 X 轴稀疏标签: 30 天里只标 ~6 个
const xTickIndices = computed(() => {
  const n = daily.value.length
  if (n === 0) return [] as number[]
  const want = 6
  const step = Math.max(1, Math.floor((n - 1) / (want - 1)))
  const out: number[] = []
  for (let i = 0; i < n; i += step) out.push(i)
  if (out[out.length - 1] !== n - 1) out.push(n - 1)
  return out
})

// =============================================================
// 一年视图: GitHub 风格热力图 (保持原有实现)
// =============================================================

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

const yearGrid = computed(() => {
  const today = new Date()
  today.setHours(0, 0, 0, 0)

  // 起点: 365 天前
  const start = new Date(today)
  start.setDate(today.getDate() - 364)

  // 把 start 往前推到本周一, 让第一列从周一开始, 列 = 完整的一周.
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
  while (cur.getTime() <= today.getTime() ||
         ((cur.getDay() + 6) % 7) !== 0) {
    const week: HeatCell[] = []
    for (let row = 0; row < 7; row++) {
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
    const headDate = new Date(cur)
    headDate.setDate(cur.getDate() - 7)
    const m = headDate.getMonth()
    if (m !== lastMonth && headDate.getTime() >= start.getTime()) {
      months.push({ col: weeks.length, label: `${m + 1}月` })
      lastMonth = m
    }
    weeks.push(week)
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
      <button :class="{ active: range === 'today' }" @click="range = 'today'; load()">今日统计</button>
      <button :class="{ active: range === '7' }"     @click="range = '7'; load()">最近 7 天</button>
      <button :class="{ active: range === '30' }"    @click="range = '30'; load()">最近 30 天</button>
      <button :class="{ active: range === '365' }"   @click="range = '365'; load()">最近一年</button>
      <button :class="{ active: range === 'all' }"   @click="range = 'all'; load()">全部统计</button>
    </div>

    <div v-if="loading" class="muted" style="text-align: center; padding: 40px 0;">
      数据加载中…
    </div>

    <template v-else>
      <!-- ============ 今日统计 ============ -->
      <template v-if="isToday">
        <div v-if="summary" class="stats-grid">
          <div class="stat-card">
            <div class="label">今日完成</div>
            <div class="value success-text">{{ summary.completed_today }}</div>
          </div>
          <div class="stat-card">
            <div class="label">今日专注</div>
            <div class="value">{{ fmtDuration(summary.pomodoro_today_seconds) }}</div>
          </div>
          <div class="stat-card">
            <div class="label">今日新增</div>
            <div class="value">{{ todayBucket?.created ?? 0 }}</div>
          </div>
          <div class="stat-card">
            <div class="label">今日番茄数</div>
            <div class="value">{{ todayBucket?.pomodoro_count ?? 0 }}</div>
          </div>
          <div class="stat-card">
            <div class="label">今日开始</div>
            <div class="value">{{ summary.todos_due_today }}</div>
          </div>
        </div>
      </template>

      <!-- ============ 最近 7 天: 卡片 + 可点击柱状图 ============ -->
      <template v-else-if="is7">
        <div v-if="summary" class="stats-grid">
          <div class="stat-card">
            <div class="label">本周完成</div>
            <div class="value success-text">{{ summary.completed_this_week }}</div>
          </div>
          <div class="stat-card">
            <div class="label">本周专注</div>
            <div class="value">{{ fmtDuration(summary.pomodoro_this_week_seconds) }}</div>
          </div>
          <div class="stat-card">
            <div class="label">过期任务 · 近 7 天</div>
            <div class="value danger-text">{{ overdueIn7 }}</div>
          </div>
          <div class="stat-card">
            <div class="label">未完成 · 近 7 天</div>
            <div class="value">{{ openIn7 }}</div>
          </div>
        </div>

        <!-- 任务柱状图: 点击任意一根柱子可以查看具体数值 -->
        <div class="chart-section">
          <div class="chart-header">
            <h3>任务趋势 · 创建 vs 完成</h3>
            <div class="chart-legend">
              <span><span class="legend-dot" style="background: var(--tg-divider-strong)"></span>创建</span>
              <span><span class="legend-dot" style="background: var(--tg-primary)"></span>完成</span>
            </div>
          </div>
          <!-- 选中条目时,在图表上方显示该天的具体数值 -->
          <div class="bar-detail" :class="{ visible: selectedBar }">
            <template v-if="selectedBar">
              <span class="bar-detail-date">{{ selectedBar.date }}</span>
              <span class="bar-detail-pill"><span class="legend-dot" style="background: var(--tg-divider-strong)"></span>创建 <strong>{{ selectedBar.created }}</strong></span>
              <span class="bar-detail-pill"><span class="legend-dot" style="background: var(--tg-primary)"></span>完成 <strong>{{ selectedBar.completed }}</strong></span>
              <span class="bar-detail-pill"><span class="legend-dot" style="background: var(--tg-warn)"></span>专注 <strong>{{ fmtDuration(selectedBar.pomodoro_seconds) }}</strong>（{{ selectedBar.pomodoro_count }} 次）</span>
            </template>
            <span v-else class="muted">点击柱子查看当日明细</span>
          </div>
          <div class="chart-scroll">
            <div class="chart-container" :style="{ width: chartWidth }">
              <div class="chart-bars">
                <div
                  v-for="(b, i) in daily" :key="b.date"
                  class="bar-group"
                  :class="{ selected: isBarSelected(i) }"
                  @click="selectBar(i)"
                  :title="`${b.date}\n创建: ${b.created}\n完成: ${b.completed}`"
                >
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
          <div class="bar-detail" :class="{ visible: selectedBar }">
            <template v-if="selectedBar">
              <span class="bar-detail-date">{{ selectedBar.date }}</span>
              <span class="bar-detail-pill"><span class="legend-dot" style="background: var(--tg-warn)"></span>专注 <strong>{{ fmtDuration(selectedBar.pomodoro_seconds) }}</strong></span>
              <span class="bar-detail-pill">番茄次数 <strong>{{ selectedBar.pomodoro_count }}</strong></span>
            </template>
            <span v-else class="muted">点击柱子查看当日明细</span>
          </div>
          <div class="chart-scroll">
            <div class="chart-container" :style="{ width: chartWidth }">
              <div class="chart-bars">
                <div
                  v-for="(b, i) in daily" :key="b.date"
                  class="bar-group"
                  :class="{ selected: isBarSelected(i) }"
                  @click="selectBar(i)"
                  :title="`${b.date}\n专注时长: ${fmtDuration(b.pomodoro_seconds)}\n专注次数: ${b.pomodoro_count}`"
                >
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

      <!-- ============ 最近 30 天: 卡片 + 平滑面积图 ============ -->
      <template v-else-if="is30">
        <div v-if="summary" class="stats-grid">
          <div class="stat-card">
            <div class="label">本月完成</div>
            <div class="value success-text">{{ sumCompleted }}</div>
          </div>
          <div class="stat-card">
            <div class="label">本月专注</div>
            <div class="value">{{ fmtDuration(sumPomoSeconds) }}</div>
          </div>
          <div class="stat-card">
            <div class="label">过期任务 · 近 30 天</div>
            <div class="value danger-text">{{ overdueIn30 }}</div>
          </div>
          <div class="stat-card">
            <div class="label">未完成 · 近 30 天</div>
            <div class="value">{{ openIn30 }}</div>
          </div>
        </div>

        <!-- 任务平滑面积图 -->
        <div class="chart-section">
          <div class="chart-header">
            <h3>任务趋势 · 创建 vs 完成</h3>
            <div class="chart-legend">
              <span><span class="legend-dot" style="background: var(--tg-divider-strong)"></span>创建</span>
              <span><span class="legend-dot" style="background: var(--tg-primary)"></span>完成</span>
            </div>
          </div>
          <div class="area-wrap">
            <svg
              :viewBox="`0 0 ${areaW} ${areaH}`"
              class="area-svg"
              preserveAspectRatio="none"
              @mousemove="onAreaMove($event, 'task')"
              @mouseleave="onAreaLeave"
            >
              <defs>
                <linearGradient :id="`grad-created`" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%"   stop-color="var(--tg-divider-strong)" stop-opacity="0.55" />
                  <stop offset="100%" stop-color="var(--tg-divider-strong)" stop-opacity="0" />
                </linearGradient>
                <linearGradient :id="`grad-completed`" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%"   stop-color="var(--tg-primary)" stop-opacity="0.45" />
                  <stop offset="100%" stop-color="var(--tg-primary)" stop-opacity="0" />
                </linearGradient>
              </defs>

              <!-- 水平参考线 -->
              <g class="grid">
                <line v-for="(t, i) in yTicks" :key="`gt-${i}`"
                      :x1="areaPad.left" :x2="areaW - areaPad.right"
                      :y1="t.y" :y2="t.y" />
              </g>

              <!-- 创建: 灰色面积 + 描边 -->
              <path :d="taskCreatedSeries.area" fill="url(#grad-created)" />
              <path :d="taskCreatedSeries.line"
                    fill="none"
                    stroke="var(--tg-divider-strong)"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round" />

              <!-- 完成: 主色面积 + 描边 -->
              <path :d="taskCompletedSeries.area" fill="url(#grad-completed)" />
              <path :d="taskCompletedSeries.line"
                    fill="none"
                    stroke="var(--tg-primary)"
                    stroke-width="2.2"
                    stroke-linecap="round"
                    stroke-linejoin="round" />

              <!-- X 轴日期稀疏标签 -->
              <g class="x-axis">
                <text v-for="i in xTickIndices" :key="`x-${i}`"
                      :x="taskCreatedSeries.dots[i]?.x ?? 0"
                      :y="areaH - 10"
                      text-anchor="middle">
                  {{ daily[i]?.date.slice(5) }}
                </text>
              </g>

              <!-- hover 辅助线 + 高亮点 -->
              <template v-if="hoverInfoTask">
                <line class="hover-rule"
                      :x1="hoverInfoTask.x" :x2="hoverInfoTask.x"
                      :y1="areaPad.top" :y2="areaH - areaPad.bottom" />
                <circle class="hover-dot hover-dot-bg"
                        :cx="taskCreatedSeries.dots[hoveredDotIdx!]?.x"
                        :cy="taskCreatedSeries.dots[hoveredDotIdx!]?.y"
                        r="4" />
                <circle class="hover-dot hover-dot-fg"
                        :cx="taskCompletedSeries.dots[hoveredDotIdx!]?.x"
                        :cy="taskCompletedSeries.dots[hoveredDotIdx!]?.y"
                        r="4" />
              </template>
            </svg>

            <!-- hover 时弹出的 tooltip -->
            <div v-if="hoverInfoTask" class="area-tip"
                 :style="{ left: ((hoverInfoTask.x / areaW) * 100) + '%' }">
              <div class="area-tip-date">{{ hoverInfoTask.date }}</div>
              <div class="area-tip-row"><span class="legend-dot" style="background: var(--tg-divider-strong)"></span>创建 <strong>{{ hoverInfoTask.created }}</strong></div>
              <div class="area-tip-row"><span class="legend-dot" style="background: var(--tg-primary)"></span>完成 <strong>{{ hoverInfoTask.completed }}</strong></div>
            </div>
          </div>
        </div>

        <!-- 番茄平滑面积图 -->
        <div class="chart-section">
          <div class="chart-header">
            <h3>番茄专注 · 分钟</h3>
            <div v-if="pomoAgg" class="chart-legend muted">
              共 {{ pomoAgg.total_sessions }} 次，合计 {{ fmtDuration(pomoAgg.total_seconds) }}
            </div>
          </div>
          <div class="area-wrap">
            <svg
              :viewBox="`0 0 ${areaW} ${areaH}`"
              class="area-svg"
              preserveAspectRatio="none"
              @mousemove="onAreaMove($event, 'pomo')"
              @mouseleave="onAreaLeave"
            >
              <defs>
                <linearGradient :id="`grad-pomo`" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%"   stop-color="var(--tg-warn)" stop-opacity="0.5" />
                  <stop offset="100%" stop-color="var(--tg-warn)" stop-opacity="0" />
                </linearGradient>
              </defs>

              <g class="grid">
                <line v-for="(t, i) in yTicks" :key="`gp-${i}`"
                      :x1="areaPad.left" :x2="areaW - areaPad.right"
                      :y1="t.y" :y2="t.y" />
              </g>

              <path :d="pomoSeries.area" fill="url(#grad-pomo)" />
              <path :d="pomoSeries.line"
                    fill="none"
                    stroke="var(--tg-warn)"
                    stroke-width="2.2"
                    stroke-linecap="round"
                    stroke-linejoin="round" />

              <g class="x-axis">
                <text v-for="i in xTickIndices" :key="`px-${i}`"
                      :x="pomoSeries.dots[i]?.x ?? 0"
                      :y="areaH - 10"
                      text-anchor="middle">
                  {{ daily[i]?.date.slice(5) }}
                </text>
              </g>

              <template v-if="hoverInfoPomo">
                <line class="hover-rule"
                      :x1="hoverInfoPomo.x" :x2="hoverInfoPomo.x"
                      :y1="areaPad.top" :y2="areaH - areaPad.bottom" />
                <circle class="hover-dot hover-dot-warn"
                        :cx="pomoSeries.dots[hoveredDotIdx!]?.x"
                        :cy="pomoSeries.dots[hoveredDotIdx!]?.y"
                        r="4" />
              </template>
            </svg>

            <div v-if="hoverInfoPomo" class="area-tip"
                 :style="{ left: ((hoverInfoPomo.x / areaW) * 100) + '%' }">
              <div class="area-tip-date">{{ hoverInfoPomo.date }}</div>
              <div class="area-tip-row"><span class="legend-dot" style="background: var(--tg-warn)"></span>专注 <strong>{{ Math.round(hoverInfoPomo.minutes) }}</strong> 分钟</div>
              <div class="area-tip-row">番茄次数 <strong>{{ hoverInfoPomo.count }}</strong></div>
            </div>
          </div>
        </div>
      </template>

      <!-- ============ 最近一年: 顶部年度汇总 + 两张热力图 (按需求,不再展示今日/本周/过期/未完成卡片) ============ -->
      <template v-else-if="isYear">
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
              <div class="heatmap-months">
                <span
                  v-for="m in yearGrid.months"
                  :key="`tm-${m.col}`"
                  class="heatmap-month"
                  :style="{ left: (m.col * 14) + 'px' }"
                >{{ m.label }}</span>
              </div>
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
                      :class="[c.date ? `heat-l${c.taskLevel}` : 'heat-empty']"
                      :title="cellTitle(c, 'task')"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

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
                      :class="[c.date ? `warm-l${c.pomoLevel}` : 'heat-empty']"
                      :title="cellTitle(c, 'pomo')"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>

      <!-- ============ 全部统计 ============ -->
      <template v-else-if="isAll">
        <div v-if="summary" class="stats-grid">
          <div class="stat-card">
            <div class="label">总任务数</div>
            <div class="value">{{ summary.todos_total }}</div>
          </div>
          <div class="stat-card">
            <div class="label">已完成</div>
            <div class="value success-text">{{ summary.todos_completed }}</div>
          </div>
          <div class="stat-card">
            <div class="label">未完成</div>
            <div class="value">{{ summary.todos_open }}</div>
          </div>
          <div class="stat-card">
            <div class="label">过期任务</div>
            <div class="value danger-text">{{ summary.todos_overdue }}</div>
          </div>
          <div class="stat-card">
            <div class="label">累计专注 · 近一年</div>
            <div class="value">{{ fmtDuration(sumPomoSeconds) }}</div>
          </div>
          <div class="stat-card">
            <div class="label">番茄次数 · 近一年</div>
            <div class="value">{{ sumPomoCount }}</div>
          </div>
          <div class="stat-card">
            <div class="label">创建任务 · 近一年</div>
            <div class="value">{{ sumCreated }}</div>
          </div>
          <div class="stat-card">
            <div class="label">完成任务 · 近一年</div>
            <div class="value success-text">{{ sumCompleted }}</div>
          </div>
        </div>
        <p class="muted all-stats-hint">
          任务相关数字为账户全部数据；带 “近一年” 标记的部分受后端聚合上限（最多 366 天）限制。
        </p>
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
  transition: opacity 0.2s, background 0.18s;
  border-radius: var(--tg-radius-sm, 6px);
}
.bar-group:hover { opacity: 0.85; }
/* 选中时给柱子组一个浅色高亮背景 + 标签变粗 */
.bar-group.selected {
  background: color-mix(in srgb, var(--tg-primary) 8%, transparent);
}
.bar-group.selected .bar-label {
  color: var(--tg-primary);
  font-weight: 700;
}
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

/* === 7 天图: 选中柱子时显示当日明细的小条 === */
.bar-detail {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  padding: 10px 14px;
  margin-bottom: 12px;
  border-radius: var(--tg-radius-md);
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  font-size: 13px;
  color: var(--tg-text-secondary);
  min-height: 38px;
  transition: border-color var(--tg-trans-fast), background var(--tg-trans-fast);
}
.bar-detail.visible {
  border-color: color-mix(in srgb, var(--tg-primary) 35%, transparent);
  background: color-mix(in srgb, var(--tg-primary) 4%, var(--tg-bg-elev));
}
.bar-detail-date {
  font-weight: 700;
  color: var(--tg-text);
  font-variant-numeric: tabular-nums;
}
.bar-detail-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.bar-detail-pill strong {
  color: var(--tg-text);
  font-weight: 700;
}

/* =====================================================
   30 天: 平滑面积图
   ===================================================== */
.area-wrap {
  position: relative;
  width: 100%;
}
.area-svg {
  width: 100%;
  height: 220px;
  display: block;
  cursor: crosshair;
}
.area-svg .grid line {
  stroke: var(--tg-divider);
  stroke-width: 1;
  stroke-dasharray: 3 4;
  opacity: 0.6;
}
.area-svg .x-axis text {
  fill: var(--tg-text-tertiary);
  font-size: 10.5px;
  font-variant-numeric: tabular-nums;
}
.area-svg .hover-rule {
  stroke: var(--tg-text-tertiary);
  stroke-width: 1;
  stroke-dasharray: 2 3;
  opacity: 0.7;
  pointer-events: none;
}
.area-svg .hover-dot {
  pointer-events: none;
  filter: drop-shadow(0 1px 4px rgba(0, 0, 0, 0.18));
}
.hover-dot-bg { fill: var(--tg-divider-strong); stroke: var(--tg-side); stroke-width: 2; }
.hover-dot-fg { fill: var(--tg-primary);        stroke: var(--tg-side); stroke-width: 2; }
.hover-dot-warn { fill: var(--tg-warn);         stroke: var(--tg-side); stroke-width: 2; }

.area-tip {
  position: absolute;
  top: 8px;
  transform: translateX(-50%);
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-md);
  box-shadow: 0 6px 18px -6px rgba(0, 0, 0, 0.18);
  padding: 8px 12px;
  font-size: 12.5px;
  color: var(--tg-text-secondary);
  white-space: nowrap;
  pointer-events: none;
  z-index: 2;
  min-width: 120px;
}
.area-tip-date {
  font-weight: 700;
  color: var(--tg-text);
  margin-bottom: 4px;
  font-variant-numeric: tabular-nums;
}
.area-tip-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 2px;
}
.area-tip-row strong {
  color: var(--tg-text);
  font-weight: 700;
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
  min-height: 113px;
  min-width: 760px;
  padding-left: 22px;
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

/* 任务热力 — 用品牌主色的 5 档色阶 */
.heat-empty   { background: transparent; outline: none; }
.heat-l0      { background: color-mix(in srgb, var(--tg-text) 8%, transparent); }
.heat-l1      { background: color-mix(in srgb, var(--tg-primary) 25%, transparent); }
.heat-l2      { background: color-mix(in srgb, var(--tg-primary) 45%, transparent); }
.heat-l3      { background: color-mix(in srgb, var(--tg-primary) 70%, transparent); }
.heat-l4      { background: var(--tg-primary); }

/* 专注热力 — 用警示橙色的 5 档色阶 */
.warm-l0      { background: color-mix(in srgb, var(--tg-text) 8%, transparent); }
.warm-l1      { background: color-mix(in srgb, var(--tg-warn) 28%, transparent); }
.warm-l2      { background: color-mix(in srgb, var(--tg-warn) 50%, transparent); }
.warm-l3      { background: color-mix(in srgb, var(--tg-warn) 75%, transparent); }
.warm-l4      { background: var(--tg-warn); }

.all-stats-hint {
  margin-top: 12px;
  font-size: 12.5px;
  color: var(--tg-text-tertiary);
}
</style>

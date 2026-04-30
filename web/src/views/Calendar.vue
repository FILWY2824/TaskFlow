<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import type { Todo } from '@/types'
import { todos as todosApi, ApiError } from '@/api'
import { useDataStore } from '@/stores/data'
import { isOverdue } from '@/utils'

const data = useDataStore()
const router = useRouter()

// baseDate 表示"当前展示的月份的任意一天"。所有视图相关计算都从它派生。
const baseDate = ref(new Date())
const items = ref<Todo[]>([])
const loading = ref(false)
const errMsg = ref('')

// 计算月份的首日、末日
const monthFirst = computed(() => {
  const d = new Date(baseDate.value)
  d.setDate(1)
  d.setHours(0, 0, 0, 0)
  return d
})
const monthLast = computed(() => {
  const d = new Date(monthFirst.value)
  d.setMonth(d.getMonth() + 1)
  d.setDate(0)
  d.setHours(0, 0, 0, 0)
  return d
})

// 网格起点：月首所在周的周一（中文习惯周一开头）。
const cells = computed(() => {
  const first = monthFirst.value
  const dowFirst = (first.getDay() + 6) % 7
  const start = new Date(first)
  start.setDate(first.getDate() - dowFirst)
  const last = monthLast.value
  const dowLast = (last.getDay() + 6) % 7
  const end = new Date(last)
  end.setDate(last.getDate() + (6 - dowLast))
  const arr: Date[] = []
  const cur = new Date(start)
  while (cur.getTime() <= end.getTime()) {
    arr.push(new Date(cur))
    cur.setDate(cur.getDate() + 1)
  }
  return arr
})

// 行数（用于 grid-template-rows）
const rowCount = computed(() => cells.value.length / 7)

const monthLabel = computed(() => {
  return `${baseDate.value.getFullYear()} 年 ${baseDate.value.getMonth() + 1} 月`
})

const todoMap = computed<Record<string, Todo[]>>(() => {
  const m: Record<string, Todo[]> = {}
  for (const t of items.value) {
    if (!t.due_at) continue
    const d = new Date(t.due_at)
    const key = dayKey(d)
    ;(m[key] ??= []).push(t)
  }
  return m
})

function dayKey(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
function todayStr(): string { return dayKey(new Date()) }
function isCurrentMonth(d: Date): boolean {
  return d.getMonth() === baseDate.value.getMonth() && d.getFullYear() === baseDate.value.getFullYear()
}
function isWeekend(d: Date): boolean {
  // weekday: 0 周日, 6 周六
  const w = d.getDay()
  return w === 0 || w === 6
}

// 给某条 Todo 取分类的颜色
function todoCatColor(t: Todo): string {
  if (!t.list_id) return ''
  const l = data.lists.find((x) => x.id === t.list_id)
  return l?.color || ''
}
// 当前是否“逾期未完成”——展示用灰色
function todoOverdue(t: Todo): boolean {
  return isOverdue(t)
}

async function load() {
  loading.value = true
  errMsg.value = ''
  try {
    const start = cells.value[0]
    const end = cells.value[cells.value.length - 1]
    const after = new Date(start)
    after.setHours(0, 0, 0, 0)
    const before = new Date(end)
    before.setDate(end.getDate() + 1)
    before.setHours(0, 0, 0, 0)

    const all = await todosApi.list({ limit: 500, include_done: true, order_by: 'due_at_asc' })
    items.value = all.filter((t) => {
      if (!t.due_at) return false
      const d = new Date(t.due_at).getTime()
      return d >= after.getTime() && d < before.getTime()
    })
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  // 让日历页彻底"无滚动"——临时改写主内容区的 layout
  document.body.classList.add('on-calendar')
  await data.loadLists()
  await load()
})
onBeforeUnmount(() => {
  document.body.classList.remove('on-calendar')
})
watch(baseDate, load)

function prev() {
  const d = new Date(baseDate.value)
  d.setMonth(d.getMonth() - 1, 1)
  baseDate.value = d
}
function next() {
  const d = new Date(baseDate.value)
  d.setMonth(d.getMonth() + 1, 1)
  baseDate.value = d
}
function jumpToday() { baseDate.value = new Date() }

// 进入某一天的详情页
function gotoDay(d: Date) {
  router.push({ name: 'day', params: { date: dayKey(d) } })
}

const dows = ['一', '二', '三', '四', '五', '六', '日']

// 为日历单元格内每天的事件数最多展示 N 个，以避免拥挤
const MAX_EVENTS_PER_CELL = 3
</script>

<template>
  <div class="cal-page">
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

    <div class="calendar-card">
      <header class="cal-header">
        <div class="cal-nav">
          <button class="btn-icon nav-btn" @click="prev" title="上个月">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
          </button>
          <div class="cal-title">{{ monthLabel }}</div>
          <button class="btn-icon nav-btn" @click="next" title="下个月">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>
          </button>
        </div>
        <div class="cal-header-right">
          <span v-if="loading" class="cal-loading muted">加载中…</span>
          <button class="btn-secondary cal-today-btn" @click="jumpToday">回到今天</button>
        </div>
      </header>

      <div class="cal-grid" :style="{ '--row-count': rowCount }">
        <div v-for="(d, i) in dows" :key="`dow-${i}`" class="cal-dow"
             :class="{ 'is-weekend': i >= 5 }">
          周{{ d }}
        </div>

        <div
          v-for="(d, i) in cells"
          :key="i"
          class="cal-cell"
          :class="{
            'is-today': dayKey(d) === todayStr(),
            'in-month': isCurrentMonth(d),
            'is-weekend': isWeekend(d),
            'is-empty': (todoMap[dayKey(d)] || []).length === 0,
          }"
          @click="gotoDay(d)"
        >
          <div class="cell-head">
            <span class="cell-num">{{ d.getDate() }}</span>
            <span v-if="dayKey(d) === todayStr()" class="cell-today-badge">今</span>
          </div>

          <div class="cell-events">
            <div
              v-for="t in (todoMap[dayKey(d)] || []).slice(0, MAX_EVENTS_PER_CELL)"
              :key="t.id"
              class="cell-ev"
              :class="{
                'is-completed': t.is_completed,
                'is-overdue': todoOverdue(t),
                [`prio-${t.priority}`]: true,
              }"
              :style="todoCatColor(t) ? { '--cat-color': todoCatColor(t) } : {}"
              :title="t.title"
            >
              <span class="ev-dot" />
              <span class="ev-title">{{ t.title }}</span>
            </div>
            <div
              v-if="(todoMap[dayKey(d)] || []).length > MAX_EVENTS_PER_CELL"
              class="cell-more"
            >
              +{{ (todoMap[dayKey(d)] || []).length - MAX_EVENTS_PER_CELL }} 条
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/*
 * 关键约束：日历页全局禁滚。整个 .cal-page 占满主内容区高度，
 *           任何 cell 内的事件只展示 N 条 + "+N 条"，绝不依赖滚动条。
 *
 * 主区域的滚动通过 body.on-calendar 在全局样式中关闭（见 style.css）。
 */
.cal-page {
  height: 100%;
  width: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.calendar-card {
  flex: 1; min-height: 0;
  display: flex; flex-direction: column;
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-xl);
  box-shadow: var(--tg-shadow-md);
  overflow: hidden;
  position: relative;
}
/* 玻璃感顶部高光 */
.calendar-card::before {
  content: ''; position: absolute; inset: 0 0 auto 0; height: 64px;
  background: linear-gradient(180deg,
    color-mix(in srgb, var(--tg-primary) 7%, transparent), transparent);
  pointer-events: none;
}

/* ---------- header ---------- */
.cal-header {
  position: relative; z-index: 1;
  display: flex; align-items: center; justify-content: space-between;
  padding: 14px 18px;
  flex-shrink: 0;
  border-bottom: 1px solid var(--tg-divider);
}
.cal-nav {
  display: flex; align-items: center; gap: 6px;
}
.cal-title {
  font-family: 'Sora', sans-serif;
  font-size: 18px; font-weight: 800;
  letter-spacing: -0.022em;
  padding: 0 8px;
  min-width: 140px; text-align: center;
  background: var(--tg-grad-brand);
  -webkit-background-clip: text; background-clip: text;
  color: transparent;
}
.cal-header-right { display: flex; align-items: center; gap: 12px; }
.cal-loading { font-size: 12px; }
.nav-btn {
  width: 34px; height: 34px;
  border-radius: var(--tg-radius-pill);
}
.cal-today-btn {
  padding: 7px 16px;
  font-size: 13px; font-weight: 600;
  border-radius: var(--tg-radius-pill);
}

/* ---------- grid ---------- */
.cal-grid {
  flex: 1; min-height: 0;
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  grid-template-rows: 32px repeat(var(--row-count, 5), minmax(0, 1fr));
  gap: 1px;
  background: var(--tg-divider);
  padding: 0;
}
.cal-dow {
  display: flex; align-items: center; justify-content: center;
  background: var(--tg-bg-elev);
  font-family: 'Sora', sans-serif;
  font-size: 11.5px; font-weight: 700;
  color: var(--tg-text-secondary);
  letter-spacing: 0.06em;
  text-transform: uppercase;
}
.cal-dow.is-weekend { color: var(--tg-primary); }

.cal-cell {
  position: relative;
  background: var(--tg-bg-elev);
  display: flex; flex-direction: column;
  padding: 6px 8px 4px;
  cursor: pointer;
  min-height: 0; min-width: 0;
  overflow: hidden;
  transition: background var(--tg-trans-fast), transform var(--tg-trans-fast);
}
.cal-cell:hover {
  background: color-mix(in srgb, var(--tg-primary) 5%, var(--tg-bg-elev));
}
.cal-cell:active { transform: scale(0.99); }

.cal-cell:not(.in-month) { background: color-mix(in srgb, var(--tg-text) 2%, transparent); }
.cal-cell:not(.in-month) .cell-num { color: var(--tg-text-tertiary); opacity: 0.55; }
.cal-cell:not(.in-month) .cell-events { opacity: 0.4; }

/* 今天：强调色填充 + 角徽章 */
.cal-cell.is-today {
  background: color-mix(in srgb, var(--tg-primary) 10%, var(--tg-bg-elev));
}
.cal-cell.is-today .cell-num {
  color: var(--tg-primary);
  font-weight: 800;
}

.cell-head {
  display: flex; align-items: center; justify-content: space-between;
  flex-shrink: 0;
  margin-bottom: 4px;
}
.cell-num {
  font-family: 'Sora', sans-serif;
  font-size: 12.5px; font-weight: 700;
  color: var(--tg-text);
  letter-spacing: -0.01em;
}
.cal-cell.is-weekend:not(.is-today) .cell-num { color: var(--tg-text-secondary); }
.cell-today-badge {
  display: inline-flex; align-items: center; justify-content: center;
  min-width: 18px; height: 18px; padding: 0 5px;
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
  font-family: 'Sora', sans-serif;
  font-size: 10.5px; font-weight: 800;
  border-radius: 999px;
  box-shadow: var(--tg-shadow-sm);
}

/* ---------- events ---------- */
.cell-events {
  display: flex; flex-direction: column; gap: 3px;
  flex: 1; min-height: 0;
  overflow: hidden;
}
.cell-ev {
  --cat-color: var(--tg-primary);
  display: flex; align-items: center; gap: 5px;
  padding: 3px 7px;
  background: color-mix(in srgb, var(--cat-color) 14%, transparent);
  color: color-mix(in srgb, var(--cat-color) 65%, var(--tg-text));
  border-left: 2.5px solid var(--cat-color);
  border-radius: 5px;
  font-size: 11.5px; font-weight: 600;
  line-height: 1.25;
  white-space: nowrap; overflow: hidden;
  flex-shrink: 0;
  transition: background var(--tg-trans-fast), color var(--tg-trans-fast);
}
.cell-ev .ev-dot {
  width: 5px; height: 5px;
  border-radius: 50%;
  background: var(--cat-color);
  flex-shrink: 0;
}
.cell-ev .ev-title {
  flex: 1; min-width: 0;
  overflow: hidden; text-overflow: ellipsis;
}

/* 优先级回退色（无分类时） */
.cell-ev.prio-0:not([style*='--cat-color']) { --cat-color: var(--cat-emerald); }
.cell-ev.prio-1:not([style*='--cat-color']) { --cat-color: var(--cat-sky); }
.cell-ev.prio-2:not([style*='--cat-color']) { --cat-color: var(--cat-emerald); }
.cell-ev.prio-3:not([style*='--cat-color']) { --cat-color: var(--cat-amber); }
.cell-ev.prio-4:not([style*='--cat-color']) { --cat-color: var(--cat-rose); }

/* 完成态：删除线 */
.cell-ev.is-completed {
  background: color-mix(in srgb, var(--cat-color) 8%, transparent);
  color: color-mix(in srgb, var(--cat-color) 45%, var(--tg-text-tertiary));
  text-decoration: line-through;
  text-decoration-thickness: 1.4px;
  text-decoration-color: color-mix(in srgb, var(--cat-color) 60%, transparent);
}
.cell-ev.is-completed .ev-dot { opacity: 0.5; }

/* 逾期未完成：变灰 */
.cell-ev.is-overdue:not(.is-completed) {
  --cat-color: var(--tg-text-tertiary);
  background: color-mix(in srgb, var(--tg-text-tertiary) 12%, transparent);
  color: var(--tg-text-tertiary);
  border-left-color: var(--tg-text-tertiary);
}

.cell-more {
  font-family: 'Manrope', sans-serif;
  font-size: 10.5px; font-weight: 700;
  color: var(--tg-text-tertiary);
  padding: 0 4px;
  margin-top: 1px;
  flex-shrink: 0;
}

/* 空单元格视觉留白 */
.cal-cell.is-empty:not(.is-today):hover {
  background: color-mix(in srgb, var(--tg-primary) 4%, var(--tg-bg-elev));
}

/* ---------- 响应式 ---------- */
@media (max-width: 900px) {
  .cell-ev { font-size: 10.5px; padding: 2px 5px; }
  .cell-num { font-size: 11.5px; }
  .cal-title { font-size: 16px; min-width: 110px; }
  .cal-today-btn { padding: 6px 12px; font-size: 12px; }
}
@media (max-width: 600px) {
  .cell-events { gap: 2px; }
  .cell-ev .ev-dot { display: none; }
  .cell-ev { padding: 1px 4px; }
}
</style>

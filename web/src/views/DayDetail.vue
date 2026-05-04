<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import type { Todo } from '@/types'
import { todos as todosApi, ApiError } from '@/api'
import { fmtDurationMinutes, fmtTime, isOverdue, PRIORITY_LABELS, toRFC3339 } from '@/utils'
import { useDataStore } from '@/stores/data'
import TodoEditDrawer from '@/components/TodoEditDrawer.vue'
import PrettyTimePicker from '@/components/PrettyTimePicker.vue'
import { confirmDialog, alertDialog } from '@/dialogs'

const online = ref(navigator.onLine)
function _onOnline() { online.value = true }
function _onOffline() { online.value = false }

const props = defineProps<{
  date: string  // 形如 YYYY-MM-DD（来自路由 params）
}>()

const router = useRouter()
const data = useDataStore()

const items = ref<Todo[]>([])
const loading = ref(false)
const errMsg = ref('')
const editing = ref<Todo | null>(null)

// ======== 顶部分类筛选 ========
// 'all' 表示不筛选；'none' 表示只看未分类；数字（list.id 转字符串）表示某个分类
const filterKey = ref<'all' | 'none' | string>('all')
// 下拉气泡的开关（之前是一行 chip，分类一多就会撑爆视觉，改成"按需点开"）
const filterOpen = ref(false)

// 当前筛选项的展示信息（label / 颜色 / 数量 / 圆点样式），驱动触发器按钮的呈现。
const activeFilterLabel = computed<string>(() => {
  if (filterKey.value === 'all') return '全部'
  if (filterKey.value === 'none') return '未分类'
  const id = Number(filterKey.value)
  const l = data.lists.find((x) => x.id === id)
  return l?.name || '未知分类'
})
const activeFilterColor = computed<string>(() => {
  if (filterKey.value === 'all') return 'var(--cat-sky)'
  if (filterKey.value === 'none') return 'var(--tg-text-tertiary)'
  const id = Number(filterKey.value)
  const l = data.lists.find((x) => x.id === id)
  return l?.color || 'var(--tg-primary)'
})
const activeFilterDotClass = computed<string>(() => {
  if (filterKey.value === 'all') return 'is-all-light'
  if (filterKey.value === 'none') return 'is-none'
  return ''
})
const activeFilterCount = computed<number>(() => {
  if (filterKey.value === 'all') return items.value.length
  if (filterKey.value === 'none') return items.value.filter((t) => !t.list_id).length
  const id = Number(filterKey.value)
  return items.value.filter((t) => t.list_id === id).length
})

// 简易"点击外部关闭"指令：本组件局部用一个 directive，避免引第三方依赖。
const vClickOutside = {
  mounted(el: HTMLElement, binding: { value: () => void }) {
    const handler = (e: MouseEvent) => {
      if (!el.contains(e.target as Node)) binding.value()
    }
    ;(el as HTMLElement & { __clickOutside__?: (e: MouseEvent) => void }).__clickOutside__ = handler
    document.addEventListener('mousedown', handler)
  },
  unmounted(el: HTMLElement) {
    const target = el as HTMLElement & { __clickOutside__?: (e: MouseEvent) => void }
    if (target.__clickOutside__) {
      document.removeEventListener('mousedown', target.__clickOutside__)
      delete target.__clickOutside__
    }
  },
}

const filteredItems = computed<Todo[]>(() => {
  if (filterKey.value === 'all') return items.value
  if (filterKey.value === 'none') return items.value.filter((t) => !t.list_id)
  const id = Number(filterKey.value)
  return items.value.filter((t) => t.list_id === id)
})

// 分组：未完成 / 已完成 / 逾期未完成
const grouped = computed(() => {
  const open: Todo[] = []
  const done: Todo[] = []
  const overdue: Todo[] = []
  for (const t of filteredItems.value) {
    if (t.is_completed) done.push(t)
    else if (isOverdue(t)) overdue.push(t)
    else open.push(t)
  }
  // 内部按 due_at 升序
  const byDue = (a: Todo, b: Todo) => {
    const ta = a.due_at ? new Date(a.due_at).getTime() : 0
    const tb = b.due_at ? new Date(b.due_at).getTime() : 0
    return ta - tb
  }
  open.sort(byDue); overdue.sort(byDue); done.sort(byDue)
  return { open, overdue, done }
})

const totalCount = computed(() => items.value.length)
const doneCount = computed(() => items.value.filter((t) => t.is_completed).length)
const completionPct = computed(() =>
  totalCount.value === 0 ? 0 : Math.round((doneCount.value / totalCount.value) * 100),
)

// ========== 头部日期信息 ==========
const dateObj = computed<Date | null>(() => {
  const m = props.date.match(/^(\d{4})-(\d{2})-(\d{2})$/)
  if (!m) return null
  return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]))
})
const isToday = computed(() => {
  if (!dateObj.value) return false
  const t = new Date()
  return (
    t.getFullYear() === dateObj.value.getFullYear() &&
    t.getMonth() === dateObj.value.getMonth() &&
    t.getDate() === dateObj.value.getDate()
  )
})
const isPast = computed(() => {
  if (!dateObj.value) return false
  const t = new Date()
  const today0 = new Date(t.getFullYear(), t.getMonth(), t.getDate())
  return dateObj.value.getTime() < today0.getTime()
})
const dows = ['日', '一', '二', '三', '四', '五', '六']
const dayHeading = computed(() => {
  const d = dateObj.value
  if (!d) return props.date
  return `${d.getFullYear()} 年 ${d.getMonth() + 1} 月 ${d.getDate()} 日`
})
const dayDow = computed(() => {
  const d = dateObj.value
  return d ? `周${dows[d.getDay()]}` : ''
})

// 取分类颜色
function todoCatColor(t: Todo): string {
  if (!t.list_id) return ''
  const l = data.lists.find((x) => x.id === t.list_id)
  return l?.color || ''
}
function todoCatName(t: Todo): string {
  if (!t.list_id) return '未分类'
  const l = data.lists.find((x) => x.id === t.list_id)
  return l?.name || '未分类'
}

// ========== 数据加载 ==========
async function load() {
  if (!props.date) return
  loading.value = true
  errMsg.value = ''
  try {
    items.value = await todosApi.list({
      due_on: props.date,
      include_done: true,
      order_by: 'due_at_asc',
      limit: 500,
    })
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    loading.value = false
  }
}

watch(() => props.date, load)
onMounted(async () => {
  await data.loadLists()
  await load()
})

// ========== 完成 / 未完成切换 ==========
async function toggleDone(t: Todo, e: Event) {
  e.stopPropagation()
  try {
    const upd = t.is_completed
      ? await todosApi.uncomplete(t.id)
      : await todosApi.complete(t.id)
    const i = items.value.findIndex((x) => x.id === t.id)
    if (i >= 0) items.value[i] = upd
  } catch (err) {
    errMsg.value = err instanceof ApiError ? err.message : (err as Error).message
  }
}

// ========== 删除 ==========
async function remove(t: Todo) {
  if (!online.value) {
    alertDialog({ title: '当前无网络', message: '离线状态下无法删除任务，请在网络恢复后重试。' })
    return
  }
  const ok = await confirmDialog({
    title: '确认删除任务？',
    message: `任务 "${t.title}" 将被永久删除，包括它下面的子任务和提醒规则。此操作无法撤销。`,
    confirmText: '删除',
    cancelText: '取消',
    danger: true,
  })
  if (!ok) return
  try {
    await todosApi.remove(t.id)
    items.value = items.value.filter((x) => x.id !== t.id)
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

// ========== 新建任务对话框 ==========
const showAddDialog = ref(false)
const addTitle = ref('')
const addTimeLocal = ref('') // HH:MM
const addPriority = ref(0)
const addListId = ref<number | null>(null)
const addDurationMinutes = ref<number>(30)
const addDescription = ref('')
const addErr = ref('')
const adding = ref(false)
const DURATION_OPTIONS = [0, 15, 30, 45, 60, 90, 120]

const PRIORITY_OPTIONS = [
  { value: 0, label: '无',   color: 'var(--tg-text-tertiary)' },
  { value: 1, label: '低',   color: 'var(--cat-sky)' },
  { value: 2, label: '中',   color: 'var(--cat-emerald)' },
  { value: 3, label: '高',   color: 'var(--cat-amber)' },
  { value: 4, label: '紧急', color: 'var(--cat-rose)' },
]

function openAdd() {
  if (!online.value) {
    alertDialog({ title: '当前无网络', message: '离线状态下无法新增任务，请在网络恢复后重试。' })
    return
  }
  addTitle.value = ''
  addTimeLocal.value = isToday.value ? defaultTimeForToday() : '09:00'
  addPriority.value = 0
  // 如果当前正在筛选某个分类，新建时自动套用它
  if (filterKey.value === 'none') addListId.value = null
  else if (filterKey.value === 'all') addListId.value = null
  else addListId.value = Number(filterKey.value)
  addDurationMinutes.value = 30
  addDescription.value = ''
  addErr.value = ''
  showAddDialog.value = true
}
function defaultTimeForToday(): string {
  // 今日：默认 23:59 截止；以便快速登记
  const t = new Date()
  // 但若当前时间已晚于 22:00，就给个 +1h 的整点更合理
  if (t.getHours() >= 22) return '23:59'
  return '23:59'
}

function normalizeDurationMinutes(value: number): number {
  return Math.max(0, Math.min(1440, Math.round(Number(value) || 0)))
}

async function submitAdd() {
  addErr.value = ''
  if (!addTitle.value.trim()) {
    addErr.value = '任务标题不能为空'
    return
  }
  if (!dateObj.value) return
  adding.value = true
  try {
    const due = new Date(dateObj.value)
    if (addTimeLocal.value) {
      const [h, m] = addTimeLocal.value.split(':').map(Number)
      due.setHours(h || 0, m || 0, 0, 0)
    } else {
      due.setHours(23, 59, 0, 0)
    }
    // 复用 data store，使其它视图同步
    await data.createTodo({
      title: addTitle.value.trim(),
      description: addDescription.value || undefined,
      priority: addPriority.value,
      duration_minutes: normalizeDurationMinutes(addDurationMinutes.value),
      list_id: addListId.value,
      due_at: toRFC3339(due),
    })
    showAddDialog.value = false
    await load()
  } catch (e) {
    addErr.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    adding.value = false
  }
}

// ESC 关闭对话框 / N 快速新建任务（与 Tasks.vue 一致的快捷键体验）
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && showAddDialog.value) {
    showAddDialog.value = false
    return
  }
  // 在弹窗 / 编辑抽屉打开时不触发
  if (showAddDialog.value || editing.value) return
  // 处于输入控件时不抢键
  const tag = (e.target as HTMLElement)?.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return
  if ((e.target as HTMLElement)?.isContentEditable) return
  if (e.key === 'n' || e.key === 'N') {
    e.preventDefault()
    openAdd()
  }
}
onMounted(() => {
  window.addEventListener('keydown', onKey)
  window.addEventListener('online', _onOnline)
  window.addEventListener('offline', _onOffline)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('online', _onOnline)
  window.removeEventListener('offline', _onOffline)
})

// 编辑抽屉的回调
function onTodoUpdated(t: Todo) {
  const i = items.value.findIndex((x) => x.id === t.id)
  if (i >= 0) items.value[i] = t
  editing.value = null
  // 编辑后日期可能改变，重新拉一次
  load()
}
function onTodoRemoved(id: number) {
  items.value = items.value.filter((x) => x.id !== id)
  editing.value = null
}

// 返回日历
function backToCalendar() {
  router.push({ name: 'calendar' })
}

// 上一日 / 下一日导航
function shiftDay(delta: number) {
  if (!dateObj.value) return
  const d = new Date(dateObj.value)
  d.setDate(d.getDate() + delta)
  const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  router.replace({ name: 'day', params: { date: key } })
}
</script>

<template>
  <div class="day-page">
    <!-- ============ 顶部清晰的"返回日历"链路（独占一行） ============ -->
    <button class="day-back-link" @click="backToCalendar">
      <span class="day-back-icon" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"
             stroke-linecap="round" stroke-linejoin="round">
          <line x1="19" y1="12" x2="5" y2="12"/>
          <polyline points="12 19 5 12 12 5"/>
        </svg>
      </span>
      <span class="day-back-text">返回日历</span>
    </button>

    <!-- ============ 头部：大日期 + 进度条 + 上/下一日 ============ -->
    <header class="day-hero" :class="{ 'is-today': isToday, 'is-past': isPast }">
      <div class="day-hero-row">
        <div class="day-hero-main">
          <div class="day-hero-meta">
            {{ dayDow }}
            <span v-if="isToday" class="day-tag is-today">今天</span>
            <span v-else-if="isPast" class="day-tag is-past">已过</span>
            <span v-else class="day-tag is-future">未来</span>
          </div>
          <div class="day-hero-title">{{ dayHeading }}</div>
        </div>
        <div class="day-hero-actions">
          <button class="btn-icon nav-day" title="前一天" @click="shiftDay(-1)">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="15 18 9 12 15 6"/>
            </svg>
          </button>
          <button class="btn-icon nav-day" title="后一天" @click="shiftDay(1)">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="9 18 15 12 9 6"/>
            </svg>
          </button>
        </div>
      </div>

      <!-- 进度统计 -->
      <div class="day-stats">
        <div class="day-stat">
          <span class="num">{{ totalCount }}</span>
          <span class="lbl">总任务</span>
        </div>
        <div class="day-stat">
          <span class="num">{{ doneCount }}</span>
          <span class="lbl">已完成</span>
        </div>
        <div class="day-stat">
          <span class="num">{{ totalCount - doneCount }}</span>
          <span class="lbl">未完成</span>
        </div>
        <div class="day-progress">
          <div class="day-progress-bar">
            <div class="day-progress-fill" :style="{ width: completionPct + '%' }"></div>
          </div>
          <div class="day-progress-text">{{ completionPct }}%</div>
        </div>
      </div>
    </header>

    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

    <!-- ============ 顶部分类筛选(下拉气泡:避免分类过多撑爆视觉) ============ -->
    <!--
      之前是一行 chip,分类一多就拥挤,即便横向滚动也是噪音。改成"当前所选 + 下拉气泡"
      模式:平时只显示当前筛选项,点开才能浏览全部分类;气泡内部本身仍然是 cat-picker
      风格(全局复用),保持一致感。
    -->
    <div class="day-filter-row">
      <div class="day-filter-trigger-wrap" v-click-outside="() => filterOpen = false">
        <button
          type="button"
          class="day-filter-trigger"
          :class="{ 'is-open': filterOpen }"
          :style="{ '--cat-color': activeFilterColor }"
          @click="filterOpen = !filterOpen"
        >
          <span class="trigger-label muted">分类筛选</span>
          <span class="trigger-current">
            <span class="trigger-dot" :class="activeFilterDotClass" />
            <span class="trigger-text">{{ activeFilterLabel }}</span>
            <span class="trigger-count">{{ activeFilterCount }}</span>
          </span>
          <span class="trigger-arrow" aria-hidden="true">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                 stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="6 9 12 15 18 9"/>
            </svg>
          </span>
        </button>

        <Transition name="filter-pop">
          <div v-if="filterOpen" class="day-filter-pop" role="listbox">
            <div class="day-filter-pop-head">
              <span class="muted">按分类筛选</span>
              <span class="muted day-filter-pop-tip">共 {{ data.lists.length + 2 }} 项</span>
            </div>
            <div class="cat-picker day-filter-pop-list">
              <button
                type="button"
                class="cat-option"
                :class="{ 'is-selected': filterKey === 'all' }"
                :style="{ '--cat-color': 'var(--cat-sky)' }"
                @click="filterKey = 'all'; filterOpen = false"
              >
                <span class="dot" />
                全部
                <span class="cat-option-count">{{ items.length }}</span>
              </button>
              <button
                type="button"
                class="cat-option"
                :class="{ 'is-selected': filterKey === 'none' }"
                :style="{ '--cat-color': 'var(--tg-text-tertiary)' }"
                @click="filterKey = 'none'; filterOpen = false"
              >
                <span class="dot" />
                未分类
                <span class="cat-option-count">{{ items.filter(t => !t.list_id).length }}</span>
              </button>
              <button
                v-for="l in data.lists"
                :key="l.id"
                type="button"
                class="cat-option"
                :class="{ 'is-selected': filterKey === String(l.id) }"
                :style="{ '--cat-color': l.color || 'var(--tg-primary)' }"
                @click="filterKey = String(l.id); filterOpen = false"
              >
                <span class="dot" />
                {{ l.name }}
                <span class="cat-option-count">{{ items.filter(t => t.list_id === l.id).length }}</span>
              </button>
            </div>
          </div>
        </Transition>
      </div>
      <div class="day-filter-actions">
        <button class="btn-primary day-add-btn" @click="openAdd">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>
          </svg>
          新增任务
        </button>
        <span class="day-add-hint muted">按 <kbd>N</kbd> 也可快速新建</span>
      </div>
    </div>

    <!-- ============ 任务列表 ============ -->
    <div v-if="loading" class="muted day-loading">加载中…</div>

    <template v-else>
      <div v-if="filteredItems.length === 0" class="day-empty">
        <div class="empty-emoji">{{ isToday ? '✨' : '🗓' }}</div>
        <div class="empty-title">这一天还没有任务</div>
        <div class="empty-sub muted">点击右上角「新增任务」开始登记</div>
      </div>

      <div v-else class="day-sections">
        <!-- 逾期未完成 -->
        <section v-if="grouped.overdue.length" class="day-section">
          <h3 class="day-section-title is-overdue">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>
            </svg>
            已超时未完成
            <span class="day-section-count">{{ grouped.overdue.length }}</span>
          </h3>
          <ul class="day-list">
            <li
              v-for="t in grouped.overdue"
              :key="t.id"
              class="day-item is-overdue"
              :style="todoCatColor(t) ? { '--cat-color': todoCatColor(t) } : {}"
              @click="editing = t"
            >
              <button
                class="item-check"
                :title="t.is_completed ? '取消完成' : '标记完成'"
                @click="toggleDone(t, $event)"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M5 12l5 5L20 7"/>
                </svg>
              </button>
              <div class="item-body">
                <div class="item-title">{{ t.title }}</div>
                <div class="item-meta">
                  <span v-if="t.due_at" class="meta-time">
                    <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
                    {{ fmtTime(t.due_at) }}
                  </span>
                  <span class="meta-cat-chip" :style="{ '--cat-color': todoCatColor(t) || 'var(--tg-text-tertiary)' }">
                    <span class="chip-dot-inline" />{{ todoCatName(t) }}
                  </span>
                  <span v-if="t.priority > 0" class="meta-prio" :class="`prio-${t.priority}`">
                    {{ PRIORITY_LABELS[t.priority] }}
                  </span>
                  <span v-if="t.duration_minutes > 0" class="meta-duration">
                    {{ fmtDurationMinutes(t.duration_minutes) }}
                  </span>
                </div>
              </div>
              <div class="item-actions">
                <button class="btn-ghost btn-danger" title="删除" @click.stop="remove(t)">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="3 6 5 6 21 6"/>
                    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                  </svg>
                </button>
              </div>
            </li>
          </ul>
        </section>

        <!-- 进行中 / 待办 -->
        <section v-if="grouped.open.length" class="day-section">
          <h3 class="day-section-title">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10"/>
            </svg>
            待办
            <span class="day-section-count">{{ grouped.open.length }}</span>
          </h3>
          <ul class="day-list">
            <li
              v-for="t in grouped.open"
              :key="t.id"
              class="day-item"
              :style="todoCatColor(t) ? { '--cat-color': todoCatColor(t) } : {}"
              @click="editing = t"
            >
              <button
                class="item-check"
                :title="'标记完成'"
                @click="toggleDone(t, $event)"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M5 12l5 5L20 7"/>
                </svg>
              </button>
              <div class="item-body">
                <div class="item-title">{{ t.title }}</div>
                <div class="item-meta">
                  <span v-if="t.due_at" class="meta-time">
                    <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
                    {{ fmtTime(t.due_at) }}
                  </span>
                  <span class="meta-cat-chip" :style="{ '--cat-color': todoCatColor(t) || 'var(--tg-text-tertiary)' }">
                    <span class="chip-dot-inline" />{{ todoCatName(t) }}
                  </span>
                  <span v-if="t.priority > 0" class="meta-prio" :class="`prio-${t.priority}`">
                    {{ PRIORITY_LABELS[t.priority] }}
                  </span>
                  <span v-if="t.duration_minutes > 0" class="meta-duration">
                    {{ fmtDurationMinutes(t.duration_minutes) }}
                  </span>
                </div>
              </div>
              <div class="item-actions">
                <button class="btn-ghost btn-danger" title="删除" @click.stop="remove(t)">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="3 6 5 6 21 6"/>
                    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                  </svg>
                </button>
              </div>
            </li>
          </ul>
        </section>

        <!-- 已完成 -->
        <section v-if="grouped.done.length" class="day-section">
          <h3 class="day-section-title is-done">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="20 6 9 17 4 12"/>
            </svg>
            已完成
            <span class="day-section-count">{{ grouped.done.length }}</span>
          </h3>
          <ul class="day-list">
            <li
              v-for="t in grouped.done"
              :key="t.id"
              class="day-item is-completed"
              :style="todoCatColor(t) ? { '--cat-color': todoCatColor(t) } : {}"
              @click="editing = t"
            >
              <button
                class="item-check is-checked"
                :title="'取消完成'"
                @click="toggleDone(t, $event)"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M5 12l5 5L20 7"/>
                </svg>
              </button>
              <div class="item-body">
                <div class="item-title">{{ t.title }}</div>
                <div class="item-meta">
                  <span v-if="t.due_at" class="meta-time">
                    <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
                    {{ fmtTime(t.due_at) }}
                  </span>
                  <span class="meta-cat-chip" :style="{ '--cat-color': todoCatColor(t) || 'var(--tg-text-tertiary)' }">
                    <span class="chip-dot-inline" />{{ todoCatName(t) }}
                  </span>
                  <span v-if="t.duration_minutes > 0" class="meta-duration">
                    {{ fmtDurationMinutes(t.duration_minutes) }}
                  </span>
                </div>
              </div>
              <div class="item-actions">
                <button class="btn-ghost btn-danger" title="删除" @click.stop="remove(t)">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="3 6 5 6 21 6"/>
                    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                  </svg>
                </button>
              </div>
            </li>
          </ul>
        </section>
      </div>
    </template>

    <!-- ============ 详情/编辑 抽屉 ============ -->
    <Transition name="slide-fade">
      <TodoEditDrawer
        v-if="editing"
        :todo="editing"
        @close="editing = null"
        @updated="onTodoUpdated"
        @removed="onTodoRemoved"
      />
    </Transition>

    <!-- ============ 新建任务 Modal ============ -->
    <Transition name="modal">
      <div v-if="showAddDialog" class="modal-backdrop" @click.self="showAddDialog = false">
        <div class="modal-card add-modal">
          <header class="modal-head">
            <div class="modal-title-wrap">
              <span class="modal-title">新增任务</span>
              <span class="modal-subtitle">{{ dayHeading }} · {{ dayDow }}</span>
            </div>
            <button class="btn-icon" @click="showAddDialog = false" aria-label="关闭">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </header>

          <div class="modal-body">
            <div v-if="addErr" class="auth-error">{{ addErr }}</div>

            <div class="form-field">
              <label>标题 <span class="required">*</span></label>
              <div class="pretty-input-wrap">
                <input
                  v-model="addTitle"
                  class="pretty-input"
                  placeholder="任务名称…"
                  autofocus
                  maxlength="200"
                  @keydown.enter="submitAdd"
                />
                <span class="pretty-input-glow" aria-hidden="true" />
              </div>
            </div>

            <div class="form-field">
              <label>描述（可选）</label>
              <div class="pretty-input-wrap">
                <textarea v-model="addDescription" class="pretty-input pretty-textarea" rows="2" placeholder="补充说明…" />
                <span class="pretty-input-glow" aria-hidden="true" />
              </div>
            </div>

            <div class="form-field">
              <label>分类</label>
              <div class="cat-picker">
                <button
                  type="button"
                  class="cat-option"
                  :class="{ 'is-selected': addListId === null }"
                  :style="{ '--cat-color': 'var(--tg-text-tertiary)' }"
                  @click="addListId = null"
                >
                  <span class="dot" />
                  未分类
                </button>
                <button
                  v-for="l in data.lists"
                  :key="l.id"
                  type="button"
                  class="cat-option"
                  :class="{ 'is-selected': addListId === l.id }"
                  :style="{ '--cat-color': l.color || 'var(--tg-primary)' }"
                  @click="addListId = l.id"
                >
                  <span class="dot" />
                  {{ l.name }}
                </button>
              </div>
            </div>

            <div class="form-field">
              <label>优先级</label>
              <div class="cat-picker">
                <button
                  v-for="p in PRIORITY_OPTIONS"
                  :key="p.value"
                  type="button"
                  class="cat-option"
                  :class="{ 'is-selected': addPriority === p.value }"
                  :style="{ '--cat-color': p.color }"
                  @click="addPriority = p.value"
                >
                  <span class="dot" />
                  {{ p.label }}
                </button>
              </div>
            </div>

            <div class="form-field">
              <label>时间（可选）</label>
              <PrettyTimePicker
                v-model="addTimeLocal"
                placeholder="选择时间，不填则默认 23:59"
                default-time="23:59"
              />
              <div class="form-hint muted">不填则默认为当天 23:59</div>
            </div>

            <div class="form-field">
              <label>
                预计时长
                <span class="duration-summary">{{ fmtDurationMinutes(normalizeDurationMinutes(addDurationMinutes)) }}</span>
              </label>
              <div class="duration-picker">
                <button
                  v-for="m in DURATION_OPTIONS"
                  :key="m"
                  type="button"
                  class="duration-chip"
                  :class="{ 'is-selected': normalizeDurationMinutes(addDurationMinutes) === m }"
                  @click="addDurationMinutes = m"
                >
                  {{ m === 0 ? '不设置' : fmtDurationMinutes(m) }}
                </button>
                <div class="duration-custom pretty-input-wrap">
                  <input
                    v-model.number="addDurationMinutes"
                    class="pretty-input"
                    type="number"
                    min="0"
                    max="1440"
                    step="5"
                    inputmode="numeric"
                    aria-label="自定义预计时长分钟数"
                    @blur="addDurationMinutes = normalizeDurationMinutes(addDurationMinutes)"
                  />
                  <span class="duration-unit">分钟</span>
                  <span class="pretty-input-glow" aria-hidden="true" />
                </div>
              </div>
            </div>
          </div>

          <footer class="modal-foot">
            <button class="btn-secondary" @click="showAddDialog = false">取消</button>
            <button class="btn-primary" :disabled="!addTitle.trim() || adding" @click="submitAdd">
              {{ adding ? '创建中…' : '创建任务' }}
            </button>
          </footer>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.day-page {
  position: relative;
  padding-bottom: 60px;
}

/* ============ 返回日历 链路（独占一行，区别于 "前一天" 按钮） ============
 *
 * 之前的设计把"返回"做成一个仅有"<"图标的圆形按钮，又紧挨"前一天/后一天"两个
 * 同样是"<" / ">" 图标的按钮，几乎所有用户都会把它误认为"再前一天"。这里把"返回"
 * 单独提到 hero 之外、做成"<- 返回日历" 文字链路：
 *   1) 图标改成长箭头 (←)，与"前一天"用的 chevron (<) 视觉上彻底区别开；
 *   2) 增加文字"返回日历"，让目的地非常明确；
 *   3) 横向布局占据一整行，hover 时高亮，是一个常见的面包屑/返回模式；
 *   4) "前一天/后一天"两个按钮单独留在 hero 右上角的导航组里，不再混淆。
 */
.day-back-link {
  display: inline-flex; align-items: center; gap: 8px;
  margin-bottom: 14px;
  padding: 7px 14px 7px 10px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-pill);
  color: var(--tg-text-secondary);
  font-family: 'Sora', sans-serif;
  font-size: 13px; font-weight: 700;
  letter-spacing: -0.005em;
  cursor: pointer;
  box-shadow: var(--tg-shadow-xs);
  transition: all var(--tg-trans-fast);
}
.day-back-link:hover {
  background: color-mix(in srgb, var(--tg-primary) 8%, var(--tg-bg-elev));
  border-color: color-mix(in srgb, var(--tg-primary) 35%, var(--tg-divider));
  color: var(--tg-primary);
  transform: translateX(-2px);
  box-shadow: var(--tg-shadow-sm);
}
.day-back-link:active { transform: translateX(0) scale(0.98); }
.day-back-icon {
  display: inline-flex; align-items: center; justify-content: center;
  width: 22px; height: 22px;
  border-radius: 50%;
  background: var(--tg-grad-brand-soft);
  color: var(--tg-primary);
  transition: transform var(--tg-trans-fast);
}
.day-back-link:hover .day-back-icon {
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
  transform: translateX(-2px);
}
.day-back-icon svg { width: 13px; height: 13px; }
.day-back-text {
  font-variant-numeric: tabular-nums;
}

/* ============ Hero ============ */
.day-hero {
  position: relative;
  padding: 22px 24px 18px;
  margin-bottom: 18px;
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-xl);
  box-shadow: var(--tg-shadow-sm);
  overflow: hidden;
}
.day-hero::before {
  content: ''; position: absolute; inset: 0;
  background:
    radial-gradient(circle at 12% 20%, color-mix(in srgb, var(--tg-primary) 14%, transparent), transparent 55%),
    radial-gradient(circle at 90% 100%, color-mix(in srgb, var(--tg-accent) 12%, transparent), transparent 55%);
  pointer-events: none;
  opacity: 0.7;
}
.day-hero.is-today::before {
  background:
    radial-gradient(circle at 15% 15%, color-mix(in srgb, var(--tg-primary) 22%, transparent), transparent 55%),
    radial-gradient(circle at 85% 100%, color-mix(in srgb, #14b8a6 16%, transparent), transparent 55%);
}
.day-hero.is-past::before {
  background: linear-gradient(180deg, color-mix(in srgb, var(--tg-text) 4%, transparent), transparent);
  opacity: 1;
}

.day-hero-row {
  position: relative; z-index: 1;
  display: flex; align-items: center; gap: 12px;
}
.day-hero-main { flex: 1; min-width: 0; }
.day-hero-meta {
  display: inline-flex; align-items: center; gap: 8px;
  font-family: 'Sora', sans-serif;
  font-size: 12.5px; font-weight: 700;
  color: var(--tg-text-secondary);
  letter-spacing: 0.04em;
}
.day-tag {
  display: inline-flex; align-items: center;
  padding: 2px 9px;
  font-size: 10.5px; font-weight: 800;
  letter-spacing: 0.04em;
  border-radius: 999px;
}
.day-tag.is-today {
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
}
.day-tag.is-past {
  background: var(--tg-hover);
  color: var(--tg-text-tertiary);
}
.day-tag.is-future {
  background: var(--tg-info-soft);
  color: var(--tg-info);
}
.day-hero-title {
  font-family: 'Sora', sans-serif;
  font-size: 26px; font-weight: 800;
  letter-spacing: -0.025em;
  margin-top: 4px;
  color: var(--tg-text);
}
.day-hero.is-today .day-hero-title {
  background: var(--tg-grad-brand);
  -webkit-background-clip: text; background-clip: text;
  color: transparent;
}

.day-hero-actions { display: flex; gap: 4px; }
.nav-day {
  width: 32px; height: 32px;
  border-radius: var(--tg-radius-pill);
}

/* ============ stats / progress ============ */
.day-stats {
  position: relative; z-index: 1;
  margin-top: 16px;
  display: flex; align-items: center; gap: 24px;
  flex-wrap: wrap;
}
.day-stat { display: flex; flex-direction: column; }
.day-stat .num {
  font-family: 'Sora', sans-serif;
  font-size: 22px; font-weight: 800;
  letter-spacing: -0.022em;
  line-height: 1;
  color: var(--tg-text);
}
.day-stat .lbl {
  margin-top: 4px;
  font-size: 11.5px; font-weight: 600;
  color: var(--tg-text-tertiary);
  letter-spacing: 0.04em;
}

.day-progress {
  flex: 1; min-width: 200px;
  display: flex; align-items: center; gap: 12px;
}
.day-progress-bar {
  flex: 1;
  height: 8px;
  background: var(--tg-hover);
  border-radius: 999px;
  overflow: hidden;
}
.day-progress-fill {
  height: 100%;
  background: var(--tg-grad-brand);
  border-radius: inherit;
  transition: width 0.5s cubic-bezier(0.32, 0.72, 0, 1);
}
.day-progress-text {
  font-family: 'Sora', sans-serif;
  font-size: 13px; font-weight: 800;
  color: var(--tg-text-secondary);
  font-variant-numeric: tabular-nums;
  min-width: 38px; text-align: right;
}

/* ============ filter row ============
 *
 * 改造后的方案:
 *   - 平时只显示一个"当前筛选"的下拉触发器,极简且永远占一行;
 *   - 点击展开 popover, popover 内部复用 .cat-picker 样式(保持设计一致);
 *   - "新增任务"按钮永远在右侧,不会被分类塞挤动;
 *   - "全部"专用浅色亮调(青蓝渐变),不再用品牌深色品牌渐变。
 */
.day-filter-row {
  display: flex; align-items: center; gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.day-filter-trigger-wrap {
  position: relative;
  flex: 1; min-width: 0;
  max-width: 360px;
}
.day-filter-trigger {
  --cat-color: var(--tg-primary);
  width: 100%;
  display: flex; align-items: center; gap: 10px;
  padding: 9px 12px 9px 14px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-pill);
  font-family: 'Sora', sans-serif;
  font-size: 13px; font-weight: 600;
  color: var(--tg-text-secondary);
  cursor: pointer;
  transition: all var(--tg-trans-fast);
  text-align: left;
  box-shadow: var(--tg-shadow-xs);
}
.day-filter-trigger:hover {
  border-color: color-mix(in srgb, var(--cat-color) 50%, var(--tg-divider-strong));
  box-shadow: var(--tg-shadow-sm);
  transform: translateY(-1px);
}
.day-filter-trigger.is-open {
  border-color: var(--cat-color);
  background: color-mix(in srgb, var(--cat-color) 6%, var(--tg-bg-elev));
  box-shadow: 0 4px 12px -4px color-mix(in srgb, var(--cat-color) 25%, transparent);
}
.day-filter-trigger .trigger-label {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  flex-shrink: 0;
}
.day-filter-trigger .trigger-current {
  flex: 1; min-width: 0;
  display: inline-flex; align-items: center; gap: 7px;
  color: var(--cat-color);
}
.day-filter-trigger .trigger-text {
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.day-filter-trigger .trigger-dot {
  width: 9px; height: 9px;
  border-radius: 50%;
  background: var(--cat-color);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cat-color) 22%, transparent);
  flex-shrink: 0;
}
/* "全部"使用浅亮色调（青/天蓝渐变），告别原本的深品牌色 */
.day-filter-trigger .trigger-dot.is-all-light {
  background: linear-gradient(135deg, #5eead4 0%, #38bdf8 100%);
  box-shadow: 0 0 0 3px rgba(56, 189, 248, 0.18);
}
.day-filter-trigger .trigger-dot.is-none {
  background: repeating-linear-gradient(
    45deg, var(--tg-text-tertiary),
    var(--tg-text-tertiary) 2px,
    transparent 2px, transparent 4px
  );
}
.day-filter-trigger .trigger-count {
  display: inline-flex; align-items: center; justify-content: center;
  min-width: 22px; height: 20px; padding: 0 8px;
  background: color-mix(in srgb, var(--cat-color) 14%, transparent);
  color: var(--cat-color);
  font-size: 11px; font-weight: 700;
  border-radius: 999px;
  font-variant-numeric: tabular-nums;
  margin-left: auto;
}
.day-filter-trigger .trigger-arrow {
  display: inline-flex;
  color: var(--tg-text-tertiary);
  flex-shrink: 0;
  transition: transform var(--tg-trans-fast), color var(--tg-trans-fast);
}
.day-filter-trigger.is-open .trigger-arrow {
  transform: rotate(180deg);
  color: var(--cat-color);
}

/* 下拉气泡 */
.day-filter-pop {
  position: absolute;
  top: calc(100% + 8px); left: 0;
  z-index: 30;
  width: max(100%, 320px);
  max-width: 480px;
  padding: 12px;
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-lg);
  box-shadow: var(--tg-shadow-lg);
}
.day-filter-pop-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 0 4px 8px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.04em;
  border-bottom: 1px dashed var(--tg-divider);
  margin-bottom: 10px;
}
.day-filter-pop-tip { font-size: 11px; }
.day-filter-pop-list {
  max-height: 280px;
  overflow-y: auto;
  padding-right: 2px;
}
.day-filter-pop-list .cat-option {
  position: relative;
}
.day-filter-pop-list .cat-option .cat-option-count {
  display: inline-flex; align-items: center; justify-content: center;
  min-width: 18px; height: 18px; padding: 0 6px;
  margin-left: 4px;
  background: color-mix(in srgb, var(--cat-color) 14%, transparent);
  color: var(--cat-color);
  font-size: 10.5px; font-weight: 700;
  border-radius: 999px;
  font-variant-numeric: tabular-nums;
}

/* 气泡过渡 */
.filter-pop-enter-active,
.filter-pop-leave-active { transition: all 0.18s cubic-bezier(0.32, 0.72, 0, 1); }
.filter-pop-enter-from,
.filter-pop-leave-to {
  opacity: 0;
  transform: translateY(-6px) scale(0.98);
}

.day-filter-actions {
  display: flex; flex-direction: column; align-items: flex-end; gap: 4px;
  flex-shrink: 0;
}
.day-add-btn {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 9px 16px;
  font-size: 13.5px;
  white-space: nowrap;
}
.day-add-hint {
  font-size: 11.5px;
}
.day-add-hint kbd {
  display: inline-block;
  padding: 1px 6px;
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider-strong);
  border-bottom-width: 2px;
  border-radius: 4px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 10.5px; font-weight: 600;
  color: var(--tg-text-secondary);
  line-height: 1;
}

/* ============ sections / items ============ */
.day-loading { text-align: center; padding: 30px 0; }
.day-empty {
  padding: 50px 24px;
  text-align: center;
  background: var(--tg-bg-elev);
  border: 1px dashed var(--tg-divider-strong);
  border-radius: var(--tg-radius-lg);
}
.empty-emoji { font-size: 36px; }
.empty-title {
  margin-top: 8px;
  font-family: 'Sora', sans-serif;
  font-size: 16px; font-weight: 700;
  color: var(--tg-text);
}
.empty-sub { margin-top: 4px; font-size: 13px; }

.day-sections {
  display: flex; flex-direction: column;
  gap: 22px;
}
.day-section { display: flex; flex-direction: column; gap: 8px; }
.day-section-title {
  display: flex; align-items: center; gap: 8px;
  margin: 0;
  font-family: 'Sora', sans-serif;
  font-size: 13px; font-weight: 700;
  color: var(--tg-text-secondary);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}
.day-section-title.is-overdue { color: var(--tg-text-tertiary); }
.day-section-title.is-done { color: var(--tg-success); }
.day-section-count {
  display: inline-flex; align-items: center; justify-content: center;
  min-width: 22px; height: 20px; padding: 0 7px;
  background: var(--tg-hover);
  color: var(--tg-text-secondary);
  font-size: 11px; font-weight: 700;
  border-radius: 999px;
  font-variant-numeric: tabular-nums;
}

.day-list {
  list-style: none;
  margin: 0; padding: 0;
  display: flex; flex-direction: column;
  gap: 8px;
}
.day-item {
  --cat-color: var(--tg-primary);
  display: flex; align-items: center; gap: 12px;
  padding: 13px 14px;
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-md);
  cursor: pointer;
  position: relative; overflow: hidden;
  transition: border-color var(--tg-trans-fast), box-shadow var(--tg-trans),
              transform var(--tg-trans-fast);
}
.day-item::before {
  content: '';
  position: absolute; left: 0; top: 0; bottom: 0;
  width: 3px;
  background: var(--cat-color);
  transition: width var(--tg-trans-fast);
}
.day-item:hover {
  border-color: color-mix(in srgb, var(--cat-color) 55%, transparent);
  box-shadow: var(--tg-shadow-sm);
  transform: translateY(-1px);
}
.day-item:hover::before { width: 5px; }

/* 完成态 */
.day-item.is-completed {
  background: color-mix(in srgb, var(--cat-color) 4%, var(--tg-bg-elev));
  border-color: color-mix(in srgb, var(--cat-color) 20%, var(--tg-divider));
}
.day-item.is-completed .item-title {
  color: var(--tg-text-tertiary);
  text-decoration: line-through;
  text-decoration-thickness: 1.5px;
  text-decoration-color: var(--tg-text-tertiary);
}

/* 逾期未完成 */
.day-item.is-overdue {
  --cat-color: var(--tg-text-tertiary);
  background: color-mix(in srgb, var(--tg-text-tertiary) 4%, var(--tg-bg-elev));
}
.day-item.is-overdue .item-title { color: var(--tg-text-tertiary); }
.day-item.is-overdue .item-meta { color: var(--tg-text-tertiary); }

.item-check {
  width: 24px; height: 24px;
  display: inline-flex; align-items: center; justify-content: center;
  background: transparent;
  color: transparent;
  border: 2px solid var(--tg-divider-strong);
  border-radius: 50%;
  flex-shrink: 0;
  cursor: pointer;
  transition: border-color var(--tg-trans-fast), background var(--tg-trans-fast),
              color var(--tg-trans-fast), transform var(--tg-trans-fast);
}
.item-check:hover {
  border-color: var(--cat-color);
  transform: scale(1.05);
}
.item-check svg { width: 14px; height: 14px; }
.item-check.is-checked {
  background: var(--cat-color);
  border-color: var(--cat-color);
  color: var(--tg-on-primary);
}
.day-item.is-completed .item-check {
  background: var(--cat-color);
  border-color: var(--cat-color);
  color: var(--tg-on-primary);
}
.day-item.is-overdue .item-check {
  border-color: var(--tg-text-tertiary);
}

.item-body { flex: 1; min-width: 0; }
.item-title {
  font-family: 'Sora', sans-serif;
  font-size: 15px; font-weight: 600;
  color: var(--tg-text);
  letter-spacing: -0.01em;
  word-break: break-word;
  line-height: 1.35;
}
.item-meta {
  display: flex; flex-wrap: wrap; align-items: center;
  gap: 10px;
  margin-top: 5px;
  font-size: 11.5px;
  color: var(--tg-text-tertiary);
  font-weight: 500;
}
.meta-time { display: inline-flex; align-items: center; gap: 4px; font-variant-numeric: tabular-nums; }
.meta-cat-chip {
  --cat-color: var(--tg-text-tertiary);
  display: inline-flex; align-items: center; gap: 5px;
  padding: 1px 8px 1px 6px;
  background: color-mix(in srgb, var(--cat-color) 12%, transparent);
  color: color-mix(in srgb, var(--cat-color) 75%, var(--tg-text));
  border-radius: 999px;
  font-weight: 600;
  font-size: 11px;
}
.chip-dot-inline {
  width: 6px; height: 6px;
  border-radius: 50%;
  background: var(--cat-color);
  flex-shrink: 0;
}
.meta-prio {
  padding: 1px 8px;
  border-radius: 999px;
  font-weight: 700;
  font-size: 10.5px;
}
.meta-prio.prio-1 { background: color-mix(in srgb, var(--cat-sky) 14%, transparent); color: var(--cat-sky); }
.meta-prio.prio-2 { background: color-mix(in srgb, var(--cat-emerald) 14%, transparent); color: var(--cat-emerald); }
.meta-prio.prio-3 { background: color-mix(in srgb, var(--cat-amber) 14%, transparent); color: var(--cat-amber); }
.meta-prio.prio-4 { background: color-mix(in srgb, var(--cat-rose) 14%, transparent); color: var(--cat-rose); }
.meta-duration {
  padding: 1px 8px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--tg-primary) 10%, transparent);
  color: var(--tg-primary);
  font-weight: 700;
  font-size: 10.5px;
}

.item-actions {
  display: flex; gap: 4px;
  opacity: 0;
  transform: translateX(8px);
  transition: opacity var(--tg-trans-fast), transform var(--tg-trans-fast);
}
.day-item:hover .item-actions {
  opacity: 1; transform: translateX(0);
}
.item-actions .btn-ghost {
  width: 30px; height: 30px;
  padding: 0;
  display: inline-flex; align-items: center; justify-content: center;
  border-radius: var(--tg-radius-sm);
}

/* ============ Modal （沿用全局 modal 样式 + 局部覆盖） ============ */
.add-modal { width: min(520px, 95vw); }
.modal-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 18px 22px;
  border-bottom: 1px solid var(--tg-divider);
}
.modal-title-wrap { display: flex; flex-direction: column; gap: 2px; }
.modal-title {
  font-family: 'Sora', sans-serif;
  font-size: 17px; font-weight: 700;
  letter-spacing: -0.018em;
}
.modal-subtitle { font-size: 12.5px; color: var(--tg-text-secondary); font-weight: 500; }
.modal-body {
  padding: 22px;
  display: flex; flex-direction: column; gap: 16px;
  max-height: 70vh; overflow-y: auto;
}
.modal-foot {
  display: flex; gap: 10px; justify-content: flex-end;
  padding: 14px 22px;
  border-top: 1px solid var(--tg-divider);
}
.form-field { display: flex; flex-direction: column; gap: 8px; }
.form-field label {
  font-size: 12px; font-weight: 700;
  color: var(--tg-text-secondary);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}
.form-field .required { color: var(--tg-danger); }
.form-hint { font-size: 11.5px; }

@media (max-width: 600px) {
  .day-hero-title { font-size: 22px; }
  .day-stat .num { font-size: 18px; }
  .day-progress { min-width: 0; }
  .item-actions { opacity: 1; transform: translateX(0); }
  .day-add-btn { width: auto; }
  .day-add-hint { display: none; }
  .day-filter-actions { align-items: stretch; }
  .day-filter-trigger-wrap { max-width: none; flex-basis: 100%; }
  .day-back-link { padding: 6px 12px 6px 8px; font-size: 12.5px; }
}
</style>

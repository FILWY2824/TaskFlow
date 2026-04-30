<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import type { Todo } from '@/types'
import { todos as todosApi, ApiError } from '@/api'
import { fmtTime, isOverdue, PRIORITY_LABELS, toRFC3339 } from '@/utils'
import { useDataStore } from '@/stores/data'
import TodoEditDrawer from '@/components/TodoEditDrawer.vue'

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
  if (!confirm(`确认删除任务 "${t.title}"？`)) return
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
const addDescription = ref('')
const addErr = ref('')
const adding = ref(false)

const PRIORITY_OPTIONS = [
  { value: 0, label: '无',   color: 'var(--tg-text-tertiary)' },
  { value: 1, label: '低',   color: 'var(--cat-sky)' },
  { value: 2, label: '中',   color: 'var(--cat-emerald)' },
  { value: 3, label: '高',   color: 'var(--cat-amber)' },
  { value: 4, label: '紧急', color: 'var(--cat-rose)' },
]

function openAdd() {
  addTitle.value = ''
  addTimeLocal.value = isToday.value ? defaultTimeForToday() : '09:00'
  addPriority.value = 0
  // 如果当前正在筛选某个分类，新建时自动套用它
  if (filterKey.value === 'none') addListId.value = null
  else if (filterKey.value === 'all') addListId.value = null
  else addListId.value = Number(filterKey.value)
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

// ESC 关闭对话框
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && showAddDialog.value) {
    showAddDialog.value = false
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

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
    <!-- ============ 头部：返回 + 大日期 + 进度条 ============ -->
    <header class="day-hero" :class="{ 'is-today': isToday, 'is-past': isPast }">
      <div class="day-hero-row">
        <button class="btn-icon back-btn" @click="backToCalendar" title="返回日历">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="15 18 9 12 15 6"/>
          </svg>
        </button>
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

    <!-- ============ 顶部分类筛选 ============ -->
    <div class="day-filter-row">
      <div class="day-filter-chips">
        <button
          class="filter-chip"
          :class="{ 'is-active': filterKey === 'all' }"
          @click="filterKey = 'all'"
        >
          <span class="chip-dot all-dot" />
          全部
          <span class="chip-count">{{ items.length }}</span>
        </button>
        <button
          class="filter-chip"
          :class="{ 'is-active': filterKey === 'none' }"
          @click="filterKey = 'none'"
        >
          <span class="chip-dot none-dot" />
          未分类
          <span class="chip-count">{{ items.filter(t => !t.list_id).length }}</span>
        </button>
        <button
          v-for="l in data.lists"
          :key="l.id"
          class="filter-chip"
          :class="{ 'is-active': filterKey === String(l.id) }"
          :style="{ '--cat-color': l.color || 'var(--tg-primary)' }"
          @click="filterKey = String(l.id)"
        >
          <span class="chip-dot" />
          {{ l.name }}
          <span class="chip-count">{{ items.filter(t => t.list_id === l.id).length }}</span>
        </button>
      </div>
      <button class="btn-primary day-add-btn" @click="openAdd">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>
        </svg>
        新增任务
      </button>
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
              <div class="pretty-input-wrap">
                <input v-model="addTimeLocal" class="pretty-input" type="time" />
                <span class="pretty-input-glow" aria-hidden="true" />
              </div>
              <div class="form-hint muted">不填则默认为当天 23:59</div>
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
.back-btn {
  width: 36px; height: 36px;
  flex-shrink: 0;
  border-radius: var(--tg-radius-pill);
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

/* ============ filter row ============ */
.day-filter-row {
  display: flex; align-items: center; gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.day-filter-chips {
  flex: 1;
  display: flex; flex-wrap: wrap; gap: 6px;
}
.filter-chip {
  --cat-color: var(--tg-primary);
  display: inline-flex; align-items: center; gap: 6px;
  padding: 7px 12px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: 999px;
  font-size: 13px; font-weight: 600;
  color: var(--tg-text-secondary);
  cursor: pointer;
  transition: all var(--tg-trans-fast);
}
.filter-chip:hover {
  border-color: color-mix(in srgb, var(--cat-color) 50%, transparent);
  color: color-mix(in srgb, var(--cat-color) 75%, var(--tg-text));
  transform: translateY(-1px);
}
.filter-chip.is-active {
  background: color-mix(in srgb, var(--cat-color) 14%, transparent);
  border-color: var(--cat-color);
  color: var(--cat-color);
  box-shadow: 0 4px 12px -4px color-mix(in srgb, var(--cat-color) 30%, transparent);
}
.chip-dot {
  width: 8px; height: 8px;
  border-radius: 50%;
  background: var(--cat-color);
  flex-shrink: 0;
}
.chip-dot.all-dot {
  background: var(--tg-grad-brand);
}
.chip-dot.none-dot {
  background: repeating-linear-gradient(
    45deg, var(--tg-text-tertiary),
    var(--tg-text-tertiary) 2px,
    transparent 2px, transparent 4px
  );
}
.chip-count {
  display: inline-flex; align-items: center; justify-content: center;
  min-width: 18px; height: 18px; padding: 0 6px;
  background: color-mix(in srgb, var(--cat-color) 12%, transparent);
  color: var(--cat-color);
  font-size: 10.5px; font-weight: 700;
  border-radius: 999px;
  font-variant-numeric: tabular-nums;
}
.filter-chip.is-active .chip-count {
  background: var(--cat-color);
  color: var(--tg-on-primary);
}

.day-add-btn {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 9px 16px;
  font-size: 13.5px;
  white-space: nowrap;
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
  .day-add-btn { width: 100%; }
}
</style>

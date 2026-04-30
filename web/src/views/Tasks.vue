<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useDataStore } from '@/stores/data'
import type { Todo, TodoFilterName } from '@/types'
import { fromDatetimeLocal, isOverdue, toRFC3339 } from '@/utils'
import TodoItem from '@/components/TodoItem.vue'
import TodoEditDrawer from '@/components/TodoEditDrawer.vue'
import PrettyDateTimePicker from '@/components/PrettyDateTimePicker.vue'
import { ApiError, reminders as remindersApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { DEFAULT_TIMEZONE } from '@/timezones'
import { confirmDialog } from '@/dialogs'

const props = defineProps<{
  filter?: TodoFilterName
  filterGroup?: 'schedule'
  listId?: number
  titleZh?: string
}>()

const data = useDataStore()
const auth = useAuthStore()
const route = useRoute()

const currentFilter = ref<TodoFilterName>('today')

watch(
  () => props.filterGroup,
  (g) => {
    if (g === 'schedule') currentFilter.value = 'today'
  },
  { immediate: true },
)

const activeFilter = computed<TodoFilterName>(() => {
  if (props.filterGroup === 'schedule') {
    // 「日程·全部」严格只显示"有日期"的任务，与「无日期」视图互斥。
    // 服务端用专门的 'scheduled' 过滤器返回所有 due_at IS NOT NULL 的任务（含已完成）。
    if (currentFilter.value === 'all') return 'scheduled'
    return currentFilter.value
  }
  return props.filter || 'today'
})

// 当前是否处于"无日期"视图（限制：新建时不写入日期、不允许周期）
const isNoDateView = computed(() => activeFilter.value === 'no_date')

const editing = ref<Todo | null>(null)
const errMsg = ref('')

watch(
  [() => activeFilter.value, () => props.listId, () => route.query.q],
  async () => {
    const f = activeFilter.value
    const search = (route.query.q as string) || ''
    try {
      await data.setFilter(f, props.listId, search)
    } catch (e) {
      errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
    }
  },
  { immediate: true },
)

onMounted(async () => {
  await data.loadLists()
})

// 应用顶栏「状态筛选」: all / open / done / expired
//
// 关键修复: 「未完成」与「已过期」必须互斥 ——
//   - open    = 未完成 AND 未过期（也即"还没到点的待办"）
//   - expired = 未完成 AND 已过期（也即"已超时但仍未完成"）
//   - done    = 已完成
//   - all     = 不过滤
// 把已过期错误地塞进未完成里是严重的语义错误：用户在「未完成」中本不该看到那些
// 已经超过截止时间的事项，否则根本没法快速识别"哪些是还来得及做的"。
function passStatus(t: Todo): boolean {
  switch (data.statusFilter) {
    case 'open':    return !t.is_completed && !isOverdue(t)
    case 'done':    return t.is_completed
    case 'expired': return isOverdue(t)
    case 'all':
    default:        return true
  }
}

const filteredTodos = computed(() => {
  let arr = data.todos
  // 双保险: 「日程」分组下的任意 tab（今日 / 明天 / 本周 / 近一周 / 近一个月 / 全部）
  // 都不允许显示 due_at 为空的任务 —— 它们是「无日期」视图的专属。即使后端意外
  // 漏放了一条无日期记录进来,这里也会过滤掉。
  if (props.filterGroup === 'schedule') {
    arr = arr.filter((t) => !!t.due_at)
  }
  // 反向: 「无日期」视图下,绝不能出现任何带日期的任务
  if (activeFilter.value === 'no_date') {
    arr = arr.filter((t) => !t.due_at)
  }
  return arr.filter(passStatus)
})

const groupedTodos = computed(() => {
  const items = filteredTodos.value
  const open = items.filter((t) => !t.is_completed && !isOverdue(t))
  const overdue = items.filter((t) => !t.is_completed && isOverdue(t))
  const done = items.filter((t) => t.is_completed)
  return { open, overdue, done }
})

// =========== 新建任务对话框 ==============
const PRIORITY_OPTIONS = [
  { value: 0, label: '无',   color: 'var(--tg-text-tertiary)' },
  { value: 1, label: '低',   color: 'var(--cat-sky)' },
  { value: 2, label: '中',   color: 'var(--cat-emerald)' },
  { value: 3, label: '高',   color: 'var(--cat-amber)' },
  { value: 4, label: '紧急', color: 'var(--cat-rose)' },
]

// 周期单位 → RRULE FREQ 映射
type RecurUnit = 'DAILY' | 'WEEKLY' | 'MONTHLY' | 'YEARLY'
const RECUR_UNITS: { value: RecurUnit; label: string }[] = [
  { value: 'DAILY',   label: '天' },
  { value: 'WEEKLY',  label: '周' },
  { value: 'MONTHLY', label: '月' },
  { value: 'YEARLY',  label: '年' },
]

const showAddDialog = ref(false)
const addTitle = ref('')
const addDueLocal = ref('')
const addPriority = ref(0)
const addListId = ref<number | null>(null)
const addEffort = ref(0)
const addDescription = ref('')
// 周期相关
const addIsRecurring = ref(false)
const addRecurInterval = ref<number>(1)
const addRecurUnit = ref<RecurUnit>('DAILY')
const adding = ref(false)
const addErr = ref('')

function openAdd() {
  addTitle.value = ''
  addDueLocal.value = ''
  addPriority.value = 0
  addListId.value = props.listId ?? null
  addEffort.value = 0
  addDescription.value = ''
  addIsRecurring.value = false
  addRecurInterval.value = 1
  addRecurUnit.value = 'DAILY'
  addErr.value = ''
  // 「无日期」视图：保持日期为空，且禁用周期；其它视图：根据筛选预填一个合理的截止时间
  if (isNoDateView.value) {
    // 显式置空，防御重复打开后的脏值
    addDueLocal.value = ''
  } else if (activeFilter.value === 'today') {
    const d = new Date()
    d.setHours(23, 59, 0, 0)
    addDueLocal.value = toLocalInputValue(d)
  } else if (activeFilter.value === 'tomorrow') {
    const d = new Date()
    d.setDate(d.getDate() + 1)
    d.setHours(9, 0, 0, 0)
    addDueLocal.value = toLocalInputValue(d)
  } else if (
    activeFilter.value === 'recent_week' ||
    activeFilter.value === 'recent_month' ||
    activeFilter.value === 'this_week'
  ) {
    const d = new Date()
    d.setHours(23, 59, 0, 0)
    addDueLocal.value = toLocalInputValue(d)
  } else if (props.filterGroup === 'schedule') {
    // 日程组的其它分支（如 all），同样默认给一个今天 23:59
    const d = new Date()
    d.setHours(23, 59, 0, 0)
    addDueLocal.value = toLocalInputValue(d)
  }
  showAddDialog.value = true
}

function toLocalInputValue(d: Date): string {
  const y = d.getFullYear()
  const mo = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  const h = String(d.getHours()).padStart(2, '0')
  const m = String(d.getMinutes()).padStart(2, '0')
  return `${y}-${mo}-${dd}T${h}:${m}`
}

// 周期摘要文字（用于预览展示）
const recurSummary = computed(() => {
  if (!addIsRecurring.value) return ''
  const n = Math.max(1, Math.floor(addRecurInterval.value || 1))
  const unitLabel = RECUR_UNITS.find((u) => u.value === addRecurUnit.value)?.label ?? '天'
  return n === 1 ? `每${unitLabel}` : `每 ${n} ${unitLabel}`
})

async function submitAdd() {
  addErr.value = ''
  if (!addTitle.value.trim()) {
    addErr.value = '任务标题不能为空'
    return
  }
  // 「无日期」视图：永远不写入日期 / 不允许周期
  if (isNoDateView.value) {
    addDueLocal.value = ''
    addIsRecurring.value = false
  } else if (props.filterGroup === 'schedule') {
    // 日程视图：日期为必填
    if (!addDueLocal.value) {
      addErr.value = '日程任务必须填写截止日期'
      return
    }
  }
  if (addIsRecurring.value && !addDueLocal.value) {
    addErr.value = '周期日程必须设置一个起始的截止时间'
    return
  }
  if (addIsRecurring.value && (!addRecurInterval.value || addRecurInterval.value < 1)) {
    addErr.value = '周期数需大于等于 1'
    return
  }
  adding.value = true
  try {
    const dueDate = addDueLocal.value ? fromDatetimeLocal(addDueLocal.value) : null
    const todo = await data.createTodo({
      title: addTitle.value.trim(),
      description: addDescription.value || undefined,
      priority: addPriority.value,
      effort: addEffort.value,
      due_at: dueDate ? toRFC3339(dueDate) : null,
      list_id: addListId.value || props.listId || null,
    })
    // 如果是周期日程，再绑定一条 RRULE 提醒。
    if (addIsRecurring.value && dueDate) {
      const interval = Math.max(1, Math.floor(addRecurInterval.value || 1))
      const rrule = `FREQ=${addRecurUnit.value};INTERVAL=${interval}`
      try {
        await remindersApi.create({
          todo_id: todo.id,
          title: todo.title,
          rrule,
          dtstart: toRFC3339(dueDate),
          timezone: auth.user?.timezone || DEFAULT_TIMEZONE,
          channel_local: true,
          channel_telegram: false,
          ringtone: 'default',
          vibrate: true,
          fullscreen: true,
        })
      } catch (e) {
        // 任务已建好，提醒失败仅作提示，不阻塞流程
        console.warn('创建周期提醒失败：', e)
      }
    }
    showAddDialog.value = false
  } catch (e) {
    addErr.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    adding.value = false
  }
}

function open(t: Todo) { editing.value = t }

async function remove(t: Todo) {
  const ok = await confirmDialog({
    title: '确认删除任务？',
    message: `任务 "${t.title}" 将被永久删除，包括它下面的子任务和提醒规则。此操作无法撤销。`,
    confirmText: '删除',
    cancelText: '取消',
    danger: true,
  })
  if (!ok) return
  try { await data.removeTodo(t.id) }
  catch (e) { errMsg.value = e instanceof ApiError ? e.message : (e as Error).message }
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && showAddDialog.value) {
    showAddDialog.value = false
    return
  }
  if (showAddDialog.value || editing.value) return
  const tag = (e.target as HTMLElement)?.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return
  if (e.key === 'n' || e.key === 'N') {
    e.preventDefault()
    openAdd()
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

// 状态筛选标签（用于空态文案）
const statusLabel = computed(() => {
  switch (data.statusFilter) {
    case 'open':    return '未完成'
    case 'done':    return '已完成'
    case 'expired': return '已过期'
    default:        return ''
  }
})
</script>

<template>
  <div class="tasks-page">
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

    <!-- 注：原来当进入某个分类（list/:id）时这里会显示一个 cat-header 卡片；
         按需求统一改为顶栏标题展示分类名，这里不再重复呈现。 -->

    <div v-if="filterGroup === 'schedule'" class="segment-control">
      <button :class="{ active: currentFilter === 'today' }" @click="currentFilter = 'today'">今日</button>
      <button :class="{ active: currentFilter === 'tomorrow' }" @click="currentFilter = 'tomorrow'">明天</button>
      <button :class="{ active: currentFilter === 'this_week' }" @click="currentFilter = 'this_week'">本周</button>
      <button :class="{ active: currentFilter === 'recent_week' }" @click="currentFilter = 'recent_week'">近一周</button>
      <button :class="{ active: currentFilter === 'recent_month' }" @click="currentFilter = 'recent_month'">近一个月</button>
      <button :class="{ active: currentFilter === 'all' }" @click="currentFilter = 'all'">全部</button>
    </div>

    <div v-if="activeFilter !== 'completed'" class="add-bar">
      <button class="btn-primary add-task-btn" @click="openAdd">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>
        </svg>
        新增任务
      </button>
      <span class="add-bar-hint muted">提示：按 <kbd>N</kbd> 也可快速新建</span>
    </div>

    <div v-if="data.todosLoading" class="muted" style="text-align:center;padding:32px 0">加载中…</div>

    <div v-else-if="filteredTodos.length === 0" class="empty">
      <div class="empty-icon">✨</div>
      <div class="empty-title">
        {{ statusLabel ? `没有「${statusLabel}」的任务` : '这里空空如也' }}
      </div>
      <div class="empty-hint">
        <template v-if="statusLabel">试试切换顶栏的状态筛选，或</template>
        点上方"新增任务"，或按 <kbd>N</kbd>
      </div>
    </div>

    <template v-else>
      <!-- 已超时未完成（仅在 statusFilter=all/open 时也合并显示，让用户更早注意） -->
      <div v-if="groupedTodos.overdue.length > 0">
        <div class="section-divider section-divider-warn">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
               stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>
          </svg>
          已超时未完成 · {{ groupedTodos.overdue.length }}
        </div>
        <TransitionGroup name="list" tag="div" class="todo-list">
          <TodoItem
            v-for="t in groupedTodos.overdue"
            :key="t.id"
            :todo="t"
            @open="open"
            @remove="remove"
          />
        </TransitionGroup>
      </div>

      <div v-if="groupedTodos.open.length > 0">
        <div v-if="groupedTodos.overdue.length > 0" class="section-divider">
          待办（未到截止时间） · {{ groupedTodos.open.length }}
        </div>
        <TransitionGroup name="list" tag="div" class="todo-list">
          <TodoItem
            v-for="t in groupedTodos.open"
            :key="t.id"
            :todo="t"
            @open="open"
            @remove="remove"
          />
        </TransitionGroup>
      </div>

      <div v-if="groupedTodos.done.length > 0">
        <div class="section-divider">已完成 · {{ groupedTodos.done.length }}</div>
        <TransitionGroup name="list" tag="div" class="todo-list">
          <TodoItem
            v-for="t in groupedTodos.done"
            :key="t.id"
            :todo="t"
            @open="open"
            @remove="remove"
          />
        </TransitionGroup>
      </div>
    </template>

    <!-- 移动端 FAB -->
    <button v-if="activeFilter !== 'completed'" class="fab" @click="openAdd" aria-label="新增任务">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
        <line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>
      </svg>
    </button>

    <Transition name="slide-fade">
      <TodoEditDrawer
        v-if="editing"
        :todo="editing"
        @close="editing = null"
        @updated="editing = null"
        @removed="editing = null"
      />
    </Transition>

    <!-- 新建任务 Modal -->
    <Transition name="modal">
      <div v-if="showAddDialog" class="modal-backdrop" @click.self="showAddDialog = false">
        <div class="modal-card add-modal">
          <header class="modal-head">
            <span class="modal-title">
              <span class="modal-title-dot" />
              新增任务
            </span>
            <button class="btn-icon" @click="showAddDialog = false" aria-label="关闭">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </header>
          <div class="modal-body">
            <div v-if="addErr" class="auth-error">{{ addErr }}</div>
            <div v-if="isNoDateView" class="no-date-hint">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>
              </svg>
              「无日期」任务专用于"想做但没排期"的事项，与日程视图完全隔离 —— 它不会出现在「日程」里，也无法选择时间。一旦创建后将无法再添加日期；如需安排进日程，请删除后到「日程」中重建。
            </div>

            <!-- ============ 标题：自定义漂亮输入框 ============ -->
            <div class="pretty-field">
              <label class="pretty-field-label">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M12 20h9"/><path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/>
                </svg>
                标题
                <span class="required">*</span>
              </label>
              <div class="pretty-input-wrap">
                <input
                  v-model="addTitle"
                  class="pretty-input"
                  placeholder="给任务起个名字…"
                  autofocus
                  maxlength="200"
                  @keydown.enter="submitAdd"
                />
                <span class="pretty-input-glow" aria-hidden="true" />
              </div>
            </div>

            <!-- ============ 描述：自定义 textarea ============ -->
            <div class="pretty-field">
              <label class="pretty-field-label">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                  <line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/>
                </svg>
                描述
                <span class="optional">可选</span>
              </label>
              <div class="pretty-input-wrap">
                <textarea
                  v-model="addDescription"
                  class="pretty-input pretty-textarea"
                  rows="2"
                  placeholder="补充一些细节…"
                />
                <span class="pretty-input-glow" aria-hidden="true" />
              </div>
            </div>

            <!-- ============ 分类 ============ -->
            <div class="pretty-field">
              <label class="pretty-field-label">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M3 7h7l2 2h9v11a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V8a1 1 0 0 1 1-1z"/>
                </svg>
                分类
              </label>
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
              <div v-if="data.lists.length === 0" class="form-hint muted">
                还没有自定义分类。可在顶栏「分类」按钮中新建。
              </div>
            </div>

            <!-- ============ 优先级 ============ -->
            <div class="pretty-field">
              <label class="pretty-field-label">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M4 15s1-1 4-1 5 2 8 2 4-1 4-1V3s-1 1-4 1-5-2-8-2-4 1-4 1z"/><line x1="4" y1="22" x2="4" y2="15"/>
                </svg>
                优先级
              </label>
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

            <!-- ============ 截止时间（无日期视图下完全隐藏） ============ -->
            <div v-if="!isNoDateView" class="pretty-field">
              <label class="pretty-field-label">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                  <circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>
                </svg>
                截止时间
                <span v-if="filterGroup === 'schedule'" class="required">*</span>
                <span v-else class="optional">可选</span>
              </label>
              <PrettyDateTimePicker
                v-model="addDueLocal"
                :allow-clear="filterGroup !== 'schedule'"
                :placeholder="filterGroup === 'schedule' ? '日程任务必须设置时间' : '点此选择日期与时间'"
              />
              <div v-if="filterGroup === 'schedule'" class="form-hint muted">
                · 日程任务必须设置时间
              </div>
            </div>

            <!-- ============ 周期：一次性 / 周期性（无日期视图下完全隐藏） ============ -->
            <div v-if="!isNoDateView" class="pretty-field">
              <label class="pretty-field-label">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
                </svg>
                重复
              </label>
              <div class="recur-toggle">
                <button
                  type="button"
                  class="recur-toggle-btn"
                  :class="{ 'is-selected': !addIsRecurring }"
                  @click="addIsRecurring = false"
                >
                  <span class="recur-toggle-dot" />
                  一次性
                </button>
                <button
                  type="button"
                  class="recur-toggle-btn"
                  :class="{ 'is-selected': addIsRecurring }"
                  @click="addIsRecurring = true"
                >
                  <span class="recur-toggle-dot is-recur" />
                  周期性
                </button>
              </div>

              <Transition name="recur">
                <div v-if="addIsRecurring" class="recur-row">
                  <span class="recur-label-pre">每</span>
                  <div class="recur-num-wrap">
                    <button
                      type="button"
                      class="recur-num-btn"
                      :disabled="addRecurInterval <= 1"
                      title="减"
                      @click="addRecurInterval = Math.max(1, addRecurInterval - 1)"
                    >
                      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"><line x1="5" y1="12" x2="19" y2="12"/></svg>
                    </button>
                    <input
                      v-model.number="addRecurInterval"
                      class="recur-num-input"
                      type="number"
                      min="1"
                      max="999"
                      inputmode="numeric"
                    />
                    <button
                      type="button"
                      class="recur-num-btn"
                      :disabled="addRecurInterval >= 999"
                      title="加"
                      @click="addRecurInterval = Math.min(999, (addRecurInterval || 0) + 1)"
                    >
                      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
                    </button>
                  </div>
                  <div class="recur-unit-wrap">
                    <button
                      v-for="u in RECUR_UNITS"
                      :key="u.value"
                      type="button"
                      class="recur-unit-btn"
                      :class="{ 'is-selected': addRecurUnit === u.value }"
                      @click="addRecurUnit = u.value"
                    >{{ u.label }}</button>
                  </div>
                </div>
              </Transition>

              <div v-if="addIsRecurring" class="recur-summary">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                  <circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>
                </svg>
                {{ recurSummary }}
                <span v-if="!addDueLocal" class="recur-summary-warn">（请设置截止时间作为起始点）</span>
                <span v-else class="recur-summary-meta">
                  · 起始 {{ addDueLocal.replace('T', ' ') }}
                </span>
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
.tasks-page { position: relative; padding-bottom: 60px; }

.add-bar {
  display: flex; align-items: center; gap: 14px;
  margin-bottom: 18px;
}
.add-task-btn {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 10px 20px;
  font-size: 14px; font-weight: 600;
}
.add-bar-hint { font-size: 12px; }

/* Mobile FAB */
.fab {
  display: none;
  position: fixed; right: 18px; bottom: 18px;
  width: 56px; height: 56px;
  border-radius: 50%;
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
  border: none;
  box-shadow: var(--tg-shadow-lg);
  cursor: pointer;
  align-items: center; justify-content: center;
  z-index: 50;
  transition: transform var(--tg-trans), box-shadow var(--tg-trans);
}
.fab:hover { transform: translateY(-3px) scale(1.04); box-shadow: var(--tg-shadow-lg), var(--tg-shadow-glow); }
.fab:active { transform: translateY(0) scale(1); }

/* ============== Modal head/body/foot ============== */
.add-modal { width: min(560px, 95vw); }
.modal-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 18px 22px;
  border-bottom: 1px solid var(--tg-divider);
  background:
    linear-gradient(135deg,
      color-mix(in srgb, var(--tg-primary) 6%, transparent),
      color-mix(in srgb, var(--tg-accent) 6%, transparent));
}
.modal-title {
  display: inline-flex; align-items: center; gap: 10px;
  font-family: 'Sora', sans-serif;
  font-size: 17px; font-weight: 700; letter-spacing: -0.018em;
}
.modal-title-dot {
  width: 8px; height: 8px;
  border-radius: 50%;
  background: var(--tg-grad-brand);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--tg-primary) 18%, transparent);
}
.modal-body {
  padding: 22px;
  display: flex; flex-direction: column; gap: 18px;
  max-height: 70vh; overflow-y: auto;
}
.modal-foot {
  display: flex; gap: 10px; justify-content: flex-end;
  padding: 14px 22px;
  border-top: 1px solid var(--tg-divider);
}

/* =================================================== */
/* ============== 自定义"漂亮输入框" =================== */
/* =================================================== */
.pretty-field { display: flex; flex-direction: column; gap: 8px; }
.pretty-field-label {
  display: inline-flex; align-items: center; gap: 6px;
  font-size: 12px; font-weight: 700;
  color: var(--tg-text-secondary);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}
.pretty-field-label svg { color: var(--tg-primary); flex-shrink: 0; }
.pretty-field-label .required {
  color: var(--tg-danger); font-weight: 800;
  text-transform: none; letter-spacing: 0;
}
.pretty-field-label .optional {
  margin-left: 4px;
  padding: 1px 8px;
  background: var(--tg-hover);
  color: var(--tg-text-tertiary);
  font-size: 10px; font-weight: 700;
  border-radius: 999px;
  letter-spacing: 0;
  text-transform: none;
}

.pretty-input-wrap {
  position: relative;
  border-radius: var(--tg-radius-md);
  /* 渐变描边：通过两层 background-clip 实现 */
  background:
    linear-gradient(var(--tg-bg-elev), var(--tg-bg-elev)) padding-box,
    linear-gradient(135deg, var(--tg-divider), var(--tg-divider)) border-box;
  border: 1.5px solid transparent;
  transition: transform var(--tg-trans-fast), box-shadow var(--tg-trans-fast),
              background var(--tg-trans-fast);
}
.pretty-input-wrap:hover {
  background:
    linear-gradient(color-mix(in srgb, var(--tg-primary) 2%, var(--tg-bg-elev)),
                    color-mix(in srgb, var(--tg-primary) 2%, var(--tg-bg-elev))) padding-box,
    linear-gradient(135deg,
      color-mix(in srgb, var(--tg-primary) 30%, var(--tg-divider-strong)),
      color-mix(in srgb, var(--tg-accent) 30%, var(--tg-divider-strong))) border-box;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}
.pretty-input-wrap:focus-within {
  background:
    linear-gradient(var(--tg-bg-elev), var(--tg-bg-elev)) padding-box,
    var(--tg-grad-brand) border-box;
  box-shadow:
    0 0 0 4px color-mix(in srgb, var(--tg-primary) 14%, transparent),
    0 6px 18px -6px color-mix(in srgb, var(--tg-primary) 38%, transparent);
  transform: translateY(-1px);
}

.pretty-input-glow {
  pointer-events: none;
  position: absolute; inset: 0;
  border-radius: inherit;
  background: var(--tg-grad-brand-soft);
  opacity: 0;
  transition: opacity var(--tg-trans-fast);
  z-index: 0;
}
.pretty-input-wrap:focus-within .pretty-input-glow { opacity: 1; }

/* 主输入元素 — 透明背景，描边由 wrap 提供 */
.pretty-input {
  position: relative; z-index: 1;
  display: block; width: 100%;
  padding: 13px 16px;
  background: transparent;
  border: none; outline: none;
  color: var(--tg-text);
  font-family: inherit;
  font-size: 14.5px;
  font-weight: 500;
  letter-spacing: -0.005em;
  caret-color: var(--tg-primary);
}
.pretty-input::placeholder {
  color: var(--tg-text-tertiary);
  font-weight: 400;
  transition: color var(--tg-trans-fast), opacity var(--tg-trans-fast);
}
.pretty-input:focus::placeholder { opacity: 0.55; }
.pretty-textarea {
  resize: vertical; min-height: 76px; line-height: 1.55;
}

/* ===== 漂亮的 datetime-local ===== */
.pretty-date-wrap { display: flex; align-items: center; }
.pretty-date-icon {
  position: relative; z-index: 1;
  margin-left: 14px;
  color: var(--tg-primary);
  flex-shrink: 0;
  transition: transform var(--tg-trans-fast);
}
.pretty-date-wrap:focus-within .pretty-date-icon { transform: scale(1.08); }
.pretty-date-input {
  padding-left: 10px;
  font-variant-numeric: tabular-nums;
  font-feature-settings: "tnum";
  letter-spacing: 0.01em;
}
/* 隐藏原生日历图标(我们已用左侧 svg 占位) */
.pretty-date-input::-webkit-calendar-picker-indicator {
  opacity: 0.35;
  cursor: pointer;
  filter: invert(35%) sepia(80%) saturate(2400%) hue-rotate(225deg) brightness(95%);
  transition: opacity var(--tg-trans-fast), transform var(--tg-trans-fast);
}
.pretty-date-input:hover::-webkit-calendar-picker-indicator { opacity: 0.7; }
.pretty-date-input:focus::-webkit-calendar-picker-indicator { opacity: 1; }

/* =================================================== */
/* =============== 周期性日程相关样式 =================== */
/* =================================================== */
.recur-toggle {
  display: inline-flex;
  padding: 4px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-pill);
  box-shadow: var(--tg-shadow-xs);
  align-self: flex-start;
}
.recur-toggle-btn {
  display: inline-flex; align-items: center; gap: 8px;
  padding: 8px 18px;
  background: transparent;
  border: none;
  border-radius: var(--tg-radius-pill);
  font-size: 13px; font-weight: 600;
  color: var(--tg-text-secondary);
  cursor: pointer;
  transition: background var(--tg-trans), color var(--tg-trans),
              transform var(--tg-trans-fast);
}
.recur-toggle-btn:hover { color: var(--tg-text); }
.recur-toggle-btn.is-selected {
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
  box-shadow: var(--tg-shadow-sm);
}
.recur-toggle-dot {
  width: 7px; height: 7px;
  border-radius: 50%;
  background: var(--tg-text-tertiary);
}
.recur-toggle-btn.is-selected .recur-toggle-dot {
  background: rgba(255,255,255,0.95);
  box-shadow: 0 0 0 3px rgba(255,255,255,0.25);
}
.recur-toggle-dot.is-recur {
  background: linear-gradient(135deg, var(--tg-primary), var(--tg-accent));
}

.recur-row {
  display: flex; align-items: center; gap: 10px;
  flex-wrap: wrap;
  padding: 12px 14px;
  background:
    linear-gradient(135deg,
      color-mix(in srgb, var(--tg-primary) 6%, var(--tg-bg-elev)),
      color-mix(in srgb, var(--tg-accent) 6%, var(--tg-bg-elev)));
  border: 1.5px solid color-mix(in srgb, var(--tg-primary) 22%, transparent);
  border-radius: var(--tg-radius-md);
  box-shadow:
    0 1px 2px rgba(15, 23, 42, 0.04),
    inset 0 0 0 1px rgba(255,255,255,0.6);
}
.recur-label-pre {
  font-size: 14px; font-weight: 700;
  color: var(--tg-text-secondary);
}

.recur-num-wrap {
  display: inline-flex; align-items: stretch;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-pill);
  overflow: hidden;
  box-shadow: var(--tg-shadow-xs);
  transition: border-color var(--tg-trans-fast), box-shadow var(--tg-trans-fast);
}
.recur-num-wrap:focus-within {
  border-color: var(--tg-primary);
  box-shadow:
    0 0 0 3px color-mix(in srgb, var(--tg-primary) 14%, transparent);
}
.recur-num-btn {
  display: inline-flex; align-items: center; justify-content: center;
  width: 32px;
  background: transparent;
  border: none;
  color: var(--tg-text-secondary);
  cursor: pointer;
  transition: background var(--tg-trans-fast), color var(--tg-trans-fast);
}
.recur-num-btn:hover:not(:disabled) {
  background: var(--tg-hover);
  color: var(--tg-primary);
}
.recur-num-btn:disabled { opacity: 0.4; cursor: not-allowed; }
.recur-num-input {
  width: 56px;
  padding: 8px 0;
  background: transparent;
  border: none; outline: none;
  text-align: center;
  font-variant-numeric: tabular-nums;
  font-size: 15px; font-weight: 700;
  color: var(--tg-text);
  font-family: 'Sora', sans-serif;
  -moz-appearance: textfield;
}
.recur-num-input::-webkit-outer-spin-button,
.recur-num-input::-webkit-inner-spin-button {
  -webkit-appearance: none; margin: 0;
}

.recur-unit-wrap {
  display: inline-flex; gap: 4px;
  padding: 4px;
  background: var(--tg-bg-elev);
  border: 1.5px solid var(--tg-divider);
  border-radius: var(--tg-radius-pill);
  box-shadow: var(--tg-shadow-xs);
}
.recur-unit-btn {
  padding: 6px 14px;
  background: transparent;
  border: none;
  border-radius: var(--tg-radius-pill);
  font-size: 13px; font-weight: 600;
  color: var(--tg-text-secondary);
  cursor: pointer;
  min-width: 36px;
  transition: background var(--tg-trans-fast), color var(--tg-trans-fast);
}
.recur-unit-btn:hover { color: var(--tg-text); }
.recur-unit-btn.is-selected {
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
  box-shadow: var(--tg-shadow-sm);
}

.recur-summary {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 6px 12px;
  align-self: flex-start;
  background: color-mix(in srgb, var(--tg-primary) 10%, transparent);
  color: var(--tg-primary);
  border: 1px solid color-mix(in srgb, var(--tg-primary) 28%, transparent);
  border-radius: 999px;
  font-size: 12.5px; font-weight: 700;
}
.recur-summary svg { flex-shrink: 0; }
.recur-summary-meta {
  font-weight: 500;
  color: color-mix(in srgb, var(--tg-primary) 70%, var(--tg-text-secondary));
  font-variant-numeric: tabular-nums;
}
.recur-summary-warn {
  font-weight: 600;
  color: var(--tg-warn);
}

/* recur 行展开/收起动画 */
.recur-enter-active, .recur-leave-active {
  transition: opacity var(--tg-trans), transform var(--tg-trans),
              max-height var(--tg-trans);
  overflow: hidden;
}
.recur-enter-from, .recur-leave-to {
  opacity: 0;
  transform: translateY(-4px);
  max-height: 0;
}
.recur-enter-to, .recur-leave-from {
  opacity: 1;
  transform: translateY(0);
  max-height: 200px;
}

/* form-hint 复用 */
.form-hint { font-size: 11.5px; }

/* 「无日期」视图新建任务时的解释横幅 */
.no-date-hint {
  display: flex; align-items: flex-start; gap: 8px;
  padding: 10px 14px;
  background: color-mix(in srgb, var(--tg-primary) 8%, transparent);
  color: var(--tg-primary);
  border: 1.5px solid color-mix(in srgb, var(--tg-primary) 28%, transparent);
  border-radius: var(--tg-radius-md);
  font-size: 12.5px; font-weight: 600; line-height: 1.55;
}
.no-date-hint svg { flex-shrink: 0; margin-top: 2px; }

@media (max-width: 600px) {
  .add-bar-hint { display: none; }
  .fab { display: flex; }
  .recur-row { gap: 8px; padding: 10px 12px; }
  .recur-unit-btn { padding: 5px 10px; min-width: 32px; }
}
</style>

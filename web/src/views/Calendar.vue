<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Todo } from '@/types'
import { todos as todosApi, ApiError } from '@/api'
import { fmtTime, toRFC3339 } from '@/utils'
import TodoEditDrawer from '@/components/TodoEditDrawer.vue'
import { useDataStore } from '@/stores/data'

const data = useDataStore()

// baseDate 表示"当前展示的月份的任意一天"。所有视图相关计算都从它派生。
const baseDate = ref(new Date())
const items = ref<Todo[]>([])
const loading = ref(false)
const errMsg = ref('')
const editing = ref<Todo | null>(null)

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
// 网格行数：4-6 行不等；这里固定生成必要的行数以"刚好覆盖整个月份"。
const cells = computed(() => {
  const first = monthFirst.value
  const dowFirst = (first.getDay() + 6) % 7   // 转换为 周一=0 … 周日=6
  const start = new Date(first)
  start.setDate(first.getDate() - dowFirst)
  // 结尾：月末那天所在周的周日
  const last = monthLast.value
  const dowLast = (last.getDay() + 6) % 7
  const end = new Date(last)
  end.setDate(last.getDate() + (6 - dowLast))
  // 生成日期数组（从 start 到 end，闭区间）
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
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    ;(m[key] ??= []).push(t)
  }
  return m
})

function todayStr(): string {
  const t = new Date()
  return `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`
}
function dayKey(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
function isCurrentMonth(d: Date): boolean {
  return d.getMonth() === baseDate.value.getMonth() && d.getFullYear() === baseDate.value.getFullYear()
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
  await data.loadLists()
  await load()
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

// 新建任务对话框
const showAddDialog = ref(false)
const addTitle = ref('')
const addDate = ref<Date | null>(null)
const addTime = ref('')
const addDuration = ref(30)
const addPriority = ref(0)
const addListId = ref<number | null>(null)
const adding = ref(false)
const addErr = ref('')

const calculatedEndTime = computed(() => {
  if (!addTime.value || !addDate.value || !addDuration.value) return ''
  const [h, m] = addTime.value.split(':').map(Number)
  const d = new Date(addDate.value)
  d.setHours(h, m + addDuration.value, 0, 0)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
})

function quickAdd(d: Date) {
  addDate.value = d
  addTitle.value = ''
  addTime.value = ''
  addDuration.value = 30
  addPriority.value = 0
  addListId.value = null
  addErr.value = ''
  showAddDialog.value = true
}

async function submitAdd() {
  addErr.value = ''
  if (!addTitle.value.trim() || !addDate.value) return
  adding.value = true
  try {
    const due = new Date(addDate.value)
    if (addTime.value) {
      const [h, m] = addTime.value.split(':').map(Number)
      due.setHours(h, m + addDuration.value, 0, 0)
      const start = new Date(addDate.value)
      start.setHours(h, m, 0, 0)
      await todosApi.create({
        title: addTitle.value.trim(),
        priority: addPriority.value,
        list_id: addListId.value,
        start_at: toRFC3339(start),
        due_at: toRFC3339(due),
        effort: addDuration.value >= 60 ? Math.ceil(addDuration.value / 60) : 0,
      })
    } else {
      due.setHours(9, 0, 0, 0)
      await todosApi.create({
        title: addTitle.value.trim(),
        priority: addPriority.value,
        list_id: addListId.value,
        due_at: toRFC3339(due),
      })
    }
    showAddDialog.value = false
    await load()
  } catch (e) {
    addErr.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    adding.value = false
  }
}

// ESC 关闭弹窗
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && showAddDialog.value) showAddDialog.value = false
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

const dows = ['一', '二', '三', '四', '五', '六', '日']
</script>

<template>
  <div class="cal-page">
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

    <div class="calendar-card">
      <div class="cal-header">
        <div class="cal-nav">
          <button class="btn-icon" @click="prev" title="上个月">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
          </button>
          <div class="cal-title">{{ monthLabel }}</div>
          <button class="btn-icon" @click="next" title="下个月">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>
          </button>
        </div>
        <button class="btn-secondary" @click="jumpToday">回到今天</button>
      </div>

      <div class="cal-grid" :style="{ '--row-count': rowCount }">
        <div v-for="d in dows" :key="d" class="cal-dow">周{{ d }}</div>
        <div
          v-for="(d, i) in cells"
          :key="i"
          class="cal-cell"
          :class="{
            today: dayKey(d) === todayStr(),
            'in-month': isCurrentMonth(d),
            'has-events': (todoMap[dayKey(d)] || []).length > 0,
          }"
          @click="quickAdd(d)"
        >
          <div class="cal-num">{{ d.getDate() }}</div>
          <div class="cal-events">
            <div
              v-for="t in (todoMap[dayKey(d)] || []).slice(0, 3)"
              :key="t.id"
              class="cal-ev"
              :class="[`prio-${t.priority}`, { completed: t.is_completed }]"
              :title="t.title"
              @click.stop="editing = t"
            >
              <span v-if="t.due_at && !t.due_all_day" class="ev-time">{{ fmtTime(t.due_at) }}</span>
              <span class="ev-title">{{ t.title }}</span>
            </div>
            <div v-if="(todoMap[dayKey(d)] || []).length > 3" class="cal-more" @click.stop>
              +{{ (todoMap[dayKey(d)] || []).length - 3 }} 更多
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 新建任务对话框 -->
    <Transition name="modal">
      <div v-if="showAddDialog" class="modal-backdrop" @click.self="showAddDialog = false">
        <div class="modal-card add-modal">
          <header class="modal-head">
            <div class="modal-title-wrap">
              <span class="modal-title">新建任务</span>
              <span class="modal-subtitle">{{ addDate ? dayKey(addDate) : '' }}</span>
            </div>
            <button class="btn-icon" @click="showAddDialog = false" aria-label="关闭">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </header>
          <div class="modal-body">
            <div v-if="addErr" class="auth-error">{{ addErr }}</div>
            <div class="form-field">
              <label>标题</label>
              <input v-model="addTitle" placeholder="任务名称…" autofocus @keydown.enter="submitAdd" />
            </div>
            <div class="form-grid">
              <div class="form-field">
                <label>优先级</label>
                <select v-model.number="addPriority">
                  <option :value="0">无</option>
                  <option :value="1">低</option>
                  <option :value="2">中</option>
                  <option :value="3">高</option>
                  <option :value="4">紧急</option>
                </select>
              </div>
              <div class="form-field">
                <label>清单</label>
                <select v-model.number="addListId">
                  <option :value="null">无清单</option>
                  <option v-for="l in data.lists" :key="l.id" :value="l.id">{{ l.name }}</option>
                </select>
              </div>
            </div>
            <div class="form-field">
              <label>开始时间（可选）</label>
              <input v-model="addTime" type="time" />
            </div>
            <div v-if="addTime" class="form-field">
              <label>时长（分钟）</label>
              <input v-model.number="addDuration" type="number" min="5" max="1440" step="5" />
              <div class="muted form-hint" v-if="calculatedEndTime">结束于 {{ calculatedEndTime }}</div>
            </div>
          </div>
          <footer class="modal-foot">
            <button class="btn-secondary" @click="showAddDialog = false">取消</button>
            <button class="btn-primary" :disabled="!addTitle.trim() || adding" @click="submitAdd">
              {{ adding ? '创建中…' : '创建' }}
            </button>
          </footer>
        </div>
      </div>
    </Transition>

    <Transition name="slide-fade">
      <TodoEditDrawer
        v-if="editing"
        :todo="editing"
        @close="editing = null"
        @updated="(t) => { const i = items.findIndex(x => x.id === t.id); if (i >= 0) items[i] = t; editing = null }"
        @removed="(id) => { items = items.filter(x => x.id !== id); editing = null }"
      />
    </Transition>
  </div>
</template>

<style scoped>
/* 整个日历页：高度刚好填满主区，不允许整页滚动。
   页面允许滚动的元素只剩"事件溢出（cal-events）"和弹窗内容。 */
.cal-page {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 110px); /* 减去顶栏 + 内边距，避免主页滚动条 */
  min-height: 460px;
  overflow: hidden;
}

.calendar-card {
  background: var(--tg-side);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-lg);
  padding: 12px 14px 14px;
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  box-shadow: var(--tg-shadow-sm);
}
.cal-header {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 8px;
  flex-shrink: 0;
}
.cal-nav { display: flex; align-items: center; gap: 4px; }
.cal-title {
  font-size: 16px; font-weight: 700; letter-spacing: -0.2px;
  padding: 0 10px; min-width: 130px; text-align: center;
}

/* 真正的网格：第一行是星期标题，剩下 row-count 行平均分配剩余高度。
   关键点：grid-template-rows 用 auto + repeat(--row-count, 1fr)，
   使日历铺满容器、不允许滚动。 */
.cal-grid {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  grid-template-rows: 26px repeat(var(--row-count, 5), 1fr);
  gap: 4px;
  flex: 1;
  min-height: 0;
}
.cal-dow {
  text-align: center;
  padding: 4px 0;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--tg-text-tertiary);
  letter-spacing: 0.3px;
}
.cal-cell {
  position: relative;
  background: var(--tg-side);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-sm);
  padding: 4px 6px;
  display: flex;
  flex-direction: column;
  gap: 3px;
  overflow: hidden;
  cursor: pointer;
  min-height: 0;
  transition: background var(--tg-trans-fast), border-color var(--tg-trans-fast);
}
.cal-cell:not(.in-month) {
  background: transparent;
  border-color: transparent;
}
.cal-cell:not(.in-month) .cal-num { color: var(--tg-text-tertiary); opacity: 0.6; }
.cal-cell:hover { background: var(--tg-hover); border-color: var(--tg-divider-strong); }
.cal-cell.today {
  border-color: var(--tg-primary);
  background: var(--tg-primary-soft);
}
.cal-cell.today .cal-num { color: var(--tg-primary); font-weight: 700; }

.cal-num {
  font-size: 12px;
  font-weight: 600;
  text-align: right;
  padding: 1px 4px;
  align-self: flex-end;
  flex-shrink: 0;
}
.cal-events {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
.cal-ev {
  font-size: 11px;
  background: var(--tg-primary-soft);
  color: var(--tg-primary);
  padding: 2px 5px;
  border-radius: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
  font-weight: 500;
  border-left: 2px solid var(--tg-primary);
  transition: background var(--tg-trans-fast);
  flex-shrink: 0;
}
.cal-ev:hover { background: var(--tg-primary); color: #fff; }
.cal-ev.completed { opacity: 0.45; text-decoration: line-through; }
.cal-ev.prio-3 { border-left-color: #f59e0b; }
.cal-ev.prio-4 { border-left-color: #ef4444; background: var(--tg-danger-soft); color: var(--tg-danger); }
.cal-ev.prio-4:hover { background: var(--tg-danger); color: #fff; }
.cal-ev .ev-time {
  font-weight: 700; margin-right: 3px; font-variant-numeric: tabular-nums;
}
.cal-more {
  font-size: 10.5px;
  color: var(--tg-text-tertiary);
  padding: 0 4px;
  cursor: default;
}

/* ============= Modal 增强样式 ============= */
.add-modal { width: min(440px, 95vw); }
.modal-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--tg-divider);
}
.modal-title-wrap { display: flex; flex-direction: column; gap: 2px; }
.modal-title { font-size: 16px; font-weight: 600; }
.modal-subtitle { font-size: 12.5px; color: var(--tg-text-secondary); font-weight: 400; }
.modal-body {
  padding: 18px 20px;
  display: flex; flex-direction: column;
  gap: 14px;
  max-height: 60vh;
  overflow-y: auto;
}
.modal-foot {
  display: flex; gap: 10px; justify-content: flex-end;
  padding: 14px 20px;
  border-top: 1px solid var(--tg-divider);
  background: var(--tg-bg);
}
.form-field { display: flex; flex-direction: column; gap: 6px; }
.form-field label {
  font-size: 12px;
  font-weight: 600;
  color: var(--tg-primary);
  letter-spacing: 0.2px;
}
.form-field input,
.form-field select {
  padding: 9px 12px;
  border-radius: var(--tg-radius-sm);
  border: 1.5px solid var(--tg-divider);
  background: var(--tg-bg);
  color: var(--tg-text);
  font-size: 14px;
  transition: border-color var(--tg-trans-fast);
}
.form-field input:focus,
.form-field select:focus {
  border-color: var(--tg-primary); outline: none;
}
.form-grid {
  display: grid; grid-template-columns: 1fr 1fr; gap: 14px;
}
.form-hint { font-size: 12px; margin-top: 2px; }

/* Modal transition */
.modal-enter-active, .modal-leave-active { transition: opacity 0.2s; }
.modal-enter-from, .modal-leave-to { opacity: 0; }
.modal-enter-active .modal-card,
.modal-leave-active .modal-card {
  transition: transform 0.22s cubic-bezier(0.4,0,0.2,1), opacity 0.22s;
}
.modal-enter-from .modal-card,
.modal-leave-to .modal-card { transform: translateY(8px) scale(0.97); opacity: 0; }

@media (max-width: 768px) {
  .cal-page { height: calc(100vh - 92px); }
  .cal-ev { font-size: 10px; padding: 1px 4px; }
  .cal-ev .ev-time { display: none; }
  .cal-num { font-size: 11.5px; }
  .form-grid { grid-template-columns: 1fr; }
}
</style>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { Todo } from '@/types'
import { todos as todosApi, ApiError } from '@/api'
import { fmtTime, toRFC3339 } from '@/utils'
import TodoEditDrawer from '@/components/TodoEditDrawer.vue'

const baseDate = ref(new Date())
const items = ref<Todo[]>([])
const loading = ref(false)
const errMsg = ref('')
const editing = ref<Todo | null>(null)

// 35 格固定窗口（5 周），以 baseDate 所在周为中央。
const cells = computed(() => {
  const target = baseDate.value
  const dow = (target.getDay() + 6) % 7
  const start = new Date(target)
  start.setDate(target.getDate() - dow - 14)
  const arr: Date[] = []
  for (let i = 0; i < 35; i++) {
    const d = new Date(start)
    d.setDate(start.getDate() + i)
    arr.push(d)
  }
  return arr
})

const rangeLabel = computed(() => {
  const start = cells.value[0]
  const end = cells.value[cells.value.length - 1]
  if (start.getFullYear() === end.getFullYear() && start.getMonth() === end.getMonth()) {
    return `${start.getFullYear()} 年 ${start.getMonth() + 1} 月`
  } else if (start.getFullYear() === end.getFullYear()) {
    return `${start.getFullYear()} 年 ${start.getMonth() + 1} - ${end.getMonth() + 1} 月`
  }
  return `${start.getFullYear()}年${start.getMonth() + 1}月 - ${end.getFullYear()}年${end.getMonth() + 1}月`
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
  return d.getMonth() === baseDate.value.getMonth()
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

onMounted(load)
watch(baseDate, load)

function prev() {
  const d = new Date(baseDate.value)
  d.setDate(d.getDate() - 28)
  baseDate.value = d
}
function next() {
  const d = new Date(baseDate.value)
  d.setDate(d.getDate() + 28)
  baseDate.value = d
}
function jumpToday() {
  baseDate.value = new Date()
}

const showAddDialog = ref(false)
const addTitle = ref('')
const addDate = ref<Date | null>(null)
const addTime = ref('')
const addDuration = ref(30)

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
  showAddDialog.value = true
}

async function submitAdd() {
  if (!addTitle.value.trim() || !addDate.value) return
  try {
    const due = new Date(addDate.value)
    if (addTime.value) {
      const [h, m] = addTime.value.split(':').map(Number)
      due.setHours(h, m + addDuration.value, 0, 0)
      const start = new Date(addDate.value)
      start.setHours(h, m, 0, 0)
      await todosApi.create({
        title: addTitle.value.trim(),
        start_at: toRFC3339(start),
        due_at: toRFC3339(due),
        effort: addDuration.value >= 60 ? Math.ceil(addDuration.value / 60) : 0,
      })
    } else {
      due.setHours(9, 0, 0, 0)
      await todosApi.create({ title: addTitle.value.trim(), due_at: toRFC3339(due) })
    }
    showAddDialog.value = false
    await load()
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

const dows = ['一', '二', '三', '四', '五', '六', '日']
</script>

<template>
  <div>
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

    <div class="calendar-card">
      <div class="cal-header">
        <div class="cal-nav">
          <button class="btn-icon" @click="prev" title="前 4 周">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
          </button>
          <div class="cal-title">{{ rangeLabel }}</div>
          <button class="btn-icon" @click="next" title="后 4 周">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>
          </button>
        </div>
        <button class="btn-secondary" @click="jumpToday">回到今天</button>
      </div>

      <div class="cal-grid">
        <div v-for="d in dows" :key="d" class="cal-dow">周{{ d }}</div>
        <!-- BUGFIX: 之前外层用 @click.self，点击 .cal-num 会被吃掉。
             现在改为外层无监听，专门用 .cal-empty 占满空白处，
             点击日期数字也能触发 quickAdd。 -->
        <div
          v-for="(d, i) in cells"
          :key="i"
          class="cal-cell"
          :class="{
            today: dayKey(d) === todayStr(),
            'in-month': isCurrentMonth(d),
            'has-events': (todoMap[dayKey(d)] || []).length > 0,
          }"
        >
          <div class="cal-num" @click="quickAdd(d)">{{ d.getDate() }}</div>
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
            <div
              v-if="(todoMap[dayKey(d)] || []).length > 3"
              class="cal-more"
              @click.stop
            >
              +{{ (todoMap[dayKey(d)] || []).length - 3 }} 更多
            </div>
          </div>
          <button class="cal-empty" @click="quickAdd(d)" :title="`在 ${dayKey(d)} 添加`" tabindex="-1"></button>
        </div>
      </div>
    </div>

    <Transition name="fade">
      <div v-if="showAddDialog" class="modal-backdrop" @click.self="showAddDialog = false">
        <div class="modal-card">
          <header style="display:flex;align-items:center;justify-content:space-between;padding:14px 18px;border-bottom:1px solid var(--tg-divider)">
            <span style="font-size:16px;font-weight:600">
              新建任务
              <span class="muted" style="font-weight:400;font-size:13px"> · {{ addDate ? dayKey(addDate) : '' }}</span>
            </span>
            <button class="btn-icon" @click="showAddDialog = false">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </header>
          <div style="padding:18px;display:flex;flex-direction:column;gap:14px">
            <div class="field">
              <label style="font-size:12px;font-weight:600;color:var(--tg-primary);display:block;margin-bottom:6px">标题</label>
              <input v-model="addTitle" placeholder="任务名称…" autofocus @keydown.enter="submitAdd" />
            </div>
            <div class="field">
              <label style="font-size:12px;font-weight:600;color:var(--tg-primary);display:block;margin-bottom:6px">开始时间（可选）</label>
              <input v-model="addTime" type="time" />
            </div>
            <div v-if="addTime" class="field">
              <label style="font-size:12px;font-weight:600;color:var(--tg-primary);display:block;margin-bottom:6px">时长（分钟）</label>
              <input v-model.number="addDuration" type="number" min="5" max="1440" step="5" />
              <div class="muted" style="font-size:12px;margin-top:6px" v-if="calculatedEndTime">
                结束于 {{ calculatedEndTime }}
              </div>
            </div>
          </div>
          <footer style="display:flex;gap:10px;justify-content:flex-end;padding:12px 18px;border-top:1px solid var(--tg-divider)">
            <button class="btn-secondary" @click="showAddDialog = false">取消</button>
            <button class="btn-primary" :disabled="!addTitle.trim()" @click="submitAdd">创建</button>
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
.calendar-card {
  background: var(--tg-side);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-lg);
  padding: 16px 18px;
  overflow: hidden;
}
.cal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
}
.cal-nav {
  display: flex;
  align-items: center;
  gap: 4px;
}
.cal-title {
  font-size: 16px;
  font-weight: 700;
  letter-spacing: -0.2px;
  padding: 0 8px;
  min-width: 130px;
  text-align: center;
}

.cal-grid {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  gap: 2px;
}
.cal-dow {
  text-align: center;
  padding: 6px 0;
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
  height: 96px;
  padding: 4px 6px 4px 6px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  overflow: hidden;
  transition: background var(--tg-trans-fast), border-color var(--tg-trans-fast);
}
.cal-cell:not(.in-month) {
  background: transparent;
  border-color: transparent;
}
.cal-cell:not(.in-month) .cal-num {
  color: var(--tg-text-tertiary);
}
.cal-cell:hover {
  background: var(--tg-hover);
  border-color: var(--tg-divider-strong);
}
.cal-cell.today {
  border-color: var(--tg-primary);
  background: var(--tg-primary-soft);
}
.cal-cell.today .cal-num {
  color: var(--tg-primary);
  font-weight: 700;
}

.cal-num {
  font-size: 12.5px;
  font-weight: 600;
  text-align: right;
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 4px;
  align-self: flex-end;
  z-index: 2;
}
.cal-num:hover {
  background: var(--tg-press);
}

.cal-events {
  display: flex;
  flex-direction: column;
  gap: 1px;
  flex: 1;
  min-height: 0;
  overflow: hidden;
  z-index: 2;
}
.cal-ev {
  font-size: 11px;
  background: var(--tg-primary-soft);
  color: var(--tg-primary);
  padding: 1px 5px;
  border-radius: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
  font-weight: 500;
  border-left: 2px solid var(--tg-primary);
  transition: background var(--tg-trans-fast);
}
.cal-ev:hover { background: var(--tg-primary); color: #fff; }
.cal-ev.completed { opacity: 0.45; text-decoration: line-through; }
.cal-ev.prio-3 { border-left-color: #f59e0b; }
.cal-ev.prio-4 { border-left-color: #ef4444; background: var(--tg-danger-soft); color: var(--tg-danger); }
.cal-ev.prio-4:hover { background: var(--tg-danger); color: #fff; }
.cal-ev .ev-time {
  font-weight: 700;
  margin-right: 3px;
  font-variant-numeric: tabular-nums;
}
.cal-more {
  font-size: 10.5px;
  color: var(--tg-text-tertiary);
  padding: 0 4px;
  cursor: default;
}

/* 占满整个 cell 的透明按钮，点击空白处 = quickAdd；不抢日期数字与事件的点击 */
.cal-empty {
  position: absolute;
  inset: 0;
  background: transparent;
  border: none;
  cursor: pointer;
  z-index: 1;
  border-radius: inherit;
  padding: 0;
}
.cal-empty:focus { outline: none; }

@media (max-width: 768px) {
  .cal-cell { height: 70px; }
  .cal-ev { font-size: 10px; padding: 0 4px; }
  .cal-ev .ev-time { display: none; }
  .cal-num { font-size: 11.5px; }
}
</style>

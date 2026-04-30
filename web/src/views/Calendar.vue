<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { Todo } from '@/types'
import { todos as todosApi, ApiError } from '@/api'
import { fmtTime, toRFC3339 } from '@/utils'
import TodoEditDrawer from '@/components/TodoEditDrawer.vue'

// Base date to center the calendar
const baseDate = ref(new Date())
const items = ref<Todo[]>([])
const loading = ref(false)
const errMsg = ref('')
const editing = ref<Todo | null>(null)

// 35-day window centered on the week of baseDate
const cells = computed(() => {
  const target = baseDate.value
  const dow = (target.getDay() + 6) % 7 // Monday = 0
  const start = new Date(target)
  start.setDate(target.getDate() - dow - 14) // Start 2 weeks before current week
  
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
    return `${start.getFullYear()}年 ${start.getMonth() + 1}月`
  } else if (start.getFullYear() === end.getFullYear()) {
    return `${start.getFullYear()}年 ${start.getMonth() + 1}月 - ${end.getMonth() + 1}月`
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
  d.setDate(d.getDate() - 28) // Move back 4 weeks
  baseDate.value = d
}
function next() {
  const d = new Date(baseDate.value)
  d.setDate(d.getDate() + 28) // Move forward 4 weeks
  baseDate.value = d
}
function jumpToday() {
  baseDate.value = new Date()
}

const showAddDialog = ref(false)
const addTitle = ref('')
const addDate = ref<Date | null>(null)
const addTime = ref('')
const addDuration = ref(30) // Default duration: 30 minutes

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
      due.setHours(h, m + addDuration.value, 0, 0) // due_at is the end time
      // If we want start_at we can set it:
      const start = new Date(addDate.value)
      start.setHours(h, m, 0, 0)
      await todosApi.create({ 
        title: addTitle.value.trim(), 
        start_at: toRFC3339(start),
        due_at: toRFC3339(due),
        effort: addDuration.value >= 60 ? Math.ceil(addDuration.value / 60) : 0 // optional mapping
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
  <div class="calendar-page">
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>
    
    <div class="calendar-card">
      <div class="cal-header">
        <div class="cal-nav">
          <button class="btn-icon" @click="prev">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"></polyline></svg>
          </button>
          <div class="cal-title">{{ rangeLabel }}</div>
          <button class="btn-icon" @click="next">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"></polyline></svg>
          </button>
        </div>
        <button class="btn-today" @click="jumpToday">回到今天</button>
      </div>

      <Transition name="fade" mode="out-in">
        <div class="cal-grid" :key="rangeLabel">
          <div v-for="d in dows" :key="d" class="cal-dow">周{{ d }}</div>
          <div
            v-for="(d, i) in cells"
            :key="i"
            class="cal-cell"
            :class="{ today: dayKey(d) === todayStr() }"
            @click.self="quickAdd(d)"
          >
            <div class="cal-num">{{ d.getDate() }}</div>
            <div class="cal-events">
              <div
                v-for="t in (todoMap[dayKey(d)] || []).slice(0, 4)"
                :key="t.id"
                class="cal-ev"
                :class="{ completed: t.is_completed }"
                :title="t.title"
                @click.stop="editing = t"
              >
                <span class="ev-time" v-if="t.due_at && !t.due_all_day">{{ fmtTime(t.due_at) }}</span>
                <span class="ev-title">{{ t.title }}</span>
              </div>
              <div
                v-if="(todoMap[dayKey(d)] || []).length > 4"
                class="cal-more"
              >
                +{{ (todoMap[dayKey(d)] || []).length - 4 }} 更多
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </div>
    
    <p class="muted" style="margin-top: 16px; font-size: 13px; text-align: center;">
      点击空白日期可快速添加任务，点击任务可进行编辑
    </p>

    <!-- Edit Task Drawer -->
    <Transition name="slide-fade">
      <TodoEditDrawer
        v-if="editing"
        :todo="editing"
        @close="editing = null"
        @updated="editing = null; load()"
        @removed="editing = null; load()"
      />
    </Transition>

    <!-- Beautiful Add Task Modal -->
    <Transition name="modal-fade">
      <div v-if="showAddDialog" class="modal-backdrop" @click.self="showAddDialog = false">
        <div class="modal-card">
          <div class="modal-header">
            <span class="title">新建任务</span>
            <div class="modal-subtitle" v-if="addDate">{{ dayKey(addDate) }}</div>
            <button class="btn-close" @click="showAddDialog = false">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>
            </button>
          </div>
          <div class="modal-body">
            <div class="field">
              <label>任务内容</label>
              <input v-model="addTitle" placeholder="准备做什么？" autofocus @keydown.enter="submitAdd" />
            </div>
            <div class="field-row">
              <div class="field" style="flex: 1;">
                <label>开始时间 (可选)</label>
                <input v-model="addTime" type="time" class="time-input" />
              </div>
              <div class="field" style="flex: 1;" v-if="addTime">
                <label>持续时间 (分钟)</label>
                <div style="display: flex; align-items: center; gap: 8px;">
                  <input v-model.number="addDuration" type="number" min="1" max="1440" style="width: 80px;" />
                  <span v-if="calculatedEndTime" style="font-size: 13px; color: var(--c-text-soft);">
                    至 {{ calculatedEndTime }}
                  </span>
                </div>
              </div>
            </div>
          </div>
          <div class="modal-footer">
            <button class="btn-secondary" @click="showAddDialog = false">取消</button>
            <button class="btn-primary" @click="submitAdd">创建任务</button>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.calendar-page {
  max-width: 1000px;
  margin: 0 auto;
}

.calendar-card {
  background: var(--c-surface);
  border: 1px solid var(--c-border);
  border-radius: var(--radius-xl);
  padding: 24px;
  box-shadow: var(--shadow-md);
}

.cal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24px;
  padding: 0 8px;
}

.cal-nav {
  display: flex;
  align-items: center;
  gap: 16px;
}

.cal-title {
  font-size: 18px;
  font-weight: 700;
  color: var(--c-text);
  min-width: 160px;
  text-align: center;
}

.btn-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: var(--c-surface-2);
  color: var(--c-text-soft);
  transition: all 0.2s ease;
}
.btn-icon:hover {
  background: var(--c-primary-soft);
  color: var(--c-primary);
  transform: scale(1.05);
}

.btn-today {
  padding: 8px 16px;
  font-size: 14px;
  font-weight: 600;
  border-radius: var(--radius-md);
  background: var(--c-surface-2);
  color: var(--c-text);
  transition: all 0.2s ease;
}
.btn-today:hover {
  background: var(--c-border);
}

.cal-grid {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  gap: 10px;
}

.cal-dow {
  text-align: center;
  font-size: 13px;
  font-weight: 700;
  color: var(--c-text-muted);
  padding: 8px 0;
  text-transform: uppercase;
  letter-spacing: 1px;
}

.cal-cell {
  background: var(--c-surface);
  border: 1px solid var(--c-border);
  border-radius: var(--radius-md);
  padding: 8px;
  min-height: 110px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  transition: all 0.2s ease;
}
.cal-cell:hover {
  border-color: var(--c-primary-soft);
  box-shadow: var(--shadow-sm);
  transform: translateY(-2px);
}
.cal-cell.today {
  border-color: var(--c-primary);
  background: var(--c-primary-soft);
}

.cal-num {
  font-size: 14px;
  font-weight: 600;
  color: var(--c-text-soft);
  margin-bottom: 8px;
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
}
.cal-cell.today .cal-num {
  background: var(--c-primary);
  color: white;
}

.cal-events {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.cal-ev {
  display: flex;
  align-items: center;
  gap: 4px;
  background: var(--c-surface-2);
  color: var(--c-text);
  border-radius: 6px;
  padding: 4px 6px;
  font-size: 12px;
  transition: all 0.2s;
}
.cal-ev:hover {
  background: var(--c-border);
}
.cal-ev.completed {
  opacity: 0.6;
  text-decoration: line-through;
  background: transparent;
  border: 1px dashed var(--c-border);
}
.ev-time {
  font-size: 11px;
  color: var(--c-primary);
  font-weight: 600;
}
.ev-title {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-weight: 500;
}
.cal-more {
  font-size: 11px;
  color: var(--c-text-muted);
  text-align: center;
  padding-top: 2px;
  font-weight: 600;
}

/* Beautiful Modal */
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(4px);
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
}
.modal-card {
  background: var(--c-surface);
  border-radius: var(--radius-xl);
  width: min(420px, 92vw);
  box-shadow: var(--shadow-lg);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.modal-header {
  padding: 20px 24px;
  display: flex;
  align-items: center;
  border-bottom: 1px solid var(--c-border);
  position: relative;
}
.modal-header .title {
  font-size: 18px;
  font-weight: 700;
  flex: 1;
}
.modal-subtitle {
  font-size: 14px;
  font-weight: 600;
  color: var(--c-primary);
  background: var(--c-primary-soft);
  padding: 4px 10px;
  border-radius: 12px;
  margin-right: 12px;
}
.modal-body {
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}
.modal-body .field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.field-row {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}
.modal-body label {
  font-size: 13px;
  font-weight: 600;
  color: var(--c-text-soft);
}
.modal-body input {
  padding: 12px 16px;
  font-size: 15px;
  background: var(--c-surface-2);
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  transition: all 0.2s;
}
.modal-body input:focus {
  background: var(--c-surface);
  border-color: var(--c-primary);
  box-shadow: 0 0 0 3px var(--c-primary-soft);
}
.time-input {
  width: 140px !important;
}
.modal-footer {
  padding: 16px 24px;
  background: var(--c-bg);
  border-top: 1px solid var(--c-border);
  display: flex;
  gap: 12px;
  justify-content: flex-end;
}
.modal-footer button {
  padding: 10px 20px;
  font-size: 15px;
  border-radius: var(--radius-md);
}

/* Modal Transition */
.modal-fade-enter-active,
.modal-fade-leave-active {
  transition: all 0.3s cubic-bezier(0.25, 1, 0.5, 1);
}
.modal-fade-enter-from,
.modal-fade-leave-to {
  opacity: 0;
}
.modal-fade-enter-from .modal-card,
.modal-fade-leave-to .modal-card {
  transform: scale(0.95) translateY(10px);
}

@media (max-width: 768px) {
  .calendar-card {
    padding: 16px 12px;
  }
  .cal-grid {
    gap: 6px;
  }
  .cal-cell {
    min-height: 80px;
    padding: 4px;
  }
  .cal-ev {
    padding: 2px 4px;
  }
  .ev-time {
    display: none;
  }
}
</style>

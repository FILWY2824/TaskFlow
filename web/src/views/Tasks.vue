<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useDataStore } from '@/stores/data'
import type { Todo, TodoFilterName } from '@/types'
import { fromDatetimeLocal, toRFC3339 } from '@/utils'
import TodoItem from '@/components/TodoItem.vue'
import TodoEditDrawer from '@/components/TodoEditDrawer.vue'
import { ApiError } from '@/api'

const props = defineProps<{
  filter?: TodoFilterName
  filterGroup?: 'schedule' | 'archive'
  listId?: number
  titleZh?: string
}>()

const data = useDataStore()
const route = useRoute()

const currentFilter = ref<TodoFilterName>('today')

watch(
  () => props.filterGroup,
  (g) => {
    if (g === 'schedule') currentFilter.value = 'today'
    else if (g === 'archive') currentFilter.value = 'completed'
  },
  { immediate: true },
)

const activeFilter = computed<TodoFilterName>(() => {
  if (props.filterGroup) return currentFilter.value
  return props.filter || 'today'
})

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
  // 必须保证 lists 已加载，否则下拉框是空的
  await data.loadLists()
})

const groupedTodos = computed(() => {
  const items = data.todos
  const open = items.filter((t) => !t.is_completed)
  const done = items.filter((t) => t.is_completed)
  return { open, done }
})

// =========== 新建任务对话框 ==============
const showAddDialog = ref(false)
const addTitle = ref('')
const addDueLocal = ref('')
const addPriority = ref(0)
const addListId = ref<number | null>(null)
const addEffort = ref(0)
const addDescription = ref('')
const adding = ref(false)
const addErr = ref('')

function openAdd() {
  addTitle.value = ''
  addDueLocal.value = ''
  addPriority.value = 0
  addListId.value = props.listId ?? null
  addEffort.value = 0
  addDescription.value = ''
  addErr.value = ''
  // 根据当前过滤给一个合理的默认 due_at
  if (activeFilter.value === 'today') {
    const d = new Date()
    d.setHours(23, 59, 0, 0)
    addDueLocal.value = toLocalInputValue(d)
  } else if (activeFilter.value === 'tomorrow') {
    const d = new Date()
    d.setDate(d.getDate() + 1)
    d.setHours(9, 0, 0, 0)
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

async function submitAdd() {
  addErr.value = ''
  if (!addTitle.value.trim()) {
    addErr.value = '任务标题不能为空'
    return
  }
  adding.value = true
  try {
    const dueDate = addDueLocal.value ? fromDatetimeLocal(addDueLocal.value) : null
    await data.createTodo({
      title: addTitle.value.trim(),
      description: addDescription.value || undefined,
      priority: addPriority.value,
      effort: addEffort.value,
      due_at: dueDate ? toRFC3339(dueDate) : null,
      list_id: addListId.value || props.listId || null,
    })
    showAddDialog.value = false
  } catch (e) {
    addErr.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    adding.value = false
  }
}

function open(t: Todo) { editing.value = t }

async function remove(t: Todo) {
  if (!confirm(`确认删除任务 "${t.title}"？`)) return
  try { await data.removeTodo(t.id) }
  catch (e) { errMsg.value = e instanceof ApiError ? e.message : (e as Error).message }
}

// 键盘快捷键：N 打开新建（避免与 input 冲突）
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
</script>

<template>
  <div class="tasks-page">
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

    <div v-if="filterGroup === 'schedule'" class="segment-control">
      <button :class="{ active: currentFilter === 'today' }" @click="currentFilter = 'today'">今日</button>
      <button :class="{ active: currentFilter === 'tomorrow' }" @click="currentFilter = 'tomorrow'">明天</button>
      <button :class="{ active: currentFilter === 'this_week' }" @click="currentFilter = 'this_week'">本周</button>
    </div>
    <div v-if="filterGroup === 'archive'" class="segment-control">
      <button :class="{ active: currentFilter === 'completed' }" @click="currentFilter = 'completed'">已完成</button>
      <button :class="{ active: currentFilter === 'overdue' }" @click="currentFilter = 'overdue'">已过期</button>
    </div>

    <!-- 顶部"新增任务"按钮：明显、固定可见，避免之前 quick-add 表单空标题静默 return 的问题。 -->
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

    <div v-else-if="data.todos.length === 0" class="empty">
      <div class="empty-icon">✨</div>
      <div class="empty-title">这里空空如也</div>
      <div class="empty-hint">点上方"新增任务"，或按 <kbd>N</kbd></div>
    </div>

    <template v-else>
      <TransitionGroup name="list" tag="div" class="todo-list">
        <TodoItem
          v-for="t in groupedTodos.open"
          :key="t.id"
          :todo="t"
          @open="open"
          @remove="remove"
        />
      </TransitionGroup>
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

    <!-- 移动端 FAB：永远在右下角 -->
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
            <span class="modal-title">新增任务</span>
            <button class="btn-icon" @click="showAddDialog = false" aria-label="关闭">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </header>
          <div class="modal-body">
            <div v-if="addErr" class="auth-error">{{ addErr }}</div>
            <div class="form-field">
              <label>标题 <span class="required">*</span></label>
              <input
                v-model="addTitle"
                placeholder="任务名称…"
                autofocus
                maxlength="200"
                @keydown.enter="submitAdd"
              />
            </div>
            <div class="form-field">
              <label>描述（可选）</label>
              <textarea v-model="addDescription" rows="2" placeholder="补充说明…" />
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
              <label>截止时间（可选）</label>
              <input v-model="addDueLocal" type="datetime-local" />
              <div class="form-hint muted">时区跟随账号设置（可在「设置 → 时区」中修改）</div>
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
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}
.add-task-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 9px 18px !important;
  font-size: 14px !important;
  font-weight: 600;
  border-radius: var(--tg-radius-md) !important;
}
.add-task-btn svg { flex-shrink: 0; }
.add-bar-hint { font-size: 12px; }
.add-bar-hint kbd {
  background: var(--tg-hover);
  border: 1px solid var(--tg-divider);
  border-radius: 4px;
  padding: 1px 5px;
  font-size: 11px;
  font-family: ui-monospace, SFMono-Regular, monospace;
  margin: 0 2px;
}

.empty .empty-hint kbd {
  background: var(--tg-hover);
  border: 1px solid var(--tg-divider);
  border-radius: 4px;
  padding: 1px 5px;
  font-size: 11px;
  font-family: ui-monospace, SFMono-Regular, monospace;
}

/* 移动端浮动 + 按钮 */
.fab {
  display: none;
  position: fixed;
  right: 18px;
  bottom: 18px;
  width: 52px; height: 52px;
  border-radius: 50%;
  background: var(--tg-primary);
  color: #fff;
  border: none;
  box-shadow: var(--tg-shadow-lg);
  cursor: pointer;
  align-items: center;
  justify-content: center;
  z-index: 50;
  transition: transform var(--tg-trans-fast), background var(--tg-trans-fast);
}
.fab:hover { background: var(--tg-primary-hover); transform: translateY(-2px); }
.fab:active { transform: translateY(0); }

/* ============ Modal ============ */
.add-modal { width: min(480px, 95vw); }
.modal-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 16px 20px; border-bottom: 1px solid var(--tg-divider);
}
.modal-title { font-size: 16px; font-weight: 600; }
.modal-body {
  padding: 18px 20px; display: flex; flex-direction: column; gap: 14px;
  max-height: 65vh; overflow-y: auto;
}
.modal-foot {
  display: flex; gap: 10px; justify-content: flex-end;
  padding: 14px 20px; border-top: 1px solid var(--tg-divider);
  background: var(--tg-bg);
}
.form-field { display: flex; flex-direction: column; gap: 6px; }
.form-field label {
  font-size: 12px; font-weight: 600;
  color: var(--tg-primary); letter-spacing: 0.2px;
}
.form-field .required { color: var(--tg-danger); }
.form-field input,
.form-field select,
.form-field textarea {
  padding: 9px 12px;
  border-radius: var(--tg-radius-sm);
  border: 1.5px solid var(--tg-divider);
  background: var(--tg-bg);
  color: var(--tg-text);
  font-size: 14px;
  font-family: inherit;
  transition: border-color var(--tg-trans-fast);
  resize: vertical;
}
.form-field input:focus,
.form-field select:focus,
.form-field textarea:focus { border-color: var(--tg-primary); outline: none; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
.form-hint { font-size: 11.5px; margin-top: 2px; }

.modal-enter-active, .modal-leave-active { transition: opacity 0.2s; }
.modal-enter-from, .modal-leave-to { opacity: 0; }
.modal-enter-active .modal-card,
.modal-leave-active .modal-card { transition: transform 0.22s cubic-bezier(0.4,0,0.2,1), opacity 0.22s; }
.modal-enter-from .modal-card,
.modal-leave-to .modal-card { transform: translateY(8px) scale(0.97); opacity: 0; }

@media (max-width: 600px) {
  .form-grid { grid-template-columns: 1fr; }
  .add-bar-hint { display: none; }
  .fab { display: flex; }
}
</style>

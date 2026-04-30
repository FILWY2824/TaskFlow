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
  await data.loadLists()
})

const groupedTodos = computed(() => {
  const items = data.todos
  const open = items.filter((t) => !t.is_completed)
  const done = items.filter((t) => t.is_completed)
  return { open, done }
})

// 当前所在的分类（在 list 路由下）
const currentList = computed(() => {
  if (!props.listId) return null
  return data.lists.find((l) => l.id === props.listId) || null
})

// =========== 新建任务对话框 ==============
const PRIORITY_OPTIONS = [
  { value: 0, label: '无',   color: 'var(--tg-text-tertiary)' },
  { value: 1, label: '低',   color: 'var(--cat-sky)' },
  { value: 2, label: '中',   color: 'var(--cat-emerald)' },
  { value: 3, label: '高',   color: 'var(--cat-amber)' },
  { value: 4, label: '紧急', color: 'var(--cat-rose)' },
]

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

    <!-- 当前分类的"头部条"，让进入某个分类时立刻能看到色彩归属 -->
    <div v-if="currentList" class="cat-header" :style="{ '--cat-color': currentList.color || 'var(--tg-primary)' }">
      <span class="cat-header-dot" />
      <div class="cat-header-info">
        <div class="cat-header-name">{{ currentList.name }}</div>
        <div class="cat-header-meta">{{ data.todos.length }} 个任务</div>
      </div>
    </div>

    <div v-if="filterGroup === 'schedule'" class="segment-control">
      <button :class="{ active: currentFilter === 'today' }" @click="currentFilter = 'today'">今日</button>
      <button :class="{ active: currentFilter === 'tomorrow' }" @click="currentFilter = 'tomorrow'">明天</button>
      <button :class="{ active: currentFilter === 'this_week' }" @click="currentFilter = 'this_week'">本周</button>
    </div>
    <div v-if="filterGroup === 'archive'" class="segment-control">
      <button :class="{ active: currentFilter === 'completed' }" @click="currentFilter = 'completed'">已完成</button>
      <button :class="{ active: currentFilter === 'overdue' }" @click="currentFilter = 'overdue'">已过期</button>
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

            <!-- 视觉化的分类选择器：色块 chip -->
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
              <div v-if="data.lists.length === 0" class="form-hint muted">
                还没有分类。可在左侧栏「我的分类」处新建。
              </div>
            </div>

            <!-- 优先级 chip -->
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

.cat-header {
  display: flex; align-items: center; gap: 14px;
  padding: 14px 18px;
  margin-bottom: 18px;
  background: linear-gradient(135deg,
    color-mix(in srgb, var(--cat-color) 14%, var(--tg-bg-elev)),
    var(--tg-bg-elev));
  border: 1px solid color-mix(in srgb, var(--cat-color) 30%, transparent);
  border-radius: var(--tg-radius-lg);
  box-shadow: var(--tg-shadow-xs);
}
.cat-header-dot {
  width: 14px; height: 14px;
  background: var(--cat-color);
  border-radius: 50%;
  box-shadow: 0 0 0 5px color-mix(in srgb, var(--cat-color) 18%, transparent);
  flex-shrink: 0;
}
.cat-header-info { flex: 1; min-width: 0; }
.cat-header-name {
  font-family: 'Sora', sans-serif;
  font-size: 17px; font-weight: 700;
  letter-spacing: -0.018em;
  color: color-mix(in srgb, var(--cat-color) 70%, var(--tg-text));
}
.cat-header-meta { font-size: 12px; color: var(--tg-text-tertiary); margin-top: 2px; }

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

/* Modal */
.add-modal { width: min(520px, 95vw); }
.modal-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 18px 22px;
  border-bottom: 1px solid var(--tg-divider);
}
.modal-title {
  font-family: 'Sora', sans-serif;
  font-size: 17px; font-weight: 700; letter-spacing: -0.018em;
}
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
.form-hint { font-size: 11.5px; }

@media (max-width: 600px) {
  .add-bar-hint { display: none; }
  .fab { display: flex; }
}
</style>

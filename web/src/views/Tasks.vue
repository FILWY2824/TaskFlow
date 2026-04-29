<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useDataStore } from '@/stores/data'
import type { Todo, TodoFilterName } from '@/types'
import { fromDatetimeLocal, toRFC3339 } from '@/utils'
import TodoItem from '@/components/TodoItem.vue'
import TodoEditDrawer from '@/components/TodoEditDrawer.vue'
import { ApiError } from '@/api'

const props = defineProps<{
  filter?: TodoFilterName
  listId?: number
  titleZh?: string
}>()

const data = useDataStore()
const route = useRoute()

// Quick add 表单
const newTitle = ref('')
const newDueLocal = ref('')
const newPriority = ref(0)
const newListId = ref<number | null>(null)

const editing = ref<Todo | null>(null)
const errMsg = ref('')

watch(
  [() => props.filter, () => props.listId, () => route.query.q],
  async () => {
    const f = (props.filter as TodoFilterName) || 'today'
    const search = (route.query.q as string) || ''
    try {
      await data.setFilter(f, props.listId, search)
    } catch (e) {
      errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
    }
  },
  { immediate: true },
)

watch(
  () => props.listId,
  (l) => {
    if (l) newListId.value = l
  },
  { immediate: true },
)

const groupedTodos = computed(() => {
  const items = data.todos
  const open = items.filter((t) => !t.is_completed)
  const done = items.filter((t) => t.is_completed)
  return { open, done }
})

async function addQuick() {
  errMsg.value = ''
  if (!newTitle.value.trim()) return
  const due = newDueLocal.value ? fromDatetimeLocal(newDueLocal.value) : null

  // 基于当前过滤推断默认 due_at:今日 -> 今天 09:00,明日 -> 明天 09:00
  let inferredDue: string | null = null
  if (!due) {
    if (props.filter === 'today') {
      const d = new Date()
      d.setHours(23, 59, 0, 0)
      inferredDue = toRFC3339(d)
    } else if (props.filter === 'tomorrow') {
      const d = new Date()
      d.setDate(d.getDate() + 1)
      d.setHours(9, 0, 0, 0)
      inferredDue = toRFC3339(d)
    }
  }

  try {
    await data.createTodo({
      title: newTitle.value.trim(),
      priority: newPriority.value,
      due_at: due ? toRFC3339(due) : inferredDue,
      list_id: newListId.value || props.listId || null,
    })
    newTitle.value = ''
    newDueLocal.value = ''
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

function open(t: Todo) {
  editing.value = t
}
async function remove(t: Todo) {
  if (!confirm(`确认删除任务 "${t.title}"?`)) return
  try {
    await data.removeTodo(t.id)
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}
</script>

<template>
  <div>
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

    <form
      v-if="filter !== 'completed'"
      class="quick-add"
      @submit.prevent="addQuick"
    >
      <input
        v-model="newTitle"
        placeholder="+ 快速添加任务,回车保存"
        type="text"
      />
      <div class="qa-meta">
        <select v-model.number="newPriority" title="优先级">
          <option :value="0">无优先级</option>
          <option :value="1">低</option>
          <option :value="2">中</option>
          <option :value="3">高</option>
          <option :value="4">紧急</option>
        </select>
        <input v-model="newDueLocal" type="datetime-local" title="截止时间" />
        <select v-if="!listId" v-model.number="newListId" title="清单">
          <option :value="null">无清单</option>
          <option v-for="l in data.lists" :key="l.id" :value="l.id">{{ l.name }}</option>
        </select>
        <button type="submit" class="btn-primary">添加</button>
      </div>
    </form>

    <div v-if="data.todosLoading" class="muted">加载中…</div>

    <div v-else-if="data.todos.length === 0" class="empty">
      <div style="font-size: 48px">🎉</div>
      <div>这里没有任务了。</div>
    </div>

    <template v-else>
      <div class="todo-list">
        <TodoItem
          v-for="t in groupedTodos.open"
          :key="t.id"
          :todo="t"
          @open="open"
          @remove="remove"
        />
      </div>
      <div v-if="groupedTodos.done.length > 0" style="margin-top: 24px">
        <div class="muted" style="margin-bottom: 8px">已完成({{ groupedTodos.done.length }})</div>
        <div class="todo-list">
          <TodoItem
            v-for="t in groupedTodos.done"
            :key="t.id"
            :todo="t"
            @open="open"
            @remove="remove"
          />
        </div>
      </div>
    </template>

    <TodoEditDrawer
      v-if="editing"
      :todo="editing"
      @close="editing = null"
      @updated="editing = null"
      @removed="editing = null"
    />
  </div>
</template>

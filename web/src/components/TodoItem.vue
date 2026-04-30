<script setup lang="ts">
import { computed } from 'vue'
import type { Todo } from '@/types'
import { fmtRelative, isOverdue, PRIORITY_LABELS } from '@/utils'
import { useDataStore } from '@/stores/data'

const props = defineProps<{ todo: Todo }>()
const emit = defineEmits<{
  (e: 'open', todo: Todo): void
  (e: 'remove', todo: Todo): void
}>()

const data = useDataStore()
const overdue = computed(() => isOverdue(props.todo))

const list = computed(() => {
  const id = props.todo.list_id
  if (!id) return null
  return data.lists.find((l) => l.id === id) || null
})
const listName = computed(() => list.value?.name || '')
const listColor = computed(() => list.value?.color || '')

// 给整个 .todo-item 注入 --cat-color：有分类→分类色；无分类→走 priority-N 默认色
const itemStyle = computed(() => {
  if (listColor.value) return { '--cat-color': listColor.value }
  return {}
})

async function toggleDone(e: Event) {
  e.stopPropagation()
  await data.toggleTodoComplete(props.todo)
}
</script>

<template>
  <div
    class="todo-item"
    :class="[
      `priority-${todo.priority}`,
      { 'is-completed': todo.is_completed, 'is-overdue': overdue },
    ]"
    :style="itemStyle"
    @click="emit('open', todo)"
  >
    <span
      class="check"
      :title="todo.is_completed ? '取消完成' : '完成'"
      :aria-label="todo.is_completed ? '取消完成' : '标记完成'"
      @click="toggleDone"
    >
      <svg class="check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
        <path d="M5 12l5 5L20 7" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </span>

    <div class="todo-body">
      <div class="todo-title">
        <span :title="todo.priority > 0 ? `优先级：${PRIORITY_LABELS[todo.priority]}` : ''">
          {{ todo.title }}
        </span>
      </div>

      <div v-if="todo.due_at || listName || todo.effort > 0" class="todo-meta">
        <span v-if="todo.due_at" class="todo-due">
          <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10"/>
            <polyline points="12 6 12 12 16 14"/>
          </svg>
          {{ fmtRelative(todo.due_at) }}
        </span>

        <span v-if="listName" class="cat-chip" :style="{ '--cat-color': listColor || 'var(--tg-primary)' }">
          <span class="cat-dot" />
          {{ listName }}
        </span>

        <span v-if="todo.effort > 0" :title="`工作量：${todo.effort}`">
          <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M2 20h.01"/><path d="M7 20v-4"/><path d="M12 20v-8"/><path d="M17 20V8"/><path d="M22 4v16"/>
          </svg>
          工作量 {{ todo.effort }}
        </span>
      </div>
    </div>

    <div class="todo-actions">
      <button class="btn-ghost btn-danger" title="删除" @click.stop="emit('remove', todo)">
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="3 6 5 6 21 6"/>
          <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
          <line x1="10" y1="11" x2="10" y2="17"/>
          <line x1="14" y1="11" x2="14" y2="17"/>
        </svg>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Todo } from '@/types'
import { fmtRelative, isOverdue, PRIORITY_COLORS, PRIORITY_LABELS } from '@/utils'
import { useDataStore } from '@/stores/data'

const props = defineProps<{ todo: Todo }>()
const emit = defineEmits<{
  (e: 'open', todo: Todo): void
  (e: 'remove', todo: Todo): void
}>()

const data = useDataStore()
const overdue = computed(() => isOverdue(props.todo))
const listName = computed(() => {
  const id = props.todo.list_id
  if (!id) return ''
  return data.lists.find((l) => l.id === id)?.name || ''
})

async function toggleDone(e: Event) {
  e.stopPropagation()
  await data.toggleTodoComplete(props.todo)
}
</script>

<template>
  <div
    class="todo-item"
    :class="{ 'is-completed': todo.is_completed, 'is-overdue': overdue }"
    @click="emit('open', todo)"
  >
    <span class="check" :title="todo.is_completed ? '取消完成' : '完成'" @click="toggleDone">
      <svg v-if="todo.is_completed" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
        <path d="M5 12l5 5L20 7" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </span>
    <div>
      <div class="todo-title">
        <span
          v-if="todo.priority > 0"
          class="priority-dot"
          :style="{ background: PRIORITY_COLORS[todo.priority] }"
          :title="`优先级:${PRIORITY_LABELS[todo.priority]}`"
        />
        {{ todo.title }}
      </div>
      <div class="todo-meta">
        <span v-if="todo.due_at" class="todo-due">⏱ {{ fmtRelative(todo.due_at) }}</span>
        <span v-if="listName">📂 {{ listName }}</span>
        <span v-if="todo.effort > 0">💪 {{ todo.effort }}</span>
      </div>
    </div>
    <div class="todo-actions">
      <button class="btn-ghost btn-danger" title="删除" @click.stop="emit('remove', todo)">🗑</button>
    </div>
  </div>
</template>

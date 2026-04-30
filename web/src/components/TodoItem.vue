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
      <svg class="check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
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
        <span v-if="todo.due_at" class="todo-due">
          <svg style="width:12px;height:12px;vertical-align:text-bottom;margin-right:2px" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>
          {{ fmtRelative(todo.due_at) }}
        </span>
        <span v-if="listName">
          <svg style="width:12px;height:12px;vertical-align:text-bottom;margin-right:2px" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>
          {{ listName }}
        </span>
        <span v-if="todo.effort > 0">
          <svg style="width:12px;height:12px;vertical-align:text-bottom;margin-right:2px" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 20h.01"></path><path d="M7 20v-4"></path><path d="M12 20v-8"></path><path d="M17 20V8"></path><path d="M22 4v16"></path></svg>
          {{ todo.effort }}
        </span>
      </div>
    </div>
    <div class="todo-actions">
      <button class="btn-ghost btn-danger" title="删除" @click.stop="emit('remove', todo)">
        <svg style="width:16px;height:16px" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path><line x1="10" y1="11" x2="10" y2="17"></line><line x1="14" y1="11" x2="14" y2="17"></line></svg>
      </button>
    </div>
  </div>
</template>

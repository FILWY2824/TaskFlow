import { defineStore } from 'pinia'
import { lists as listsApi, todos as todosApi, subtasks as subtasksApi, reminders as remindersApi } from '@/api'
import type { List, ReminderRule, Subtask, Todo, TodoFilterName, TodoInput } from '@/types'

export const useDataStore = defineStore('data', {
  state: () => ({
    lists: [] as List[],
    listsLoaded: false,

    todos: [] as Todo[],
    todosLoading: false,
    currentFilter: { name: 'today' as TodoFilterName, listId: undefined as number | undefined, search: '' },

    selectedTodoId: null as number | null,
    subtasksByTodo: {} as Record<number, Subtask[]>,
    remindersByTodo: {} as Record<number, ReminderRule[]>,
  }),
  actions: {
    // ------ Lists ------
    async loadLists(force = false) {
      if (this.listsLoaded && !force) return
      this.lists = await listsApi.list()
      this.listsLoaded = true
    },
    async createList(input: Partial<List>) {
      const l = await listsApi.create(input)
      this.lists.push(l)
      return l
    },
    async updateList(id: number, input: Partial<List>) {
      const l = await listsApi.update(id, input)
      const idx = this.lists.findIndex((x) => x.id === id)
      if (idx >= 0) this.lists[idx] = l
      return l
    },
    async removeList(id: number) {
      await listsApi.remove(id)
      this.lists = this.lists.filter((x) => x.id !== id)
    },

    // ------ Todos ------
    async setFilter(name: TodoFilterName | undefined, listId?: number, search?: string) {
      this.currentFilter = {
        name: name || 'all',
        listId,
        search: search || '',
      }
      await this.loadTodos()
    },
    async loadTodos() {
      this.todosLoading = true
      try {
        const f = this.currentFilter
        this.todos = await todosApi.list({
          filter: f.name,
          list_id: f.listId,
          search: f.search || undefined,
          order_by: f.name === 'completed' ? 'created_desc' : 'due_at_asc',
        })
      } finally {
        this.todosLoading = false
      }
    },
    async createTodo(input: TodoInput): Promise<Todo> {
      const t = await todosApi.create(input)
      // 简单地刷新当前列表(避免本地过滤逻辑出错)
      await this.loadTodos()
      return t
    },
    async updateTodo(id: number, input: TodoInput) {
      const t = await todosApi.update(id, input)
      this.replaceTodo(t)
      return t
    },
    async toggleTodoComplete(t: Todo) {
      const updated = t.is_completed
        ? await todosApi.uncomplete(t.id)
        : await todosApi.complete(t.id)
      this.replaceTodo(updated)
      // 在 today/no_date 等过滤下,完成/取消完成会改变是否归属此过滤
      if (this.currentFilter.name !== 'completed' && this.currentFilter.name !== 'all') {
        if (updated.is_completed) {
          this.todos = this.todos.filter((x) => x.id !== updated.id)
        }
      }
    },
    async removeTodo(id: number) {
      await todosApi.remove(id)
      this.todos = this.todos.filter((x) => x.id !== id)
      delete this.subtasksByTodo[id]
      delete this.remindersByTodo[id]
      if (this.selectedTodoId === id) this.selectedTodoId = null
    },
    replaceTodo(t: Todo) {
      const idx = this.todos.findIndex((x) => x.id === t.id)
      if (idx >= 0) this.todos[idx] = t
    },

    // ------ Subtasks ------
    async loadSubtasks(todoId: number) {
      const items = await subtasksApi.list(todoId)
      this.subtasksByTodo[todoId] = items
    },
    async addSubtask(todoId: number, title: string) {
      const s = await subtasksApi.create(todoId, { title })
      const arr = this.subtasksByTodo[todoId] || []
      arr.push(s)
      this.subtasksByTodo[todoId] = arr
    },
    async toggleSubtask(s: Subtask) {
      const updated = s.is_completed
        ? await subtasksApi.uncomplete(s.id)
        : await subtasksApi.complete(s.id)
      const arr = this.subtasksByTodo[s.todo_id] || []
      const i = arr.findIndex((x) => x.id === s.id)
      if (i >= 0) arr[i] = updated
    },
    async updateSubtask(s: Subtask, title: string) {
      const updated = await subtasksApi.update(s.id, { title })
      const arr = this.subtasksByTodo[s.todo_id] || []
      const i = arr.findIndex((x) => x.id === s.id)
      if (i >= 0) arr[i] = updated
    },
    async removeSubtask(s: Subtask) {
      await subtasksApi.remove(s.id)
      const arr = this.subtasksByTodo[s.todo_id] || []
      this.subtasksByTodo[s.todo_id] = arr.filter((x) => x.id !== s.id)
    },

    // ------ Reminders ------
    async loadReminders(todoId: number) {
      const items = await remindersApi.list({ todo_id: todoId })
      this.remindersByTodo[todoId] = items
    },
    async loadAllReminders() {
      const items = await remindersApi.list({})
      const grouped: Record<number, ReminderRule[]> = {}
      for (const r of items) {
        if (r.todo_id) {
          (grouped[r.todo_id] ??= []).push(r)
        }
      }
      this.remindersByTodo = grouped
      return items
    },
  },
})

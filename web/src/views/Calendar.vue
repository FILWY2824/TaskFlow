<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { Todo } from '@/types'
import { todos as todosApi, ApiError } from '@/api'
import { fmtTime, toRFC3339 } from '@/utils'
import TodoEditDrawer from '@/components/TodoEditDrawer.vue'

const cur = ref(new Date(new Date().getFullYear(), new Date().getMonth(), 1))
const items = ref<Todo[]>([])
const loading = ref(false)
const errMsg = ref('')
const editing = ref<Todo | null>(null)

const monthLabel = computed(() => `${cur.value.getFullYear()}年 ${cur.value.getMonth() + 1}月`)

// 先把第一天补到周一(周一为周起点),共 6 行 * 7 列 = 42 格
const cells = computed(() => {
  const first = cur.value
  const dow = (first.getDay() + 6) % 7 // 周一=0
  const start = new Date(first)
  start.setDate(first.getDate() - dow)
  const arr: Date[] = []
  for (let i = 0; i < 42; i++) {
    const d = new Date(start)
    d.setDate(start.getDate() + i)
    arr.push(d)
  }
  return arr
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
    // 后端没有暴露 due_after/due_before 给 GET /api/todos,
    // 这里用 include_done + 大 limit 拉一个月份内"全部任务",前端再过滤。
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
watch(cur, load)

function prev() {
  cur.value = new Date(cur.value.getFullYear(), cur.value.getMonth() - 1, 1)
}
function next() {
  cur.value = new Date(cur.value.getFullYear(), cur.value.getMonth() + 1, 1)
}
function jumpToday() {
  const now = new Date()
  cur.value = new Date(now.getFullYear(), now.getMonth(), 1)
}

async function quickAdd(d: Date) {
  const title = prompt(`在 ${dayKey(d)} 添加任务:`)
  if (!title || !title.trim()) return
  try {
    const due = new Date(d)
    due.setHours(9, 0, 0, 0)
    await todosApi.create({ title: title.trim(), due_at: toRFC3339(due) })
    await load()
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

function isOutside(d: Date): boolean {
  return d.getMonth() !== cur.value.getMonth()
}

const dows = ['一', '二', '三', '四', '五', '六', '日']
</script>

<template>
  <div>
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>
    <div class="calendar">
      <div class="cal-header">
        <div class="row-flex">
          <button class="btn-secondary" @click="prev">‹</button>
          <strong>{{ monthLabel }}</strong>
          <button class="btn-secondary" @click="next">›</button>
        </div>
        <button class="btn-secondary" @click="jumpToday">回到今天</button>
      </div>
      <div class="cal-grid">
        <div v-for="d in dows" :key="d" class="cal-dow">周{{ d }}</div>
        <div
          v-for="(d, i) in cells"
          :key="i"
          class="cal-cell"
          :class="{ outside: isOutside(d), today: dayKey(d) === todayStr() }"
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
              {{ fmtTime(t.due_at!) }} {{ t.title }}
            </div>
            <div
              v-if="(todoMap[dayKey(d)] || []).length > 4"
              class="muted"
              style="font-size: 11px"
            >
              +{{ (todoMap[dayKey(d)] || []).length - 4 }} 更多
            </div>
          </div>
        </div>
      </div>
    </div>
    <p class="muted" style="margin-top: 12px; font-size: 12px">
      点击空白单元格可快速添加任务。点击事件可编辑。
    </p>

    <TodoEditDrawer
      v-if="editing"
      :todo="editing"
      @close="editing = null"
      @updated="editing = null; load()"
      @removed="editing = null; load()"
    />
  </div>
</template>

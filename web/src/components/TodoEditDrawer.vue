<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ReminderRule, Subtask, Todo } from '@/types'
import { useDataStore } from '@/stores/data'
import { useAuthStore } from '@/stores/auth'
import { reminders as remindersApi, ApiError } from '@/api'
import { DEFAULT_TIMEZONE } from '@/timezones'
import {
  fmtDateTime,
  fmtRelative,
  fromDatetimeLocal,
  PRIORITY_LABELS,
  toDatetimeLocal,
  toRFC3339,
} from '@/utils'
import PrettyDateTimePicker from '@/components/PrettyDateTimePicker.vue'
import { confirmDialog } from '@/dialogs'

const props = defineProps<{
  todo: Todo
}>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'updated', todo: Todo): void
  (e: 'removed', id: number): void
}>()

const data = useDataStore()
const authStore = useAuthStore()

// 表单字段
const title = ref(props.todo.title)
const description = ref(props.todo.description)
const listId = ref<number | null>(props.todo.list_id ?? null)
const priority = ref(props.todo.priority)
const effort = ref(props.todo.effort)
const dueAtLocal = ref(toDatetimeLocal(props.todo.due_at ? new Date(props.todo.due_at) : null))
const dueAllDay = ref(props.todo.due_all_day)
// 时区永远跟随当前账号设置（在"设置 → 时区"里统一管理）；这里只读不可编辑。
const tz = ref(authStore.user?.timezone || props.todo.timezone || DEFAULT_TIMEZONE)

// ─── 「无日期」 vs「日程任务」 互不串通的强约束 ────────────────────────────────
// 业务规则:
//   - 一旦任务被创建为"无日期"（due_at 为空），它在编辑时就不能再被加上日期 ——
//     这种任务专门用来登记"想做但还没排期"的事项，不该跑到日程里搅局；
//   - 反过来，一旦任务被创建为"有日期"（即在日程视图里的任务），它在编辑时
//     也不能被把日期清空 —— 不能让它意外地"跌出"日程进入无日期视图。
// 也就是说："是否有日期"是任务的一个不可变属性。需要切换的话，请删除并重建。
//
// 实现上：
//   - originallyHadDueAt 在抽屉首次打开和切换 todo 时刻"快照"任务原始状态；
//   - 模板里根据它分别渲染"已锁定的无日期标识"或"已锁定的日期时间选择器"；
//   - PrettyDateTimePicker 在前一种情况下完全不渲染；后一种情况下传 allow-clear=false。
const originallyHadDueAt = ref<boolean>(!!props.todo.due_at)
const isNoDateTask = computed(() => !originallyHadDueAt.value)

const errMsg = ref('')
const saving = ref(false)

const subtasks = computed<Subtask[]>(() => data.subtasksByTodo[props.todo.id] || [])
const reminders = computed<ReminderRule[]>(() => data.remindersByTodo[props.todo.id] || [])

// 新子任务输入
const newSubtaskTitle = ref('')

// 新提醒表单
const showReminderDialog = ref(false)
const remTitle = ref('')
const remTriggerLocal = ref('')
// BUGFIX: 此前 select 和 input 都 v-model 到 remRRule，互相覆盖。改成
// remRRulePreset (select) + remRRuleCustom (input)，最终值在提交时合成。
const remRRulePreset = ref('')   // 预设：'' 不重复 / 'FREQ=DAILY' 等
const remRRuleCustom = ref('')   // 自定义自由文本（优先级高于预设）
const remDtstartLocal = ref('')
const remChannelLocal = ref(true)
const remChannelTelegram = ref(false)
const remRingtone = ref('default')
const remVibrate = ref(true)
const remFullscreen = ref(true)
const remErr = ref('')

// 切换 todo 时重新加载详情
watch(
  () => props.todo.id,
  async () => {
    title.value = props.todo.title
    description.value = props.todo.description
    listId.value = props.todo.list_id ?? null
    priority.value = props.todo.priority
    effort.value = props.todo.effort
    dueAtLocal.value = toDatetimeLocal(props.todo.due_at ? new Date(props.todo.due_at) : null)
    dueAllDay.value = props.todo.due_all_day
    tz.value = authStore.user?.timezone || props.todo.timezone || DEFAULT_TIMEZONE
    // 重新拍摄"原始是否有日期"的快照（这是任务的不可变属性）
    originallyHadDueAt.value = !!props.todo.due_at
    errMsg.value = ''
    await Promise.all([data.loadSubtasks(props.todo.id), data.loadReminders(props.todo.id)]).catch(() => {})
  },
  { immediate: true },
)

async function save() {
  errMsg.value = ''
  if (!title.value.trim()) {
    errMsg.value = '标题不能为空'
    return
  }
  // 强约束：日程任务不能把日期清空，无日期任务不能加日期。
  // UI 上已通过禁用控件防止用户操作；这里再做一次双保险，防止意外路径绕过。
  if (originallyHadDueAt.value && !dueAtLocal.value) {
    errMsg.value = '日程任务必须保留截止时间。如需把它改为"无日期"，请先删除再重建。'
    return
  }
  if (!originallyHadDueAt.value && dueAtLocal.value) {
    errMsg.value = '"无日期"任务不能添加截止时间。如需安排进日程，请先删除再重建。'
    return
  }
  saving.value = true
  try {
    // 严格按"原始是否有日期"决定提交内容：无日期任务永远提交 null，杜绝任何
    // 隐式状态切换。
    const due = originallyHadDueAt.value
      ? (dueAtLocal.value ? fromDatetimeLocal(dueAtLocal.value) : null)
      : null
    const updated = await data.updateTodo(props.todo.id, {
      title: title.value.trim(),
      description: description.value,
      priority: priority.value,
      effort: effort.value,
      list_id: listId.value,
      due_at: due ? toRFC3339(due) : null,
      due_all_day: originallyHadDueAt.value ? dueAllDay.value : false,
      timezone: tz.value,
    })
    emit('updated', updated)
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    saving.value = false
  }
}

async function remove() {
  const ok = await confirmDialog({
    title: '确认删除任务？',
    message: `任务 "${props.todo.title}" 将被永久删除，包括它下面的子任务和提醒规则。此操作无法撤销。`,
    confirmText: '删除',
    cancelText: '取消',
    danger: true,
  })
  if (!ok) return
  try {
    await data.removeTodo(props.todo.id)
    emit('removed', props.todo.id)
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

async function addSub() {
  const t = newSubtaskTitle.value.trim()
  if (!t) return
  try {
    await data.addSubtask(props.todo.id, t)
    newSubtaskTitle.value = ''
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

function openReminderDialog() {
  remTitle.value = ''
  remTriggerLocal.value = ''
  remRRulePreset.value = ''
  remRRuleCustom.value = ''
  remDtstartLocal.value = ''
  remChannelLocal.value = true
  remChannelTelegram.value = false
  remErr.value = ''
  showReminderDialog.value = true
}

// 最终合成 RRULE：优先用自定义文本；否则用预设；都为空则视为单次
const effectiveRRule = computed(() => remRRuleCustom.value.trim() || remRRulePreset.value)

async function createReminder() {
  remErr.value = ''
  const rrule = effectiveRRule.value
  const isOnce = !rrule
  if (isOnce && !remTriggerLocal.value) {
    remErr.value = '请选择触发时间，或选择/填写 RRULE 周期'
    return
  }
  if (!isOnce && !remDtstartLocal.value) {
    remErr.value = '周期提醒必须指定起始时间（dtstart）'
    return
  }
  try {
    const body: Record<string, unknown> = {
      todo_id: props.todo.id,
      title: remTitle.value.trim() || props.todo.title,
      timezone: tz.value,
      channel_local: remChannelLocal.value,
      channel_telegram: remChannelTelegram.value,
      ringtone: remRingtone.value,
      vibrate: remVibrate.value,
      fullscreen: remFullscreen.value,
    }
    if (isOnce) {
      const t = fromDatetimeLocal(remTriggerLocal.value)
      if (!t) throw new Error('无效的触发时间')
      body.trigger_at = toRFC3339(t)
    } else {
      const d = fromDatetimeLocal(remDtstartLocal.value)
      if (!d) throw new Error('无效的起始时间')
      body.rrule = rrule
      body.dtstart = toRFC3339(d)
    }
    await remindersApi.create(body)
    await data.loadReminders(props.todo.id)
    showReminderDialog.value = false
  } catch (e) {
    remErr.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

async function toggleReminder(r: ReminderRule) {
  try {
    if (r.is_enabled) await remindersApi.disable(r.id)
    else await remindersApi.enable(r.id)
    await data.loadReminders(props.todo.id)
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

async function removeReminder(r: ReminderRule) {
  const ok = await confirmDialog({
    title: '删除这条提醒？',
    message: r.title
      ? `提醒 "${r.title}" 将被永久删除，对应的下次触发也会一并取消。`
      : '这条提醒将被永久删除，对应的下次触发也会一并取消。',
    confirmText: '删除',
    cancelText: '取消',
    danger: true,
  })
  if (!ok) return
  try {
    await remindersApi.remove(r.id)
    await data.loadReminders(props.todo.id)
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

const rrulePresets = [
  { label: '不重复', value: '' },
  { label: '每天', value: 'FREQ=DAILY;INTERVAL=1' },
  { label: '每周', value: 'FREQ=WEEKLY;INTERVAL=1' },
  { label: '每月', value: 'FREQ=MONTHLY;INTERVAL=1' },
  { label: '每 6 个月', value: 'FREQ=MONTHLY;INTERVAL=6' },
  { label: '每年', value: 'FREQ=YEARLY;INTERVAL=1' },
]
</script>

<template>
  <div class="drawer-backdrop" @click="emit('close')" />
  <div class="drawer">
    <header>
      <span class="title">编辑任务</span>
      <button class="btn-close" @click="emit('close')">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
      </button>
    </header>
    <div class="body">
      <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>

      <div class="field">
        <label>标题</label>
        <div class="pretty-input-wrap">
          <input v-model="title" class="pretty-input" autofocus />
          <span class="pretty-input-glow" aria-hidden="true" />
        </div>
      </div>
      <div class="field">
        <label>描述</label>
        <div class="pretty-input-wrap">
          <textarea v-model="description" class="pretty-input pretty-textarea" rows="3" placeholder="补充信息（可选）" />
          <span class="pretty-input-glow" aria-hidden="true" />
        </div>
      </div>

      <!-- 视觉化分类选择 -->
      <div class="field">
        <label>分类</label>
        <div class="cat-picker">
          <button
            type="button"
            class="cat-option"
            :class="{ 'is-selected': listId === null }"
            :style="{ '--cat-color': 'var(--tg-text-tertiary)' }"
            @click="listId = null"
          >
            <span class="dot" />
            未分类
          </button>
          <button
            v-for="l in data.lists"
            :key="l.id"
            type="button"
            class="cat-option"
            :class="{ 'is-selected': listId === l.id }"
            :style="{ '--cat-color': l.color || 'var(--tg-primary)' }"
            @click="listId = l.id"
          >
            <span class="dot" />
            {{ l.name }}
          </button>
        </div>
      </div>

      <div class="row">
        <div class="field">
          <label>优先级</label>
          <div class="pretty-input-wrap">
            <select v-model.number="priority" class="pretty-input">
              <option v-for="(lab, i) in PRIORITY_LABELS" :key="i" :value="i">{{ lab }}</option>
            </select>
            <span class="pretty-input-glow" aria-hidden="true" />
          </div>
        </div>
        <div class="field">
          <label>工作量</label>
          <div class="pretty-input-wrap">
            <select v-model.number="effort" class="pretty-input">
              <option v-for="i in 6" :key="i - 1" :value="i - 1">{{ i - 1 }}</option>
            </select>
            <span class="pretty-input-glow" aria-hidden="true" />
          </div>
        </div>
      </div>

      <!-- ============ 截止时间 / "无日期"锁定标识 ============
           "无日期" 与 "日程任务" 互不串通；任务一旦创建为某种类型就不能跨过去。 -->
      <template v-if="isNoDateTask">
        <div class="field">
          <label>类型</label>
          <div class="locked-no-date">
            <span class="locked-no-date-icon" aria-hidden="true">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                   stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                <rect x="3" y="4" width="18" height="18" rx="2" ry="2"/>
                <line x1="3" y1="10" x2="21" y2="10"/>
                <line x1="6" y1="15" x2="18" y2="15"/>
              </svg>
            </span>
            <div class="locked-no-date-text">
              <strong>「无日期」任务</strong>
              <span class="locked-no-date-desc">
                这是一条无日期任务，不会出现在日程视图里。
                "无日期" 与 "日程" 互不串通：如需把它安排进日程，请先删除再重建。
              </span>
            </div>
          </div>
        </div>
      </template>
      <template v-else>
        <div class="field">
          <label>截止时间 <span class="required" title="日程任务必填">*</span></label>
          <PrettyDateTimePicker v-model="dueAtLocal" :allow-clear="false" />
          <div class="form-hint muted">
            日程任务必须保留截止时间。
          </div>
        </div>
        <div class="field">
          <label>全天</label>
          <label class="field-inline" style="padding-top:8px"><input v-model="dueAllDay" type="checkbox" /> 全天任务</label>
        </div>
      </template>

      <hr />

      <div class="row-flex">
        <strong style="font-size:14px">子任务 ({{ subtasks.length }})</strong>
      </div>
      <ul class="subtasks">
        <li v-for="s in subtasks" :key="s.id" :class="{ done: s.is_completed }">
          <input
            type="checkbox"
            :checked="s.is_completed"
            @change="data.toggleSubtask(s)"
          />
          <span class="stitle">{{ s.title }}</span>
          <button class="btn-ghost btn-danger" title="删除子任务" @click="data.removeSubtask(s)">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
          </button>
        </li>
      </ul>
      <div class="add-subtask">
        <div class="pretty-input-wrap" style="flex:1">
          <input v-model="newSubtaskTitle" class="pretty-input" placeholder="新增子任务…" @keydown.enter="addSub" />
          <span class="pretty-input-glow" aria-hidden="true" />
        </div>
        <button class="btn-secondary" @click="addSub">+ 添加</button>
      </div>

      <hr />

      <div class="row-flex">
        <strong style="font-size:14px">提醒 ({{ reminders.length }})</strong>
        <span class="spacer" />
        <button class="btn-secondary" @click="openReminderDialog">+ 新增</button>
      </div>
      <div v-if="reminders.length === 0" class="muted" style="font-size:13px">还没有提醒。可添加单次或周期（如每 6 个月）提醒。</div>
      <div v-for="r in reminders" :key="r.id" class="reminder-rule">
        <div>
          <div>
            <span v-if="!r.is_enabled" class="muted">[已停用] </span>
            {{ r.title || '(未命名)' }}
            <span class="muted"> · {{ r.rrule ? r.rrule : '单次' }}</span>
          </div>
          <div class="rmeta">
            <span v-if="r.next_fire_at">下一次：{{ fmtRelative(r.next_fire_at) }}</span>
            <span v-else-if="r.trigger_at">触发于：{{ fmtDateTime(r.trigger_at) }}</span>
            <span v-if="r.channel_telegram"> · TG</span>
            <span v-if="r.channel_local"> · 本地</span>
          </div>
        </div>
        <div class="rule-actions">
          <button class="btn-ghost" @click="toggleReminder(r)">{{ r.is_enabled ? '停用' : '启用' }}</button>
          <button class="btn-ghost btn-danger" title="删除" @click="removeReminder(r)">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
          </button>
        </div>
      </div>
    </div>
    <footer>
      <button class="btn-ghost btn-danger" @click="remove">删除任务</button>
      <span class="spacer" />
      <button class="btn-secondary" @click="emit('close')">取消</button>
      <button class="btn-primary" :disabled="saving" @click="save">
        {{ saving ? '保存中…' : '保存' }}
      </button>
    </footer>
  </div>

  <!-- 新增提醒对话框 -->
  <Transition name="fade">
    <div v-if="showReminderDialog" class="modal-backdrop" @click.self="showReminderDialog = false">
      <div class="modal-card" style="width:min(420px,95vw)">
        <header style="display:flex;align-items:center;justify-content:space-between;padding:14px 18px;border-bottom:1px solid var(--tg-divider)">
          <span style="font-size:16px;font-weight:600">新增提醒</span>
          <button class="btn-icon" @click="showReminderDialog = false">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </header>
        <div style="padding:18px;display:flex;flex-direction:column;gap:14px">
          <div v-if="remErr" class="auth-error">{{ remErr }}</div>
          <div class="field">
            <label>标题（默认沿用任务标题）</label>
            <div class="pretty-input-wrap">
              <input v-model="remTitle" class="pretty-input" />
              <span class="pretty-input-glow" aria-hidden="true" />
            </div>
          </div>
          <div class="field">
            <label>重复（预设）</label>
            <div class="pretty-input-wrap">
              <select v-model="remRRulePreset" class="pretty-input">
                <option v-for="p in rrulePresets" :key="p.label" :value="p.value">{{ p.label }}</option>
              </select>
              <span class="pretty-input-glow" aria-hidden="true" />
            </div>
          </div>
          <div class="field">
            <label>或自定义 RRULE（优先级高于预设）</label>
            <div class="pretty-input-wrap">
              <input v-model="remRRuleCustom" class="pretty-input" placeholder="例如：FREQ=DAILY;INTERVAL=2" />
              <span class="pretty-input-glow" aria-hidden="true" />
            </div>
          </div>
          <div v-if="!effectiveRRule" class="field">
            <label>触发时间（单次）</label>
            <PrettyDateTimePicker v-model="remTriggerLocal" />
          </div>
          <div v-else class="field">
            <label>起始时间（dtstart，周期从这里展开）</label>
            <PrettyDateTimePicker v-model="remDtstartLocal" />
          </div>
          <div class="field">
            <label>通道</label>
            <label class="field-inline"><input v-model="remChannelLocal" type="checkbox" /> 服务端通知中心 / 本地</label>
            <label class="field-inline"><input v-model="remChannelTelegram" type="checkbox" /> Telegram 推送</label>
          </div>
        </div>
        <footer style="display:flex;gap:10px;justify-content:flex-end;padding:12px 18px;border-top:1px solid var(--tg-divider)">
          <button class="btn-secondary" @click="showReminderDialog = false">取消</button>
          <button class="btn-primary" @click="createReminder">创建</button>
        </footer>
      </div>
    </div>
  </Transition>
</template>

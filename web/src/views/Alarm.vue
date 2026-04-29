<script setup lang="ts">
// Strong-reminder fullscreen window (Tauri only).
//
// 路由:/alarm?id=<rule_id>&title=<...>&fire_at=<rfc3339>
//
// 这个页面只在 Tauri 弹出来,但走的是同一个 Web SPA 入口。如果有人误点开
// 浏览器中的 /alarm,我们也优雅展示一个静态视图。
//
// 三个动作:
//   - 停止响铃   -> tauri.stopAlarm(ruleId),关闭窗口
//   - 完成任务   -> 在线时把 todo / reminder 状态推到服务端,然后 stopAlarm
//                  离线时仅 stopAlarm + 提示用户回头再确认(规格 §4)
//   - 稍后提醒   -> 不动服务端,只关本窗口;5 分钟后会被本地调度器再触发
//                  (因为 next_fire_at 还没被推进)

import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { reminders, todos, ApiError } from '@/api'
import { tauri, isTauri } from '@/tauri'
import { fmtDateTime } from '@/utils'

const route = useRoute()
const ruleId = computed(() => Number(route.query.id || 0))
const title = computed(() => decodeURIComponent(String(route.query.title || '提醒')))
const fireAt = computed(() => String(route.query.fire_at || ''))

const errMsg = ref('')
const ok = ref('')
const ackedLocal = ref(false)

const elapsed = ref(0)
const tickHandle = ref<number | null>(null)

const hasReminder = computed(() => ruleId.value > 0)
const online = ref(navigator.onLine)
function handleOnline() {
  online.value = true
}
function handleOffline() {
  online.value = false
}

onMounted(() => {
  // 计算超时多久(显示在 UI)
  if (fireAt.value) {
    const start = new Date(fireAt.value).getTime()
    elapsed.value = Math.max(0, Math.floor((Date.now() - start) / 1000))
    tickHandle.value = window.setInterval(() => {
      elapsed.value = Math.max(0, Math.floor((Date.now() - start) / 1000))
    }, 1000)
  }
  window.addEventListener('online', handleOnline)
  window.addEventListener('offline', handleOffline)
})

onBeforeUnmount(() => {
  if (tickHandle.value) window.clearInterval(tickHandle.value)
  window.removeEventListener('online', handleOnline)
  window.removeEventListener('offline', handleOffline)
})

async function stopOnly() {
  if (isTauri()) await tauri.stopAlarm(ruleId.value)
  ackedLocal.value = true
}

async function snooze() {
  // 规格 §4:不改服务端 next_fire_at;Tauri 本地端会在下次 tick 重新弹出。
  // 关本窗口即可。我们也清理本地 alarm_log 让它重新触发?
  // 简单点:窗口本身 5 分钟后由本地调度器再次拉起,这里只关窗口。
  if (isTauri()) await tauri.stopAlarm(ruleId.value)
  ackedLocal.value = true
}

async function complete() {
  errMsg.value = ''
  if (!online.value) {
    errMsg.value = '当前离线,无法提交完成。已停止本地响铃,联网后请在主界面再次确认完成。'
    if (isTauri()) await tauri.stopAlarm(ruleId.value)
    ackedLocal.value = true
    return
  }
  try {
    // 拉一次 reminder,看下绑定的 todo 是哪个
    const r = await reminders.get(ruleId.value)
    if (r.todo_id) {
      await todos.complete(r.todo_id)
    }
    ok.value = '已标记完成 ✓'
    if (isTauri()) await tauri.stopAlarm(ruleId.value)
    ackedLocal.value = true
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}
</script>

<template>
  <div class="alarm-page">
    <div class="alarm-card">
      <div class="alarm-icon">⏰</div>
      <h1 class="alarm-title">{{ title }}</h1>
      <div class="alarm-when">
        <div>触发时间:{{ fmtDateTime(fireAt) || '现在' }}</div>
        <div v-if="elapsed > 0" class="muted">已响铃 {{ elapsed }} 秒</div>
        <div v-if="!online" class="danger-text">⚠ 当前离线 — 仅可本地停止响铃</div>
      </div>

      <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>
      <div v-if="ok" class="success-text">{{ ok }}</div>

      <div class="alarm-actions">
        <button v-if="!ackedLocal" class="btn-primary" @click="stopOnly">停止响铃</button>
        <button v-if="!ackedLocal" class="btn-secondary" @click="snooze">稍后再提醒</button>
        <button
          v-if="!ackedLocal && hasReminder"
          class="btn-secondary success-text"
          @click="complete"
        >完成任务</button>
        <button v-if="ackedLocal" class="btn-primary" @click="tauri.quit()">关闭窗口</button>
      </div>

      <p v-if="!isTauri()" class="muted" style="margin-top: 16px; font-size: 13px">
        这个页面是 Tauri 桌面客户端的强提醒窗口。在普通浏览器里它没有真正的"停止响铃"动作。
      </p>
    </div>
  </div>
</template>

<style>
.alarm-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: linear-gradient(135deg, var(--c-primary), #1858bb);
  padding: 20px;
}
.alarm-card {
  background: var(--c-surface);
  border-radius: 16px;
  padding: 32px 28px;
  width: 100%;
  max-width: 420px;
  box-shadow: 0 24px 48px rgba(0, 0, 0, 0.3);
  text-align: center;
}
.alarm-icon {
  font-size: 56px;
  line-height: 1;
  margin-bottom: 12px;
  animation: bell 1.2s infinite;
}
@keyframes bell {
  0%, 60%, 100% { transform: rotate(0); }
  10%, 30%, 50% { transform: rotate(15deg); }
  20%, 40% { transform: rotate(-15deg); }
}
.alarm-title {
  font-size: 24px;
  margin: 0 0 16px;
  word-break: break-word;
}
.alarm-when {
  font-size: 13px;
  color: var(--c-text-soft);
  margin-bottom: 20px;
  line-height: 1.6;
}
.alarm-actions {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.alarm-actions button {
  padding: 12px 16px;
  font-weight: 600;
  font-size: 15px;
}
</style>

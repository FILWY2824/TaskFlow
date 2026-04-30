// 本地任务到期提醒调度器（与服务端的 reminder rules 完全独立）。
// 当用户在「设置」打开「任务截止本地提醒」时，遍历当前 data.todos，
// 对所有未完成、due_at 在未来不超过 24 小时的任务，
// 设置 setTimeout 在到期时刻弹应用内 toast（并按设置触发桌面通知）。
//
// 浏览器关闭/刷新后定时器会丢失——这是预期行为，下一次刷新会重新挂载。

import { useDataStore } from '@/stores/data'
import { useNotificationsStore } from '@/stores/notifications'
import { usePrefsStore } from '@/stores/prefs'
import { watch } from 'vue'

interface Scheduled {
  todoId: number
  fireAt: number
  handle: number
}

let scheduled = new Map<number, Scheduled>()

function clearAll() {
  for (const s of scheduled.values()) window.clearTimeout(s.handle)
  scheduled = new Map()
}

function fire(todoId: number, title: string) {
  const notif = useNotificationsStore()
  const prefs = usePrefsStore()
  if (!prefs.todoDueToast) return
  // 应用内 toast（pushToast 内部已经检查 inAppToast 偏好）
  notif.pushToast({
    id: -todoId, // 用负数避免与服务端 notification_id 冲突
    title: '任务到期：' + title,
    body: '点击查看详情',
  })
  // 桌面通知（pushToast 走的是 SSE 路径，本地路径自己再补一次）
  try {
    if (prefs.desktopNotification && 'Notification' in window && Notification.permission === 'granted') {
      new Notification('任务到期：' + title, { body: '点击查看详情', tag: 'due-' + todoId })
    }
  } catch { /* ignore */ }
}

function rescheduleAll() {
  clearAll()
  const data = useDataStore()
  const prefs = usePrefsStore()
  if (!prefs.todoDueToast) return

  const now = Date.now()
  const horizon = 24 * 3600 * 1000  // 仅对 24 小时内的任务调度，避免长 timeout

  for (const t of data.todos) {
    if (t.is_completed || !t.due_at) continue
    const fireAt = new Date(t.due_at).getTime()
    const delta = fireAt - now
    if (delta <= 0 || delta > horizon) continue
    const id = t.id
    const title = t.title
    const handle = window.setTimeout(() => fire(id, title), delta)
    scheduled.set(id, { todoId: id, fireAt, handle })
  }
}

let installed = false

export function installTodoDueScheduler() {
  if (installed) return
  installed = true
  const data = useDataStore()
  const prefs = usePrefsStore()
  // 监听 todos 列表变化与偏好变化，重新计算调度。
  watch(() => data.todos, rescheduleAll, { deep: true })
  watch(() => prefs.todoDueToast, rescheduleAll)
  // 首次启动
  rescheduleAll()
  // 每 5 分钟做一次"安全网"重新调度（捕获 24 小时窗口外的、刚刚滑入窗口的任务）
  setInterval(rescheduleAll, 5 * 60 * 1000)
}

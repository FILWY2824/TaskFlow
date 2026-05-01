import { defineStore } from 'pinia'
import { notifications as notifApi, loadTokens } from '@/api'
import type { Notification, SSENotification } from '@/types'

interface SSEHandle {
  close(): void
}

interface ToastItem {
  key: number     // 唯一自增 key，避免同一 notification_id 被多次推送时 <TransitionGroup> 报重复 key 警告
  id: number      // 后端 notification_id
  title: string
  body: string
}

let toastSeq = 0

// 用 fetch + ReadableStream 自己读 SSE，因为浏览器原生 EventSource 不能加 Authorization header。
function openSSE(token: string, onEvent: (e: SSENotification) => void): SSEHandle {
  const ctrl = new AbortController()
  let closed = false

  ;(async () => {
    while (!closed) {
      try {
        const res = await fetch('/ws/events', {
          headers: { Authorization: `Bearer ${token}`, Accept: 'text/event-stream' },
          signal: ctrl.signal,
        })
        if (!res.ok || !res.body) {
          throw new Error('SSE bad response: ' + res.status)
        }
        const reader = res.body.getReader()
        const dec = new TextDecoder('utf-8')
        let buf = ''
        let eventName = ''
        while (true) {
          const { value, done } = await reader.read()
          if (done) break
          buf += dec.decode(value, { stream: true })
          let idx
          while ((idx = buf.indexOf('\n')) >= 0) {
            const line = buf.slice(0, idx).replace(/\r$/, '')
            buf = buf.slice(idx + 1)
            if (line === '') {
              eventName = ''
              continue
            }
            if (line.startsWith(':')) continue
            if (line.startsWith('event:')) {
              eventName = line.slice(6).trim()
            } else if (line.startsWith('data:')) {
              const data = line.slice(5).trim()
              if (eventName === 'notification' || eventName === '') {
                try {
                  const parsed = JSON.parse(data) as SSENotification
                  if (parsed && parsed.type === 'notification') {
                    onEvent(parsed)
                  }
                } catch {
                  // ignore malformed event
                }
              }
            }
          }
        }
      } catch {
        if (closed) break
        // 断流后退避 3s 重连
        await new Promise((r) => setTimeout(r, 3000))
      }
    }
  })()

  return {
    close() {
      closed = true
      ctrl.abort()
    },
  }
}

export const useNotificationsStore = defineStore('notifications', {
  state: () => ({
    items: [] as Notification[],
    unread: 0,
    sse: null as SSEHandle | null,
    toastQueue: [] as ToastItem[],
  }),
  actions: {
    async refresh() {
      const r = await notifApi.list({ limit: 50 })
      this.items = r.items || []
      this.unread = r.unread_count || 0
    },
    async refreshUnread() {
      const r = await notifApi.unreadCount()
      this.unread = r.count
    },
    async markRead(id: number) {
      await notifApi.markRead(id)
      const item = this.items.find((x) => x.id === id)
      if (item && !item.is_read) {
        item.is_read = true
        this.unread = Math.max(0, this.unread - 1)
      }
    },
    async markAllRead() {
      await notifApi.markAllRead()
      for (const x of this.items) x.is_read = true
      this.unread = 0
    },
    pushToast(t: { id: number; title: string; body: string }) {
      // 应用内 toast 的偏好开关由 settings 控制；关闭时直接丢弃。
      try {
        const raw = localStorage.getItem('taskflow.prefs.v1')
        if (raw) {
          const p = JSON.parse(raw) as { inAppToast?: boolean }
          if (p && p.inAppToast === false) return
        }
      } catch { /* ignore */ }
      const key = ++toastSeq
      this.toastQueue.push({ key, id: t.id, title: t.title, body: t.body })
      setTimeout(() => {
        this.toastQueue = this.toastQueue.filter((x) => x.key !== key)
      }, 6000)
    },
    dismissToast(key: number) {
      this.toastQueue = this.toastQueue.filter((x) => x.key !== key)
    },
    startSSE() {
      if (this.sse) return
      const t = loadTokens()
      if (!t) return
      this.sse = openSSE(t.accessToken, (ev) => {
        this.unread += 1
        this.pushToast({
          id: ev.notification_id,
          title: ev.title,
          body: ev.body || '',
        })
        this.refresh().catch(() => {
          // ignore
        })
        try {
          // 桌面通知开关
          let allow = true
          try {
            const raw = localStorage.getItem('taskflow.prefs.v1')
            if (raw) {
              const p = JSON.parse(raw) as { desktopNotification?: boolean }
              if (p && p.desktopNotification === false) allow = false
            }
          } catch { /* ignore */ }
          if (allow && 'Notification' in window && Notification.permission === 'granted') {
            new Notification(ev.title, { body: ev.body || '', tag: 'todolist-' + ev.notification_id })
          }
        } catch {
          // ignore
        }
      })
    },
    stopSSE() {
      this.sse?.close()
      this.sse = null
    },
  },
})

import { defineStore } from 'pinia'
import { notifications as notifApi, loadTokens } from '@/api'
import type { Notification, SSENotification } from '@/types'

interface SSEHandle {
  close(): void
}

// 用 fetch + ReadableStream 自己读 SSE,因为浏览器原生 EventSource 不能加 Authorization header。
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
      } catch (e) {
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
    toastQueue: [] as { id: number; title: string; body: string }[],
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
      this.toastQueue.push(t)
      // 自动关闭
      setTimeout(() => {
        this.toastQueue = this.toastQueue.filter((x) => x.id !== t.id)
      }, 6000)
    },
    dismissToast(id: number) {
      this.toastQueue = this.toastQueue.filter((x) => x.id !== id)
    },
    startSSE() {
      if (this.sse) return
      const t = loadTokens()
      if (!t) return
      this.sse = openSSE(t.accessToken, (ev) => {
        // 收到通知时自增未读 + 弹 toast + 浏览器 Notification(若用户授权)
        this.unread += 1
        this.pushToast({
          id: ev.notification_id,
          title: ev.title,
          body: ev.body || '',
        })
        // 触发后台拉取最新 5 条通知
        this.refresh().catch(() => {
          // ignore
        })
        try {
          if ('Notification' in window && Notification.permission === 'granted') {
            new Notification(ev.title, { body: ev.body || '', tag: 'todoalarm-' + ev.notification_id })
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

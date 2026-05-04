// 时间/格式化辅助函数。
//
// 设计要点:
//   - 服务端时间统一是 RFC3339 UTC,前端拿到字符串就 new Date(...) 直接是 UTC 解析。
//   - 显示用 toLocaleString,会自动落到浏览器时区(用户希望用某个固定时区时,可以扩展用 Intl.DateTimeFormat)。

export function parseDate(s: string | null | undefined): Date | null {
  if (!s) return null
  const d = new Date(s)
  return isNaN(d.getTime()) ? null : d
}

const DOW = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']

export function fmtDate(d: Date | string | null | undefined): string {
  const x = typeof d === 'string' ? parseDate(d) : d
  if (!x) return ''
  const y = x.getFullYear()
  const mo = String(x.getMonth() + 1).padStart(2, '0')
  const dd = String(x.getDate()).padStart(2, '0')
  return `${y}-${mo}-${dd}`
}

export function fmtTime(d: Date | string | null | undefined): string {
  const x = typeof d === 'string' ? parseDate(d) : d
  if (!x) return ''
  const h = String(x.getHours()).padStart(2, '0')
  const m = String(x.getMinutes()).padStart(2, '0')
  return `${h}:${m}`
}

export function fmtDateTime(d: Date | string | null | undefined): string {
  const x = typeof d === 'string' ? parseDate(d) : d
  if (!x) return ''
  return `${fmtDate(x)} ${fmtTime(x)}`
}

export function fmtRelative(d: Date | string | null | undefined): string {
  const x = typeof d === 'string' ? parseDate(d) : d
  if (!x) return ''
  const now = new Date()
  const today0 = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const that0 = new Date(x.getFullYear(), x.getMonth(), x.getDate())
  const days = Math.round((that0.getTime() - today0.getTime()) / (24 * 3600 * 1000))
  const hm = fmtTime(x)
  if (days === 0) return `今天 ${hm}`
  if (days === 1) return `明天 ${hm}`
  if (days === -1) return `昨天 ${hm}`
  if (days > 1 && days <= 6) return `${DOW[x.getDay()]} ${hm}`
  if (days < 0 && days >= -6) return `${days} 天前 ${hm}`
  return fmtDateTime(x)
}

export function fmtDuration(seconds: number): string {
  if (!seconds || seconds < 0) return '0 分'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (h > 0) return `${h} 时 ${m} 分`
  return `${m} 分`
}

// 把 Date 转成 RFC3339 UTC 字符串(发服务端用)。
export function toRFC3339(d: Date): string {
  return d.toISOString()
}

// 从 datetime-local 输入控件的字符串(本地时区,无 TZ 偏移)解析为 Date。
export function fromDatetimeLocal(s: string): Date | null {
  if (!s) return null
  const d = new Date(s)
  return isNaN(d.getTime()) ? null : d
}

// 把 Date 转成 datetime-local 输入控件接受的字符串(本地时区,无 TZ)。
export function toDatetimeLocal(d: Date | null | undefined): string {
  if (!d) return ''
  const y = d.getFullYear()
  const mo = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  const h = String(d.getHours()).padStart(2, '0')
  const m = String(d.getMinutes()).padStart(2, '0')
  return `${y}-${mo}-${dd}T${h}:${m}`
}

export const PRIORITY_LABELS = ['无', '低', '中', '高', '紧急']
export function fmtDurationMinutes(minutes: number | null | undefined): string {
  const total = Math.max(0, Math.round(Number(minutes) || 0))
  if (total <= 0) return '未设置'
  const h = Math.floor(total / 60)
  const m = total % 60
  if (h > 0 && m > 0) return `${h} 小时 ${m} 分钟`
  if (h > 0) return `${h} 小时`
  return `${m} 分钟`
}

export const PRIORITY_COLORS = ['#9ca3af', '#3b82f6', '#10b981', '#f59e0b', '#ef4444']

export function taskStartAt(t: { start_at?: string | null; due_at?: string | null }): string | null {
  return t.start_at || t.due_at || null
}

export function isOverdue(t: { start_at?: string | null; due_at?: string | null; is_completed: boolean }): boolean {
  const start = taskStartAt(t)
  if (t.is_completed || !start) return false
  return new Date(start).getTime() < Date.now()
}

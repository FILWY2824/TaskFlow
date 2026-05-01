// 应用本地偏好（与服务端无关，只在浏览器 localStorage 里存）。
// 关键策略：所有"提醒/通知/桌面通知"开关都放这里，由 UI（Settings）唯一负责调整。
// 业务页面（如 Tasks、Pomodoro、SSE）只读取这里，不再直接弹"是否允许通知"。

import { defineStore } from 'pinia'

const KEY = 'taskflow.prefs.v1'

export interface Prefs {
  // 应用内 toast 弹窗（SSE 收到提醒、番茄到点等）
  inAppToast: boolean
  // 浏览器系统级桌面通知（需用户授权 Notification.permission）
  desktopNotification: boolean
  // 番茄到点是否声音提示（仅 Tauri 强提醒；Web 端只用 toast/桌面通知）
  pomodoroSound: boolean
  // 番茄到点自动结束并入库（false 时停留在 0:00 等用户点"完成"）
  pomodoroAutoComplete: boolean
  // 任务到截止日时是否本地弹窗提醒（与服务端 reminder 独立，纯本地）
  todoDueToast: boolean
}

const DEFAULTS: Prefs = {
  inAppToast: true,
  desktopNotification: true,
  pomodoroSound: true,
  pomodoroAutoComplete: true,
  todoDueToast: true,
}

function load(): Prefs {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return { ...DEFAULTS }
    const parsed = JSON.parse(raw) as Partial<Prefs>
    return { ...DEFAULTS, ...parsed }
  } catch {
    return { ...DEFAULTS }
  }
}

function save(p: Prefs) {
  try {
    localStorage.setItem(KEY, JSON.stringify(p))
  } catch {
    /* ignore */
  }
}

export const usePrefsStore = defineStore('prefs', {
  state: () => load(),
  actions: {
    set<K extends keyof Prefs>(k: K, v: Prefs[K]) {
      this[k] = v
      save(this.$state)
    },
    reset() {
      Object.assign(this, DEFAULTS)
      save(this.$state)
    },
  },
})

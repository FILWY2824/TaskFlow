// 应用本地偏好。
//
// 重要变化(规格 §17 阶段 13):
//   - 偏好的 source-of-truth 是服务端 user_preferences 表。
//     UI 只在登录后第一次 hydrate() 一次,之后的写入都经过 prefsApi.putOne 去服务端,
//     localStorage 只是离线 / 启动期间的快速回放缓存。
//   - 本 store 自动按运行环境决定 scope:
//        浏览器(Web)        scope = 'web'
//        Tauri WebView (Windows)  scope = 'windows'
//     两个 scope 在数据库里是独立的行,即同一用户在 Web 端 / Windows 端各自的开关
//     互不污染,但都同步到 user_preferences 表。Android 端不走这份 ts(它有原生 PreferenceRepository)。
//   - 服务端不解释 value,我们这里把布尔值统一序列化为 "1" / "0"。

import { defineStore } from 'pinia'
import { prefsApi, ApiError } from '@/api'
import { isTauri } from '@/tauri'

const KEY = 'taskflow.prefs.v1'   // localStorage 离线缓存键(保持兼容)

/** 当前进程的 scope:Web 浏览器 -> 'web';Tauri 桌面 -> 'windows'。 */
function currentScope(): 'web' | 'windows' {
  return isTauri() ? 'windows' : 'web'
}

export interface Prefs {
  // 应用内 toast 弹窗(SSE 收到提醒、番茄到点等)
  inAppToast: boolean
  // 平台系统级桌面通知:
  //   Web    -> 浏览器 Notification API(需 Notification.permission = granted)
  //   Windows-> Tauri toast(系统通知中心)
  desktopNotification: boolean
  // 番茄到点是否声音提示
  //   Web    -> 仅 web audio 蜂鸣
  //   Windows-> Tauri rodio 响铃(可设备级)
  pomodoroSound: boolean
  // 番茄到点自动结束并入库(false 时停留在 0:00 等用户点"完成")
  pomodoroAutoComplete: boolean
  // 任务到开始时间时是否本地弹窗提醒(与服务端 reminder 独立,纯本地)
  todoDueToast: boolean
  // 仅 Windows 有效:到点把窗口"总在最前"弹出来,直到用户停掉响铃
  alwaysOnTopAlarm: boolean
}

const DEFAULTS: Prefs = {
  inAppToast: true,
  desktopNotification: true,
  pomodoroSound: true,
  pomodoroAutoComplete: true,
  todoDueToast: true,
  alwaysOnTopAlarm: true,
}

// 客户端键空间(写入服务端 user_preferences.key)。命名遵守服务端校验:[a-z0-9._-]、<=64 字符。
const KEY_MAP: Record<keyof Prefs, string> = {
  inAppToast: 'notification.in_app_toast',
  desktopNotification: 'notification.desktop',
  pomodoroSound: 'pomodoro.sound',
  pomodoroAutoComplete: 'pomodoro.auto_complete',
  todoDueToast: 'notification.todo_due_toast',
  alwaysOnTopAlarm: 'notification.always_on_top_alarm',
}

function boolToStr(b: boolean): string {
  return b ? '1' : '0'
}
function strToBool(s: string, fallback: boolean): boolean {
  if (s === '1' || s === 'true') return true
  if (s === '0' || s === 'false') return false
  return fallback
}

function loadLocal(): Prefs {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return { ...DEFAULTS }
    const parsed = JSON.parse(raw) as Partial<Prefs>
    return { ...DEFAULTS, ...parsed }
  } catch {
    return { ...DEFAULTS }
  }
}

function saveLocal(p: Prefs) {
  try {
    localStorage.setItem(KEY, JSON.stringify(p))
  } catch {
    /* ignore */
  }
}

export const usePrefsStore = defineStore('prefs', {
  state: () => ({
    ...loadLocal(),
    /** 是否已经从服务端 hydrate 过(防止登录前的 set() 误触发同步) */
    _hydrated: false as boolean,
  }),
  actions: {
    /**
     * 在登录成功(或刷新页面带着 token 启动)之后调用一次。
     * 拉服务端的本端 scope 偏好,缺省值用 DEFAULTS 兜底。
     * 失败时不抛异常 —— 离线状态用本地缓存照常工作,等下次成功 hydrate。
     */
    async hydrate() {
      try {
        const items = await prefsApi.list(currentScope())
        const next: Prefs = { ...DEFAULTS, ...loadLocal() }
        const inverse: Record<string, keyof Prefs> = {}
        for (const k of Object.keys(KEY_MAP) as Array<keyof Prefs>) inverse[KEY_MAP[k]] = k
        for (const it of items) {
          const localKey = inverse[it.key]
          if (localKey) {
            ;(next as Record<keyof Prefs, boolean>)[localKey] = strToBool(
              it.value,
              DEFAULTS[localKey],
            )
          }
        }
        Object.assign(this, next)
        saveLocal(this.toPlain())
        this._hydrated = true
      } catch (e) {
        // 401 不在这里处理,api.ts 已经清 token + 跳登录;其他错误吞掉,留待下次 hydrate
        if (!(e instanceof ApiError)) {
          // eslint-disable-next-line no-console
          console.warn('prefs hydrate failed:', e)
        }
      }
    },

    /**
     * 单条写入。乐观更新:立刻改本地,后台异步推到服务端;失败时静默(不回滚,
     * 因为这些是非关键开关,下次 hydrate 会按服务端重置)。
     */
    set<K extends keyof Prefs>(k: K, v: Prefs[K]) {
      this[k] = v
      saveLocal(this.toPlain())
      // 仅在 hydrate 之后才下发服务端,防止登录前的初始读取触发空写
      if (this._hydrated) {
        prefsApi
          .putOne(currentScope(), KEY_MAP[k], boolToStr(v as boolean))
          .catch((e) => {
            // eslint-disable-next-line no-console
            console.warn('prefs sync failed:', e)
          })
      }
    },

    reset() {
      Object.assign(this, DEFAULTS)
      saveLocal(this.toPlain())
      if (this._hydrated) {
        const scope = currentScope()
        const items = (Object.keys(DEFAULTS) as Array<keyof Prefs>).map((k) => ({
          scope,
          key: KEY_MAP[k],
          value: boolToStr(DEFAULTS[k]),
        }))
        prefsApi.putBulk(items).catch(() => { /* ignore */ })
      }
    },

    /** 把 store 转回纯对象(剥掉 _hydrated 等内部字段),用于写 localStorage。 */
    toPlain(): Prefs {
      return {
        inAppToast: this.inAppToast,
        desktopNotification: this.desktopNotification,
        pomodoroSound: this.pomodoroSound,
        pomodoroAutoComplete: this.pomodoroAutoComplete,
        todoDueToast: this.todoDueToast,
        alwaysOnTopAlarm: this.alwaysOnTopAlarm,
      }
    },
  },
})


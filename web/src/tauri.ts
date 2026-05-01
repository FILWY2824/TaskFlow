// Tauri runtime bridge.
//
// 当 Web 端运行在 Tauri WebView 里时,这个模块会把关键事件桥到 Rust:
//   - 登录 / refresh 拿到新 token  -> invoke('set_tokens', ...)
//   - 退出登录                     -> invoke('clear_tokens')
//   - 用户改服务端 URL             -> invoke('set_server_config', ...)
//   - 强提醒窗口的"停止"按钮       -> invoke('stop_alarm', { ruleId })
//
// 在普通浏览器里,window.__TAURI_INTERNALS__ 不存在,所有调用降级为空操作。
// 这样同一份 Vue 代码可以跑在浏览器 / Tauri 里都不需要分支。

interface TauriInternal {
  invoke?: <T>(cmd: string, args?: Record<string, unknown>) => Promise<T>
}

interface TauriWindow extends Window {
  __TAURI_INTERNALS__?: TauriInternal
}

export const isTauri = (): boolean => {
  return typeof window !== 'undefined' && !!(window as TauriWindow).__TAURI_INTERNALS__
}

async function invoke<T = void>(cmd: string, args?: Record<string, unknown>): Promise<T | undefined> {
  if (!isTauri()) return undefined
  const w = window as TauriWindow
  const inv = w.__TAURI_INTERNALS__?.invoke
  if (!inv) return undefined
  try {
    return await inv<T>(cmd, args)
  } catch (e) {
    // 不让 Tauri IPC 失败把 UI 流程打断
    console.warn(`[tauri] invoke '${cmd}' failed:`, e)
    return undefined
  }
}

export const tauri = {
  /** 让 Rust 知道当前会话 token,后台 sync 才能拉数据 */
  async setTokens(args: {
    access_token: string | null
    refresh_token?: string | null
    timezone?: string
  }) {
    await invoke('set_tokens', args as Record<string, unknown>)
  },
  async clearTokens() {
    await invoke('clear_tokens')
  },
  async setServerConfig(args: { server_url: string; timezone?: string }) {
    await invoke('set_server_config', args as Record<string, unknown>)
  },
  async getServerConfig() {
    return invoke<{ server_url: string; timezone: string; autostart: boolean }>('get_server_config')
  },
  /**
   * 取打包时烧进去的"默认服务端 URL"(env VITE_TASKFLOW_DEFAULT_SERVER /
   * TASKFLOW_DEFAULT_SERVER_URL)。第一次启动时如果用户还没改过 server_url,
   * 我们用这个值。空串 = 没烧入,前端走"必须先到设置页填地址"的引导。
   */
  async getDefaultServerUrl(): Promise<string> {
    const v = await invoke<string>('get_default_server_url')
    return (v || '').trim()
  },
  /** 把窗口拉到屏幕最前 + 抢焦点。会话过期、收到强提醒时调一下。 */
  async bringToFront(label?: string) {
    await invoke('bring_window_to_front', { label })
  },
  async setAutostart(enabled: boolean) {
    await invoke('set_autostart', { enabled })
  },
  async isAutostartEnabled(): Promise<boolean> {
    const v = await invoke<boolean>('is_autostart_enabled')
    return v ?? false
  },
  async syncNow() {
    await invoke('sync_now')
  },
  async stopAlarm(ruleId: number) {
    await invoke('stop_alarm', { ruleId })
  },
  async openExternal(url: string) {
    await invoke('open_external', { url })
  },
  async quit() {
    await invoke('quit_app')
  },
}

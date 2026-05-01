import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { setUnauthorizedHandler, setApiBase, getApiBase } from './api'
import { useAuthStore } from './stores/auth'
import { isTauri, tauri } from './tauri'

import './style.css'

// 在 Vue 启动前应用用户偏好的主题，避免出现先白后黑的闪烁。
;(function applyTheme() {
  try {
    const saved = localStorage.getItem('taskflow.theme')
    if (saved === 'light' || saved === 'dark') {
      document.documentElement.setAttribute('data-theme', saved)
    }
  } catch {
    /* 无 localStorage 权限时降级为系统默认 */
  }
})()

// ============================================================
// API base URL 解析(Tauri 客户端启动的关键)
//
// 浏览器场景:apiBase = ""(同源相对路径,fetch('/api/...') 直接命中)。
// Tauri 场景:webview 跑在 tauri://localhost/,fetch('/api/...') 命中的是
// tauri://localhost/api/... —— 那里没有任何 API。所以:
//   1) 优先取 localStorage 缓存(用户上次设置的 server_url)。
//   2) 其次问 Rust:Rust 持久化的 server_url。
//   3) 还没有就用打包时烧进去的默认值(VITE_TASKFLOW_DEFAULT_SERVER /
//      TASKFLOW_DEFAULT_SERVER_URL,例如 https://taskflow.teamcy.eu.cc)。
// 解析必须在 mount 之前完成,否则 OAuthCallback / 自动登录 等启动期请求会
// 命中错误的 base。
// ============================================================
async function resolveApiBase(): Promise<void> {
  // 浏览器:同源就行,什么也不用做。
  if (!isTauri()) {
    // 但开发模式下 vite 可能开在 1420,API 在 8080 —— vite proxy 会处理 /api,
    // 所以同源相对路径仍然有效。这里仅做一次显式 setApiBase('') 兜底。
    setApiBase('')
    return
  }

  // 已经设置过的话,沿用 —— 用户可能在设置页改过。
  const cached = getApiBase()
  if (cached) return

  // 1) Rust 侧持久化的 server_url
  let chosen = ''
  try {
    const cfg = await tauri.getServerConfig()
    if (cfg && cfg.server_url) chosen = String(cfg.server_url).trim()
  } catch {
    /* ignore */
  }

  // 2) 出厂默认(打包时通过 env VITE_TASKFLOW_DEFAULT_SERVER 注入)
  if (!chosen) {
    const fromVite = String(
      (import.meta as unknown as { env?: Record<string, string> }).env?.VITE_TASKFLOW_DEFAULT_SERVER || '',
    ).trim()
    if (fromVite) chosen = fromVite
  }

  // 3) Rust 编译时 / 运行时 env(双保险)
  if (!chosen) {
    try {
      chosen = await tauri.getDefaultServerUrl()
    } catch {
      chosen = ''
    }
  }

  // 4) 兜底:127.0.0.1:8080(本地开发)
  if (!chosen) chosen = 'http://127.0.0.1:8080'

  setApiBase(chosen.replace(/\/+$/, ''))
  // 把 Rust 那边的 server_url 同步成同一个值,方便后台 sync loop 使用。
  try {
    await tauri.setServerConfig({ server_url: getApiBase() })
  } catch {
    /* ignore */
  }
}

async function bootstrap() {
  await resolveApiBase()

  const app = createApp(App)
  const pinia = createPinia()
  app.use(pinia)
  app.use(router)

  // 401 时由 router 处理跳登录(此时 pinia 已 use,store 可安全使用)
  setUnauthorizedHandler(() => {
    const a = useAuthStore()
    a.user = null
    if (
      router.currentRoute.value.name !== 'login' &&
      router.currentRoute.value.name !== 'register'
    ) {
      router.replace({
        name: 'login',
        query: { redirect: router.currentRoute.value.fullPath },
      })
    }
  })

  app.mount('#app')
}

void bootstrap()

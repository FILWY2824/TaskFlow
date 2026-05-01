import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { setUnauthorizedHandler } from './api'
import { useAuthStore } from './stores/auth'

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

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)

// 401 时由 router 处理跳登录（此时 pinia 已 use，store 可安全使用）
setUnauthorizedHandler(() => {
  const a = useAuthStore()
  a.user = null
  if (router.currentRoute.value.name !== 'login' && router.currentRoute.value.name !== 'register') {
    router.replace({ name: 'login', query: { redirect: router.currentRoute.value.fullPath } })
  }
})

app.mount('#app')

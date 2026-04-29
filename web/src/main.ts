import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { setUnauthorizedHandler } from './api'
import { useAuthStore } from './stores/auth'

import './style.css'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)

// 401 时由 router 处理跳登录(此时 pinia 已 use,store 可安全使用)
setUnauthorizedHandler(() => {
  const a = useAuthStore()
  a.user = null
  if (router.currentRoute.value.name !== 'login' && router.currentRoute.value.name !== 'register') {
    router.replace({ name: 'login', query: { redirect: router.currentRoute.value.fullPath } })
  }
})

app.mount('#app')

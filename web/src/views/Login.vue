<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ApiError, getApiBase } from '@/api'
import { isTauri, tauri } from '@/tauri'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const email = ref('')
const password = ref('')
const errMsg = ref('')
const loading = ref(false)
// 拉到 /api/auth/config 之前先标记加载中,避免 UI 闪烁(先看到表单再切到按钮)。
const configLoading = ref(true)

onMounted(async () => {
  await auth.loadAuthConfig()
  configLoading.value = false
  // 如果是从 /oauth/callback 错误跳回来的,把错误描述展示出来。
  const oauthErr = (route.query.oauth_error as string) || ''
  if (oauthErr) {
    errMsg.value = decodeURIComponent(oauthErr)
  }
})

const oauthEnabled = computed(() => auth.authConfig?.oauth_enabled === true)
const oauthProvider = computed(() => auth.authConfig?.oauth_provider || '')
const oauthStartURL = computed(() => auth.authConfig?.oauth_start_url || '/api/auth/oauth/start')

// 「通过认证中心登录」—— 整页跳转到后端的 start 端点,后端再 302 到认证中心。
//
// 关键:在 Tauri 里 webview 跑在 tauri://localhost/,把它的 location.href 改成
// 一个 https:// URL 不会触发跨协议跳转(Tauri 拒绝从内部协议跳到外部 https)。
// 所以 Tauri 客户端的 OAuth 必须用"系统默认浏览器"打开,完成回调后用户回到
// Tauri 客户端,这里通过提示让用户重启或手动回到登录页。
//
// 此外,即使是浏览器场景,start URL 是相对路径('/api/auth/oauth/start'),
// 当客户端跑在 tauri://localhost 时,它会被解析成 tauri://localhost/api/...
// —— 我们必须先把它改写成绝对的 https://taskflow.example.com/api/auth/oauth/start。
function startOAuth() {
  let url = oauthStartURL.value
  if (!/^https?:\/\//i.test(url)) {
    const base = getApiBase()
    if (!base) {
      errMsg.value = '尚未配置服务端地址,请到设置页填写后再尝试登录。'
      return
    }
    url = base + (url.startsWith('/') ? url : '/' + url)
  }

  const target = (route.query.redirect as string) || '/'
  try {
    sessionStorage.setItem('taskflow.oauth_redirect', target)
  } catch {
    // 隐私模式下 sessionStorage 不可用,忽略 —— 默认跳 /
  }

  if (isTauri()) {
    // 在 Tauri 客户端里,用系统默认浏览器打开。用户在浏览器登录后会被重定向回
    // 我们的服务端 + 前端 —— 但是是浏览器里的前端。Tauri 客户端这边需要等用户
    // 回来后手动登一次,或者我们 poll session(下个版本再做)。
    void tauri.openExternal(url)
    errMsg.value =
      '已在系统浏览器中打开认证中心。\n登录完成后,请回到本程序点击"通过认证中心登录"再次确认会话。'
    return
  }

  window.location.href = url
}

async function submit() {
  errMsg.value = ''
  if (!email.value || !password.value) {
    errMsg.value = '请填写邮箱与密码'
    return
  }
  loading.value = true
  try {
    await auth.login(email.value.trim(), password.value)
    const redirect = (route.query.redirect as string) || '/'
    router.replace(redirect)
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <!-- OAuth 模式:只展示「通过认证中心登录」按钮 -->
    <div v-if="!configLoading && oauthEnabled" class="auth-card">
      <div class="auth-logo" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="20 6 9 17 4 12"></polyline>
        </svg>
      </div>
      <h2>欢迎回来</h2>
      <div class="auth-subtitle">
        TaskFlow 已接入统一认证<span v-if="oauthProvider">：{{ oauthProvider }}</span>
      </div>
      <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>
      <div class="actions" style="margin-top:18px">
        <button type="button" class="btn-primary" @click="startOAuth">
          通过认证中心登录
        </button>
      </div>
      <div class="switch" style="font-size:12px;color:var(--muted)">
        登录与注册都在认证中心完成；首次登录会自动在 TaskFlow 创建账号。
      </div>
    </div>

    <!-- 本地邮箱密码模式(后端没启用 OAuth 时退化到原表单) -->
    <form v-else-if="!configLoading" class="auth-card" @submit.prevent="submit">
      <div class="auth-logo" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="20 6 9 17 4 12"></polyline>
        </svg>
      </div>
      <h2>欢迎回来</h2>
      <div class="auth-subtitle">登录到 TaskFlow，继续你高效的一天</div>
      <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>
      <div class="field">
        <label>邮箱</label>
        <div class="pretty-input-wrap">
          <input v-model="email" class="pretty-input" type="email" autocomplete="email" autofocus required />
          <span class="pretty-input-glow" aria-hidden="true" />
        </div>
      </div>
      <div class="field">
        <label>密码</label>
        <div class="pretty-input-wrap">
          <input v-model="password" class="pretty-input" type="password" autocomplete="current-password" required />
          <span class="pretty-input-glow" aria-hidden="true" />
        </div>
      </div>
      <div class="actions">
        <button type="submit" class="btn-primary" :disabled="loading">
          {{ loading ? '登录中…' : '登录' }}
        </button>
      </div>
      <div class="switch">
        还没有账号？<RouterLink to="/register">立即注册</RouterLink>
      </div>
    </form>

    <!-- 等 /api/auth/config 返回前的占位,避免 UI 闪烁 -->
    <div v-else class="auth-card" aria-busy="true">
      <div class="auth-subtitle" style="text-align:center">加载登录配置…</div>
    </div>
  </div>
</template>

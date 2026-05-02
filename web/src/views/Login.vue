<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { auth as authApi, ApiError, getApiBase } from '@/api'
import { isTauri, tauri } from '@/tauri'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const errMsg = ref('')
const status = ref<'idle' | 'launching' | 'waiting' | 'finalizing'>('idle')
// 拉到 /api/auth/config 之前先标记加载中,避免 UI 闪烁。
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

// 根据当前运行环境决定 client kind:
//   - 浏览器 -> "web",走原有的"重定向 + 前端 finalize"
//   - Tauri  -> "desktop",走"系统浏览器 + 服务端 poll"
const clientKind = computed<'web' | 'desktop'>(() => (isTauri() ? 'desktop' : 'web'))

// 桌面端 poll 用的 device_id;每次按"通过认证中心登录"按钮时重新生成。
const deviceId = ref<string>('')

// poll 控制
let pollTimer: number | null = null
let pollAbortAt = 0
const POLL_INTERVAL_MS = 1500
const POLL_TIMEOUT_MS = 5 * 60 * 1000 // 5 分钟没拿到就放弃

onUnmounted(() => {
  stopPolling()
})

function stopPolling() {
  if (pollTimer != null) {
    window.clearTimeout(pollTimer)
    pollTimer = null
  }
}

function buildAbsoluteStartURL(deviceIdParam?: string): string | null {
  let url = oauthStartURL.value
  if (!/^https?:\/\//i.test(url)) {
    const base = getApiBase()
    if (base) {
      url = base + (url.startsWith('/') ? url : '/' + url)
    } else if (isTauri()) {
      errMsg.value = '尚未配置服务端地址,请到设置页填写后再尝试登录。'
      return null
    }
    // 浏览器同源部署时 apiBase 为空是合法状态,保留相对路径 /api/auth/oauth/start。
  }
  // 拼上 client / device_id 参数
  const sep = url.includes('?') ? '&' : '?'
  const parts = [`client=${clientKind.value}`]
  if (deviceIdParam) parts.push(`device_id=${encodeURIComponent(deviceIdParam)}`)
  return url + sep + parts.join('&')
}

function generateDeviceId(): string {
  // 32 字节随机 -> 64 hex 字符,长度 / 熵均超过 OAuth state 推荐
  const buf = new Uint8Array(32)
  crypto.getRandomValues(buf)
  return Array.from(buf, (b) => b.toString(16).padStart(2, '0')).join('')
}

// 「通过认证中心登录」
//   浏览器场景:整页跳转到后端 /api/auth/oauth/start?client=web,
//     OAuth 完成后服务端把用户带到 /oauth/callback#code=...,前端 OAuthCallback.vue 调 finalize。
//   Tauri 场景:打开系统默认浏览器到 .../start?client=desktop&device_id=<random>,
//     此处定时 poll /api/auth/oauth/poll?device_id=...;poll 到 handoff 后调 finalize 入会话。
async function startOAuth() {
  errMsg.value = ''
  if (!isTauri()) {
    // ====== 浏览器分支:整页跳转 ======
    const url = buildAbsoluteStartURL()
    if (!url) return
    const target = (route.query.redirect as string) || '/'
    try {
      sessionStorage.setItem('taskflow.oauth_redirect', target)
    } catch {
      /* 隐私模式下 sessionStorage 不可用,默认跳 / */
    }
    window.location.href = url
    return
  }

  // ====== Tauri 分支:系统浏览器 + poll ======
  status.value = 'launching'
  deviceId.value = generateDeviceId()
  const url = buildAbsoluteStartURL(deviceId.value)
  if (!url) {
    status.value = 'idle'
    return
  }
  try {
    await tauri.openExternal(url)
  } catch (e) {
    errMsg.value = '无法打开系统浏览器:' + ((e as Error)?.message || '未知错误')
    status.value = 'idle'
    return
  }
  status.value = 'waiting'
  pollAbortAt = Date.now() + POLL_TIMEOUT_MS
  schedulePoll()
}

function schedulePoll() {
  stopPolling()
  pollTimer = window.setTimeout(pollOnce, POLL_INTERVAL_MS)
}

async function pollOnce() {
  // 用户已经跑路 / 主动取消
  if (status.value !== 'waiting') return
  if (Date.now() > pollAbortAt) {
    errMsg.value = '5 分钟内未完成登录,请重新尝试。'
    status.value = 'idle'
    return
  }
  try {
    const r = await authApi.oauthPoll(deviceId.value)
    if (r && r.code) {
      status.value = 'finalizing'
      await auth.loginViaOAuth(r.code)
      const target = (route.query.redirect as string) || '/'
      void router.replace(target)
      return
    }
    // 还没准备好,继续 poll
    schedulePoll()
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
    status.value = 'idle'
  }
}

function cancelTauriOAuth() {
  stopPolling()
  status.value = 'idle'
  deviceId.value = ''
}
</script>

<template>
  <div class="auth-page">
    <!-- OAuth 模式:三端都走这里(浏览器跳转 / Tauri 系统浏览器 + poll) -->
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

      <!-- 三种状态切换:idle / waiting / finalizing -->
      <div v-if="status === 'idle'" class="actions" style="margin-top:18px">
        <button type="button" class="btn-primary" @click="startOAuth">
          通过认证中心登录
        </button>
      </div>
      <div v-else class="auth-waiting" style="margin-top:18px">
        <div class="auth-subtitle" style="text-align:center">
          <template v-if="status === 'launching'">正在打开系统浏览器…</template>
          <template v-else-if="status === 'waiting'">已在系统浏览器中打开认证中心。<br/>请在浏览器里完成登录,本程序会自动接收登录态。</template>
          <template v-else>正在完成登录…</template>
        </div>
        <div class="actions" style="margin-top:14px">
          <button v-if="status === 'waiting'" type="button" class="btn-ghost" @click="cancelTauriOAuth">取消</button>
        </div>
      </div>

      <div class="switch" style="font-size:12px;color:var(--muted)">
        登录与注册都在认证中心完成；首次登录会自动在 TaskFlow 创建账号。
      </div>
    </div>

    <!-- 等 /api/auth/config 返回前的占位,避免 UI 闪烁 -->
    <div v-else-if="configLoading" class="auth-card" aria-busy="true">
      <div class="auth-subtitle" style="text-align:center">加载登录配置…</div>
    </div>

    <!-- 没启用 OAuth 时的兜底:本项目要求 OAuth 必须启用,这里只展示明确报错。 -->
    <div v-else class="auth-card">
      <div class="auth-logo" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line>
          <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path>
        </svg>
      </div>
      <h2>未配置认证中心</h2>
      <div class="auth-subtitle">
        服务端未启用 OAuth 登录。请管理员检查 <code>.env</code> 中的 <code>OAUTH_ENABLED</code> 与相关字段。
      </div>
    </div>
  </div>
</template>

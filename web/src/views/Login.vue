<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { auth as authApi, ApiError, getApiBase } from '@/api'
import { alertDialog } from '@/dialogs'
import { useAuthStore } from '@/stores/auth'
import { isTauri, tauri } from '@/tauri'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const errMsg = ref('')
const status = ref<'idle' | 'launching' | 'waiting' | 'finalizing'>('idle')
const configLoading = ref(true)

const oauthEnabled = computed(() => auth.authConfig?.oauth_enabled === true)
const oauthStartURL = computed(() => auth.authConfig?.oauth_start_url || '/api/auth/oauth/start')
const clientKind = computed<'web' | 'desktop'>(() => (isTauri() ? 'desktop' : 'web'))

const deviceId = ref('')

let pollTimer: number | null = null
let pollAbortAt = 0
const POLL_INTERVAL_MS = 1500
const POLL_TIMEOUT_MS = 5 * 60 * 1000

onMounted(async () => {
  await auth.loadAuthConfig()
  configLoading.value = false

  const oauthErr = (route.query.oauth_error as string) || ''
  if (oauthErr) {
    showLoginError(decodeURIComponent(oauthErr))
  }
})

onUnmounted(() => {
  stopPolling()
})

function stopPolling() {
  if (pollTimer != null) {
    window.clearTimeout(pollTimer)
    pollTimer = null
  }
}

function clearLoginError() {
  errMsg.value = ''
}

function showLoginError(message: string, title = '登录遇到问题') {
  if (!message) return
  errMsg.value = message
  void alertDialog({
    title,
    message,
    confirmText: '知道了',
  })
}

function buildAbsoluteStartURL(deviceIdParam?: string): string | null {
  let url = oauthStartURL.value
  if (!/^https?:\/\//i.test(url)) {
    const base = getApiBase()
    if (base) {
      url = base + (url.startsWith('/') ? url : '/' + url)
    } else if (isTauri()) {
      showLoginError('还没有设置连接地址，请先在设置里填写后再登录。')
      return null
    }
  }

  const sep = url.includes('?') ? '&' : '?'
  const parts = [`client=${clientKind.value}`]
  if (deviceIdParam) parts.push(`device_id=${encodeURIComponent(deviceIdParam)}`)
  return url + sep + parts.join('&')
}

function generateDeviceId(): string {
  const buf = new Uint8Array(32)
  crypto.getRandomValues(buf)
  return Array.from(buf, (b) => b.toString(16).padStart(2, '0')).join('')
}

async function startOAuth() {
  clearLoginError()
  if (!isTauri()) {
    const url = buildAbsoluteStartURL()
    if (!url) return
    const target = (route.query.redirect as string) || '/'
    try {
      sessionStorage.setItem('taskflow.oauth_redirect', target)
    } catch {
      // 隐私模式中 sessionStorage 可能不可用，默认跳回首页即可。
    }
    window.location.href = url
    return
  }

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
    showLoginError('暂时无法打开浏览器，请稍后重试。' + ((e as Error)?.message || ''))
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
  if (status.value !== 'waiting') return
  if (Date.now() > pollAbortAt) {
    showLoginError('登录等待已超时，请重新打开登录流程。')
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
    schedulePoll()
  } catch (e) {
    showLoginError(e instanceof ApiError ? e.message : (e as Error).message)
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
  <div class="auth-page auth-product-page">
    <section v-if="!configLoading && oauthEnabled" class="auth-card auth-product-card">
      <div class="auth-brand-row">
        <div class="auth-logo" aria-hidden="true">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
            <path d="M5 12.5 9.2 16.7 19 6.8"></path>
          </svg>
        </div>
        <div>
          <div class="auth-brand-name">TaskFlow</div>
          <div class="auth-brand-subtitle">任务、日程与强提醒</div>
        </div>
      </div>

      <div class="auth-copy">
        <p class="auth-kicker">三端同步 · 到点不漏</p>
        <h1>把今天要做的事，稳稳推进</h1>
        <p>
          规划任务、安排提醒、查看统计，手机、网页和 Windows 桌面端会保持一致。登录后就能继续使用你的清单。
        </p>
      </div>

      <div class="auth-feature-grid" aria-label="TaskFlow 能为你做什么">
        <div class="auth-feature">
          <span class="auth-feature-dot"></span>
          <div>
            <strong>清晰安排</strong>
            <span>按日期、清单和预计时长整理任务。</span>
          </div>
        </div>
        <div class="auth-feature">
          <span class="auth-feature-dot"></span>
          <div>
            <strong>强提醒</strong>
            <span>Android 与 Windows 到点主动唤醒。</span>
          </div>
        </div>
        <div class="auth-feature">
          <span class="auth-feature-dot"></span>
          <div>
            <strong>专注记录</strong>
            <span>番茄钟和统计帮你复盘节奏。</span>
          </div>
        </div>
      </div>

      <div v-if="status === 'idle'" class="actions auth-actions">
        <button type="button" class="btn-primary" @click="startOAuth">
          继续登录
        </button>
      </div>
      <div v-else class="auth-waiting">
        <div class="auth-status-copy">
          <template v-if="status === 'launching'">正在打开浏览器...</template>
          <template v-else-if="status === 'waiting'">请在刚刚打开的浏览器里完成登录，完成后这里会自动继续。</template>
          <template v-else>正在完成登录...</template>
        </div>
        <div class="actions auth-actions">
          <button v-if="status === 'waiting'" type="button" class="btn-ghost" @click="cancelTauriOAuth">取消</button>
        </div>
      </div>

      <div class="switch auth-footnote">
        首次登录会自动创建 TaskFlow 账号。
      </div>
    </section>

    <section v-else-if="configLoading" class="auth-card auth-loading-card" aria-busy="true">
      <div class="auth-logo" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 2v4"></path><path d="M12 18v4"></path><path d="m4.93 4.93 2.83 2.83"></path><path d="m16.24 16.24 2.83 2.83"></path>
        </svg>
      </div>
      <h2>正在准备登录</h2>
      <div class="auth-subtitle">请稍候片刻。</div>
    </section>

    <section v-else class="auth-card auth-loading-card">
      <div class="auth-logo" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line>
          <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path>
        </svg>
      </div>
      <h2>暂时不能登录</h2>
      <div class="auth-subtitle">登录入口还没有准备好，请稍后再试或联系管理员处理。</div>
    </section>
  </div>
</template>

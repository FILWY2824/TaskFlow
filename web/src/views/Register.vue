<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api'
import { TIMEZONE_GROUPS, DEFAULT_TIMEZONE } from '@/timezones'

const auth = useAuthStore()
const router = useRouter()

const email = ref('')
const password = ref('')
const password2 = ref('')
const displayName = ref('')
// 默认中国上海。用户可在下拉里选其他常见时区，注册后还可在设置里改。
const timezone = ref<string>(DEFAULT_TIMEZONE)
const errMsg = ref('')
const loading = ref(false)
const configLoading = ref(true)

onMounted(async () => {
  await auth.loadAuthConfig()
  configLoading.value = false
})

const oauthEnabled = computed(() => auth.authConfig?.oauth_enabled === true)
const oauthProvider = computed(() => auth.authConfig?.oauth_provider || '')
const oauthStartURL = computed(() => auth.authConfig?.oauth_start_url || '/api/auth/oauth/start')

// OAuth 模式下,「注册」与「登录」是同一回事 —— 直接把用户送到认证中心,首次登录会自动创建本地账号。
function goToOAuth() {
  try {
    sessionStorage.setItem('taskflow.oauth_redirect', '/')
  } catch {
    /* 忽略 */
  }
  window.location.href = oauthStartURL.value
}

async function submit() {
  errMsg.value = ''
  if (!email.value || !password.value) {
    errMsg.value = '请填写邮箱与密码'
    return
  }
  if (password.value.length < 8) {
    errMsg.value = '密码至少 8 位'
    return
  }
  if (password.value !== password2.value) {
    errMsg.value = '两次输入的密码不一致'
    return
  }
  loading.value = true
  try {
    await auth.register({
      email: email.value.trim(),
      password: password.value,
      display_name: displayName.value.trim() || undefined,
      timezone: timezone.value || DEFAULT_TIMEZONE,
    })
    router.replace('/')
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <!-- OAuth 模式:本地注册不可用,引导用户去认证中心 -->
    <div v-if="!configLoading && oauthEnabled" class="auth-card">
      <div class="auth-logo" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="20 6 9 17 4 12"></polyline>
        </svg>
      </div>
      <h2>注册请前往认证中心</h2>
      <div class="auth-subtitle">
        TaskFlow 已接入统一认证<span v-if="oauthProvider">：{{ oauthProvider }}</span>。
        在认证中心注册账号后,本应用会在你首次登录时自动创建对应的资料。
      </div>
      <div class="actions" style="margin-top:18px">
        <button type="button" class="btn-primary" @click="goToOAuth">
          前往认证中心
        </button>
      </div>
      <div class="switch">
        已有账号？<RouterLink to="/login">去登录</RouterLink>
      </div>
    </div>

    <!-- 本地模式:保留原注册表单 -->
    <form v-else-if="!configLoading" class="auth-card" @submit.prevent="submit">
      <div class="auth-logo" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="20 6 9 17 4 12"></polyline>
        </svg>
      </div>
      <h2>创建账号</h2>
      <div class="auth-subtitle">开启你在 TaskFlow 的高效之旅</div>
      <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>
      <div class="field">
        <label>邮箱</label>
        <div class="pretty-input-wrap">
          <input v-model="email" class="pretty-input" type="email" autocomplete="email" autofocus required />
          <span class="pretty-input-glow" aria-hidden="true" />
        </div>
      </div>
      <div class="field">
        <label>显示名（可选）</label>
        <div class="pretty-input-wrap">
          <input v-model="displayName" class="pretty-input" type="text" maxlength="64" />
          <span class="pretty-input-glow" aria-hidden="true" />
        </div>
      </div>
      <div class="field">
        <label>密码（≥8 位）</label>
        <div class="pretty-input-wrap">
          <input v-model="password" class="pretty-input" type="password" autocomplete="new-password" required />
          <span class="pretty-input-glow" aria-hidden="true" />
        </div>
      </div>
      <div class="field">
        <label>确认密码</label>
        <div class="pretty-input-wrap">
          <input v-model="password2" class="pretty-input" type="password" autocomplete="new-password" required />
          <span class="pretty-input-glow" aria-hidden="true" />
        </div>
      </div>
      <div class="field">
        <label>时区</label>
        <div class="pretty-input-wrap">
          <select v-model="timezone" class="pretty-input">
            <optgroup v-for="g in TIMEZONE_GROUPS" :key="g.label" :label="g.label">
              <option v-for="o in g.options" :key="o.value" :value="o.value">{{ o.label }}</option>
            </optgroup>
          </select>
          <span class="pretty-input-glow" aria-hidden="true" />
        </div>
        <div class="muted" style="font-size:11.5px;margin-top:6px">注册后可在「设置 → 时区」中修改。</div>
      </div>
      <div class="actions">
        <button type="submit" class="btn-primary" :disabled="loading">
          {{ loading ? '注册中…' : '注册并登录' }}
        </button>
      </div>
      <div class="switch">
        已有账号？<RouterLink to="/login">去登录</RouterLink>
      </div>
    </form>

    <div v-else class="auth-card" aria-busy="true">
      <div class="auth-subtitle" style="text-align:center">加载登录配置…</div>
    </div>
  </div>
</template>

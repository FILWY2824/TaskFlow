<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api'

const auth = useAuthStore()
const router = useRouter()

const email = ref('')
const password = ref('')
const password2 = ref('')
const displayName = ref('')
const timezone = ref(Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai')
const errMsg = ref('')
const loading = ref(false)

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
      timezone: timezone.value || undefined,
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
    <form class="auth-card" @submit.prevent="submit">
      <div class="auth-logo" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="20 6 9 17 4 12"></polyline>
        </svg>
      </div>
      <h2>创建账号</h2>
      <div class="auth-subtitle">开始你在 ToDo List 的高效之旅</div>
      <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>
      <div class="field">
        <label>邮箱</label>
        <input v-model="email" type="email" autocomplete="email" autofocus required />
      </div>
      <div class="field">
        <label>显示名（可选）</label>
        <input v-model="displayName" type="text" />
      </div>
      <div class="field">
        <label>密码（≥8 位）</label>
        <input v-model="password" type="password" autocomplete="new-password" required />
      </div>
      <div class="field">
        <label>确认密码</label>
        <input v-model="password2" type="password" autocomplete="new-password" required />
      </div>
      <div class="field">
        <label>时区</label>
        <input v-model="timezone" type="text" placeholder="Asia/Shanghai" />
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
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const email = ref('')
const password = ref('')
const errMsg = ref('')
const loading = ref(false)

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
    <form class="auth-card" @submit.prevent="submit">
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
        <input v-model="email" type="email" autocomplete="email" autofocus required />
      </div>
      <div class="field">
        <label>密码</label>
        <input v-model="password" type="password" autocomplete="current-password" required />
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
  </div>
</template>

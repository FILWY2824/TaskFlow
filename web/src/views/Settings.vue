<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { fmtDateTime } from '@/utils'

const auth = useAuthStore()
const ok = ref('')

// ---- 主题切换 ----
type ThemeMode = 'auto' | 'light' | 'dark'
const themeMode = ref<ThemeMode>('auto')

function readSavedTheme(): ThemeMode {
  try {
    const v = localStorage.getItem('todolist.theme')
    if (v === 'light' || v === 'dark') return v
  } catch {
    /* ignore */
  }
  return 'auto'
}
function applyTheme(m: ThemeMode) {
  if (m === 'auto') {
    document.documentElement.removeAttribute('data-theme')
    try { localStorage.removeItem('todolist.theme') } catch { /* ignore */ }
  } else {
    document.documentElement.setAttribute('data-theme', m)
    try { localStorage.setItem('todolist.theme', m) } catch { /* ignore */ }
  }
}
function chooseTheme(m: ThemeMode) {
  themeMode.value = m
  applyTheme(m)
}

onMounted(() => {
  themeMode.value = readSavedTheme()
})

// ---- 浏览器通知权限 ----
function permState(): string {
  if (!('Notification' in window)) return '此浏览器不支持桌面通知'
  return Notification.permission
}
const perm = ref(permState())

async function requestPerm() {
  if (!('Notification' in window)) return
  await Notification.requestPermission()
  perm.value = permState()
  if (perm.value === 'granted') ok.value = '已开启浏览器通知'
}
</script>

<template>
  <div>
    <div class="section-card">
      <h3>账号信息</h3>
      <div class="kv"><div class="k">邮箱</div><div class="v">{{ auth.user?.email }}</div></div>
      <div class="kv"><div class="k">显示名</div><div class="v">{{ auth.user?.display_name || '(未设置)' }}</div></div>
      <div class="kv"><div class="k">时区</div><div class="v">{{ auth.user?.timezone }}</div></div>
      <div class="kv"><div class="k">注册时间</div><div class="v">{{ fmtDateTime(auth.user?.created_at) }}</div></div>
    </div>

    <div class="section-card">
      <h3>外观</h3>
      <p class="muted">选择浅色 / 深色，或跟随系统设置自动切换。</p>
      <div class="theme-options">
        <div
          class="theme-option preview-auto"
          :class="{ 'is-selected': themeMode === 'auto' }"
          @click="chooseTheme('auto')"
        >
          <div class="preview" />
          <span>跟随系统</span>
        </div>
        <div
          class="theme-option preview-light"
          :class="{ 'is-selected': themeMode === 'light' }"
          @click="chooseTheme('light')"
        >
          <div class="preview" />
          <span>浅色</span>
        </div>
        <div
          class="theme-option preview-dark"
          :class="{ 'is-selected': themeMode === 'dark' }"
          @click="chooseTheme('dark')"
        >
          <div class="preview" />
          <span>深色</span>
        </div>
      </div>
    </div>

    <div class="section-card">
      <h3>浏览器通知</h3>
      <p class="muted">
        Web 端只是普通管理端，不承担本地强提醒。但开启浏览器通知后，提醒到点时会弹出系统级桌面通知。
      </p>
      <div class="kv">
        <div class="k">权限状态</div>
        <div class="v">
          <span :class="perm === 'granted' ? 'success-text' : (perm === 'denied' ? 'danger-text' : 'muted')">
            {{ perm }}
          </span>
        </div>
        <button v-if="perm === 'default'" class="btn-primary" @click="requestPerm">请求权限</button>
      </div>
      <div v-if="ok" class="success-text" style="margin-top:8px">{{ ok }}</div>
    </div>

    <div class="section-card">
      <h3>关于</h3>
      <p class="muted">ToDo List Web v0.3.0</p>
      <p class="muted">多用户 TODO + Android / Windows 强提醒</p>
    </div>
  </div>
</template>

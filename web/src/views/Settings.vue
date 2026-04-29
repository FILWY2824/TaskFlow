<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { fmtDateTime } from '@/utils'

const auth = useAuthStore()
const ok = ref('')

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
      <h3>浏览器通知</h3>
      <p class="muted" style="font-size: 13px">
        Web 端只是普通管理端,不承担本地强提醒。但开启浏览器通知后,提醒到点时本页会弹出系统级桌面通知。
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
      <div v-if="ok" class="success-text">{{ ok }}</div>
    </div>

    <div class="section-card">
      <h3>关于</h3>
      <p class="muted" style="font-size: 13px">TodoAlarm Web 管理端 v0.3.0</p>
      <p class="muted" style="font-size: 13px">规格文档:多用户 TODO + Android/Windows 强提醒 v2.2</p>
    </div>
  </div>
</template>

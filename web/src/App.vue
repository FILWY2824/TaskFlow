<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import { useNotificationsStore } from '@/stores/notifications'

const notif = useNotificationsStore()
const online = ref(navigator.onLine)

function handleOnline() { online.value = true }
function handleOffline() { online.value = false }

onMounted(() => {
  window.addEventListener('online', handleOnline)
  window.addEventListener('offline', handleOffline)
})
onBeforeUnmount(() => {
  window.removeEventListener('online', handleOnline)
  window.removeEventListener('offline', handleOffline)
})
</script>

<template>
  <div v-if="!online" style="position: fixed; top: 0; left: 0; right: 0; padding: 6px; text-align: center; background: var(--c-warn); color: white; z-index: 999;">
    离线中 — 写操作已停用,等待网络恢复
  </div>
  <RouterView />
  <div class="toast-stack">
    <div
      v-for="t in notif.toastQueue"
      :key="t.id"
      class="toast"
      role="status"
      @click="notif.dismissToast(t.id)"
    >
      <div class="toast-title">{{ t.title }}</div>
      <div v-if="t.body" class="toast-body">{{ t.body }}</div>
    </div>
  </div>
</template>

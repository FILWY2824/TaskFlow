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
  <Transition name="fade">
    <div v-if="!online" style="position: fixed; top: 0; left: 0; right: 0; padding: 8px; text-align: center; background: var(--c-warn); color: white; z-index: 999; font-weight: 500; font-size: 14px; box-shadow: var(--shadow-sm);">
      <svg style="width:14px;height:14px;vertical-align:middle;margin-right:6px" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="1" y1="1" x2="23" y2="23"></line><path d="M16.72 11.06A10.94 10.94 0 0 1 19 12.55"></path><path d="M5 12.55a10.94 10.94 0 0 1 5.17-2.39"></path><path d="M10.71 5.05A16 16 0 0 1 22.58 9"></path><path d="M1.42 9a15.91 15.91 0 0 1 4.7-2.88"></path><path d="M8.53 16.11a6 6 0 0 1 6.95 0"></path><line x1="12" y1="20" x2="12.01" y2="20"></line></svg>
      离线中 — 写操作已停用,等待网络恢复
    </div>
  </Transition>
  <RouterView />
  <TransitionGroup name="list" tag="div" class="toast-stack">
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
  </TransitionGroup>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useNotificationsStore } from '@/stores/notifications'
import { ApiError, notifications as notifApi } from '@/api'
import type { NotificationDetail } from '@/types'
import { fmtDateTime, fmtRelative } from '@/utils'

const store = useNotificationsStore()
const errMsg = ref('')
const onlyUnread = ref(false)
const detail = ref<NotificationDetail | null>(null)

async function load() {
  errMsg.value = ''
  try {
    if (onlyUnread.value) {
      const r = await notifApi.list({ only_unread: true, limit: 100 })
      store.items = r.items
      store.unread = r.unread_count
    } else {
      await store.refresh()
    }
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

async function open(id: number) {
  try {
    detail.value = await notifApi.get(id)
    if (!detail.value.is_read) await store.markRead(id)
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>
    <div class="row-flex" style="margin-bottom: 12px">
      <label><input v-model="onlyUnread" type="checkbox" @change="load" /> 只看未读 ({{ store.unread }})</label>
      <span class="spacer" />
      <button class="btn-secondary" :disabled="store.unread === 0" @click="store.markAllRead">全部标记已读</button>
      <button class="btn-secondary" @click="load">刷新</button>
    </div>

    <div v-if="store.items.length === 0" class="empty">
      <div style="font-size: 48px">🔕</div>
      <div>暂无通知</div>
    </div>

    <div v-else class="notif-list">
      <div
        v-for="n in store.items"
        :key="n.id"
        class="notif-item"
        :class="{ 'is-read': n.is_read }"
        @click="open(n.id)"
      >
        <div class="ntitle">{{ n.title }}</div>
        <div v-if="n.body" class="nbody">{{ n.body }}</div>
        <div class="ntime">{{ fmtRelative(n.fire_at) }} · {{ fmtDateTime(n.fire_at) }}</div>
      </div>
    </div>

    <template v-if="detail">
      <div class="drawer-backdrop" @click="detail = null" />
      <div class="drawer" style="width: min(420px, 95vw)">
        <header>
          <span class="title">通知详情</span>
          <button class="btn-ghost" @click="detail = null">✕</button>
        </header>
        <div class="body">
          <div class="kv"><div class="k">标题</div><div class="v">{{ detail.title }}</div></div>
          <div v-if="detail.body" class="kv"><div class="k">正文</div><div class="v">{{ detail.body }}</div></div>
          <div class="kv"><div class="k">触发时间</div><div class="v">{{ fmtDateTime(detail.fire_at) }}</div></div>
          <div class="kv"><div class="k">已读</div><div class="v">{{ detail.is_read ? '是' : '否' }}</div></div>
          <h3 style="margin: 14px 0 6px">投递记录</h3>
          <div v-if="!detail.deliveries || detail.deliveries.length === 0" class="muted">尚未投递</div>
          <div v-else>
            <div v-for="d in detail.deliveries" :key="d.id" class="kv" style="font-size: 12px">
              <div class="k">{{ d.channel }}</div>
              <div class="v">
                <span :class="d.status === 'delivered' ? 'success-text' : 'danger-text'">{{ d.status }}</span>
                <span v-if="d.error" class="danger-text"> · {{ d.error }}</span>
                <span v-if="d.delivered_at" class="muted"> · {{ fmtDateTime(d.delivered_at) }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

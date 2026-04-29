<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useDataStore } from '@/stores/data'
import { useNotificationsStore } from '@/stores/notifications'
import { ApiError } from '@/api'

const auth = useAuthStore()
const data = useDataStore()
const notif = useNotificationsStore()
const router = useRouter()
const route = useRoute()

const search = ref('')
const showNewListDialog = ref(false)
const newListName = ref('')
const newListColor = ref('#1f6feb')
const errMsg = ref('')

onMounted(async () => {
  try {
    await Promise.all([data.loadLists(), notif.refreshUnread()])
  } catch (e) {
    // 401 已被 api 层处理跳走;这里仅吞掉
  }
  notif.startSSE()
  // 浏览器通知权限请求(温和:只在没有决定时请求一次)
  if ('Notification' in window && Notification.permission === 'default') {
    Notification.requestPermission().catch(() => {})
  }
})

onBeforeUnmount(() => {
  notif.stopSSE()
})

const sidebarFilters = [
  { name: 'today', label: '今日', icon: '☀' },
  { name: 'tomorrow', label: '明天', icon: '➡' },
  { name: 'this-week', label: '本周', icon: '📅' },
  { name: 'overdue', label: '过期', icon: '⏰' },
  { name: 'no-date', label: '无日期', icon: '◯' },
  { name: 'all', label: '全部', icon: '📋' },
  { name: 'completed', label: '已完成', icon: '✓' },
] as const

const sidebarTools = [
  { name: 'calendar', label: '日历', icon: '🗓' },
  { name: 'pomodoro', label: '番茄专注', icon: '🍅' },
  { name: 'stats', label: '数据复盘', icon: '📊' },
  { name: 'notifications', label: '通知中心', icon: '🔔' },
  { name: 'telegram', label: 'Telegram', icon: '✈' },
  { name: 'settings', label: '设置', icon: '⚙' },
] as const

const isActive = (name: string) => route.name === name

async function logout() {
  await auth.logout()
  router.replace({ name: 'login' })
}

async function submitSearch() {
  // 触发当前 Tasks 视图重新加载
  // Tasks 视图监听 route 与 query.q
  router.push({ ...route, query: { ...route.query, q: search.value || undefined } })
}

function openNewList() {
  showNewListDialog.value = true
  newListName.value = ''
  newListColor.value = '#1f6feb'
  errMsg.value = ''
}
async function createList() {
  errMsg.value = ''
  const name = newListName.value.trim()
  if (!name) {
    errMsg.value = '名称不能为空'
    return
  }
  try {
    await data.createList({ name, color: newListColor.value })
    showNewListDialog.value = false
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  }
}

const userInitial = computed(() => {
  const u = auth.user
  if (!u) return '?'
  return (u.display_name || u.email || '?').charAt(0).toUpperCase()
})

const userLabel = computed(() => auth.user?.display_name || auth.user?.email || '')
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <h1>TodoAlarm</h1>
      <nav>
        <div class="group-title">任务视图</div>
        <RouterLink
          v-for="f in sidebarFilters"
          :key="f.name"
          :to="{ name: f.name }"
          :class="{ active: isActive(f.name) }"
        >
          <span><span style="margin-right: 6px">{{ f.icon }}</span>{{ f.label }}</span>
        </RouterLink>

        <div class="group-title-row">
          <span>我的清单</span>
          <button class="btn-ghost" title="新建清单" @click="openNewList">+</button>
        </div>
        <RouterLink
          v-for="l in data.lists"
          :key="l.id"
          :to="{ name: 'list', params: { id: l.id } }"
          :class="{ active: route.name === 'list' && Number(route.params.id) === l.id }"
        >
          <span>
            <span :style="{ color: l.color || 'var(--c-primary)' }" style="margin-right: 6px">●</span>
            {{ l.name }}
          </span>
        </RouterLink>

        <div class="group-title">工具</div>
        <RouterLink
          v-for="t in sidebarTools"
          :key="t.name"
          :to="{ name: t.name }"
          :class="{ active: isActive(t.name) }"
        >
          <span><span style="margin-right: 6px">{{ t.icon }}</span>{{ t.label }}</span>
          <span v-if="t.name === 'notifications' && notif.unread > 0" class="badge danger">{{ notif.unread }}</span>
        </RouterLink>
      </nav>
      <div class="footer">
        <div class="row-flex">
          <div
            style="width: 28px; height: 28px; border-radius: 50%; background: var(--c-primary); color: white; display: flex; align-items: center; justify-content: center; font-weight: 600;"
          >{{ userInitial }}</div>
          <div style="flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px;">
            {{ userLabel }}
          </div>
          <button class="btn-ghost" title="退出登录" @click="logout">⏻</button>
        </div>
      </div>
    </aside>

    <main class="main">
      <header class="topbar">
        <div class="title">
          <span>{{ pageTitle(route) }}</span>
        </div>
        <div class="topbar-actions">
          <input
            v-model="search"
            type="search"
            placeholder="搜索任务标题/描述…"
            @keydown.enter="submitSearch"
          />
        </div>
      </header>
      <section class="content">
        <RouterView />
      </section>
    </main>

    <!-- 新建清单 dialog -->
    <template v-if="showNewListDialog">
      <div class="drawer-backdrop" @click="showNewListDialog = false"></div>
      <div class="drawer" style="width: min(420px, 95vw)">
        <header>
          <span class="title">新建清单</span>
          <button class="btn-ghost" @click="showNewListDialog = false">✕</button>
        </header>
        <div class="body">
          <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>
          <div class="field">
            <label>名称</label>
            <input v-model="newListName" autofocus @keydown.enter="createList" />
          </div>
          <div class="field">
            <label>颜色</label>
            <input v-model="newListColor" type="color" style="height: 36px" />
          </div>
        </div>
        <footer>
          <button class="btn-secondary" @click="showNewListDialog = false">取消</button>
          <button class="btn-primary" @click="createList">创建</button>
        </footer>
      </div>
    </template>
  </div>
</template>

<script lang="ts">
import type { RouteLocationNormalizedLoaded } from 'vue-router'

function pageTitle(route: RouteLocationNormalizedLoaded): string {
  const m: Record<string, string> = {
    today: '今日',
    tomorrow: '明天',
    'this-week': '本周',
    overdue: '过期',
    'no-date': '无日期',
    all: '全部任务',
    completed: '已完成',
    list: '清单',
    calendar: '日历',
    pomodoro: '番茄专注',
    stats: '数据复盘',
    notifications: '通知中心',
    telegram: 'Telegram 绑定',
    settings: '设置',
  }
  return m[String(route.name || '')] || ''
}
</script>
